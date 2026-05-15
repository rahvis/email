package batch_mail

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/kumo"
	"billionmail-core/internal/service/outbound"
	"billionmail-core/internal/service/sending_profiles"

	"github.com/stretchr/testify/require"
)

type fakeOutboundMailer struct {
	called bool
	req    outbound.OutboundMessage
	err    error
}

func (f *fakeOutboundMailer) Send(ctx context.Context, req outbound.OutboundMessage) (*outbound.OutboundResult, error) {
	f.called = true
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	return &outbound.OutboundResult{
		Engine:          outbound.EngineKumoMTA,
		MessageID:       req.MessageID,
		InjectionStatus: kumo.InjectionStatusQueued,
		QueueName:       outbound.QueueNameForMessage(req),
		AcceptedAt:      time.Now().Unix(),
		RawResponse:     `{"success_count":1}`,
	}, nil
}

type fakePostfixCampaignMailer struct {
	called bool
	closed bool
	req    outbound.OutboundMessage
	err    error
}

func (f *fakePostfixCampaignMailer) Send(ctx context.Context, req outbound.OutboundMessage) (*outbound.OutboundResult, error) {
	f.called = true
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	return &outbound.OutboundResult{
		Engine:          outbound.EnginePostfix,
		MessageID:       req.MessageID,
		InjectionStatus: kumo.InjectionStatusQueued,
		AcceptedAt:      time.Now().Unix(),
	}, nil
}

func (f *fakePostfixCampaignMailer) GenerateMessageID() string {
	return "<postfix-message@example.com>"
}

func (f *fakePostfixCampaignMailer) Close() {
	f.closed = true
}

func restoreCampaignDeliveryHooks() func() {
	origPostfix := newPostfixCampaignMailer
	origKumoMailer := newKumoCampaignMailer
	origConfig := loadKumoCampaignConfig
	origRecord := recordKumoCampaignInjection
	origBaseURL := getCampaignBaseURLBySender
	origSuppressed := isCampaignRecipientSuppressed
	origGuard := guardKumoCampaignSend
	origRelease := releaseKumoCampaignQuota
	origRecordQuota := recordKumoCampaignQuotaQueued
	guardKumoCampaignSend = func(ctx context.Context, input sending_profiles.SendGuardInput) (*sending_profiles.GuardResult, error) {
		return &sending_profiles.GuardResult{Allowed: true}, nil
	}
	releaseKumoCampaignQuota = func(ctx context.Context, reservation sending_profiles.QuotaReservation) {}
	recordKumoCampaignQuotaQueued = func(ctx context.Context, tenantID, profileID int64, workflow string, count int64) error { return nil }
	return func() {
		newPostfixCampaignMailer = origPostfix
		newKumoCampaignMailer = origKumoMailer
		loadKumoCampaignConfig = origConfig
		recordKumoCampaignInjection = origRecord
		getCampaignBaseURLBySender = origBaseURL
		isCampaignRecipientSuppressed = origSuppressed
		guardKumoCampaignSend = origGuard
		releaseKumoCampaignQuota = origRelease
		recordKumoCampaignQuotaQueued = origRecordQuota
	}
}

func baseCampaignTask() *entity.EmailTask {
	return &entity.EmailTask{
		Id:               981,
		TenantId:         42,
		Addresser:        "news@example.com",
		FullName:         "News",
		Subject:          "Subject",
		TrackOpen:        0,
		TrackClick:       0,
		DeliveryEngine:   CampaignDeliveryEngineKumoMTA,
		SendingProfileId: 7,
	}
}

func baseRecipient() *entity.RecipientInfo {
	return &entity.RecipientInfo{
		Id:              12345,
		TaskId:          981,
		Recipient:       "user@gmail.com",
		InjectionStatus: kumo.InjectionStatusPending,
		DeliveryStatus:  kumo.DeliveryStatusPending,
	}
}

func TestNormalizeCampaignDeliveryEngine(t *testing.T) {
	require.Equal(t, CampaignDeliveryEnginePostfix, normalizeCampaignDeliveryEngine(""))
	require.Equal(t, CampaignDeliveryEnginePostfix, normalizeCampaignDeliveryEngine("local"))
	require.Equal(t, CampaignDeliveryEngineKumoMTA, normalizeCampaignDeliveryEngine("kumo"))
	require.Equal(t, CampaignDeliveryEngineTenantDefault, normalizeCampaignDeliveryEngine("inherit"))
	require.Equal(t, CampaignDeliveryEnginePostfix, normalizeCampaignDeliveryEngine("unexpected"))
}

func TestKumoCampaignMessageQueuesAndRecordsInjection(t *testing.T) {
	defer restoreCampaignDeliveryHooks()()

	fakeMailer := &fakeOutboundMailer{}
	var recorded kumo.MessageInjectionRecord
	newKumoCampaignMailer = func(ctx context.Context) (outbound.OutboundMailer, error) {
		return fakeMailer, nil
	}
	loadKumoCampaignConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, CampaignsEnabled: true, BaseURL: "https://kumo.example.com", InjectPath: "/api/inject/v1"}, nil
	}
	recordKumoCampaignInjection = func(ctx context.Context, record kumo.MessageInjectionRecord) error {
		recorded = record
		return nil
	}
	getCampaignBaseURLBySender = func(sender string) string { return "https://mail.example.com" }
	isCampaignRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return false, nil }

	result := NewTaskExecutor(context.Background()).sendPreparedCampaignMessage(context.Background(), baseCampaignTask(), baseRecipient(), "<p>Hello</p>", "Subject")
	require.True(t, result.Success)
	require.True(t, fakeMailer.called)
	require.Equal(t, outbound.EngineKumoMTA, result.Engine)
	require.Equal(t, kumo.InjectionStatusQueued, result.InjectionStatus)
	require.Equal(t, kumo.DeliveryStatusPending, result.DeliveryStatus)
	require.Equal(t, "campaign_981:tenant_42@gmail.com", result.KumoQueue)
	require.Equal(t, int64(12345), recorded.RecipientInfoID)
	require.Equal(t, "campaign_981:tenant_42@gmail.com", recorded.QueueName)

	raw, err := outbound.BuildRFC822(fakeMailer.req)
	require.NoError(t, err)
	msg := string(raw)
	require.Contains(t, msg, "X-BM-Tenant-ID: 42")
	require.Contains(t, msg, "X-BM-Campaign-ID: 981")
	require.Contains(t, msg, "X-BM-Recipient-ID: 12345")
	require.Contains(t, msg, "X-BM-Sending-Profile-ID: 7")
}

func TestKumoCampaignRetryableErrorDoesNotMarkDelivered(t *testing.T) {
	defer restoreCampaignDeliveryHooks()()

	newKumoCampaignMailer = func(ctx context.Context) (outbound.OutboundMailer, error) {
		return &fakeOutboundMailer{err: &outbound.SendError{Class: outbound.ErrClassRetryable, Message: "timeout"}}, nil
	}
	loadKumoCampaignConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, CampaignsEnabled: true, BaseURL: "https://kumo.example.com"}, nil
	}
	recordKumoCampaignInjection = func(ctx context.Context, record kumo.MessageInjectionRecord) error {
		return fmt.Errorf("should not record failed injection")
	}
	getCampaignBaseURLBySender = func(sender string) string { return "https://mail.example.com" }
	isCampaignRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return false, nil }

	result := NewTaskExecutor(context.Background()).sendPreparedCampaignMessage(context.Background(), baseCampaignTask(), baseRecipient(), "<p>Hello</p>", "Subject")
	require.False(t, result.Success)
	require.True(t, result.Retryable)
	require.Equal(t, kumo.InjectionStatusRetrying, result.InjectionStatus)
	require.Equal(t, kumo.DeliveryStatusPending, result.DeliveryStatus)
	require.NotZero(t, result.NextRetryAt)
}

func TestKumoCampaignPermanentErrorIsFailedAndSanitized(t *testing.T) {
	defer restoreCampaignDeliveryHooks()()

	newKumoCampaignMailer = func(ctx context.Context) (outbound.OutboundMailer, error) {
		return &fakeOutboundMailer{err: &outbound.SendError{Class: outbound.ErrClassPermanent, StatusCode: 422, Message: "bad\nmessage"}}, nil
	}
	loadKumoCampaignConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, CampaignsEnabled: true, BaseURL: "https://kumo.example.com"}, nil
	}
	getCampaignBaseURLBySender = func(sender string) string { return "https://mail.example.com" }
	isCampaignRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return false, nil }

	result := NewTaskExecutor(context.Background()).sendPreparedCampaignMessage(context.Background(), baseCampaignTask(), baseRecipient(), "<p>Hello</p>", "Subject")
	require.False(t, result.Success)
	require.False(t, result.Retryable)
	require.Equal(t, kumo.InjectionStatusFailed, result.InjectionStatus)
	require.Equal(t, kumo.DeliveryStatusUnknown, result.DeliveryStatus)
	require.NotContains(t, result.LastResponse, "\n")
}

func TestSuppressedCampaignRecipientIsNotInjected(t *testing.T) {
	defer restoreCampaignDeliveryHooks()()

	fakeMailer := &fakeOutboundMailer{}
	newKumoCampaignMailer = func(ctx context.Context) (outbound.OutboundMailer, error) { return fakeMailer, nil }
	loadKumoCampaignConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, CampaignsEnabled: true, BaseURL: "https://kumo.example.com"}, nil
	}
	isCampaignRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return true, nil }

	result := NewTaskExecutor(context.Background()).sendPreparedCampaignMessage(context.Background(), baseCampaignTask(), baseRecipient(), "<p>Hello</p>", "Subject")
	require.False(t, result.Success)
	require.True(t, result.Suppressed)
	require.Equal(t, kumo.InjectionStatusCancelled, result.InjectionStatus)
	require.Equal(t, kumo.DeliveryStatusSuppressed, result.DeliveryStatus)
	require.False(t, fakeMailer.called)
}

func TestKumoDisabledFallsBackToPostfixCampaignPath(t *testing.T) {
	defer restoreCampaignDeliveryHooks()()

	fakePostfix := &fakePostfixCampaignMailer{}
	newPostfixCampaignMailer = func(addresser string) (postfixCampaignMailer, error) {
		return fakePostfix, nil
	}
	loadKumoCampaignConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, CampaignsEnabled: false, BaseURL: "https://kumo.example.com"}, nil
	}
	getCampaignBaseURLBySender = func(sender string) string { return "https://mail.example.com" }
	isCampaignRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return false, nil }

	result := NewTaskExecutor(context.Background()).sendPreparedCampaignMessage(context.Background(), baseCampaignTask(), baseRecipient(), "<p>Hello</p>", "Subject")
	require.True(t, result.Success)
	require.True(t, fakePostfix.called)
	require.True(t, fakePostfix.closed)
	require.Equal(t, outbound.EnginePostfix, result.Engine)
	require.Equal(t, "<postfix-message@example.com>", result.MessageID)
	require.False(t, strings.Contains(fakePostfix.req.HTML, "X-BM-Tenant-ID"))
}

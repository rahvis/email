package batch_mail

import (
	"context"
	"fmt"
	"testing"

	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/kumo"
	"billionmail-core/internal/service/outbound"
	"billionmail-core/internal/service/sending_profiles"

	"github.com/stretchr/testify/require"
)

func restoreAPIDeliveryHooks() func() {
	origPostfix := newPostfixAPIMailer
	origKumoMailer := newKumoAPIMailer
	origConfig := loadKumoAPIConfig
	origRecord := recordKumoAPIInjection
	origBaseURL := getAPIBaseURLBySender
	origSuppressed := isAPIRecipientSuppressed
	origGuard := guardKumoAPISend
	origRelease := releaseKumoAPIQuota
	origRecordQuota := recordKumoAPIQuotaQueued
	guardKumoAPISend = func(ctx context.Context, input sending_profiles.SendGuardInput) (*sending_profiles.GuardResult, error) {
		return &sending_profiles.GuardResult{Allowed: true}, nil
	}
	releaseKumoAPIQuota = func(ctx context.Context, reservation sending_profiles.QuotaReservation) {}
	recordKumoAPIQuotaQueued = func(ctx context.Context, tenantID, profileID int64, workflow string, count int64) error { return nil }
	return func() {
		newPostfixAPIMailer = origPostfix
		newKumoAPIMailer = origKumoMailer
		loadKumoAPIConfig = origConfig
		recordKumoAPIInjection = origRecord
		getAPIBaseURLBySender = origBaseURL
		isAPIRecipientSuppressed = origSuppressed
		guardKumoAPISend = origGuard
		releaseKumoAPIQuota = origRelease
		recordKumoAPIQuotaQueued = origRecordQuota
	}
}

func baseAPITemplate() *entity.ApiTemplates {
	return &entity.ApiTemplates{
		Id:               17,
		TenantId:         42,
		TemplateId:       100,
		Subject:          "Password reset",
		Addresser:        "support@example.com",
		FullName:         "Support",
		TrackOpen:        0,
		TrackClick:       0,
		DeliveryEngine:   APIDeliveryEngineKumoMTA,
		SendingProfileId: 7,
	}
}

func baseAPIMailLog() ApiMailLog {
	return ApiMailLog{
		Id:        77441,
		ApiId:     17,
		TenantId:  42,
		Recipient: "user@gmail.com",
		Addresser: "support@example.com",
		MessageId: "api-message@example.com",
	}
}

func TestNormalizeAPIDeliveryEngine(t *testing.T) {
	require.Equal(t, APIDeliveryEnginePostfix, normalizeAPIDeliveryEngine(""))
	require.Equal(t, APIDeliveryEnginePostfix, normalizeAPIDeliveryEngine("smtp"))
	require.Equal(t, APIDeliveryEngineKumoMTA, normalizeAPIDeliveryEngine("kumo"))
	require.Equal(t, APIDeliveryEngineTenantDefault, normalizeAPIDeliveryEngine("inherited"))
	require.Equal(t, APIDeliveryEnginePostfix, normalizeAPIDeliveryEngine("unexpected"))
}

func TestKumoAPIMailQueuesAndRecordsInjection(t *testing.T) {
	defer restoreAPIDeliveryHooks()()

	fakeMailer := &fakeOutboundMailer{}
	var recorded kumo.MessageInjectionRecord
	newKumoAPIMailer = func(ctx context.Context) (outbound.OutboundMailer, error) {
		return fakeMailer, nil
	}
	loadKumoAPIConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, APIEnabled: true, BaseURL: "https://kumo.example.com", InjectPath: "/api/inject/v1"}, nil
	}
	recordKumoAPIInjection = func(ctx context.Context, record kumo.MessageInjectionRecord) error {
		recorded = record
		return nil
	}
	getAPIBaseURLBySender = func(sender string) string { return "https://mail.example.com" }
	isAPIRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return false, nil }

	result := sendApiMailPrepared(context.Background(), baseAPITemplate(), "Password reset", "<p>Hello</p>", baseAPIMailLog())
	require.Equal(t, StatusSuccess, result.Status)
	require.Equal(t, outbound.EngineKumoMTA, result.Engine)
	require.Equal(t, kumo.InjectionStatusQueued, result.InjectionStatus)
	require.Equal(t, kumo.DeliveryStatusPending, result.DeliveryStatus)
	require.Equal(t, "api_17:tenant_42@gmail.com", result.KumoQueue)
	require.True(t, fakeMailer.called)
	require.Equal(t, int64(77441), recorded.APILogID)
	require.Equal(t, int64(17), recorded.APIID)
	require.Equal(t, "api_17:tenant_42@gmail.com", recorded.QueueName)

	raw, err := outbound.BuildRFC822(fakeMailer.req)
	require.NoError(t, err)
	msg := string(raw)
	require.Contains(t, msg, "X-BM-Tenant-ID: 42")
	require.Contains(t, msg, "X-BM-Api-ID: 17")
	require.Contains(t, msg, "X-BM-Api-Log-ID: 77441")
	require.Contains(t, msg, "X-BM-Sending-Profile-ID: 7")
}

func TestKumoAPIRetryableErrorStaysPending(t *testing.T) {
	defer restoreAPIDeliveryHooks()()

	newKumoAPIMailer = func(ctx context.Context) (outbound.OutboundMailer, error) {
		return &fakeOutboundMailer{err: &outbound.SendError{Class: outbound.ErrClassRetryable, Message: "timeout"}}, nil
	}
	loadKumoAPIConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, APIEnabled: true, BaseURL: "https://kumo.example.com"}, nil
	}
	recordKumoAPIInjection = func(ctx context.Context, record kumo.MessageInjectionRecord) error {
		return fmt.Errorf("should not record failed injection")
	}
	getAPIBaseURLBySender = func(sender string) string { return "https://mail.example.com" }
	isAPIRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return false, nil }

	result := sendApiMailPrepared(context.Background(), baseAPITemplate(), "Password reset", "<p>Hello</p>", baseAPIMailLog())
	require.Equal(t, StatusPending, result.Status)
	require.Equal(t, outbound.EngineKumoMTA, result.Engine)
	require.True(t, result.Retryable)
	require.Equal(t, kumo.InjectionStatusRetrying, result.InjectionStatus)
	require.Equal(t, kumo.DeliveryStatusPending, result.DeliveryStatus)
	require.NotZero(t, result.NextRetryAt)
}

func TestKumoAPIDisabledFallsBackToPostfix(t *testing.T) {
	defer restoreAPIDeliveryHooks()()

	fakePostfix := &fakePostfixCampaignMailer{}
	newPostfixAPIMailer = func(addresser string) (postfixAPIMailer, error) {
		return fakePostfix, nil
	}
	loadKumoAPIConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, APIEnabled: false, BaseURL: "https://kumo.example.com"}, nil
	}
	getAPIBaseURLBySender = func(sender string) string { return "https://mail.example.com" }
	isAPIRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return false, nil }

	result := sendApiMailPrepared(context.Background(), baseAPITemplate(), "Password reset", "<p>Hello</p>", baseAPIMailLog())
	require.Equal(t, StatusSuccess, result.Status)
	require.Equal(t, outbound.EnginePostfix, result.Engine)
	require.True(t, fakePostfix.called)
	require.True(t, fakePostfix.closed)
	require.Equal(t, kumo.InjectionStatusQueued, result.InjectionStatus)
	require.Equal(t, kumo.DeliveryStatusPending, result.DeliveryStatus)
}

func TestSuppressedAPIRecipientIsNotInjected(t *testing.T) {
	defer restoreAPIDeliveryHooks()()

	fakeMailer := &fakeOutboundMailer{}
	newKumoAPIMailer = func(ctx context.Context) (outbound.OutboundMailer, error) { return fakeMailer, nil }
	loadKumoAPIConfig = func(ctx context.Context) (*kumo.StoredConfig, error) {
		return &kumo.StoredConfig{Enabled: true, APIEnabled: true, BaseURL: "https://kumo.example.com"}, nil
	}
	isAPIRecipientSuppressed = func(ctx context.Context, tenantID int64, recipient string) (bool, error) { return true, nil }

	result := sendApiMailPrepared(context.Background(), baseAPITemplate(), "Password reset", "<p>Hello</p>", baseAPIMailLog())
	require.Equal(t, StatusFailed, result.Status)
	require.Equal(t, kumo.InjectionStatusCancelled, result.InjectionStatus)
	require.Equal(t, kumo.DeliveryStatusSuppressed, result.DeliveryStatus)
	require.False(t, fakeMailer.called)
}

package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/kumo"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/outbound"
	"billionmail-core/internal/service/sending_profiles"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	CampaignDeliveryEngineTenantDefault = "tenant_default"
	CampaignDeliveryEngineKumoMTA       = outbound.EngineKumoMTA
	CampaignDeliveryEnginePostfix       = outbound.EnginePostfix
)

type postfixCampaignMailer interface {
	outbound.OutboundMailer
	GenerateMessageID() string
	Close()
}

var (
	newPostfixCampaignMailer = func(addresser string) (postfixCampaignMailer, error) {
		return outbound.NewPostfixSMTPMailer(addresser)
	}
	newKumoCampaignMailer = func(ctx context.Context) (outbound.OutboundMailer, error) {
		return outbound.NewKumoHTTPMailer(ctx)
	}
	loadKumoCampaignConfig        = kumo.LoadConfig
	recordKumoCampaignInjection   = kumo.RecordMessageInjection
	getCampaignBaseURLBySender    = domains.GetBaseURLBySender
	isCampaignRecipientSuppressed = dbIsCampaignRecipientSuppressed
	guardKumoCampaignSend         = sending_profiles.GuardSend
	releaseKumoCampaignQuota      = sending_profiles.ReleaseReservation
	recordKumoCampaignQuotaQueued = sending_profiles.RecordQueued
)

func normalizeCampaignDeliveryEngine(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "", "postfix", "local", "smtp":
		return CampaignDeliveryEnginePostfix
	case "kumo", "kumomta":
		return CampaignDeliveryEngineKumoMTA
	case "tenant_default", "default", "inherit":
		return CampaignDeliveryEngineTenantDefault
	default:
		return CampaignDeliveryEnginePostfix
	}
}

func NormalizeCampaignDeliveryEngineForAPI(engine string) string {
	return normalizeCampaignDeliveryEngine(engine)
}

func (e *TaskExecutor) resolveCampaignDeliveryEngine(ctx context.Context, task *entity.EmailTask) (string, error) {
	engine := normalizeCampaignDeliveryEngine(task.DeliveryEngine)
	if engine == CampaignDeliveryEngineTenantDefault {
		cfg, err := loadKumoCampaignConfig(ctx)
		if err == nil && cfg != nil && cfg.Enabled && cfg.CampaignsEnabled && strings.TrimSpace(cfg.BaseURL) != "" {
			return CampaignDeliveryEngineKumoMTA, nil
		}
		return CampaignDeliveryEnginePostfix, nil
	}
	if engine != CampaignDeliveryEngineKumoMTA {
		return CampaignDeliveryEnginePostfix, nil
	}

	cfg, err := loadKumoCampaignConfig(ctx)
	if err != nil {
		return CampaignDeliveryEngineKumoMTA, fmt.Errorf("load KumoMTA config failed: %w", err)
	}
	if cfg == nil || !cfg.Enabled || !cfg.CampaignsEnabled {
		g.Log().Infof(ctx, "campaign %d requested KumoMTA but campaigns are disabled; falling back to Postfix", task.Id)
		return CampaignDeliveryEnginePostfix, nil
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return CampaignDeliveryEngineKumoMTA, fmt.Errorf("KumoMTA base URL is not configured")
	}

	return CampaignDeliveryEngineKumoMTA, nil
}

func (e *TaskExecutor) sendPreparedCampaignMessage(ctx context.Context, task *entity.EmailTask, recipient *entity.RecipientInfo, renderedContent, renderedSubject string) *SendResult {
	if suppressed, err := isCampaignRecipientSuppressed(ctx, int64(task.TenantId), recipient.Recipient); err != nil {
		g.Log().Warningf(ctx, "campaign %d suppression check failed for recipient %d: %v", task.Id, recipient.Id, err)
	} else if suppressed {
		return &SendResult{
			RecipientID:     recipient.Id,
			Success:         false,
			Engine:          normalizeCampaignDeliveryEngine(task.DeliveryEngine),
			InjectionStatus: kumo.InjectionStatusCancelled,
			DeliveryStatus:  kumo.DeliveryStatusSuppressed,
			Suppressed:      true,
			Error:           fmt.Errorf("recipient suppressed"),
		}
	}

	engine, err := e.resolveCampaignDeliveryEngine(ctx, task)
	if err != nil {
		return &SendResult{
			RecipientID:     recipient.Id,
			Success:         false,
			Engine:          engine,
			InjectionStatus: kumo.InjectionStatusFailed,
			DeliveryStatus:  kumo.DeliveryStatusUnknown,
			LastResponse:    sanitizeCampaignResponse(err.Error()),
			Error:           err,
		}
	}

	if engine == CampaignDeliveryEngineKumoMTA {
		return e.sendKumoCampaignMessage(ctx, task, recipient, renderedContent, renderedSubject)
	}
	return e.sendPostfixCampaignMessage(ctx, task, recipient, renderedContent, renderedSubject)
}

func (e *TaskExecutor) sendPostfixCampaignMessage(ctx context.Context, task *entity.EmailTask, recipient *entity.RecipientInfo, renderedContent, renderedSubject string) *SendResult {
	mailer, err := newPostfixCampaignMailer(task.Addresser)
	if err != nil {
		g.Log().Error(ctx, "create email sender failed: %v", err)
		return &SendResult{
			RecipientID:     recipient.Id,
			Success:         false,
			Engine:          outbound.EnginePostfix,
			InjectionStatus: kumo.InjectionStatusFailed,
			DeliveryStatus:  kumo.DeliveryStatusUnknown,
			LastResponse:    sanitizeCampaignResponse(err.Error()),
			Error:           fmt.Errorf("create email sender failed: %w", err),
		}
	}
	defer mailer.Close()

	messageID := mailer.GenerateMessageID()
	renderedContent = e.applyCampaignTracking(task, recipient, renderedContent, messageID)

	result, err := mailer.Send(ctx, buildCampaignOutboundMessage(task, recipient, renderedContent, renderedSubject, messageID))
	if err != nil {
		g.Log().Error(ctx, "send email to %s failed: %v", recipient.Recipient, err)
		return &SendResult{
			RecipientID:     recipient.Id,
			Success:         false,
			Engine:          outbound.EnginePostfix,
			InjectionStatus: kumo.InjectionStatusFailed,
			DeliveryStatus:  kumo.DeliveryStatusUnknown,
			LastResponse:    sanitizeCampaignResponse(err.Error()),
			Error:           fmt.Errorf("send email failed: %w", err),
		}
	}

	return &SendResult{
		RecipientID:     recipient.Id,
		MessageID:       result.MessageID,
		Success:         true,
		Engine:          outbound.EnginePostfix,
		InjectionStatus: kumo.InjectionStatusQueued,
		DeliveryStatus:  kumo.DeliveryStatusPending,
		ProviderQueueID: result.ProviderQueueID,
		LastResponse:    sanitizeCampaignResponse(result.RawResponse),
		Error:           nil,
	}
}

func (e *TaskExecutor) sendKumoCampaignMessage(ctx context.Context, task *entity.EmailTask, recipient *entity.RecipientInfo, renderedContent, renderedSubject string) *SendResult {
	messageID := outbound.GenerateMessageID(task.Addresser)
	renderedContent = e.applyCampaignTracking(task, recipient, renderedContent, messageID)

	msg := buildCampaignOutboundMessage(task, recipient, renderedContent, renderedSubject, messageID)
	guard, err := guardKumoCampaignSend(ctx, sending_profiles.SendGuardInput{
		TenantID:         msg.TenantID,
		SendingProfileID: msg.SendingProfileID,
		Workflow:         "campaign",
		FromEmail:        msg.FromEmail,
		Recipient:        msg.Recipient,
		Count:            1,
	})
	if err != nil {
		g.Log().Error(ctx, "KumoMTA campaign guard failed: %v", err)
		return failedKumoCampaignResult(recipient, err)
	}
	if guard == nil || !guard.Allowed {
		reason := "KumoMTA campaign sending is blocked"
		if guard != nil && guard.Reason != "" {
			reason = guard.Reason
		}
		return &SendResult{
			RecipientID:     recipient.Id,
			Success:         false,
			Engine:          outbound.EngineKumoMTA,
			InjectionStatus: kumo.InjectionStatusCancelled,
			DeliveryStatus:  kumo.DeliveryStatusPending,
			LastResponse:    sanitizeCampaignResponse(reason),
			Error:           errors.New(reason),
		}
	}

	mailer, err := newKumoCampaignMailer(ctx)
	if err != nil {
		g.Log().Error(ctx, "create KumoMTA mailer failed: %v", err)
		releaseKumoCampaignQuota(ctx, guard.Reservation)
		return failedKumoCampaignResult(recipient, err)
	}

	result, err := mailer.Send(ctx, msg)
	if err != nil {
		g.Log().Error(ctx, "KumoMTA campaign injection to %s failed: %v", recipient.Recipient, err)
		releaseKumoCampaignQuota(ctx, guard.Reservation)
		return failedKumoCampaignResult(recipient, err)
	}
	if err := recordKumoCampaignQuotaQueued(ctx, msg.TenantID, msg.SendingProfileID, "campaign", 1); err != nil {
		g.Log().Warningf(ctx, "record tenant campaign quota usage failed: tenant=%d profile=%d error=%v", msg.TenantID, msg.SendingProfileID, err)
	}

	if err := recordKumoCampaignInjection(ctx, kumo.MessageInjectionRecord{
		TenantID:         msg.TenantID,
		MessageID:        result.MessageID,
		Recipient:        msg.Recipient,
		RecipientDomain:  msg.DestinationDomain,
		CampaignID:       msg.CampaignID,
		TaskID:           msg.TaskID,
		RecipientInfoID:  msg.RecipientID,
		SendingProfileID: msg.SendingProfileID,
		QueueName:        result.QueueName,
		InjectionStatus:  kumo.InjectionStatusQueued,
		DeliveryStatus:   kumo.DeliveryStatusPending,
		AttemptCount:     recipient.AttemptCount + 1,
		AcceptedAt:       result.AcceptedAt,
	}); err != nil {
		g.Log().Errorf(ctx, "record KumoMTA campaign injection failed: task=%d recipient_id=%d queue=%s error=%v", task.Id, recipient.Id, result.QueueName, err)
	}

	return &SendResult{
		RecipientID:     recipient.Id,
		MessageID:       result.MessageID,
		Success:         true,
		Engine:          outbound.EngineKumoMTA,
		InjectionStatus: kumo.InjectionStatusQueued,
		DeliveryStatus:  kumo.DeliveryStatusPending,
		KumoQueue:       result.QueueName,
		ProviderQueueID: result.ProviderQueueID,
		LastResponse:    sanitizeCampaignResponse(result.RawResponse),
		Error:           nil,
	}
}

func failedKumoCampaignResult(recipient *entity.RecipientInfo, err error) *SendResult {
	retryable := false
	injectionStatus := kumo.InjectionStatusFailed
	deliveryStatus := kumo.DeliveryStatusUnknown
	var sendErr *outbound.SendError
	if errors.As(err, &sendErr) && sendErr.Retryable() {
		retryable = true
		injectionStatus = kumo.InjectionStatusRetrying
		deliveryStatus = kumo.DeliveryStatusPending
	}

	nextRetryAt := int64(0)
	if retryable {
		nextRetryAt = nextCampaignRetryAt(recipient.AttemptCount)
	}

	return &SendResult{
		RecipientID:     recipient.Id,
		Success:         false,
		Engine:          outbound.EngineKumoMTA,
		InjectionStatus: injectionStatus,
		DeliveryStatus:  deliveryStatus,
		Retryable:       retryable,
		NextRetryAt:     nextRetryAt,
		LastResponse:    sanitizeCampaignResponse(err.Error()),
		Error:           err,
	}
}

func (e *TaskExecutor) applyCampaignTracking(task *entity.EmailTask, recipient *entity.RecipientInfo, renderedContent, messageID string) string {
	baseURL := getCampaignBaseURLBySender(task.Addresser)
	mailTracker := maillog_stat.NewMailTracker(renderedContent, task.Id, messageID, recipient.Recipient, baseURL)
	if task.TrackClick == 1 {
		mailTracker.TrackLinks()
	}
	if task.TrackOpen == 1 {
		mailTracker.AppendTrackingPixel()
	}
	return mailTracker.GetHTML()
}

func buildCampaignOutboundMessage(task *entity.EmailTask, recipient *entity.RecipientInfo, html, subject, messageID string) outbound.OutboundMessage {
	return outbound.OutboundMessage{
		TenantID:          int64(task.TenantId),
		CampaignID:        int64(task.Id),
		TaskID:            int64(task.Id),
		RecipientID:       int64(recipient.Id),
		FromEmail:         task.Addresser,
		FromName:          task.FullName,
		Recipient:         recipient.Recipient,
		Subject:           subject,
		HTML:              html,
		MessageID:         messageID,
		SenderDomain:      outbound.SenderDomain(task.Addresser),
		DestinationDomain: outbound.DestinationDomain(recipient.Recipient),
		SendingProfileID:  int64(task.SendingProfileId),
	}
}

func dbIsCampaignRecipientSuppressed(ctx context.Context, tenantID int64, recipient string) (bool, error) {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	if recipient == "" {
		return false, nil
	}
	model := g.DB().Model("abnormal_recipient").
		Ctx(ctx).
		Where("recipient", recipient)
	if tenantID > 0 {
		model = model.Where("tenant_id", tenantID)
	}
	count, err := model.Where("count >= ?", 3).Count()
	return count > 0, err
}

func nextCampaignRetryAt(attemptCount int) int64 {
	if attemptCount < 0 {
		attemptCount = 0
	}
	delay := time.Duration(1<<minInt(attemptCount, 6)) * time.Minute
	if delay > time.Hour {
		delay = time.Hour
	}
	return time.Now().Add(delay).Unix()
}

func sanitizeCampaignResponse(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
	if len(value) > 500 {
		return value[:500] + "..."
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

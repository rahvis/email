package batch_mail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/kumo"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/outbound"
	"billionmail-core/internal/service/sending_profiles"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	APIDeliveryEngineTenantDefault = "tenant_default"
	APIDeliveryEngineKumoMTA       = outbound.EngineKumoMTA
	APIDeliveryEnginePostfix       = outbound.EnginePostfix
)

type postfixAPIMailer interface {
	outbound.OutboundMailer
	GenerateMessageID() string
	Close()
}

type apiMailSendResult struct {
	Status          int
	Engine          string
	MessageID       string
	InjectionStatus string
	DeliveryStatus  string
	KumoQueue       string
	ProviderQueueID string
	LastResponse    string
	Retryable       bool
	NextRetryAt     int64
	ErrorMessage    string
}

var (
	newPostfixAPIMailer = func(addresser string) (postfixAPIMailer, error) {
		return outbound.NewPostfixSMTPMailer(addresser)
	}
	newKumoAPIMailer = func(ctx context.Context) (outbound.OutboundMailer, error) {
		return outbound.NewKumoHTTPMailer(ctx)
	}
	loadKumoAPIConfig        = kumo.LoadConfig
	recordKumoAPIInjection   = kumo.RecordMessageInjection
	getAPIBaseURLBySender    = domains.GetBaseURLBySender
	isAPIRecipientSuppressed = dbIsAPIRecipientSuppressed
	guardKumoAPISend         = sending_profiles.GuardSend
	releaseKumoAPIQuota      = sending_profiles.ReleaseReservation
	recordKumoAPIQuotaQueued = sending_profiles.RecordQueued
)

func normalizeAPIDeliveryEngine(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "", "postfix", "local", "smtp":
		return APIDeliveryEnginePostfix
	case "kumo", "kumomta":
		return APIDeliveryEngineKumoMTA
	case "tenant_default", "default", "inherit", "inherited":
		return APIDeliveryEngineTenantDefault
	default:
		return APIDeliveryEnginePostfix
	}
}

func NormalizeAPIDeliveryEngineForAPI(engine string) string {
	return normalizeAPIDeliveryEngine(engine)
}

func ResolveAPIDeliveryEngine(ctx context.Context, apiTemplate *entity.ApiTemplates) (string, error) {
	if apiTemplate == nil {
		return APIDeliveryEnginePostfix, fmt.Errorf("API template is required")
	}
	engine := normalizeAPIDeliveryEngine(apiTemplate.DeliveryEngine)
	if engine == APIDeliveryEngineTenantDefault {
		cfg, err := loadKumoAPIConfig(ctx)
		if err == nil && cfg != nil && cfg.Enabled && cfg.APIEnabled && strings.TrimSpace(cfg.BaseURL) != "" {
			return APIDeliveryEngineKumoMTA, nil
		}
		return APIDeliveryEnginePostfix, nil
	}
	if engine != APIDeliveryEngineKumoMTA {
		return APIDeliveryEnginePostfix, nil
	}

	cfg, err := loadKumoAPIConfig(ctx)
	if err != nil {
		return APIDeliveryEngineKumoMTA, fmt.Errorf("load KumoMTA config failed: %w", err)
	}
	if cfg == nil || !cfg.Enabled || !cfg.APIEnabled {
		g.Log().Infof(ctx, "Send API %d requested KumoMTA but API sending is disabled; falling back to Postfix", apiTemplate.Id)
		return APIDeliveryEnginePostfix, nil
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return APIDeliveryEngineKumoMTA, fmt.Errorf("KumoMTA base URL is not configured")
	}
	return APIDeliveryEngineKumoMTA, nil
}

func sendApiMailPrepared(ctx context.Context, apiTemplate *entity.ApiTemplates, subject string, content string, log ApiMailLog) apiMailSendResult {
	if suppressed, err := isAPIRecipientSuppressed(ctx, int64(apiTemplate.TenantId), log.Recipient); err != nil {
		g.Log().Warningf(ctx, "Send API %d suppression check failed for recipient %s: %v", apiTemplate.Id, log.Recipient, err)
	} else if suppressed {
		return apiMailSendResult{
			Status:          StatusFailed,
			Engine:          normalizeAPIDeliveryEngine(apiTemplate.DeliveryEngine),
			MessageID:       canonicalAPIMessageID(log.MessageId, log.Addresser),
			InjectionStatus: kumo.InjectionStatusCancelled,
			DeliveryStatus:  kumo.DeliveryStatusSuppressed,
			ErrorMessage:    "recipient suppressed",
		}
	}

	engine, err := ResolveAPIDeliveryEngine(ctx, apiTemplate)
	if err != nil {
		return apiMailSendResult{
			Status:          StatusFailed,
			Engine:          engine,
			MessageID:       canonicalAPIMessageID(log.MessageId, log.Addresser),
			InjectionStatus: kumo.InjectionStatusFailed,
			DeliveryStatus:  kumo.DeliveryStatusUnknown,
			LastResponse:    sanitizeCampaignResponse(err.Error()),
			ErrorMessage:    err.Error(),
		}
	}

	messageID := canonicalAPIMessageID(log.MessageId, log.Addresser)
	content = applyAPITracking(apiTemplate, log, content, messageID)

	if engine == APIDeliveryEngineKumoMTA {
		return sendKumoAPIMessage(ctx, apiTemplate, subject, content, log, messageID)
	}
	return sendPostfixAPIMessage(ctx, apiTemplate, subject, content, log, messageID)
}

func sendPostfixAPIMessage(ctx context.Context, apiTemplate *entity.ApiTemplates, subject string, content string, log ApiMailLog, messageID string) apiMailSendResult {
	mailer, err := newPostfixAPIMailer(log.Addresser)
	if err != nil {
		return apiMailSendResult{
			Status:          StatusFailed,
			Engine:          outbound.EnginePostfix,
			MessageID:       messageID,
			InjectionStatus: kumo.InjectionStatusFailed,
			DeliveryStatus:  kumo.DeliveryStatusUnknown,
			LastResponse:    sanitizeCampaignResponse(err.Error()),
			ErrorMessage:    err.Error(),
		}
	}
	defer mailer.Close()

	msg := buildAPIOutboundMessage(apiTemplate, log, content, subject, messageID)
	result, err := mailer.Send(ctx, msg)
	if err != nil {
		return apiMailSendResult{
			Status:          StatusFailed,
			Engine:          outbound.EnginePostfix,
			MessageID:       messageID,
			InjectionStatus: kumo.InjectionStatusFailed,
			DeliveryStatus:  kumo.DeliveryStatusUnknown,
			LastResponse:    sanitizeCampaignResponse(err.Error()),
			ErrorMessage:    err.Error(),
		}
	}

	return apiMailSendResult{
		Status:          StatusSuccess,
		Engine:          outbound.EnginePostfix,
		MessageID:       result.MessageID,
		InjectionStatus: kumo.InjectionStatusQueued,
		DeliveryStatus:  kumo.DeliveryStatusPending,
		ProviderQueueID: result.ProviderQueueID,
		LastResponse:    sanitizeCampaignResponse(result.RawResponse),
	}
}

func sendKumoAPIMessage(ctx context.Context, apiTemplate *entity.ApiTemplates, subject string, content string, log ApiMailLog, messageID string) apiMailSendResult {
	msg := buildAPIOutboundMessage(apiTemplate, log, content, subject, messageID)
	guard, err := guardKumoAPISend(ctx, sending_profiles.SendGuardInput{
		TenantID:         msg.TenantID,
		SendingProfileID: msg.SendingProfileID,
		Workflow:         "api",
		FromEmail:        msg.FromEmail,
		Recipient:        msg.Recipient,
		Count:            1,
	})
	if err != nil {
		return failedKumoAPIResult(log, messageID, err)
	}
	if guard == nil || !guard.Allowed {
		reason := "KumoMTA Send API sending is blocked"
		if guard != nil && guard.Reason != "" {
			reason = guard.Reason
		}
		return apiMailSendResult{
			Status:          StatusFailed,
			Engine:          outbound.EngineKumoMTA,
			MessageID:       messageID,
			InjectionStatus: kumo.InjectionStatusCancelled,
			DeliveryStatus:  kumo.DeliveryStatusPending,
			LastResponse:    sanitizeCampaignResponse(reason),
			ErrorMessage:    reason,
		}
	}

	mailer, err := newKumoAPIMailer(ctx)
	if err != nil {
		releaseKumoAPIQuota(ctx, guard.Reservation)
		return failedKumoAPIResult(log, messageID, err)
	}

	result, err := mailer.Send(ctx, msg)
	if err != nil {
		releaseKumoAPIQuota(ctx, guard.Reservation)
		return failedKumoAPIResult(log, messageID, err)
	}
	if err := recordKumoAPIQuotaQueued(ctx, msg.TenantID, msg.SendingProfileID, "api", 1); err != nil {
		g.Log().Warningf(ctx, "record tenant API quota usage failed: tenant=%d profile=%d error=%v", msg.TenantID, msg.SendingProfileID, err)
	}

	if err := recordKumoAPIInjection(ctx, kumo.MessageInjectionRecord{
		TenantID:         msg.TenantID,
		MessageID:        result.MessageID,
		Recipient:        msg.Recipient,
		RecipientDomain:  msg.DestinationDomain,
		APIID:            msg.APIID,
		APILogID:         msg.APILogID,
		SendingProfileID: msg.SendingProfileID,
		QueueName:        result.QueueName,
		InjectionStatus:  kumo.InjectionStatusQueued,
		DeliveryStatus:   kumo.DeliveryStatusPending,
		AttemptCount:     log.AttemptCount + 1,
		AcceptedAt:       result.AcceptedAt,
	}); err != nil {
		g.Log().Errorf(ctx, "record KumoMTA API injection failed: api_id=%d api_log_id=%d queue=%s error=%v", apiTemplate.Id, log.Id, result.QueueName, err)
	}

	return apiMailSendResult{
		Status:          StatusSuccess,
		Engine:          outbound.EngineKumoMTA,
		MessageID:       result.MessageID,
		InjectionStatus: kumo.InjectionStatusQueued,
		DeliveryStatus:  kumo.DeliveryStatusPending,
		KumoQueue:       result.QueueName,
		ProviderQueueID: result.ProviderQueueID,
		LastResponse:    sanitizeCampaignResponse(result.RawResponse),
	}
}

func failedKumoAPIResult(log ApiMailLog, messageID string, err error) apiMailSendResult {
	retryable := false
	injectionStatus := kumo.InjectionStatusFailed
	deliveryStatus := kumo.DeliveryStatusUnknown
	status := StatusFailed
	var sendErr *outbound.SendError
	if errors.As(err, &sendErr) && sendErr.Retryable() {
		retryable = true
		status = StatusPending
		injectionStatus = kumo.InjectionStatusRetrying
		deliveryStatus = kumo.DeliveryStatusPending
	}

	nextRetryAt := int64(0)
	if retryable {
		nextRetryAt = nextAPIRetryAt(log.AttemptCount)
	}

	return apiMailSendResult{
		Status:          status,
		Engine:          outbound.EngineKumoMTA,
		MessageID:       messageID,
		InjectionStatus: injectionStatus,
		DeliveryStatus:  deliveryStatus,
		Retryable:       retryable,
		NextRetryAt:     nextRetryAt,
		LastResponse:    sanitizeCampaignResponse(err.Error()),
		ErrorMessage:    err.Error(),
	}
}

func applyAPITracking(apiTemplate *entity.ApiTemplates, log ApiMailLog, content, messageID string) string {
	baseURL := getAPIBaseURLBySender(log.Addresser)
	apiCampaignID := apiTemplate.Id + 1000000000
	mailTracker := maillog_stat.NewMailTracker(content, apiCampaignID, messageID, log.Recipient, baseURL)
	if apiTemplate.TrackClick == 1 {
		mailTracker.TrackLinks()
	}
	if apiTemplate.TrackOpen == 1 {
		mailTracker.AppendTrackingPixel()
	}
	return mailTracker.GetHTML()
}

func buildAPIOutboundMessage(apiTemplate *entity.ApiTemplates, log ApiMailLog, html, subject, messageID string) outbound.OutboundMessage {
	tenantID := int64(apiTemplate.TenantId)
	if tenantID == 0 {
		tenantID = log.TenantId
	}
	return outbound.OutboundMessage{
		TenantID:          tenantID,
		APIID:             int64(apiTemplate.Id),
		APILogID:          log.Id,
		FromEmail:         log.Addresser,
		FromName:          apiTemplate.FullName,
		Recipient:         log.Recipient,
		Subject:           subject,
		HTML:              html,
		MessageID:         messageID,
		SenderDomain:      outbound.SenderDomain(log.Addresser),
		DestinationDomain: outbound.DestinationDomain(log.Recipient),
		SendingProfileID:  int64(apiTemplate.SendingProfileId),
	}
}

func applyAPIMailSendResult(ctx context.Context, log ApiMailLog, result apiMailSendResult) error {
	now := time.Now().Unix()
	data := g.Map{
		"status":        result.Status,
		"error_message": result.ErrorMessage,
		"send_time":     now,
		"attempt_count": gdb.Raw("attempt_count + 1"),
	}
	if result.Engine != "" {
		data["engine"] = result.Engine
	}
	if result.MessageID != "" {
		data["message_id"] = strings.Trim(result.MessageID, "<>")
	}
	if result.InjectionStatus != "" {
		data["injection_status"] = result.InjectionStatus
	}
	if result.DeliveryStatus != "" {
		data["delivery_status"] = result.DeliveryStatus
	}
	if result.KumoQueue != "" {
		data["kumo_queue"] = result.KumoQueue
	}
	if result.ProviderQueueID != "" {
		data["provider_queue_id"] = result.ProviderQueueID
	}
	if result.LastResponse != "" {
		data["last_delivery_response"] = result.LastResponse
	}
	if result.NextRetryAt > 0 {
		data["next_retry_at"] = result.NextRetryAt
	} else if result.Status != StatusPending {
		data["next_retry_at"] = 0
	}

	update := g.DB().Model("api_mail_logs").Ctx(ctx).Where("id", log.Id)
	if log.TenantId > 0 {
		update = update.Where("tenant_id", log.TenantId)
	}
	_, err := update.Data(data).Update()
	return err
}

func canonicalAPIMessageID(messageID, fromEmail string) string {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return outbound.GenerateMessageID(fromEmail)
	}
	messageID = strings.Trim(messageID, "<>")
	return "<" + messageID + ">"
}

func dbIsAPIRecipientSuppressed(ctx context.Context, tenantID int64, recipient string) (bool, error) {
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

func nextAPIRetryAt(attemptCount int) int64 {
	if attemptCount < 0 {
		attemptCount = 0
	}
	delay := time.Duration(1<<minInt(attemptCount, 6)) * time.Minute
	if delay > time.Hour {
		delay = time.Hour
	}
	return time.Now().Add(delay).Unix()
}

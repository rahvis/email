package kumo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type dbDeliveryEventStore struct{}

type injectionLookup struct {
	Id              int64  `orm:"id"`
	TenantId        int64  `orm:"tenant_id"`
	MessageId       string `orm:"message_id"`
	Recipient       string `orm:"recipient"`
	CampaignId      int64  `orm:"campaign_id"`
	TaskId          int64  `orm:"task_id"`
	RecipientInfoId int64  `orm:"recipient_info_id"`
	ApiId           int64  `orm:"api_id"`
	ApiLogId        int64  `orm:"api_log_id"`
	QueueName       string `orm:"queue_name"`
}

type recipientLookup struct {
	Id             int64  `orm:"id"`
	TenantId       int64  `orm:"tenant_id"`
	TaskId         int64  `orm:"task_id"`
	Recipient      string `orm:"recipient"`
	MessageId      string `orm:"message_id"`
	DeliveryStatus string `orm:"delivery_status"`
}

type apiLogLookup struct {
	Id             int64  `orm:"id"`
	TenantId       int64  `orm:"tenant_id"`
	ApiId          int64  `orm:"api_id"`
	Recipient      string `orm:"recipient"`
	MessageId      string `orm:"message_id"`
	DeliveryStatus string `orm:"delivery_status"`
}

func (dbDeliveryEventStore) IsDuplicateEvent(ctx context.Context, event NormalizedDeliveryEvent) (bool, error) {
	if event.ProviderEventID != "" {
		cacheKey := "kumo:delivery_event:event_id:" + event.ProviderEventID
		if isRedisDuplicate(ctx, cacheKey) {
			return true, nil
		}
		exists, err := g.DB().Model("kumo_delivery_events").Ctx(ctx).Where("provider_event_id", event.ProviderEventID).Exist()
		if err != nil {
			return false, err
		}
		if exists {
			cacheDuplicate(ctx, cacheKey)
			return true, nil
		}
	}

	if event.EventHash == "" {
		return false, nil
	}
	cacheKey := "kumo:delivery_event:event_hash:" + event.EventHash
	if isRedisDuplicate(ctx, cacheKey) {
		return true, nil
	}
	exists, err := g.DB().Model("kumo_delivery_events").Ctx(ctx).Where("event_hash", event.EventHash).Exist()
	if err != nil {
		return false, err
	}
	if exists {
		cacheDuplicate(ctx, cacheKey)
		return true, nil
	}
	return false, nil
}

func (dbDeliveryEventStore) StoreEvent(ctx context.Context, event NormalizedDeliveryEvent) (bool, error) {
	now := time.Now().Unix()
	result, err := g.DB().Model("kumo_delivery_events").Ctx(ctx).Data(g.Map{
		"tenant_id":         event.TenantID,
		"provider_event_id": event.ProviderEventID,
		"event_hash":        event.EventHash,
		"event_type":        event.EventType,
		"delivery_status":   event.DeliveryStatus,
		"message_id":        event.MessageID,
		"recipient":         normalizeAddress(event.Recipient),
		"queue_name":        event.QueueName,
		"campaign_id":       event.CampaignID,
		"task_id":           event.TaskID,
		"recipient_info_id": event.RecipientInfoID,
		"api_id":            event.APIID,
		"api_log_id":        event.APILogID,
		"response":          event.Response,
		"remote_mx":         event.RemoteMX,
		"raw_event":         event.RawEvent,
		"orphaned":          false,
		"occurred_at":       event.OccurredAt,
		"ingested_at":       now,
	}).InsertIgnore()
	if err != nil {
		return false, err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		cacheEventDedupe(ctx, event)
		return false, nil
	}
	cacheEventDedupe(ctx, event)
	return true, nil
}

func (s dbDeliveryEventStore) ApplyEvent(ctx context.Context, event NormalizedDeliveryEvent) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		resolved, err := s.resolveCorrelation(ctx, tx, event)
		if err != nil {
			return err
		}
		if err := s.updateDeliveryEventRow(ctx, tx, resolved); err != nil {
			return err
		}
		if resolved.Orphaned {
			RecordOrphanedDeliveryEvent(ctx, resolved)
			return nil
		}

		statusChanged := false
		if resolved.RecipientInfoID > 0 {
			changed, err := s.applyRecipientEvent(ctx, tx, resolved)
			if err != nil {
				return err
			}
			statusChanged = statusChanged || changed
		}
		if resolved.APILogID > 0 {
			changed, err := s.applyAPILogEvent(ctx, tx, resolved)
			if err != nil {
				return err
			}
			statusChanged = statusChanged || changed
		}
		if err := s.applyInjectionEvent(ctx, tx, resolved); err != nil {
			return err
		}
		if statusChanged && shouldSuppressForEvent(resolved) {
			if err := s.applySuppression(ctx, tx, resolved); err != nil {
				return err
			}
		}
		if statusChanged {
			if err := s.incrementTenantUsage(ctx, tx, resolved); err != nil {
				return err
			}
		}
		if resolved.TaskID > 0 {
			if err := s.refreshTaskDeliveryCounters(ctx, tx, resolved); err != nil {
				return err
			}
		}
		return nil
	})
}

func (dbDeliveryEventStore) resolveCorrelation(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) (NormalizedDeliveryEvent, error) {
	resolved := event
	resolved.Recipient = normalizeAddress(resolved.Recipient)
	resolved.MessageID = canonicalMessageID(resolved.MessageID)

	injection, err := lookupInjection(ctx, tx, resolved)
	if err != nil {
		return resolved, err
	}
	if injection.Id > 0 {
		if tenantConflict(resolved.TenantID, injection.TenantId) {
			resolved.Orphaned = true
			return resolved, nil
		}
		fillFromInjection(&resolved, injection)
	}

	recipient, err := lookupRecipient(ctx, tx, resolved)
	if err != nil {
		return resolved, err
	}
	if recipient.Id > 0 {
		if tenantConflict(resolved.TenantID, recipient.TenantId) {
			resolved.Orphaned = true
			return resolved, nil
		}
		resolved.RecipientInfoID = recipient.Id
		if resolved.TenantID == 0 {
			resolved.TenantID = recipient.TenantId
		}
		if resolved.TaskID == 0 {
			resolved.TaskID = recipient.TaskId
		}
		if resolved.CampaignID == 0 {
			resolved.CampaignID = recipient.TaskId
		}
		if resolved.Recipient == "" {
			resolved.Recipient = recipient.Recipient
		}
		if resolved.MessageID == "" {
			resolved.MessageID = canonicalMessageID(recipient.MessageId)
		}
	}

	apiLog, err := lookupAPILog(ctx, tx, resolved)
	if err != nil {
		return resolved, err
	}
	if apiLog.Id > 0 {
		if tenantConflict(resolved.TenantID, apiLog.TenantId) {
			resolved.Orphaned = true
			return resolved, nil
		}
		resolved.APILogID = apiLog.Id
		if resolved.TenantID == 0 {
			resolved.TenantID = apiLog.TenantId
		}
		if resolved.APIID == 0 {
			resolved.APIID = apiLog.ApiId
		}
		if resolved.Recipient == "" {
			resolved.Recipient = apiLog.Recipient
		}
		if resolved.MessageID == "" {
			resolved.MessageID = canonicalMessageID(apiLog.MessageId)
		}
	}

	resolved.Orphaned = injection.Id == 0 && recipient.Id == 0 && apiLog.Id == 0
	return resolved, nil
}

func (dbDeliveryEventStore) updateDeliveryEventRow(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) error {
	_, err := tx.Model("kumo_delivery_events").Ctx(ctx).Where("event_hash", event.EventHash).Data(g.Map{
		"tenant_id":         event.TenantID,
		"message_id":        event.MessageID,
		"recipient":         event.Recipient,
		"queue_name":        event.QueueName,
		"campaign_id":       event.CampaignID,
		"task_id":           event.TaskID,
		"recipient_info_id": event.RecipientInfoID,
		"api_id":            event.APIID,
		"api_log_id":        event.APILogID,
		"delivery_status":   event.DeliveryStatus,
		"response":          event.Response,
		"remote_mx":         event.RemoteMX,
		"orphaned":          event.Orphaned,
	}).Update()
	return err
}

func (dbDeliveryEventStore) applyRecipientEvent(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) (bool, error) {
	var current recipientLookup
	read := tx.Model("recipient_info").Ctx(ctx).Where("id", event.RecipientInfoID)
	if event.TenantID > 0 {
		read = read.Where("tenant_id", event.TenantID)
	}
	if err := read.Scan(&current); err != nil {
		return false, err
	}
	if current.Id == 0 {
		return false, nil
	}
	nextStatus, changed := NextDeliveryStatus(current.DeliveryStatus, event.DeliveryStatus)
	data := g.Map{
		"engine":                 "kumomta",
		"last_delivery_event_at": event.OccurredAt,
		"last_delivery_response": event.Response,
	}
	if changed {
		data["delivery_status"] = nextStatus
	}
	if event.QueueName != "" {
		data["kumo_queue"] = event.QueueName
	}
	update := tx.Model("recipient_info").Ctx(ctx).Where("id", event.RecipientInfoID)
	if event.TenantID > 0 {
		update = update.Where("tenant_id", event.TenantID)
	}
	_, err := update.Data(data).Update()
	return changed, err
}

func (dbDeliveryEventStore) applyAPILogEvent(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) (bool, error) {
	var current apiLogLookup
	read := tx.Model("api_mail_logs").Ctx(ctx).Where("id", event.APILogID)
	if event.TenantID > 0 {
		read = read.Where("tenant_id", event.TenantID)
	}
	if err := read.Scan(&current); err != nil {
		return false, err
	}
	if current.Id == 0 {
		return false, nil
	}
	nextStatus, changed := NextDeliveryStatus(current.DeliveryStatus, event.DeliveryStatus)
	data := g.Map{
		"engine":                 "kumomta",
		"last_delivery_event_at": event.OccurredAt,
		"last_delivery_response": event.Response,
	}
	if changed {
		data["delivery_status"] = nextStatus
	}
	if event.QueueName != "" {
		data["kumo_queue"] = event.QueueName
	}
	update := tx.Model("api_mail_logs").Ctx(ctx).Where("id", event.APILogID)
	if event.TenantID > 0 {
		update = update.Where("tenant_id", event.TenantID)
	}
	_, err := update.Data(data).Update()
	return changed, err
}

func (dbDeliveryEventStore) applyInjectionEvent(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) error {
	where := tx.Model("kumo_message_injections").Ctx(ctx)
	if event.TenantID > 0 {
		where = where.Where("tenant_id", event.TenantID)
	}
	switch {
	case event.KumoInjectionID > 0:
		where = where.Where("id", event.KumoInjectionID)
	case event.RecipientInfoID > 0:
		where = where.Where("recipient_info_id", event.RecipientInfoID)
	case event.APILogID > 0:
		where = where.Where("api_log_id", event.APILogID)
	case event.MessageID != "":
		where = where.Where("message_id", event.MessageID)
	default:
		return nil
	}
	var current struct {
		DeliveryStatus string `orm:"delivery_status"`
	}
	if err := where.Fields("delivery_status").Scan(&current); err != nil {
		return err
	}
	nextStatus, changed := NextDeliveryStatus(current.DeliveryStatus, event.DeliveryStatus)
	data := g.Map{
		"updated_at": time.Now().Unix(),
	}
	if changed {
		data["delivery_status"] = nextStatus
	}
	if shouldSuppressForEvent(event) || event.DeliveryStatus == DeliveryStatusDelivered || event.DeliveryStatus == DeliveryStatusExpired {
		data["final_event_at"] = event.OccurredAt
	}
	if event.Response != "" {
		data["last_error"] = event.Response
	}
	update := tx.Model("kumo_message_injections").Ctx(ctx)
	if event.TenantID > 0 {
		update = update.Where("tenant_id", event.TenantID)
	}
	switch {
	case event.KumoInjectionID > 0:
		update = update.Where("id", event.KumoInjectionID)
	case event.RecipientInfoID > 0:
		update = update.Where("recipient_info_id", event.RecipientInfoID)
	case event.APILogID > 0:
		update = update.Where("api_log_id", event.APILogID)
	case event.MessageID != "":
		update = update.Where("message_id", event.MessageID)
	default:
		return nil
	}
	_, err := update.Data(data).Update()
	return err
}

func (dbDeliveryEventStore) applySuppression(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) error {
	if event.Recipient == "" {
		return nil
	}
	description := fmt.Sprintf("KumoMTA %s: %s", event.DeliveryStatus, event.Response)
	if len(description) > 255 {
		description = description[:255]
	}
	var existing struct {
		Id    int64 `orm:"id"`
		Count int   `orm:"count"`
	}
	if err := tx.Model("abnormal_recipient").Ctx(ctx).Where("tenant_id", event.TenantID).Where("recipient", event.Recipient).Fields("id,count").Scan(&existing); err != nil {
		return err
	}
	if existing.Id > 0 {
		_, err := tx.Model("abnormal_recipient").Ctx(ctx).Where("id", existing.Id).Where("tenant_id", event.TenantID).Data(g.Map{
			"count":       existing.Count + 1,
			"description": description,
			"add_type":    2,
		}).Update()
		return err
	}
	_, err := tx.Model("abnormal_recipient").Ctx(ctx).Data(g.Map{
		"tenant_id":   event.TenantID,
		"recipient":   event.Recipient,
		"count":       1,
		"description": description,
		"add_type":    2,
		"create_time": time.Now().Unix(),
	}).InsertIgnore()
	return err
}

func (dbDeliveryEventStore) incrementTenantUsage(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) error {
	if event.TenantID <= 0 {
		return nil
	}
	column := ""
	switch event.DeliveryStatus {
	case DeliveryStatusDelivered:
		column = "delivered_count"
	case DeliveryStatusBounced:
		column = "bounced_count"
	case DeliveryStatusComplained:
		column = "complained_count"
	default:
		return nil
	}
	occurred := time.Unix(event.OccurredAt, 0)
	if event.OccurredAt <= 0 {
		occurred = time.Now()
	}
	_, err := tx.Exec(fmt.Sprintf(`
		INSERT INTO tenant_usage_daily
			(tenant_id, date, %s, create_time, update_time)
		VALUES
			(?, ?, 1, EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW()))
		ON CONFLICT (tenant_id, date)
		DO UPDATE SET
			%s = tenant_usage_daily.%s + 1,
			update_time = EXCLUDED.update_time
	`, column, column, column), event.TenantID, occurred.Format("2006-01-02"))
	return err
}

func (dbDeliveryEventStore) refreshTaskDeliveryCounters(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) error {
	taskID := event.TaskID
	if taskID <= 0 {
		return nil
	}
	counter := func() *gdb.Model {
		model := tx.Model("recipient_info").Ctx(ctx).Where("task_id", taskID)
		if event.TenantID > 0 {
			model = model.Where("tenant_id", event.TenantID)
		}
		return model
	}
	queued, err := counter().Where("engine", "kumomta").Where("injection_status", InjectionStatusQueued).Count()
	if err != nil {
		return err
	}
	delivered, err := counter().Where("delivery_status", DeliveryStatusDelivered).Count()
	if err != nil {
		return err
	}
	bounced, err := counter().Where("delivery_status", DeliveryStatusBounced).Count()
	if err != nil {
		return err
	}
	deferred, err := counter().Where("delivery_status", DeliveryStatusDeferred).Count()
	if err != nil {
		return err
	}
	expired, err := counter().Where("delivery_status", DeliveryStatusExpired).Count()
	if err != nil {
		return err
	}
	complained, err := counter().Where("delivery_status", DeliveryStatusComplained).Count()
	if err != nil {
		return err
	}
	update := tx.Model("email_tasks").Ctx(ctx).Where("id", taskID)
	if event.TenantID > 0 {
		update = update.Where("tenant_id", event.TenantID)
	}
	_, err = update.Data(g.Map{
		"queued_count":      queued,
		"delivered_count":   delivered,
		"bounced_count":     bounced,
		"deferred_count":    deferred,
		"expired_count":     expired,
		"complained_count":  complained,
		"stats_update_time": time.Now().Unix(),
	}).Update()
	return err
}

func lookupInjection(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) (injectionLookup, error) {
	var injection injectionLookup
	model := tx.Model("kumo_message_injections").Ctx(ctx)
	if event.TenantID > 0 {
		model = model.Where("tenant_id", event.TenantID)
	}
	switch {
	case event.RecipientInfoID > 0:
		model = model.Where("recipient_info_id", event.RecipientInfoID)
	case event.APILogID > 0:
		model = model.Where("api_log_id", event.APILogID)
	case event.MessageID != "" && event.Recipient != "":
		model = model.Where("message_id", event.MessageID).Where("recipient", event.Recipient)
	case event.MessageID != "":
		model = model.Where("message_id", event.MessageID)
	default:
		return injection, nil
	}
	err := model.Limit(1).Scan(&injection)
	return injection, err
}

func lookupRecipient(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) (recipientLookup, error) {
	var recipient recipientLookup
	model := tx.Model("recipient_info").Ctx(ctx)
	if event.TenantID > 0 {
		model = model.Where("tenant_id", event.TenantID)
	}
	switch {
	case event.RecipientInfoID > 0:
		model = model.Where("id", event.RecipientInfoID)
	case event.MessageID != "" && event.Recipient != "":
		model = model.Where("message_id", event.MessageID).Where("recipient", event.Recipient)
	case event.MessageID != "":
		model = model.Where("message_id", event.MessageID)
	default:
		return recipient, nil
	}
	err := model.Limit(1).Scan(&recipient)
	return recipient, err
}

func lookupAPILog(ctx context.Context, tx gdb.TX, event NormalizedDeliveryEvent) (apiLogLookup, error) {
	var log apiLogLookup
	model := tx.Model("api_mail_logs").Ctx(ctx)
	if event.TenantID > 0 {
		model = model.Where("tenant_id", event.TenantID)
	}
	switch {
	case event.APILogID > 0:
		model = model.Where("id", event.APILogID)
	case event.MessageID != "" && event.Recipient != "":
		model = model.Where("message_id", event.MessageID).Where("recipient", event.Recipient)
	case event.MessageID != "":
		model = model.Where("message_id", event.MessageID)
	default:
		return log, nil
	}
	err := model.Limit(1).Scan(&log)
	return log, err
}

func fillFromInjection(event *NormalizedDeliveryEvent, injection injectionLookup) {
	event.KumoInjectionID = injection.Id
	if event.TenantID == 0 {
		event.TenantID = injection.TenantId
	}
	if event.MessageID == "" {
		event.MessageID = canonicalMessageID(injection.MessageId)
	}
	if event.Recipient == "" {
		event.Recipient = injection.Recipient
	}
	if event.CampaignID == 0 {
		event.CampaignID = injection.CampaignId
	}
	if event.TaskID == 0 {
		event.TaskID = injection.TaskId
	}
	if event.RecipientInfoID == 0 {
		event.RecipientInfoID = injection.RecipientInfoId
	}
	if event.APIID == 0 {
		event.APIID = injection.ApiId
	}
	if event.APILogID == 0 {
		event.APILogID = injection.ApiLogId
	}
	if event.QueueName == "" {
		event.QueueName = injection.QueueName
	}
}

func tenantConflict(eventTenantID, recordTenantID int64) bool {
	return eventTenantID > 0 && recordTenantID > 0 && eventTenantID != recordTenantID
}

func isRedisDuplicate(ctx context.Context, key string) bool {
	if key == "" {
		return false
	}
	val, err := g.Redis().Get(ctx, key)
	if err != nil || val.IsNil() {
		return false
	}
	duplicate := strings.TrimSpace(val.String()) != ""
	if duplicate {
		RecordWebhookRedisDedupeHit(ctx)
	}
	return duplicate
}

func cacheDuplicate(ctx context.Context, key string) {
	if key != "" {
		_ = g.Redis().SetEX(ctx, key, "1", int64(webhookIdempotencyTTL.Seconds()))
	}
}

func cacheEventDedupe(ctx context.Context, event NormalizedDeliveryEvent) {
	if event.ProviderEventID != "" {
		cacheDuplicate(ctx, "kumo:delivery_event:event_id:"+event.ProviderEventID)
	}
	if event.EventHash != "" {
		cacheDuplicate(ctx, "kumo:delivery_event:event_hash:"+event.EventHash)
	}
}

func RecordMessageInjection(ctx context.Context, record MessageInjectionRecord) error {
	now := time.Now().Unix()
	messageID := canonicalMessageID(record.MessageID)
	recipient := normalizeAddress(record.Recipient)
	if messageID == "" {
		return fmt.Errorf("message_id is required")
	}
	if recipient == "" {
		return fmt.Errorf("recipient is required")
	}
	if record.DeliveryStatus == "" {
		record.DeliveryStatus = DeliveryStatusPending
	}
	if record.InjectionStatus == "" {
		record.InjectionStatus = InjectionStatusPending
	}
	_, err := g.DB().Model("kumo_message_injections").Ctx(ctx).
		Data(g.Map{
			"tenant_id":          record.TenantID,
			"message_id":         messageID,
			"recipient":          recipient,
			"recipient_domain":   record.RecipientDomain,
			"campaign_id":        record.CampaignID,
			"task_id":            record.TaskID,
			"recipient_info_id":  record.RecipientInfoID,
			"api_id":             record.APIID,
			"api_log_id":         record.APILogID,
			"sending_profile_id": record.SendingProfileID,
			"queue_name":         record.QueueName,
			"injection_status":   record.InjectionStatus,
			"delivery_status":    record.DeliveryStatus,
			"attempt_count":      record.AttemptCount,
			"next_retry_at":      record.NextRetryAt,
			"accepted_at":        record.AcceptedAt,
			"last_error":         record.LastError,
			"created_at":         now,
			"updated_at":         now,
		}).
		OnConflict("message_id,recipient").
		OnDuplicate(g.Map{
			"tenant_id":          record.TenantID,
			"recipient_domain":   record.RecipientDomain,
			"campaign_id":        record.CampaignID,
			"task_id":            record.TaskID,
			"recipient_info_id":  record.RecipientInfoID,
			"api_id":             record.APIID,
			"api_log_id":         record.APILogID,
			"sending_profile_id": record.SendingProfileID,
			"queue_name":         record.QueueName,
			"injection_status":   record.InjectionStatus,
			"delivery_status":    record.DeliveryStatus,
			"attempt_count":      record.AttemptCount,
			"next_retry_at":      record.NextRetryAt,
			"accepted_at":        record.AcceptedAt,
			"last_error":         record.LastError,
			"updated_at":         now,
		}).
		Save()
	return err
}

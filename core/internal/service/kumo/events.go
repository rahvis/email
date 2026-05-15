package kumo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	DeliveryStatusPending    = "pending"
	DeliveryStatusDelivered  = "delivered"
	DeliveryStatusDeferred   = "deferred"
	DeliveryStatusBounced    = "bounced"
	DeliveryStatusExpired    = "expired"
	DeliveryStatusComplained = "complained"
	DeliveryStatusSuppressed = "suppressed"
	DeliveryStatusUnknown    = "unknown"

	InjectionStatusPending   = "pending"
	InjectionStatusRendering = "rendering"
	InjectionStatusInjecting = "injecting"
	InjectionStatusQueued    = "queued"
	InjectionStatusRetrying  = "retrying"
	InjectionStatusFailed    = "failed"
	InjectionStatusCancelled = "cancelled"
)

type deliveryEventStore interface {
	IsDuplicateEvent(ctx context.Context, event NormalizedDeliveryEvent) (bool, error)
	StoreEvent(ctx context.Context, event NormalizedDeliveryEvent) (bool, error)
	ApplyEvent(ctx context.Context, event NormalizedDeliveryEvent) error
}

var (
	eventStoreMu     sync.RWMutex
	activeEventStore deliveryEventStore = dbDeliveryEventStore{}
)

func NormalizeDeliveryEvents(body []byte) ([]NormalizedDeliveryEvent, int, error) {
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, 0, err
	}

	rawEvents := extractRawEvents(payload)
	if len(rawEvents) == 0 {
		return nil, 1, nil
	}

	events := make([]NormalizedDeliveryEvent, 0, len(rawEvents))
	failed := 0
	for _, raw := range rawEvents {
		event, ok := normalizeOneEvent(raw)
		if !ok {
			failed++
			continue
		}
		events = append(events, event)
	}
	return events, failed, nil
}

func IngestNormalizedEvents(ctx context.Context, events []NormalizedDeliveryEvent) (*WebhookIngestResult, error) {
	result := &WebhookIngestResult{}
	store := getEventStore()

	for _, event := range events {
		if event.EventHash == "" {
			event.EventHash = eventFallbackHash(event)
		}
		duplicate, err := store.IsDuplicateEvent(ctx, event)
		if err != nil {
			result.Failed++
			g.Log().Warningf(ctx, "KumoMTA event duplicate check failed: event_hash=%s error=%s", event.EventHash, sanitizeLogText(err.Error()))
			continue
		}
		if duplicate {
			result.Duplicates++
			continue
		}

		inserted, err := store.StoreEvent(ctx, event)
		if err != nil {
			result.Failed++
			g.Log().Warningf(ctx, "KumoMTA event store failed: event_hash=%s error=%s", event.EventHash, sanitizeLogText(err.Error()))
			continue
		}
		if !inserted {
			result.Duplicates++
			continue
		}

		if err := store.ApplyEvent(ctx, event); err != nil {
			result.Failed++
			g.Log().Warningf(ctx, "KumoMTA event apply failed: event_hash=%s error=%s", event.EventHash, sanitizeLogText(err.Error()))
			continue
		}
		result.Accepted++
	}

	return result, nil
}

func eventFallbackHash(event NormalizedDeliveryEvent) string {
	body, err := json.Marshal(event)
	if err != nil {
		body = []byte(fmt.Sprintf("%s|%s|%s|%s", event.ProviderEventID, event.EventType, event.MessageID, event.Recipient))
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func NextDeliveryStatus(current, next string) (string, bool) {
	current = normalizeDeliveryStatus(current)
	next = normalizeDeliveryStatus(next)
	if next == DeliveryStatusPending || next == DeliveryStatusUnknown {
		if current == "" {
			return next, true
		}
		return current, false
	}
	if current == "" || current == DeliveryStatusPending || current == DeliveryStatusUnknown {
		return next, true
	}
	if current == DeliveryStatusDeferred {
		return next, true
	}
	if next == DeliveryStatusComplained && current == DeliveryStatusDelivered {
		return next, true
	}
	return current, false
}

func deliveryStatusForEventType(eventType string) string {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "delivery", "delivered":
		return DeliveryStatusDelivered
	case "transientfailure", "transient_failure", "deferred", "delay", "delayed", "tempfail", "temporary_failure":
		return DeliveryStatusDeferred
	case "bounce", "bounced", "adminbounce", "admin_bounce", "permanent_failure", "failed", "rejected", "rejection", "policy_failure":
		return DeliveryStatusBounced
	case "expiration", "expired":
		return DeliveryStatusExpired
	case "feedback", "complaint", "complained", "feedback_loop", "fbl":
		return DeliveryStatusComplained
	case "suppressed", "suppression":
		return DeliveryStatusSuppressed
	default:
		return DeliveryStatusUnknown
	}
}

func shouldSuppressForEvent(event NormalizedDeliveryEvent) bool {
	switch event.DeliveryStatus {
	case DeliveryStatusBounced, DeliveryStatusComplained, DeliveryStatusSuppressed:
		return true
	default:
		return false
	}
}

func extractRawEvents(payload interface{}) []map[string]interface{} {
	switch typed := payload.(type) {
	case map[string]interface{}:
		if events, ok := typed["events"].([]interface{}); ok {
			out := make([]map[string]interface{}, 0, len(events))
			for _, item := range events {
				if raw, ok := item.(map[string]interface{}); ok {
					out = append(out, raw)
				} else {
					out = append(out, nil)
				}
			}
			return out
		}
		if records, ok := typed["records"].([]interface{}); ok {
			out := make([]map[string]interface{}, 0, len(records))
			for _, item := range records {
				if raw, ok := item.(map[string]interface{}); ok {
					out = append(out, raw)
				} else {
					out = append(out, nil)
				}
			}
			return out
		}
		return []map[string]interface{}{typed}
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if raw, ok := item.(map[string]interface{}); ok {
				out = append(out, raw)
			} else {
				out = append(out, nil)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeOneEvent(raw map[string]interface{}) (NormalizedDeliveryEvent, bool) {
	if len(raw) == 0 {
		return NormalizedDeliveryEvent{}, false
	}

	headers := stringMap(raw["headers"])
	meta := stringMap(raw["meta"])
	for key, value := range stringMap(raw["metadata"]) {
		meta[key] = value
	}

	eventType := firstString(raw, "event_type", "type", "record_type")
	if strings.TrimSpace(eventType) == "" {
		return NormalizedDeliveryEvent{}, false
	}

	providerEventID := firstString(raw, "event_id", "provider_event_id")
	if providerEventID == "" && raw["event_type"] != nil {
		providerEventID = firstString(raw, "id")
	}

	event := NormalizedDeliveryEvent{
		ProviderEventID: providerEventID,
		EventType:       eventType,
		DeliveryStatus:  deliveryStatusForEventType(eventType),
		MessageID:       canonicalMessageID(firstString(raw, "message_id", "msg_id")),
		Recipient:       firstString(raw, "recipient", "recipient_address", "rcpt_to"),
		Sender:          firstString(raw, "sender", "envelope_sender", "mail_from"),
		QueueName:       firstString(raw, "queue", "queue_name"),
		Response:        responseText(raw),
		RemoteMX:        firstString(raw, "remote_mx", "site", "mx", "peer_address"),
		OccurredAt:      parseEventTime(raw),
		RawEvent:        sanitizeValue(raw).(map[string]interface{}),
	}

	if event.MessageID == "" {
		event.MessageID = canonicalMessageID(firstStringMap(headers, "Message-ID", "X-BM-Message-ID"))
	}
	if event.MessageID == "" {
		event.MessageID = canonicalMessageID(firstStringMap(meta, "message_id", "bm_message_id"))
	}
	if event.Recipient == "" {
		event.Recipient = firstRecipient(raw)
	}
	if event.QueueName == "" {
		event.QueueName = firstStringMap(meta, "queue", "queue_name", "routing_domain")
	}

	applyHeaderCorrelation(&event, headers)
	applyMetadataCorrelation(&event, meta)
	applyQueueCorrelation(&event)
	if event.TaskID == 0 && event.CampaignID > 0 {
		event.TaskID = event.CampaignID
	}

	if event.EventHash == "" {
		event.EventHash = stableDeliveryEventHash(raw)
	}
	if event.OccurredAt == 0 {
		event.OccurredAt = time.Now().Unix()
	}
	return event, true
}

func applyHeaderCorrelation(event *NormalizedDeliveryEvent, headers map[string]string) {
	if event == nil {
		return
	}
	event.TenantID = firstInt64(event.TenantID, headers, "X-BM-Tenant-ID")
	event.CampaignID = firstInt64(event.CampaignID, headers, "X-BM-Campaign-ID")
	event.TaskID = firstInt64(event.TaskID, headers, "X-BM-Task-ID")
	event.RecipientInfoID = firstInt64(event.RecipientInfoID, headers, "X-BM-Recipient-ID")
	event.APIID = firstInt64(event.APIID, headers, "X-BM-Api-ID")
	event.APILogID = firstInt64(event.APILogID, headers, "X-BM-Api-Log-ID")
	if event.MessageID == "" {
		event.MessageID = canonicalMessageID(firstStringMap(headers, "X-BM-Message-ID"))
	}
}

func applyMetadataCorrelation(event *NormalizedDeliveryEvent, meta map[string]string) {
	if event == nil {
		return
	}
	event.TenantID = firstInt64(event.TenantID, meta, "tenant_id", "bm_tenant_id")
	event.CampaignID = firstInt64(event.CampaignID, meta, "campaign_id", "bm_campaign_id")
	event.TaskID = firstInt64(event.TaskID, meta, "task_id", "bm_task_id")
	event.RecipientInfoID = firstInt64(event.RecipientInfoID, meta, "recipient_id", "recipient_info_id", "bm_recipient_id")
	event.APIID = firstInt64(event.APIID, meta, "api_id", "bm_api_id")
	event.APILogID = firstInt64(event.APILogID, meta, "api_log_id", "bm_api_log_id")
	if event.MessageID == "" {
		event.MessageID = canonicalMessageID(firstStringMap(meta, "message_id", "bm_message_id"))
	}
}

func applyQueueCorrelation(event *NormalizedDeliveryEvent) {
	if event == nil || strings.TrimSpace(event.QueueName) == "" {
		return
	}
	queue := strings.TrimSpace(event.QueueName)
	parts := strings.SplitN(queue, ":", 2)
	if len(parts) == 2 {
		switch {
		case strings.HasPrefix(parts[0], "campaign_") && event.CampaignID == 0:
			event.CampaignID = parseIDSuffix(parts[0], "campaign_")
			if event.TaskID == 0 {
				event.TaskID = event.CampaignID
			}
		case strings.HasPrefix(parts[0], "api_") && event.APIID == 0:
			event.APIID = parseIDSuffix(parts[0], "api_")
		}
		tenantPart := parts[1]
		if at := strings.Index(tenantPart, "@"); at >= 0 {
			tenantPart = tenantPart[:at]
		}
		if strings.HasPrefix(tenantPart, "tenant_") && event.TenantID == 0 {
			event.TenantID = parseIDSuffix(tenantPart, "tenant_")
		}
	}
}

func stableDeliveryEventHash(raw map[string]interface{}) string {
	body, err := json.Marshal(raw)
	if err != nil {
		body = []byte(fmt.Sprintf("%v", raw))
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func canonicalMessageID(messageID string) string {
	messageID = strings.TrimSpace(messageID)
	messageID = strings.Trim(messageID, "<>")
	return messageID
}

func responseText(raw map[string]interface{}) string {
	if response := firstString(raw, "response", "delivery_response", "reason", "message"); response != "" {
		return response
	}
	if response, ok := raw["response"].(map[string]interface{}); ok {
		code := firstString(response, "code", "status", "enhanced_code")
		text := firstString(response, "content", "message", "reason")
		return strings.TrimSpace(strings.Join([]string{code, text}, " "))
	}
	return ""
}

func firstRecipient(raw map[string]interface{}) string {
	if recipients, ok := raw["recipients"].([]interface{}); ok && len(recipients) > 0 {
		switch first := recipients[0].(type) {
		case string:
			return first
		case map[string]interface{}:
			return firstString(first, "email", "recipient")
		}
	}
	return ""
}

func parseEventTime(raw map[string]interface{}) int64 {
	for _, key := range []string{"timestamp", "occurred_at", "created", "created_at"} {
		if ts := parseTimeValue(raw[key]); ts > 0 {
			return ts
		}
	}
	for _, key := range []string{"created_time", "timestamp_time", "occurred_time"} {
		if ts := parseTimeValue(raw[key]); ts > 0 {
			return ts
		}
	}
	return 0
}

func parseTimeValue(value interface{}) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		v, _ := typed.Int64()
		return v
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return 0
		}
		if n, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return n
		}
		if ts, err := time.Parse(time.RFC3339Nano, typed); err == nil {
			return ts.Unix()
		}
	}
	return 0
}

func firstString(raw map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			if text := stringify(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func firstStringMap(values map[string]string, keys ...string) string {
	for _, key := range keys {
		for mapKey, value := range values {
			if strings.EqualFold(mapKey, key) {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func stringify(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func stringMap(value interface{}) map[string]string {
	out := map[string]string{}
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, val := range typed {
			if text := stringify(val); text != "" {
				out[key] = text
			}
		}
	case map[string]string:
		for key, val := range typed {
			if strings.TrimSpace(val) != "" {
				out[key] = strings.TrimSpace(val)
			}
		}
	}
	return out
}

func firstInt64(current int64, values map[string]string, keys ...string) int64 {
	if current > 0 {
		return current
	}
	raw := firstStringMap(values, keys...)
	if raw == "" {
		return 0
	}
	if parsed := parseID(raw); parsed > 0 {
		return parsed
	}
	return 0
}

func parseID(raw string) int64 {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "tenant_")
	raw = strings.TrimPrefix(raw, "campaign_")
	raw = strings.TrimPrefix(raw, "api_")
	raw = strings.Trim(raw, "<> ")
	v, _ := strconv.ParseInt(raw, 10, 64)
	return v
}

func parseIDSuffix(raw, prefix string) int64 {
	return parseID(strings.TrimPrefix(raw, prefix))
}

func normalizeDeliveryStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return ""
	}
	switch status {
	case DeliveryStatusPending, DeliveryStatusDelivered, DeliveryStatusDeferred,
		DeliveryStatusBounced, DeliveryStatusExpired, DeliveryStatusComplained,
		DeliveryStatusSuppressed, DeliveryStatusUnknown:
		return status
	default:
		return DeliveryStatusUnknown
	}
}

func sanitizeLogText(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.TrimSpace(value)
	if len(value) > 300 {
		value = value[:300]
	}
	return value
}

func getEventStore() deliveryEventStore {
	eventStoreMu.RLock()
	defer eventStoreMu.RUnlock()
	return activeEventStore
}

func setEventStoreForTesting(store deliveryEventStore) func() {
	eventStoreMu.Lock()
	old := activeEventStore
	activeEventStore = store
	eventStoreMu.Unlock()
	return func() {
		eventStoreMu.Lock()
		activeEventStore = old
		eventStoreMu.Unlock()
	}
}

func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	if parsed, err := mail.ParseAddress(address); err == nil {
		return parsed.Address
	}
	return address
}

package kumo

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func IngestWebhook(ctx context.Context, r *ghttp.Request) (*WebhookIngestResult, error) {
	secret, err := GetWebhookSecret(ctx)
	if err != nil {
		RecordWebhookProcessingFailure(ctx, err.Error())
		return nil, err
	}
	if strings.TrimSpace(secret) == "" {
		RecordWebhookSecurityFailure(ctx, "webhook secret is not configured")
		return nil, fmt.Errorf("KumoMTA webhook secret is not configured")
	}

	body, err := readLimitedBody(r.Request.Body, webhookBodyLimitBytes)
	if err != nil {
		RecordWebhookProcessingFailure(ctx, err.Error())
		return nil, err
	}
	if err := VerifyWebhookSignature(secret, body, r.Request.Header, time.Now()); err != nil {
		RecordWebhookSecurityFailure(ctx, err.Error())
		return nil, err
	}

	eventID := firstEventID(body)
	rawEvent := sanitizeRawEvent(body)
	if err := storeRawWebhookEvent(ctx, stableEventHash(body), eventID, rawEvent, len(body), r.GetClientIp()); err != nil {
		RecordWebhookProcessingFailure(ctx, err.Error())
		return nil, err
	}

	events, failed, err := NormalizeDeliveryEvents(body)
	if err != nil {
		RecordWebhookProcessingFailure(ctx, err.Error())
		return nil, err
	}
	result, err := IngestNormalizedEvents(ctx, events)
	if err != nil {
		RecordWebhookProcessingFailure(ctx, err.Error())
		return nil, err
	}
	result.Failed += failed
	RecordWebhookIngestMetrics(ctx, events, result)

	markWebhookSeen()
	return result, nil
}

func VerifyWebhookSignature(secret string, body []byte, headers http.Header, now time.Time) error {
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("webhook secret is not configured")
	}

	if token := firstHeader(headers, "X-BM-Kumo-Token", "X-Kumo-Webhook-Token"); token != "" {
		if constantTimeEqual(token, secret) {
			return nil
		}
	}

	if auth := headers.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if constantTimeEqual(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")), secret) {
			return nil
		}
	}

	signature := firstHeader(headers, "X-BM-Kumo-Signature", "X-Kumo-Signature")
	if signature == "" {
		return fmt.Errorf("missing webhook signature")
	}

	timestamp := firstHeader(headers, "X-BM-Kumo-Timestamp", "X-Kumo-Timestamp")
	if timestamp == "" {
		return fmt.Errorf("missing webhook timestamp")
	}
	unixTimestamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid webhook timestamp")
	}
	eventTime := time.Unix(unixTimestamp, 0)
	if now.Sub(eventTime) > webhookTimestampTolerance || eventTime.Sub(now) > webhookTimestampTolerance {
		return fmt.Errorf("webhook timestamp outside tolerance")
	}

	expected := ComputeWebhookSignature(secret, timestamp, body)
	signature = normalizeSignature(signature)
	if signature == "" || !constantTimeEqual(signature, expected) {
		return fmt.Errorf("invalid webhook signature")
	}

	return nil
}

func ComputeWebhookSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func readLimitedBody(reader io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, fmt.Errorf("webhook body exceeds %d bytes", limit)
	}
	return buf.Bytes(), nil
}

func stableEventHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func storeRawWebhookEvent(ctx context.Context, eventHash, eventID string, rawEvent interface{}, bodySize int, remoteIP string) error {
	if _, err := json.Marshal(rawEvent); err != nil {
		return err
	}
	_, err := g.DB().Model("kumo_webhook_events").Ctx(ctx).Data(g.Map{
		"event_hash":  eventHash,
		"event_id":    eventID,
		"raw_event":   rawEvent,
		"body_size":   bodySize,
		"remote_ip":   remoteIP,
		"received_at": time.Now().Unix(),
	}).InsertIgnore()
	if err != nil {
		return err
	}
	return nil
}

func firstEventID(body []byte) string {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if eventID, ok := payload["event_id"].(string); ok {
		return eventID
	}
	events, ok := payload["events"].([]interface{})
	if !ok || len(events) == 0 {
		return ""
	}
	first, ok := events[0].(map[string]interface{})
	if !ok {
		return ""
	}
	if eventID, ok := first["event_id"].(string); ok {
		return eventID
	}
	return ""
}

func sanitizeRawEvent(body []byte) interface{} {
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		raw := string(body)
		if len(raw) > 4096 {
			raw = raw[:4096]
		}
		return map[string]interface{}{"raw": raw}
	}
	return sanitizeValue(payload)
}

func sanitizeValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, val := range typed {
			if isSensitiveEventKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = sanitizeValue(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, val := range typed {
			out = append(out, sanitizeValue(val))
		}
		return out
	case string:
		if len(typed) > 4096 {
			return typed[:4096] + "...[truncated]"
		}
		return typed
	default:
		return typed
	}
}

func isSensitiveEventKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "authorization", "auth", "auth_secret", "webhook_secret", "secret", "token",
		"content", "body", "message_body", "rfc822", "raw_message", "private_key":
		return true
	default:
		return false
	}
}

func firstHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func normalizeSignature(signature string) string {
	signature = strings.TrimSpace(signature)
	signature = strings.TrimPrefix(signature, "sha256=")
	signature = strings.TrimPrefix(signature, "SHA256=")
	return strings.ToLower(signature)
}

func constantTimeEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

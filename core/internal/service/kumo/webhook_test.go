package kumo

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVerifyWebhookSignatureRejectsMissingAuth(t *testing.T) {
	err := VerifyWebhookSignature("secret", []byte(`{}`), http.Header{}, time.Now())
	require.Error(t, err)
}

func TestVerifyWebhookSignatureAcceptsBearerToken(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer webhook-secret")

	err := VerifyWebhookSignature("webhook-secret", []byte(`{}`), headers, time.Now())
	require.NoError(t, err)
}

func TestVerifyWebhookSignatureAcceptsStaticTokenHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-BM-Kumo-Token", "webhook-secret")

	err := VerifyWebhookSignature("webhook-secret", []byte(`{}`), headers, time.Now())
	require.NoError(t, err)
}

func TestVerifyWebhookSignatureAcceptsHMAC(t *testing.T) {
	body := []byte(`{"events":[]}`)
	timestamp := time.Now().Unix()
	headers := http.Header{}
	headers.Set("X-BM-Kumo-Timestamp", " "+strconvFormat(timestamp)+" ")
	headers.Set("X-BM-Kumo-Signature", "sha256="+ComputeWebhookSignature("webhook-secret", strconvFormat(timestamp), body))

	err := VerifyWebhookSignature("webhook-secret", body, headers, time.Now())
	require.NoError(t, err)
}

func TestVerifyWebhookSignatureRejectsInvalidHMAC(t *testing.T) {
	body := []byte(`{"events":[]}`)
	timestamp := strconvFormat(time.Now().Unix())
	headers := http.Header{}
	headers.Set("X-BM-Kumo-Timestamp", timestamp)
	headers.Set("X-BM-Kumo-Signature", strings.Repeat("0", 64))

	err := VerifyWebhookSignature("webhook-secret", body, headers, time.Now())
	require.Error(t, err)
}

func TestVerifyWebhookSignatureRejectsStaleTimestamp(t *testing.T) {
	body := []byte(`{"events":[]}`)
	timestamp := time.Now().Add(-10 * time.Minute).Unix()
	headers := http.Header{}
	headers.Set("X-BM-Kumo-Timestamp", strconvFormat(timestamp))
	headers.Set("X-BM-Kumo-Signature", ComputeWebhookSignature("webhook-secret", strconvFormat(timestamp), body))

	err := VerifyWebhookSignature("webhook-secret", body, headers, time.Now())
	require.Error(t, err)
}

func TestSanitizeRawEventRedactsSensitiveFields(t *testing.T) {
	sanitized := sanitizeRawEvent([]byte(`{
		"event_id": "evt-1",
		"content": "full-message",
		"headers": {
			"Authorization": "Bearer secret",
			"X-BM-Tenant-ID": "42"
		},
		"events": [{"body": "message body", "response": "250 OK"}]
	}`))

	payload, ok := sanitized.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "[redacted]", payload["content"])

	headers, ok := payload["headers"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "[redacted]", headers["Authorization"])
	require.Equal(t, "42", headers["X-BM-Tenant-ID"])

	events := payload["events"].([]interface{})
	first := events[0].(map[string]interface{})
	require.Equal(t, "[redacted]", first["body"])
	require.Equal(t, "250 OK", first["response"])
}

func TestReadLimitedBodyRejectsOversize(t *testing.T) {
	_, err := readLimitedBody(strings.NewReader("123456"), 5)
	require.Error(t, err)
}

func strconvFormat(v int64) string {
	return strconv.FormatInt(v, 10)
}

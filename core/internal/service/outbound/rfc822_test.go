package outbound

import (
	"strings"
	"testing"
)

func TestBuildRFC822AddsMissingCorrelationHeadersToExistingMessage(t *testing.T) {
	body, err := BuildRFC822(OutboundMessage{
		TenantID:  42,
		APIID:     17,
		APILogID:  77441,
		FromEmail: "news@example.com",
		Recipient: "user@gmail.com",
		MessageID: "<message@example.com>",
		RFC822:    []byte("From: News <news@example.com>\r\nTo: user@gmail.com\r\nSubject: Hi\r\n\r\nHello"),
		FromName:  "News",
		Subject:   "Hi",
		HTML:      "Hello",
	})
	if err != nil {
		t.Fatalf("BuildRFC822() error = %v", err)
	}
	content := string(body)
	for _, header := range []string{
		"X-BM-Tenant-ID: 42",
		"X-BM-Api-ID: 17",
		"X-BM-Api-Log-ID: 77441",
		"X-BM-Message-ID: <message@example.com>",
		"X-BM-Engine: kumomta",
	} {
		if !strings.Contains(content, header) {
			t.Fatalf("content missing %q:\n%s", header, content)
		}
	}
}

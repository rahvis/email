package outbound

import "testing"

func TestQueueNameForCampaign(t *testing.T) {
	got := QueueNameForCampaign(981, 42, "GMAIL.COM.")
	want := "campaign_981:tenant_42@gmail.com"
	if got != want {
		t.Fatalf("QueueNameForCampaign() = %q, want %q", got, want)
	}
}

func TestQueueNameForAPI(t *testing.T) {
	got := QueueNameForAPI(17, 42, "user@yahoo.com")
	want := "api_17:tenant_42@yahoo.com"
	if got != want {
		t.Fatalf("QueueNameForAPI() = %q, want %q", got, want)
	}
}

func TestCorrelationHeadersIncludeRequiredKumoHeaders(t *testing.T) {
	headers := CorrelationHeaders(OutboundMessage{
		TenantID:         42,
		CampaignID:       981,
		TaskID:           981,
		RecipientID:      12345,
		APILogID:         0,
		MessageID:        "<message@example.com>",
		SendingProfileID: 7,
	})

	required := map[string]string{
		"X-BM-Tenant-ID":          "42",
		"X-BM-Campaign-ID":        "981",
		"X-BM-Task-ID":            "981",
		"X-BM-Recipient-ID":       "12345",
		"X-BM-Api-Log-ID":         "0",
		"X-BM-Message-ID":         "<message@example.com>",
		"X-BM-Sending-Profile-ID": "7",
		"X-BM-Engine":             EngineKumoMTA,
	}
	for key, want := range required {
		if got := headers[key]; got != want {
			t.Fatalf("headers[%s] = %q, want %q", key, got, want)
		}
	}
}

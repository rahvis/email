package kumo

import "testing"

func TestEvaluateInjectionWindow(t *testing.T) {
	if reason := EvaluateInjectionWindow(10, 2, 20, 5); reason != "" {
		t.Fatalf("EvaluateInjectionWindow allowed = %q", reason)
	}
	if reason := EvaluateInjectionWindow(21, 2, 20, 5); reason == "" {
		t.Fatal("expected global concurrency rejection")
	}
	if reason := EvaluateInjectionWindow(10, 6, 20, 5); reason == "" {
		t.Fatal("expected tenant concurrency rejection")
	}
}

func TestShouldOpenCircuit(t *testing.T) {
	if ShouldOpenCircuit(4, 5) {
		t.Fatal("circuit opened before threshold")
	}
	if !ShouldOpenCircuit(5, 5) {
		t.Fatal("circuit did not open at threshold")
	}
	if ShouldOpenCircuit(10, 0) {
		t.Fatal("zero threshold should disable circuit opening")
	}
}

func TestQueueAgeAlert(t *testing.T) {
	now := int64(1000)
	queue := QueueRuntimeMetric{
		Queue:              "campaign_1:tenant_42@gmail.com",
		TenantID:           42,
		OldestAcceptedAt:   100,
		PendingFinalEvents: 5,
	}
	alert := QueueAgeAlert(queue, 600, now)
	if alert == nil {
		t.Fatal("expected queue age alert")
	}
	if alert.TenantID != 42 || alert.Queue != queue.Queue {
		t.Fatalf("alert correlation = tenant %d queue %q", alert.TenantID, alert.Queue)
	}
	if got := QueueAgeAlert(queue, 1200, now); got != nil {
		t.Fatalf("unexpected alert below threshold: %#v", got)
	}
}

func TestTenantRiskLevel(t *testing.T) {
	tests := []struct {
		name       string
		queued     int64
		bounced    int64
		complained int64
		want       string
	}{
		{name: "none", queued: 0, want: "none"},
		{name: "low", queued: 100000, bounced: 1000, complained: 10, want: "low"},
		{name: "medium-bounce", queued: 100000, bounced: 5000, complained: 10, want: "medium"},
		{name: "high-complaint", queued: 100000, bounced: 1000, complained: 100, want: "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TenantRiskLevel(tt.queued, tt.bounced, tt.complained); got != tt.want {
				t.Fatalf("TenantRiskLevel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuntimeAlertDeduplication(t *testing.T) {
	resetRuntimeMetricsForTesting()
	RecordRuntimeAlert(RuntimeAlert{Type: "queue_age_exceeded", TenantID: 42, Queue: "q1", Message: "first"})
	RecordRuntimeAlert(RuntimeAlert{Type: "queue_age_exceeded", TenantID: 42, Queue: "q1", Message: "second"})
	_, _, alerts := runtimeMetricsSnapshot()
	if len(alerts) != 1 {
		t.Fatalf("alerts len = %d, want 1", len(alerts))
	}
	if alerts[0].Message != "second" {
		t.Fatalf("alert message = %q, want second", alerts[0].Message)
	}
}

func TestFilterAlertsHidesOtherTenantAlerts(t *testing.T) {
	alerts := []RuntimeAlert{
		{Type: "queue_age_exceeded", TenantID: 42, Message: "tenant 42"},
		{Type: "queue_age_exceeded", TenantID: 88, Message: "tenant 88"},
		{Type: "kumo_unreachable", TenantID: 0, Message: "global"},
	}
	filtered := filterAlerts(alerts, 42, false)
	if len(filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(filtered))
	}
	for _, alert := range filtered {
		if alert.TenantID == 88 {
			t.Fatal("non-operator tenant view included another tenant alert")
		}
	}
	operator := filterAlerts(alerts, 0, true)
	if len(operator) != 3 {
		t.Fatalf("operator filtered len = %d, want 3", len(operator))
	}
}

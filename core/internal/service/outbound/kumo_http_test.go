package outbound

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"billionmail-core/internal/service/kumo"
)

func installNoopKumoRuntimeHooks(t *testing.T) {
	t.Helper()
	oldAcquire := acquireKumoInjectionSlot
	oldRelease := releaseKumoInjectionSlot
	oldAttempt := recordKumoInjectionAttempt
	oldResult := recordKumoInjectionResult
	oldBuild := buildKumoRFC822
	acquireKumoInjectionSlot = func(ctx context.Context, input kumo.InjectionControlInput) (*kumo.InjectionSlot, error) {
		return &kumo.InjectionSlot{}, nil
	}
	releaseKumoInjectionSlot = func(ctx context.Context, slot *kumo.InjectionSlot) {}
	recordKumoInjectionAttempt = func(ctx context.Context, input kumo.InjectionControlInput) {}
	recordKumoInjectionResult = func(ctx context.Context, input kumo.InjectionControlInput, success bool, latencyMS int64, err error) {
	}
	t.Cleanup(func() {
		acquireKumoInjectionSlot = oldAcquire
		releaseKumoInjectionSlot = oldRelease
		recordKumoInjectionAttempt = oldAttempt
		recordKumoInjectionResult = oldResult
		buildKumoRFC822 = oldBuild
	})
}

func TestKumoHTTPMailerInjectsOneRecipientWithQueueMetadataAndHeaders(t *testing.T) {
	installNoopKumoRuntimeHooks(t)
	var captured KumoInjectRequest
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/inject/v1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"success_count":1,"fail_count":0,"failed_recipients":[],"errors":[]}`))
	}))
	defer server.Close()

	mailer, err := NewKumoHTTPMailerFromConfig(&kumo.StoredConfig{
		BaseURL:    server.URL,
		InjectPath: "/api/inject/v1",
		TLSVerify:  true,
		AuthMode:   "bearer",
		TimeoutMS:  5000,
	}, "secret-token", server.Client())
	if err != nil {
		t.Fatalf("NewKumoHTTPMailerFromConfig() error = %v", err)
	}

	result, err := mailer.Send(context.Background(), OutboundMessage{
		TenantID:          42,
		CampaignID:        981,
		TaskID:            981,
		RecipientID:       12345,
		FromEmail:         "news@example.com",
		FromName:          "News",
		Recipient:         "user@gmail.com",
		Subject:           "Launch",
		HTML:              "<p>Hello</p>",
		MessageID:         "<message@example.com>",
		SendingProfileID:  7,
		DestinationDomain: "gmail.com",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if authHeader != "Bearer secret-token" {
		t.Fatalf("Authorization header = %q", authHeader)
	}
	if result.Engine != EngineKumoMTA {
		t.Fatalf("result.Engine = %q", result.Engine)
	}
	if result.InjectionStatus != InjectionStatusQueued {
		t.Fatalf("result.InjectionStatus = %q", result.InjectionStatus)
	}
	if result.QueueName != "campaign_981:tenant_42@gmail.com" {
		t.Fatalf("result.QueueName = %q", result.QueueName)
	}
	if captured.EnvelopeSender != "news@example.com" {
		t.Fatalf("EnvelopeSender = %q", captured.EnvelopeSender)
	}
	if captured.DeferredGeneration {
		t.Fatal("DeferredGeneration must remain false in v1")
	}
	if len(captured.Recipients) != 1 || captured.Recipients[0].Email != "user@gmail.com" {
		t.Fatalf("recipients = %#v", captured.Recipients)
	}
	metadata := captured.Recipients[0].Metadata
	if metadata["queue"] != "campaign_981:tenant_42@gmail.com" {
		t.Fatalf("metadata queue = %q", metadata["queue"])
	}
	if metadata["tenant"] != "tenant_42" || metadata["campaign"] != "campaign_981" {
		t.Fatalf("metadata tenant/campaign = %#v", metadata)
	}
	if !strings.Contains(captured.Content, "X-BM-Tenant-ID: 42") {
		t.Fatalf("content missing tenant header:\n%s", captured.Content)
	}
	if !strings.Contains(captured.Content, "X-BM-Api-Log-ID: 0") {
		t.Fatalf("content missing API log zero header:\n%s", captured.Content)
	}
	if !strings.Contains(captured.Content, "X-BM-Engine: kumomta") {
		t.Fatalf("content missing engine header:\n%s", captured.Content)
	}
	if !strings.Contains(captured.Content, "Message-ID: <message@example.com>") {
		t.Fatalf("content missing preserved Message-ID:\n%s", captured.Content)
	}
}

func TestClassifyHTTPStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusOK, ErrClassNone},
		{http.StatusCreated, ErrClassNone},
		{http.StatusBadRequest, ErrClassPermanent},
		{http.StatusUnprocessableEntity, ErrClassPermanent},
		{http.StatusUnauthorized, ErrClassAuth},
		{http.StatusForbidden, ErrClassAuth},
		{http.StatusTooManyRequests, ErrClassRetryable},
		{http.StatusInternalServerError, ErrClassRetryable},
		{http.StatusBadGateway, ErrClassRetryable},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			if got := ClassifyHTTPStatus(tt.status); got != tt.want {
				t.Fatalf("ClassifyHTTPStatus(%d) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestKumoHTTPMailerReturnsTypedErrorsForHTTPStatuses(t *testing.T) {
	installNoopKumoRuntimeHooks(t)
	tests := []struct {
		name   string
		status int
		class  string
	}{
		{"bad-request", http.StatusBadRequest, ErrClassPermanent},
		{"unprocessable", http.StatusUnprocessableEntity, ErrClassPermanent},
		{"unauthorized", http.StatusUnauthorized, ErrClassAuth},
		{"forbidden", http.StatusForbidden, ErrClassAuth},
		{"rate-limit", http.StatusTooManyRequests, ErrClassRetryable},
		{"server-error", http.StatusInternalServerError, ErrClassRetryable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"error":"nope"}`))
			}))
			defer server.Close()

			mailer, err := NewKumoHTTPMailerFromConfig(&kumo.StoredConfig{
				BaseURL:    server.URL,
				InjectPath: "/api/inject/v1",
				TLSVerify:  true,
				AuthMode:   "none",
				TimeoutMS:  5000,
			}, "", server.Client())
			if err != nil {
				t.Fatalf("NewKumoHTTPMailerFromConfig() error = %v", err)
			}

			_, err = mailer.Send(context.Background(), minimalKumoMessage())
			var sendErr *SendError
			if !errors.As(err, &sendErr) {
				t.Fatalf("error = %v, want *SendError", err)
			}
			if sendErr.Class != tt.class {
				t.Fatalf("class = %q, want %q", sendErr.Class, tt.class)
			}
		})
	}
}

func TestKumoHTTPMailerTreatsTimeoutAsRetryable(t *testing.T) {
	installNoopKumoRuntimeHooks(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte(`{"success_count":1,"fail_count":0,"failed_recipients":[],"errors":[]}`))
	}))
	defer server.Close()

	mailer, err := NewKumoHTTPMailerFromConfig(&kumo.StoredConfig{
		BaseURL:    server.URL,
		InjectPath: "/api/inject/v1",
		TLSVerify:  true,
		AuthMode:   "none",
		TimeoutMS:  25,
	}, "", nil)
	if err != nil {
		t.Fatalf("NewKumoHTTPMailerFromConfig() error = %v", err)
	}
	_, err = mailer.Send(context.Background(), minimalKumoMessage())
	var sendErr *SendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("error = %v, want *SendError", err)
	}
	if !sendErr.Retryable() {
		t.Fatalf("timeout class = %q, want retryable", sendErr.Class)
	}
}

func TestKumoHTTPMailerRejectsTwoXXRecipientFailure(t *testing.T) {
	installNoopKumoRuntimeHooks(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success_count":0,"fail_count":1,"failed_recipients":["user@gmail.com"],"errors":["recipient rejected"]}`))
	}))
	defer server.Close()

	mailer, err := NewKumoHTTPMailerFromConfig(&kumo.StoredConfig{
		BaseURL:    server.URL,
		InjectPath: "/api/inject/v1",
		TLSVerify:  true,
		AuthMode:   "none",
		TimeoutMS:  5000,
	}, "", server.Client())
	if err != nil {
		t.Fatalf("NewKumoHTTPMailerFromConfig() error = %v", err)
	}
	_, err = mailer.Send(context.Background(), minimalKumoMessage())
	var sendErr *SendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("error = %v, want *SendError", err)
	}
	if !sendErr.Permanent() {
		t.Fatalf("class = %q, want permanent", sendErr.Class)
	}
}

func TestKumoHTTPMailerReturnsRetryableRuntimeBackpressure(t *testing.T) {
	installNoopKumoRuntimeHooks(t)
	oldAcquire := acquireKumoInjectionSlot
	acquireKumoInjectionSlot = func(ctx context.Context, input kumo.InjectionControlInput) (*kumo.InjectionSlot, error) {
		return nil, errors.New("profile injection rate exceeded")
	}
	t.Cleanup(func() {
		acquireKumoInjectionSlot = oldAcquire
	})

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mailer, err := NewKumoHTTPMailerFromConfig(&kumo.StoredConfig{
		BaseURL:    server.URL,
		InjectPath: "/api/inject/v1",
		TLSVerify:  true,
		AuthMode:   "none",
		TimeoutMS:  5000,
	}, "", server.Client())
	if err != nil {
		t.Fatalf("NewKumoHTTPMailerFromConfig() error = %v", err)
	}

	_, err = mailer.Send(context.Background(), minimalKumoMessage())
	var sendErr *SendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("error = %v, want *SendError", err)
	}
	if !sendErr.Retryable() {
		t.Fatalf("backpressure class = %q, want retryable", sendErr.Class)
	}
	if requests != 0 {
		t.Fatalf("runtime backpressure should block before HTTP request; requests = %d", requests)
	}
}

func TestKumoHTTPMailerChecksRuntimeBeforeBuildingRFC822(t *testing.T) {
	installNoopKumoRuntimeHooks(t)
	oldAcquire := acquireKumoInjectionSlot
	oldBuild := buildKumoRFC822
	buildCalled := false
	acquireKumoInjectionSlot = func(ctx context.Context, input kumo.InjectionControlInput) (*kumo.InjectionSlot, error) {
		return nil, errors.New("tenant concurrency exceeded")
	}
	buildKumoRFC822 = func(req OutboundMessage) ([]byte, error) {
		buildCalled = true
		return BuildRFC822(req)
	}
	t.Cleanup(func() {
		acquireKumoInjectionSlot = oldAcquire
		buildKumoRFC822 = oldBuild
	})

	mailer, err := NewKumoHTTPMailerFromConfig(&kumo.StoredConfig{
		BaseURL:    "https://kumo.example.com",
		InjectPath: "/api/inject/v1",
		TLSVerify:  true,
		AuthMode:   "none",
		TimeoutMS:  5000,
	}, "", http.DefaultClient)
	if err != nil {
		t.Fatalf("NewKumoHTTPMailerFromConfig() error = %v", err)
	}

	_, err = mailer.Send(context.Background(), minimalKumoMessage())
	var sendErr *SendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("error = %v, want *SendError", err)
	}
	if buildCalled {
		t.Fatal("runtime backpressure must be checked before building RFC822 content")
	}
}

func minimalKumoMessage() OutboundMessage {
	return OutboundMessage{
		TenantID:     42,
		APIID:        17,
		APILogID:     77441,
		FromEmail:    "news@example.com",
		Recipient:    "user@gmail.com",
		Subject:      "Subject",
		HTML:         "<p>Hello</p>",
		MessageID:    "<message@example.com>",
		FromName:     "News",
		SenderDomain: "example.com",
	}
}

package outbound

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"billionmail-core/internal/service/kumo"

	"github.com/gogf/gf/v2/frame/g"
)

const maxRawResponseBytes = 4096

var (
	acquireKumoInjectionSlot   = kumo.AcquireInjectionSlot
	releaseKumoInjectionSlot   = kumo.ReleaseInjectionSlot
	recordKumoInjectionAttempt = kumo.RecordInjectionAttempt
	recordKumoInjectionResult  = kumo.RecordInjectionResult
	buildKumoRFC822            = BuildRFC822
)

type KumoHTTPMailer struct {
	endpoint   string
	authMode   string
	authSecret string
	client     *http.Client
}

type KumoInjectRequest struct {
	EnvelopeSender     string          `json:"envelope_sender"`
	Content            string          `json:"content"`
	Recipients         []KumoRecipient `json:"recipients"`
	DeferredGeneration bool            `json:"deferred_generation"`
}

type KumoRecipient struct {
	Email    string            `json:"email"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type KumoInjectResponse struct {
	SuccessCount     int      `json:"success_count"`
	FailCount        int      `json:"fail_count"`
	FailedRecipients []string `json:"failed_recipients"`
	Errors           []string `json:"errors"`
}

func NewKumoHTTPMailer(ctx context.Context) (*KumoHTTPMailer, error) {
	cfg, err := kumo.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil || !cfg.Enabled {
		return nil, fmt.Errorf("KumoMTA is disabled")
	}
	secret, err := kumo.GetAuthSecret(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load KumoMTA auth secret: %w", err)
	}
	return NewKumoHTTPMailerFromConfig(cfg, secret, nil)
}

func NewKumoHTTPMailerFromConfig(cfg *kumo.StoredConfig, authSecret string, client *http.Client) (*KumoHTTPMailer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("KumoMTA config is required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("KumoMTA base URL is required")
	}
	endpoint, err := joinEndpoint(cfg.BaseURL, cfg.InjectPath)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = newHTTPClient(cfg.TimeoutMS, cfg.TLSVerify)
	}
	return &KumoHTTPMailer{
		endpoint:   endpoint,
		authMode:   strings.ToLower(strings.TrimSpace(cfg.AuthMode)),
		authSecret: authSecret,
		client:     client,
	}, nil
}

func (m *KumoHTTPMailer) Send(ctx context.Context, req OutboundMessage) (*OutboundResult, error) {
	if m == nil || m.client == nil || strings.TrimSpace(m.endpoint) == "" {
		return nil, fmt.Errorf("KumoMTA mailer is not configured")
	}
	if strings.TrimSpace(req.FromEmail) == "" {
		return nil, fmt.Errorf("from email is required")
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return nil, fmt.Errorf("recipient is required")
	}

	if req.DestinationDomain == "" {
		req.DestinationDomain = DestinationDomain(req.Recipient)
	}
	if req.SenderDomain == "" {
		req.SenderDomain = SenderDomain(req.FromEmail)
	}
	queueName := QueueNameForMessage(req)
	control := kumo.InjectionControlInput{
		TenantID:         req.TenantID,
		SendingProfileID: req.SendingProfileID,
		Engine:           EngineKumoMTA,
		Queue:            queueName,
		RecipientDomain:  req.DestinationDomain,
	}

	slot, err := acquireKumoInjectionSlot(ctx, control)
	if err != nil {
		g.Log().Warningf(ctx, "KumoMTA injection paused by runtime control: tenant_id=%d profile_id=%d queue=%s reason=%s",
			req.TenantID, req.SendingProfileID, queueName, sanitizeLogString(err.Error()))
		return nil, &SendError{Class: ErrClassRetryable, Message: "KumoMTA injection backpressure active", Err: err}
	}
	defer releaseKumoInjectionSlot(ctx, slot)

	rfc822, err := buildKumoRFC822(req)
	if err != nil {
		return nil, err
	}

	payload := KumoInjectRequest{
		EnvelopeSender:     req.FromEmail,
		Content:            string(rfc822),
		DeferredGeneration: false,
		Recipients: []KumoRecipient{
			{
				Email:    req.Recipient,
				Metadata: RecipientMetadata(req, queueName),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	m.addAuthHeaders(httpReq, body)

	recordKumoInjectionAttempt(ctx, control)
	start := time.Now()
	resp, err := m.client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		class := ErrClassPermanent
		if isRetryableNetworkError(err) {
			class = ErrClassRetryable
		}
		recordKumoInjectionResult(ctx, control, false, latency.Milliseconds(), err)
		g.Log().Warningf(ctx, "KumoMTA injection failed: queue=%s recipient_domain=%s class=%s latency_ms=%d error=%s",
			queueName, req.DestinationDomain, class, latency.Milliseconds(), sanitizeLogString(err.Error()))
		return nil, &SendError{Class: class, Message: "KumoMTA injection request failed", Err: err}
	}
	defer resp.Body.Close()

	rawBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRawResponseBytes))
	if readErr != nil {
		recordKumoInjectionResult(ctx, control, false, latency.Milliseconds(), readErr)
		return nil, &SendError{Class: ErrClassRetryable, StatusCode: resp.StatusCode, Message: "failed to read KumoMTA response", Err: readErr}
	}
	rawResponse := sanitizeRawResponse(rawBody)
	class := ClassifyHTTPStatus(resp.StatusCode)
	if class != ErrClassNone {
		recordKumoInjectionResult(ctx, control, false, latency.Milliseconds(), fmt.Errorf("HTTP %d", resp.StatusCode))
		g.Log().Warningf(ctx, "KumoMTA injection rejected: queue=%s recipient_domain=%s status=%d class=%s latency_ms=%d response=%s",
			queueName, req.DestinationDomain, resp.StatusCode, class, latency.Milliseconds(), rawResponse)
		return nil, &SendError{Class: class, StatusCode: resp.StatusCode, Message: rawResponse}
	}

	injectResp := KumoInjectResponse{}
	if len(bytes.TrimSpace(rawBody)) > 0 {
		_ = json.Unmarshal(rawBody, &injectResp)
	}
	if injectResp.FailCount > 0 || containsRecipient(injectResp.FailedRecipients, req.Recipient) || (injectResp.SuccessCount == 0 && len(rawBody) > 0) {
		msg := rawResponse
		if len(injectResp.Errors) > 0 {
			msg = strings.Join(injectResp.Errors, "; ")
		}
		recordKumoInjectionResult(ctx, control, false, latency.Milliseconds(), fmt.Errorf("recipient rejected"))
		return nil, &SendError{Class: ErrClassPermanent, StatusCode: resp.StatusCode, Message: sanitizeLogString(msg)}
	}

	recordKumoInjectionResult(ctx, control, true, latency.Milliseconds(), nil)
	g.Log().Debugf(ctx, "KumoMTA injection accepted: queue=%s recipient_domain=%s status=%d latency_ms=%d",
		queueName, req.DestinationDomain, resp.StatusCode, latency.Milliseconds())
	return &OutboundResult{
		Engine:          EngineKumoMTA,
		MessageID:       req.MessageID,
		InjectionStatus: InjectionStatusQueued,
		QueueName:       queueName,
		AcceptedAt:      time.Now().Unix(),
		RawResponse:     rawResponse,
	}, nil
}

func (m *KumoHTTPMailer) addAuthHeaders(req *http.Request, body []byte) {
	switch strings.ToLower(strings.TrimSpace(m.authMode)) {
	case "bearer":
		if m.authSecret != "" {
			req.Header.Set("Authorization", "Bearer "+m.authSecret)
		}
	case "hmac":
		if m.authSecret != "" {
			timestamp := fmt.Sprintf("%d", time.Now().Unix())
			req.Header.Set("X-BM-Kumo-Timestamp", timestamp)
			req.Header.Set("X-BM-Kumo-Signature", computeHMACSignature(m.authSecret, timestamp, body))
		}
	}
}

func newHTTPClient(timeoutMS int, tlsVerify bool) *http.Client {
	if timeoutMS == 0 {
		timeoutMS = 5000
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	transport.DialContext = (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !tlsVerify} // #nosec G402 - operator-controlled for private/self-signed Kumo endpoints.
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 32
	transport.IdleConnTimeout = 90 * time.Second

	return &http.Client{
		Timeout:   time.Duration(timeoutMS) * time.Millisecond,
		Transport: transport,
	}
}

func joinEndpoint(baseURL, injectPath string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	injectPath = strings.TrimSpace(injectPath)
	if injectPath == "" {
		injectPath = "/api/inject/v1"
	}
	if !strings.HasPrefix(injectPath, "/") {
		injectPath = "/" + injectPath
	}
	parsed, err := url.Parse(baseURL + injectPath)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("KumoMTA endpoint scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("KumoMTA endpoint host is required")
	}
	return parsed.String(), nil
}

func computeHMACSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	if strings.Contains(strings.ToLower(err.Error()), "timeout") {
		return true
	}
	return true
}

func containsRecipient(recipients []string, recipient string) bool {
	for _, item := range recipients {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(recipient)) {
			return true
		}
	}
	return false
}

func sanitizeRawResponse(body []byte) string {
	raw := sanitizeLogString(string(body))
	if raw == "" {
		return "empty KumoMTA response"
	}
	return raw
}

func sanitizeLogString(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.TrimSpace(value)
	if len(value) > maxRawResponseBytes {
		return value[:maxRawResponseBytes] + "...[truncated]"
	}
	return value
}

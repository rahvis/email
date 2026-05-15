package kumo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

var (
	statusMu       sync.RWMutex
	currentStatus  = Status{}
	metricsMu      sync.RWMutex
	currentMetrics = MetricsSnapshot{}
)

func TestConnection(ctx context.Context, input TestConnectionInput) (*TestConnectionResult, error) {
	effective, err := effectiveTestInput(ctx, input)
	if err != nil {
		return nil, err
	}

	client := httpClient(effective.TimeoutMS, effective.TLSVerify)
	injectURL, err := joinURL(effective.BaseURL, effective.InjectPath)
	if err != nil {
		return nil, err
	}

	result := &TestConnectionResult{}
	result.Inject = checkEndpoint(ctx, client, http.MethodGet, injectURL, effective.AuthMode, effective.AuthSecret)
	result.HealthMS = result.Inject.LatencyMS

	if effective.MetricsURL != "" {
		result.Metrics = checkEndpoint(ctx, client, http.MethodGet, effective.MetricsURL, effective.AuthMode, effective.AuthSecret)
		result.MetricsMS = result.Metrics.LatencyMS
	}

	result.OK = result.Inject.OK
	if effective.MetricsURL != "" {
		result.OK = result.OK && result.Metrics.OK
	}
	if result.OK {
		result.Message = "KumoMTA reachable"
	} else {
		result.Message = firstFailureMessage(result.Inject, result.Metrics)
	}

	return result, nil
}

func PollHealthAndMetrics(ctx context.Context) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		updateStatusFailure(err.Error(), 0, 0)
		return
	}
	if !cfg.Enabled {
		return
	}

	authSecret, err := GetAuthSecret(ctx, cfg)
	if err != nil {
		updateStatusFailure("failed to decrypt KumoMTA auth secret", 0, 0)
		return
	}

	result, err := TestConnection(ctx, TestConnectionInput{
		BaseURL:    cfg.BaseURL,
		InjectPath: cfg.InjectPath,
		MetricsURL: cfg.MetricsURL,
		AuthMode:   cfg.AuthMode,
		AuthSecret: authSecret,
		TLSVerify:  cfg.TLSVerify,
		TimeoutMS:  cfg.TimeoutMS,
	})
	if err != nil {
		updateStatusFailure(err.Error(), 0, 0)
		return
	}

	if result.Inject.OK {
		updateStatusSuccess(result.Inject.LatencyMS, result.Metrics.LatencyMS)
	} else {
		updateStatusFailure(result.Message, result.Inject.LatencyMS, result.Metrics.LatencyMS)
	}

	if cfg.MetricsURL != "" {
		if result.Metrics.OK {
			if err := refreshMetricsSnapshot(ctx, cfg, authSecret); err != nil {
				updateMetricsFailure(err.Error())
			}
		} else if result.Metrics.Message != "" {
			updateMetricsFailure(result.Metrics.Message)
		}
	}
}

func GetStatus() Status {
	statusMu.RLock()
	defer statusMu.RUnlock()
	status := currentStatus
	if status.WebhookLastSeenAt > 0 {
		status.WebhookLagSeconds = time.Now().Unix() - status.WebhookLastSeenAt
	}
	return status
}

func GetMetricsSnapshot() MetricsSnapshot {
	metricsMu.RLock()
	defer metricsMu.RUnlock()
	snapshot := currentMetrics
	if snapshot.Queues == nil {
		snapshot.Queues = []QueueMetric{}
	}
	if snapshot.Nodes == nil {
		snapshot.Nodes = []NodeMetric{}
	}
	return snapshot
}

func effectiveTestInput(ctx context.Context, input TestConnectionInput) (TestConnectionInput, error) {
	cfg, _ := LoadConfig(ctx)
	if cfg == nil {
		cfg = defaultConfig()
	}

	if strings.TrimSpace(input.BaseURL) == "" {
		input.BaseURL = cfg.BaseURL
	}
	if strings.TrimSpace(input.InjectPath) == "" {
		input.InjectPath = cfg.InjectPath
	}
	if strings.TrimSpace(input.MetricsURL) == "" {
		input.MetricsURL = cfg.MetricsURL
	}
	if strings.TrimSpace(input.AuthMode) == "" {
		input.AuthMode = cfg.AuthMode
	}
	if input.TimeoutMS == 0 {
		input.TimeoutMS = cfg.TimeoutMS
	}
	if strings.TrimSpace(input.AuthSecret) == "" && cfg.AuthSecretEncrypted != "" {
		secret, err := GetAuthSecret(ctx, cfg)
		if err != nil {
			return input, err
		}
		input.AuthSecret = secret
	}

	if strings.TrimSpace(input.BaseURL) == "" {
		return input, fmt.Errorf("base_url is required")
	}

	if err := validateConfigInput(UpdateConfigInput{
		BaseURL:    input.BaseURL,
		InjectPath: input.InjectPath,
		MetricsURL: input.MetricsURL,
		AuthMode:   input.AuthMode,
		TimeoutMS:  input.TimeoutMS,
	}); err != nil {
		return input, err
	}
	return input, nil
}

func httpClient(timeoutMS int, tlsVerify bool) *http.Client {
	if timeoutMS == 0 {
		timeoutMS = defaultTimeoutMS
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !tlsVerify}, // #nosec G402 - operator-controlled for private/self-signed Kumo endpoints.
		Proxy:           http.ProxyFromEnvironment,
	}
	return &http.Client{
		Timeout:   time.Duration(timeoutMS) * time.Millisecond,
		Transport: transport,
	}
}

func checkEndpoint(ctx context.Context, client *http.Client, method, endpoint, authMode, authSecret string) EndpointCheck {
	start := time.Now()
	check := EndpointCheck{}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		check.Message = err.Error()
		return check
	}
	addAuthHeaders(req, authMode, authSecret, nil)

	resp, err := client.Do(req)
	check.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		check.Message = sanitizeError(err)
		return check
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	check.StatusCode = resp.StatusCode
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		check.OK = true
		check.Message = "OK"
	case resp.StatusCode == http.StatusMethodNotAllowed:
		check.OK = true
		check.Message = "reachable"
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		check.Message = "authentication or source allowlist failed"
	default:
		check.Message = fmt.Sprintf("unexpected HTTP status %d", resp.StatusCode)
	}
	return check
}

func refreshMetricsSnapshot(ctx context.Context, cfg *StoredConfig, authSecret string) error {
	client := httpClient(cfg.TimeoutMS, cfg.TLSVerify)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.MetricsURL, nil)
	if err != nil {
		return err
	}
	addAuthHeaders(req, cfg.AuthMode, authSecret, nil)

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("metrics endpoint returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	now := time.Now().Unix()

	metricsMu.Lock()
	currentMetrics = MetricsSnapshot{
		SnapshotAt:       now,
		Queues:           []QueueMetric{},
		Nodes:            []NodeMetric{},
		RawBytes:         len(body),
		MetricLines:      countMetricLines(string(body)),
		LastSuccessfulAt: now,
	}
	metricsMu.Unlock()

	statusMu.Lock()
	currentStatus.MetricsLatencyMS = latency
	statusMu.Unlock()

	return nil
}

func updateStatusSuccess(injectLatency, metricsLatency int64) {
	statusMu.Lock()
	defer statusMu.Unlock()
	now := time.Now().Unix()
	currentStatus.Connected = true
	currentStatus.LastOKAt = now
	currentStatus.LastError = ""
	currentStatus.InjectLatencyMS = injectLatency
	currentStatus.MetricsLatencyMS = metricsLatency
}

func updateStatusFailure(message string, injectLatency, metricsLatency int64) {
	statusMu.Lock()
	defer statusMu.Unlock()
	currentStatus.Connected = false
	now := time.Now().Unix()
	currentStatus.LastErrorAt = now
	currentStatus.LastError = message
	currentStatus.InjectLatencyMS = injectLatency
	currentStatus.MetricsLatencyMS = metricsLatency
	RecordRuntimeAlert(RuntimeAlert{
		Type:      "kumo_unreachable",
		Severity:  alertSeverityCritical,
		Message:   "KumoMTA health check failed: " + sanitizeLogText(message),
		CreatedAt: now,
	})
	g.Log().Warningf(context.Background(), "KumoMTA health check failed: %s", sanitizeLogText(message))
}

func updateMetricsFailure(message string) {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	currentMetrics.LastError = message
	currentMetrics.LastErrorAt = time.Now().Unix()
	if currentMetrics.Queues == nil {
		currentMetrics.Queues = []QueueMetric{}
	}
	if currentMetrics.Nodes == nil {
		currentMetrics.Nodes = []NodeMetric{}
	}
	RecordRuntimeAlert(RuntimeAlert{
		Type:     "kumo_metrics_failed",
		Severity: alertSeverityWarning,
		Message:  "KumoMTA metrics refresh failed: " + sanitizeLogText(message),
	})
}

func markWebhookSeen() {
	statusMu.Lock()
	defer statusMu.Unlock()
	currentStatus.WebhookLastSeenAt = time.Now().Unix()
	currentStatus.WebhookLagSeconds = 0
}

func addAuthHeaders(req *http.Request, authMode, authSecret string, body []byte) {
	switch normalizeAuthMode(authMode) {
	case "bearer":
		if authSecret != "" {
			req.Header.Set("Authorization", "Bearer "+authSecret)
		}
	case "hmac":
		if authSecret != "" {
			timestamp := fmt.Sprintf("%d", time.Now().Unix())
			req.Header.Set("X-BM-Kumo-Timestamp", timestamp)
			req.Header.Set("X-BM-Kumo-Signature", computeAuthHMACSignature(authSecret, timestamp, body))
		}
	}
}

func computeAuthHMACSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func joinURL(baseURL, path string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultInjectPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	u, err := url.Parse(baseURL + path)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func countMetricLines(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		count++
	}
	return count
}

func firstFailureMessage(checks ...EndpointCheck) string {
	for _, check := range checks {
		if !check.OK && check.Message != "" {
			return check.Message
		}
	}
	return "KumoMTA connection test failed"
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

func resetRuntimeStateForTesting() {
	statusMu.Lock()
	currentStatus = Status{}
	statusMu.Unlock()
	metricsMu.Lock()
	currentMetrics = MetricsSnapshot{}
	metricsMu.Unlock()
}

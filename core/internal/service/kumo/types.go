package kumo

import "time"

const (
	configOptionKey           = "kumo_config"
	defaultInjectPath         = "/api/inject/v1"
	defaultTimeoutMS          = 5000
	minTimeoutMS              = 100
	maxTimeoutMS              = 60000
	configCacheTTL            = 60 * time.Second
	webhookTimestampTolerance = 5 * time.Minute
	webhookBodyLimitBytes     = 1 << 20
	webhookIdempotencyTTL     = 24 * time.Hour
)

type StoredConfig struct {
	Enabled                bool   `json:"enabled"`
	CampaignsEnabled       bool   `json:"campaigns_enabled"`
	APIEnabled             bool   `json:"api_enabled"`
	BaseURL                string `json:"base_url"`
	InjectPath             string `json:"inject_path"`
	MetricsURL             string `json:"metrics_url"`
	TLSVerify              bool   `json:"tls_verify"`
	AuthMode               string `json:"auth_mode"`
	AuthSecretEncrypted    string `json:"auth_secret_encrypted"`
	WebhookSecretEncrypted string `json:"webhook_secret_encrypted"`
	TimeoutMS              int    `json:"timeout_ms"`
	DefaultPool            string `json:"default_pool"`
	UpdatedAt              int64  `json:"updated_at"`
}

type UpdateConfigInput struct {
	Enabled          bool   `json:"enabled"`
	CampaignsEnabled bool   `json:"campaigns_enabled"`
	APIEnabled       bool   `json:"api_enabled"`
	BaseURL          string `json:"base_url"`
	InjectPath       string `json:"inject_path"`
	MetricsURL       string `json:"metrics_url"`
	TLSVerify        bool   `json:"tls_verify"`
	AuthMode         string `json:"auth_mode"`
	AuthSecret       string `json:"auth_secret"`
	WebhookSecret    string `json:"webhook_secret"`
	TimeoutMS        int    `json:"timeout_ms"`
	DefaultPool      string `json:"default_pool"`
}

type PublicConfig struct {
	Enabled          bool   `json:"enabled"`
	CampaignsEnabled bool   `json:"campaigns_enabled"`
	APIEnabled       bool   `json:"api_enabled"`
	BaseURL          string `json:"base_url"`
	InjectPath       string `json:"inject_path"`
	MetricsURL       string `json:"metrics_url"`
	TLSVerify        bool   `json:"tls_verify"`
	AuthMode         string `json:"auth_mode"`
	HasAuthSecret    bool   `json:"has_auth_secret"`
	HasWebhookSecret bool   `json:"has_webhook_secret"`
	TimeoutMS        int    `json:"timeout_ms"`
	DefaultPool      string `json:"default_pool"`
	UpdatedAt        int64  `json:"updated_at"`
}

type EndpointCheck struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	LatencyMS  int64  `json:"latency_ms"`
	Message    string `json:"message"`
}

type TestConnectionInput struct {
	BaseURL    string `json:"base_url"`
	InjectPath string `json:"inject_path"`
	MetricsURL string `json:"metrics_url"`
	AuthMode   string `json:"auth_mode"`
	AuthSecret string `json:"auth_secret"`
	TLSVerify  bool   `json:"tls_verify"`
	TimeoutMS  int    `json:"timeout_ms"`
}

type TestConnectionResult struct {
	OK        bool          `json:"ok"`
	HealthMS  int64         `json:"health_ms"`
	MetricsMS int64         `json:"metrics_ms"`
	Message   string        `json:"message"`
	Inject    EndpointCheck `json:"inject"`
	Metrics   EndpointCheck `json:"metrics"`
}

type Status struct {
	Connected         bool   `json:"connected"`
	LastOKAt          int64  `json:"last_ok_at"`
	LastErrorAt       int64  `json:"last_error_at"`
	LastError         string `json:"last_error"`
	InjectLatencyMS   int64  `json:"inject_latency_ms"`
	MetricsLatencyMS  int64  `json:"metrics_latency_ms"`
	WebhookLastSeenAt int64  `json:"webhook_last_seen_at"`
	WebhookLagSeconds int64  `json:"webhook_lag_seconds"`
}

type QueueMetric struct {
	Queue      string `json:"queue"`
	TenantID   int64  `json:"tenant_id"`
	CampaignID int64  `json:"campaign_id"`
	Domain     string `json:"domain"`
	Ready      int64  `json:"ready"`
	Scheduled  int64  `json:"scheduled"`
	Deferred   int64  `json:"deferred"`
}

type NodeMetric struct {
	Name        string `json:"name"`
	Healthy     bool   `json:"healthy"`
	InjectRPS   int64  `json:"inject_rps"`
	DeliveryRPS int64  `json:"delivery_rps"`
}

type MetricsSnapshot struct {
	SnapshotAt       int64         `json:"snapshot_at"`
	Queues           []QueueMetric `json:"queues"`
	Nodes            []NodeMetric  `json:"nodes"`
	RawBytes         int           `json:"raw_bytes"`
	MetricLines      int           `json:"metric_lines"`
	LastError        string        `json:"last_error,omitempty"`
	LastErrorAt      int64         `json:"last_error_at,omitempty"`
	LastSuccessfulAt int64         `json:"last_successful_at,omitempty"`
}

type WebhookIngestResult struct {
	Accepted   int `json:"accepted"`
	Duplicates int `json:"duplicates"`
	Failed     int `json:"failed"`
}

type NormalizedDeliveryEvent struct {
	ProviderEventID string                 `json:"provider_event_id"`
	EventHash       string                 `json:"event_hash"`
	EventType       string                 `json:"event_type"`
	DeliveryStatus  string                 `json:"delivery_status"`
	TenantID        int64                  `json:"tenant_id"`
	CampaignID      int64                  `json:"campaign_id"`
	TaskID          int64                  `json:"task_id"`
	RecipientInfoID int64                  `json:"recipient_info_id"`
	APIID           int64                  `json:"api_id"`
	APILogID        int64                  `json:"api_log_id"`
	MessageID       string                 `json:"message_id"`
	Recipient       string                 `json:"recipient"`
	Sender          string                 `json:"sender"`
	QueueName       string                 `json:"queue_name"`
	Response        string                 `json:"response"`
	RemoteMX        string                 `json:"remote_mx"`
	OccurredAt      int64                  `json:"occurred_at"`
	RawEvent        map[string]interface{} `json:"raw_event"`
	Orphaned        bool                   `json:"orphaned"`
	KumoInjectionID int64                  `json:"-"`
}

type MessageInjectionRecord struct {
	TenantID         int64
	MessageID        string
	Recipient        string
	RecipientDomain  string
	CampaignID       int64
	TaskID           int64
	RecipientInfoID  int64
	APIID            int64
	APILogID         int64
	SendingProfileID int64
	QueueName        string
	InjectionStatus  string
	DeliveryStatus   string
	AttemptCount     int
	NextRetryAt      int64
	AcceptedAt       int64
	LastError        string
}

type PolicyFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
}

type ConfigPreview struct {
	Version          string       `json:"version"`
	GeneratedAt      int64        `json:"generated_at"`
	GeneratedBy      int64        `json:"generated_by"`
	Files            []PolicyFile `json:"files"`
	Warnings         []string     `json:"warnings"`
	ValidationErrors []string     `json:"validation_errors"`
}

type DeployConfigInput struct {
	Version string `json:"version"`
	DryRun  bool   `json:"dry_run"`
}

type DeployConfigResult struct {
	Version          string   `json:"version"`
	Status           string   `json:"status"`
	DryRun           bool     `json:"dry_run"`
	DeployedAt       int64    `json:"deployed_at"`
	RollbackVersion  string   `json:"rollback_version"`
	Message          string   `json:"message"`
	Warnings         []string `json:"warnings"`
	ValidationErrors []string `json:"validation_errors"`
}

type RuntimeSnapshot struct {
	SnapshotAt        int64                   `json:"snapshot_at"`
	Limits            RuntimeLimits           `json:"limits"`
	Status            Status                  `json:"status"`
	Injection         InjectionRuntimeMetrics `json:"injection"`
	Webhook           WebhookRuntimeMetrics   `json:"webhook"`
	Alerts            []RuntimeAlert          `json:"alerts"`
	Queues            []QueueRuntimeMetric    `json:"queues"`
	Nodes             []NodeRuntimeMetric     `json:"nodes"`
	Pools             []PoolRuntimeMetric     `json:"pools"`
	TenantRisk        []TenantRiskMetric      `json:"tenant_risk"`
	ReleaseReadiness  ReleaseReadiness        `json:"release_readiness"`
	OperatorView      bool                    `json:"operator_view"`
	FilteredTenantID  int64                   `json:"filtered_tenant_id"`
	CollectionWarning string                  `json:"collection_warning,omitempty"`
}

type RuntimeLimits struct {
	GlobalConcurrency       int64 `json:"global_concurrency"`
	TenantConcurrency       int64 `json:"tenant_concurrency"`
	ProfilePerSecond        int64 `json:"profile_per_second"`
	CircuitFailureThreshold int64 `json:"circuit_failure_threshold"`
	CircuitWindowSeconds    int64 `json:"circuit_window_seconds"`
	CircuitOpenSeconds      int64 `json:"circuit_open_seconds"`
	QueueAgeAlertSeconds    int64 `json:"queue_age_alert_seconds"`
	WebhookIdleSeconds      int64 `json:"webhook_idle_seconds"`
	SignatureFailureWindow  int64 `json:"signature_failure_window_seconds"`
	SignatureFailureSpike   int64 `json:"signature_failure_spike"`
}

type InjectionRuntimeMetrics struct {
	Attempts              int64                   `json:"attempts"`
	Successes             int64                   `json:"successes"`
	Failures              int64                   `json:"failures"`
	RateLimited           int64                   `json:"rate_limited"`
	BackpressureRejected  int64                   `json:"backpressure_rejected"`
	CircuitRejected       int64                   `json:"circuit_rejected"`
	AvgLatencyMS          int64                   `json:"avg_latency_ms"`
	InFlightGlobal        int64                   `json:"in_flight_global"`
	OpenCircuitCount      int64                   `json:"open_circuit_count"`
	ByTenantProfileEngine []InjectionMetricBucket `json:"by_tenant_profile_engine"`
}

type InjectionMetricBucket struct {
	TenantID         int64  `json:"tenant_id"`
	SendingProfileID int64  `json:"sending_profile_id"`
	Engine           string `json:"engine"`
	Attempts         int64  `json:"attempts"`
	Successes        int64  `json:"successes"`
	Failures         int64  `json:"failures"`
	AvgLatencyMS     int64  `json:"avg_latency_ms"`
}

type WebhookRuntimeMetrics struct {
	Accepted           int64           `json:"accepted"`
	Duplicates         int64           `json:"duplicates"`
	Failed             int64           `json:"failed"`
	SecurityFailures   int64           `json:"security_failures"`
	ProcessingFailures int64           `json:"processing_failures"`
	Orphaned           int64           `json:"orphaned"`
	RedisDedupeHits    int64           `json:"redis_dedupe_hits"`
	AvgIngestionLagMS  int64           `json:"avg_ingestion_lag_ms"`
	ByType             []WebhookBucket `json:"by_type"`
}

type WebhookBucket struct {
	EventType string `json:"event_type"`
	Count     int64  `json:"count"`
}

type RuntimeAlert struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	TenantID  int64  `json:"tenant_id,omitempty"`
	ProfileID int64  `json:"profile_id,omitempty"`
	Queue     string `json:"queue,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

type QueueRuntimeMetric struct {
	Queue              string `json:"queue"`
	TenantID           int64  `json:"tenant_id"`
	CampaignID         int64  `json:"campaign_id"`
	APIID              int64  `json:"api_id"`
	Domain             string `json:"domain"`
	Queued             int64  `json:"queued"`
	Deferred           int64  `json:"deferred"`
	Bounced            int64  `json:"bounced"`
	Complained         int64  `json:"complained"`
	OldestAgeSeconds   int64  `json:"oldest_age_seconds"`
	OldestAcceptedAt   int64  `json:"oldest_accepted_at"`
	Finalized          int64  `json:"finalized"`
	PendingFinalEvents int64  `json:"pending_final_events"`
}

type NodeRuntimeMetric struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Healthy     bool   `json:"healthy"`
	LastOKAt    int64  `json:"last_ok_at"`
	LastErrorAt int64  `json:"last_error_at"`
	LastError   string `json:"last_error"`
}

type PoolRuntimeMetric struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	TenantCount int64  `json:"tenant_count"`
	SourceCount int64  `json:"source_count"`
}

type TenantRiskMetric struct {
	TenantID       int64  `json:"tenant_id"`
	TenantName     string `json:"tenant_name,omitempty"`
	Queued         int64  `json:"queued"`
	Delivered      int64  `json:"delivered"`
	Bounced        int64  `json:"bounced"`
	Complained     int64  `json:"complained"`
	BouncePermille int64  `json:"bounce_per_mille"`
	ComplaintPPM   int64  `json:"complaint_ppm"`
	Risk           string `json:"risk"`
}

type ReleaseReadiness struct {
	Ready       bool     `json:"ready"`
	Checks      []string `json:"checks"`
	Blockers    []string `json:"blockers"`
	Rollback    []string `json:"rollback"`
	GeneratedAt int64    `json:"generated_at"`
}

func defaultConfig() *StoredConfig {
	return &StoredConfig{
		InjectPath: defaultInjectPath,
		TLSVerify:  true,
		AuthMode:   "bearer",
		TimeoutMS:  defaultTimeoutMS,
	}
}

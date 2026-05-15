package v1

import (
	"billionmail-core/internal/service/kumo"
	"billionmail-core/utility/types/api_v1"

	"github.com/gogf/gf/v2/frame/g"
)

type KumoConfigPayload struct {
	Enabled          bool   `json:"enabled" dc:"Enable KumoMTA"`
	CampaignsEnabled bool   `json:"campaigns_enabled" dc:"Enable campaign sending through KumoMTA"`
	APIEnabled       bool   `json:"api_enabled" dc:"Enable Send API through KumoMTA"`
	BaseURL          string `json:"base_url" dc:"KumoMTA base URL"`
	InjectPath       string `json:"inject_path" dc:"KumoMTA injection path"`
	MetricsURL       string `json:"metrics_url" dc:"KumoMTA metrics URL"`
	TLSVerify        bool   `json:"tls_verify" dc:"Verify TLS certificate"`
	AuthMode         string `json:"auth_mode" dc:"Auth mode: none, bearer, hmac"`
	AuthSecret       string `json:"auth_secret" dc:"Auth secret; empty keeps existing"`
	WebhookSecret    string `json:"webhook_secret" dc:"Webhook secret; empty keeps existing"`
	TimeoutMS        int    `json:"timeout_ms" dc:"Request timeout in milliseconds"`
	DefaultPool      string `json:"default_pool" dc:"Default KumoMTA egress pool"`
}

type GetConfigReq struct {
	g.Meta        `path:"/kumo/config" tags:"KumoMTA" method:"get" summary:"Get KumoMTA configuration"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type GetConfigRes struct {
	api_v1.StandardRes
	Data *kumo.PublicConfig `json:"data" dc:"KumoMTA public configuration"`
}

type SaveConfigReq struct {
	g.Meta        `path:"/kumo/config" tags:"KumoMTA" method:"post" summary:"Save KumoMTA configuration"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	KumoConfigPayload
}

type SaveConfigRes struct {
	api_v1.StandardRes
	Data *kumo.PublicConfig `json:"data" dc:"KumoMTA public configuration"`
}

type TestConnectionReq struct {
	g.Meta        `path:"/kumo/test_connection" tags:"KumoMTA" method:"post" summary:"Test KumoMTA connection"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	BaseURL       string `json:"base_url" dc:"KumoMTA base URL"`
	MetricsURL    string `json:"metrics_url" dc:"KumoMTA metrics URL"`
	InjectPath    string `json:"inject_path" dc:"KumoMTA injection path"`
	AuthMode      string `json:"auth_mode" dc:"Auth mode"`
	AuthSecret    string `json:"auth_secret" dc:"Optional test secret"`
	TLSVerify     bool   `json:"tls_verify" dc:"Verify TLS certificate"`
	TimeoutMS     int    `json:"timeout_ms" dc:"Request timeout in milliseconds"`
}

type TestConnectionRes struct {
	api_v1.StandardRes
	Data *kumo.TestConnectionResult `json:"data" dc:"KumoMTA test result"`
}

type GetStatusReq struct {
	g.Meta        `path:"/kumo/status" tags:"KumoMTA" method:"get" summary:"Get cached KumoMTA status"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type GetStatusRes struct {
	api_v1.StandardRes
	Data kumo.Status `json:"data" dc:"KumoMTA status"`
}

type GetMetricsReq struct {
	g.Meta        `path:"/kumo/metrics" tags:"KumoMTA" method:"get" summary:"Get cached KumoMTA metrics"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type GetMetricsRes struct {
	api_v1.StandardRes
	Data kumo.MetricsSnapshot `json:"data" dc:"KumoMTA metrics snapshot"`
}

type GetRuntimeReq struct {
	g.Meta        `path:"/kumo/runtime" tags:"KumoMTA" method:"get" summary:"Get KumoMTA runtime controls, alerts, and dashboard metrics"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type GetRuntimeRes struct {
	api_v1.StandardRes
	Data *kumo.RuntimeSnapshot `json:"data" dc:"KumoMTA runtime snapshot"`
}

type EventsReq struct {
	g.Meta `path:"/kumo/events" tags:"KumoMTA" method:"post" summary:"Receive KumoMTA webhook events"`
}

type EventsRes struct {
	api_v1.StandardRes
	Data *kumo.WebhookIngestResult `json:"data" dc:"Webhook ingest result"`
}

type PreviewConfigReq struct {
	g.Meta        `path:"/kumo/config/preview" tags:"KumoMTA" method:"post" summary:"Preview generated KumoMTA policy configuration"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type PreviewConfigRes struct {
	api_v1.StandardRes
	Data *kumo.ConfigPreview `json:"data" dc:"Generated KumoMTA policy preview"`
}

type DeployConfigReq struct {
	g.Meta        `path:"/kumo/config/deploy" tags:"KumoMTA" method:"post" summary:"Dry run or deploy generated KumoMTA policy configuration" in:"body"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Version       string `json:"version" dc:"Preview version to deploy; empty generates current preview"`
	DryRun        bool   `json:"dry_run" dc:"Dry run only"`
}

type DeployConfigRes struct {
	api_v1.StandardRes
	Data *kumo.DeployConfigResult `json:"data" dc:"KumoMTA config deploy result"`
}

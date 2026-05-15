package v1

import (
	"billionmail-core/utility/types/api_v1"

	"github.com/gogf/gf/v2/frame/g"
)

type TenantContext struct {
	TenantID             int64    `json:"tenant_id" dc:"Tenant ID"`
	TenantName           string   `json:"tenant_name" dc:"Tenant name"`
	TenantSlug           string   `json:"tenant_slug" dc:"Tenant slug"`
	Role                 string   `json:"role" dc:"Tenant role"`
	Permissions          []string `json:"permissions" dc:"Tenant permissions"`
	Plan                 string   `json:"plan" dc:"Tenant plan"`
	DailyQuota           int64    `json:"daily_quota" dc:"Daily quota"`
	DailyUsed            int64    `json:"daily_used" dc:"Daily used"`
	Status               string   `json:"status" dc:"Tenant status"`
	IsOperator           bool     `json:"is_operator" dc:"Whether account has platform operator role"`
	SendingStatus        string   `json:"sending_status" dc:"Tenant sending status"`
	SendingBlockReason   string   `json:"sending_block_reason" dc:"Tenant sending block reason"`
	SendingThrottleUntil int64    `json:"sending_throttle_until" dc:"Tenant sending throttle until"`
}

type TenantMembership struct {
	TenantID int64  `json:"tenant_id" dc:"Tenant ID"`
	Name     string `json:"name" dc:"Tenant name"`
	Slug     string `json:"slug" dc:"Tenant slug"`
	Role     string `json:"role" dc:"Tenant role"`
	Status   string `json:"status" dc:"Tenant status"`
	Plan     string `json:"plan" dc:"Tenant plan"`
}

type Tenant struct {
	TenantID        int64  `json:"tenant_id" dc:"Tenant ID"`
	Name            string `json:"name" dc:"Tenant name"`
	Slug            string `json:"slug" dc:"Tenant slug"`
	Status          string `json:"status" dc:"Tenant status"`
	PlanID          int64  `json:"plan_id" dc:"Plan ID"`
	DailyQuota      int64  `json:"daily_quota" dc:"Daily quota"`
	MonthlyQuota    int64  `json:"monthly_quota" dc:"Monthly quota"`
	DefaultKumoPool string `json:"default_kumo_pool" dc:"Default Kumo pool"`
	CreateTime      int64  `json:"create_time" dc:"Create time"`
	UpdateTime      int64  `json:"update_time" dc:"Update time"`
}

type CurrentReq struct {
	g.Meta        `path:"/tenants/current" method:"get" tags:"Tenants" summary:"Get current tenant context"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type CurrentRes struct {
	api_v1.StandardRes
	Data TenantContext `json:"data"`
}

type ListReq struct {
	g.Meta        `path:"/tenants" method:"get" tags:"Tenants" summary:"List tenant workspaces"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
}

type ListRes struct {
	api_v1.StandardRes
	Data struct {
		Tenants []TenantMembership `json:"tenants"`
	} `json:"data"`
}

type SwitchReq struct {
	g.Meta        `path:"/tenants/switch" method:"post" tags:"Tenants" summary:"Switch active tenant" in:"body"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	TenantID      int64  `json:"tenant_id" v:"required|min:1" dc:"Tenant ID"`
}

type SwitchRes struct {
	api_v1.StandardRes
	Data TenantContext `json:"data"`
}

type CreateReq struct {
	g.Meta          `path:"/tenants" method:"post" tags:"Tenants" summary:"Create tenant" in:"body"`
	Authorization   string `json:"authorization" dc:"Authorization" in:"header"`
	Name            string `json:"name" v:"required" dc:"Tenant name"`
	OwnerEmail      string `json:"owner_email" dc:"Owner email"`
	PlanID          int64  `json:"plan_id" dc:"Plan ID"`
	DailyQuota      int64  `json:"daily_quota" dc:"Daily quota"`
	MonthlyQuota    int64  `json:"monthly_quota" dc:"Monthly quota"`
	DefaultKumoPool string `json:"default_kumo_pool" dc:"Default Kumo pool"`
	Status          string `json:"status" dc:"Tenant status"`
}

type CreateRes struct {
	api_v1.StandardRes
	Data Tenant `json:"data"`
}

type SendingProfile struct {
	ID                         int64    `json:"id" dc:"Profile ID"`
	TenantID                   int64    `json:"tenant_id" dc:"Tenant ID"`
	Name                       string   `json:"name" dc:"Profile name"`
	DefaultFromDomain          string   `json:"default_from_domain" dc:"Default From domain"`
	KumoPoolID                 int64    `json:"kumo_pool_id" dc:"Kumo pool ID"`
	KumoPoolName               string   `json:"kumo_pool_name" dc:"Kumo pool name"`
	EgressMode                 string   `json:"egress_mode" dc:"Egress mode"`
	EgressProvider             string   `json:"egress_provider" dc:"Egress provider"`
	DKIMSelector               string   `json:"dkim_selector" dc:"DKIM selector"`
	DailyQuota                 int64    `json:"daily_quota" dc:"Daily quota"`
	HourlyQuota                int64    `json:"hourly_quota" dc:"Hourly quota"`
	WarmupEnabled              bool     `json:"warmup_enabled" dc:"Warmup enabled"`
	Status                     string   `json:"status" dc:"Profile status"`
	PausedReason               string   `json:"paused_reason" dc:"Paused reason"`
	ThrottleUntil              int64    `json:"throttle_until" dc:"Throttle until"`
	OperatorKillSwitch         bool     `json:"operator_kill_switch" dc:"Operator kill switch"`
	BounceThresholdPerMille    int      `json:"bounce_threshold_per_mille" dc:"Bounce threshold per mille"`
	ComplaintThresholdPerMille int      `json:"complaint_threshold_per_mille" dc:"Complaint threshold per mille"`
	Domains                    []string `json:"domains" dc:"Sender domains"`
	CreateTime                 int64    `json:"create_time" dc:"Create time"`
	UpdateTime                 int64    `json:"update_time" dc:"Update time"`
}

type ReadinessCheck struct {
	Name    string `json:"name" dc:"Check name"`
	Ready   bool   `json:"ready" dc:"Ready"`
	Message string `json:"message" dc:"Message"`
}

type DomainReadiness struct {
	Domain string           `json:"domain" dc:"Domain"`
	Ready  bool             `json:"ready" dc:"Ready"`
	Checks []ReadinessCheck `json:"checks" dc:"Checks"`
}

type ProfileReadiness struct {
	Ready   bool              `json:"ready" dc:"Ready"`
	Checks  []ReadinessCheck  `json:"checks" dc:"Checks"`
	Domains []DomainReadiness `json:"domains" dc:"Domain readiness"`
}

type ListSendingProfilesReq struct {
	g.Meta        `path:"/tenants/{id}/sending-profiles" method:"get" tags:"Tenants" summary:"List tenant sending profiles"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	ID            int64  `json:"id" in:"path" v:"required|min:1" dc:"Tenant ID"`
}

type ListSendingProfilesRes struct {
	api_v1.StandardRes
	Data struct {
		Profiles []SendingProfile `json:"profiles"`
	} `json:"data"`
}

type SendingProfileReq struct {
	g.Meta                     `path:"/tenants/{id}/sending-profile" method:"post" tags:"Tenants" summary:"Create or update tenant sending profile" in:"body"`
	Authorization              string   `json:"authorization" dc:"Authorization" in:"header"`
	ID                         int64    `json:"id" in:"path" v:"required|min:1" dc:"Tenant ID"`
	ProfileID                  int64    `json:"profile_id" dc:"Profile ID for update"`
	Name                       string   `json:"name" v:"required" dc:"Profile name"`
	SenderDomains              []string `json:"sender_domains" dc:"Sender domains"`
	DefaultFromDomain          string   `json:"default_from_domain" dc:"Default From domain"`
	KumoPoolID                 int64    `json:"kumo_pool_id" dc:"Kumo pool ID"`
	KumoPoolName               string   `json:"kumo_pool" dc:"Kumo pool name"`
	EgressMode                 string   `json:"egress_mode" dc:"Egress mode"`
	EgressProvider             string   `json:"egress_provider" dc:"Egress provider"`
	DKIMSelector               string   `json:"dkim_selector" dc:"DKIM selector"`
	DailyQuota                 int64    `json:"daily_quota" dc:"Daily quota"`
	HourlyQuota                int64    `json:"hourly_quota" dc:"Hourly quota"`
	WarmupEnabled              bool     `json:"warmup_enabled" dc:"Warmup enabled"`
	Status                     string   `json:"status" dc:"Profile status"`
	BounceThresholdPerMille    int      `json:"bounce_threshold_per_mille" dc:"Bounce threshold per mille"`
	ComplaintThresholdPerMille int      `json:"complaint_threshold_per_mille" dc:"Complaint threshold per mille"`
}

type SendingProfileRes struct {
	api_v1.StandardRes
	Data struct {
		Profile   SendingProfile   `json:"profile"`
		Readiness ProfileReadiness `json:"readiness"`
	} `json:"data"`
}

type SendingControlReq struct {
	g.Meta             `path:"/tenants/{id}/sending-control" method:"post" tags:"Tenants" summary:"Update tenant or profile sending control" in:"body"`
	Authorization      string `json:"authorization" dc:"Authorization" in:"header"`
	ID                 int64  `json:"id" in:"path" v:"required|min:1" dc:"Tenant ID"`
	ProfileID          int64  `json:"profile_id" dc:"Optional sending profile ID"`
	Status             string `json:"status" v:"required" dc:"Control status"`
	Reason             string `json:"reason" dc:"Control reason"`
	ThrottleUntil      int64  `json:"throttle_until" dc:"Throttle until"`
	OperatorKillSwitch bool   `json:"operator_kill_switch" dc:"Operator kill switch"`
}

type SendingControlRes struct {
	api_v1.StandardRes
}

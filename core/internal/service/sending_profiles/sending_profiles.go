package sending_profiles

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"billionmail-core/internal/service/kumo"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	EgressModeExternalKumoProxy  = "external_kumoproxy"
	EgressModeExternalKumoNode   = "external_kumo_node"
	EgressModeProviderHTTPAPI    = "provider_http_api"
	EgressModeProviderSMTP2525   = "provider_smtp_2525"
	EgressModeDirectToMX         = "direct_to_mx"
	EgressProviderDigitalOcean   = "digitalocean"
	ProfileStatusDraft           = "draft"
	ProfileStatusReady           = "ready"
	ProfileStatusWarming         = "warming"
	ProfileStatusPaused          = "paused"
	ProfileStatusSuspended       = "suspended"
	TenantSendingStatusActive    = "active"
	TenantSendingStatusPaused    = "paused"
	TenantSendingStatusSuspended = "suspended"
	TenantSendingStatusThrottled = "throttled"
)

const quotaCounterTTL = 48 * time.Hour

type DNSResolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

type netResolver struct{}

func (netResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func (netResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return net.DefaultResolver.LookupTXT(ctx, name)
}

func (netResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	return net.DefaultResolver.LookupMX(ctx, name)
}

func (netResolver) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	return net.DefaultResolver.LookupAddr(ctx, addr)
}

type Profile struct {
	ID                         int64    `json:"id"`
	TenantID                   int64    `json:"tenant_id"`
	Name                       string   `json:"name"`
	DefaultFromDomain          string   `json:"default_from_domain"`
	KumoPoolID                 int64    `json:"kumo_pool_id"`
	KumoPoolName               string   `json:"kumo_pool_name"`
	EgressMode                 string   `json:"egress_mode"`
	EgressProvider             string   `json:"egress_provider"`
	DKIMSelector               string   `json:"dkim_selector"`
	DailyQuota                 int64    `json:"daily_quota"`
	HourlyQuota                int64    `json:"hourly_quota"`
	WarmupEnabled              bool     `json:"warmup_enabled"`
	Status                     string   `json:"status"`
	PausedReason               string   `json:"paused_reason"`
	ThrottleUntil              int64    `json:"throttle_until"`
	SuspendedAt                int64    `json:"suspended_at"`
	OperatorKillSwitch         bool     `json:"operator_kill_switch"`
	BounceThresholdPerMille    int      `json:"bounce_threshold_per_mille"`
	ComplaintThresholdPerMille int      `json:"complaint_threshold_per_mille"`
	Domains                    []string `json:"domains"`
	CreateTime                 int64    `json:"create_time"`
	UpdateTime                 int64    `json:"update_time"`
}

type UpsertInput struct {
	ProfileID                  int64
	TenantID                   int64
	Name                       string
	SenderDomains              []string
	DefaultFromDomain          string
	KumoPoolID                 int64
	KumoPoolName               string
	EgressMode                 string
	EgressProvider             string
	DKIMSelector               string
	DailyQuota                 int64
	HourlyQuota                int64
	WarmupEnabled              bool
	Status                     string
	BounceThresholdPerMille    int
	ComplaintThresholdPerMille int
}

type ReadinessCheck struct {
	Name    string `json:"name"`
	Ready   bool   `json:"ready"`
	Message string `json:"message"`
}

type DomainReadiness struct {
	Domain string           `json:"domain"`
	Ready  bool             `json:"ready"`
	Checks []ReadinessCheck `json:"checks"`
}

type ProfileReadiness struct {
	Ready   bool              `json:"ready"`
	Checks  []ReadinessCheck  `json:"checks"`
	Domains []DomainReadiness `json:"domains"`
}

type UpsertResult struct {
	Profile   Profile          `json:"profile"`
	Readiness ProfileReadiness `json:"readiness"`
}

type SendGuardInput struct {
	TenantID         int64
	SendingProfileID int64
	Workflow         string
	FromEmail        string
	Recipient        string
	Count            int64
}

type GuardResult struct {
	Allowed     bool
	Reason      string
	Profile     Profile
	Reservation QuotaReservation
}

type QuotaReservation struct {
	TenantDailyKey  string
	ProfileDailyKey string
	ProfileHourKey  string
	Count           int64
}

type ControlInput struct {
	TenantID           int64
	ProfileID          int64
	Status             string
	Reason             string
	ThrottleUntil      int64
	OperatorKillSwitch bool
}

type quotaDecision struct {
	Allowed   bool
	UsedAfter int64
	Limit     int64
	Reason    string
}

type abuseDecision struct {
	Blocked bool
	Reason  string
}

func Upsert(ctx context.Context, input UpsertInput) (*UpsertResult, error) {
	if input.TenantID <= 0 {
		return nil, gerror.New("tenant is required")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, gerror.New("sending profile name is required")
	}
	status := normalizeProfileStatus(input.Status)
	domains := normalizeDomains(input.SenderDomains)
	if len(domains) == 0 && status != ProfileStatusDraft {
		return nil, gerror.New("at least one sender domain is required")
	}
	defaultDomain := strings.ToLower(strings.TrimSpace(input.DefaultFromDomain))
	if defaultDomain == "" && len(domains) > 0 {
		defaultDomain = domains[0]
	}
	if defaultDomain != "" && !containsString(domains, defaultDomain) {
		domains = append(domains, defaultDomain)
	}
	if err := verifyDomainsBelongToTenant(ctx, input.TenantID, domains); err != nil {
		return nil, err
	}

	pool, err := resolvePool(ctx, input.TenantID, input.KumoPoolID, input.KumoPoolName, status)
	if err != nil {
		return nil, err
	}
	egressMode := NormalizeEgressMode(input.EgressMode)
	egressProvider := strings.ToLower(strings.TrimSpace(input.EgressProvider))
	dkimSelector := strings.TrimSpace(input.DKIMSelector)
	readiness := CheckProfileReadiness(ctx, Profile{
		TenantID:                   input.TenantID,
		Name:                       name,
		DefaultFromDomain:          defaultDomain,
		KumoPoolID:                 pool.ID,
		KumoPoolName:               pool.Name,
		EgressMode:                 egressMode,
		EgressProvider:             egressProvider,
		DKIMSelector:               dkimSelector,
		DailyQuota:                 input.DailyQuota,
		HourlyQuota:                input.HourlyQuota,
		WarmupEnabled:              input.WarmupEnabled,
		Status:                     status,
		BounceThresholdPerMille:    normalizeThreshold(input.BounceThresholdPerMille, 100),
		ComplaintThresholdPerMille: normalizeThreshold(input.ComplaintThresholdPerMille, 1),
		Domains:                    domains,
	}, netResolver{})

	if requiresReady(status) && !readiness.Ready {
		return nil, gerror.New("sending profile cannot be activated until KumoMTA readiness checks pass")
	}

	now := time.Now().Unix()
	data := g.Map{
		"tenant_id":                     input.TenantID,
		"name":                          name,
		"default_from_domain":           defaultDomain,
		"kumo_pool_id":                  pool.ID,
		"kumo_pool_name":                pool.Name,
		"egress_mode":                   egressMode,
		"egress_provider":               egressProvider,
		"dkim_selector":                 dkimSelector,
		"daily_quota":                   nonNegative(input.DailyQuota),
		"hourly_quota":                  nonNegative(input.HourlyQuota),
		"warmup_enabled":                boolToSmallInt(input.WarmupEnabled),
		"status":                        status,
		"bounce_threshold_per_mille":    normalizeThreshold(input.BounceThresholdPerMille, 100),
		"complaint_threshold_per_mille": normalizeThreshold(input.ComplaintThresholdPerMille, 1),
		"update_time":                   now,
	}
	profileID := input.ProfileID
	if profileID > 0 {
		result, err := g.DB().Model("tenant_sending_profiles").Ctx(ctx).
			Where("id", profileID).
			Where("tenant_id", input.TenantID).
			Data(data).
			Update()
		if err != nil {
			return nil, err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil, gerror.New("sending profile not found")
		}
	} else {
		data["create_time"] = now
		result, err := g.DB().Model("tenant_sending_profiles").Ctx(ctx).Data(data).Insert()
		if err != nil {
			return nil, err
		}
		profileID, err = result.LastInsertId()
		if err != nil {
			return nil, err
		}
	}
	if err := replaceProfileDomains(ctx, profileID, domains); err != nil {
		return nil, err
	}
	profile, err := Load(ctx, input.TenantID, profileID)
	if err != nil {
		return nil, err
	}
	return &UpsertResult{Profile: *profile, Readiness: readiness}, nil
}

func List(ctx context.Context, tenantID int64) ([]Profile, error) {
	if tenantID <= 0 {
		return []Profile{}, nil
	}
	rows := make([]dbProfile, 0)
	if err := g.DB().Model("tenant_sending_profiles").Ctx(ctx).
		Where("tenant_id", tenantID).
		Order("id ASC").
		Scan(&rows); err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(rows))
	for _, row := range rows {
		profile := row.toProfile()
		profile.Domains = loadProfileDomains(ctx, profile.ID)
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func Load(ctx context.Context, tenantID, profileID int64) (*Profile, error) {
	if tenantID <= 0 || profileID <= 0 {
		return nil, gerror.New("sending profile is required")
	}
	var row dbProfile
	err := g.DB().Model("tenant_sending_profiles").Ctx(ctx).
		Where("id", profileID).
		Where("tenant_id", tenantID).
		Scan(&row)
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, gerror.New("sending profile not found")
	}
	profile := row.toProfile()
	profile.Domains = loadProfileDomains(ctx, profile.ID)
	return &profile, nil
}

func GuardSend(ctx context.Context, input SendGuardInput) (*GuardResult, error) {
	if input.Count <= 0 {
		input.Count = 1
	}
	if input.TenantID <= 0 {
		return deny("tenant context is required"), nil
	}
	tenant, err := loadTenantControl(ctx, input.TenantID)
	if err != nil {
		return nil, err
	}
	if tenant.ID == 0 {
		return deny("tenant not found"), nil
	}
	if reason := tenant.blockReason(time.Now().Unix()); reason != "" {
		kumo.RecordQuotaRejection(ctx, input.TenantID, 0, reason)
		return deny(reason), nil
	}

	profile, err := resolveSendProfile(ctx, input.TenantID, input.SendingProfileID)
	if err != nil {
		kumo.RecordQuotaRejection(ctx, input.TenantID, input.SendingProfileID, err.Error())
		return deny(err.Error()), nil
	}
	if reason := profileBlockReason(*profile, time.Now().Unix()); reason != "" {
		kumo.RecordQuotaRejection(ctx, input.TenantID, profile.ID, reason)
		return deny(reason), nil
	}
	fromDomain := strings.ToLower(strings.TrimSpace(senderDomain(input.FromEmail)))
	if fromDomain == "" {
		return deny("sender domain is required"), nil
	}
	if !containsString(profile.Domains, fromDomain) {
		kumo.RecordQuotaRejection(ctx, input.TenantID, profile.ID, "sender domain is not assigned to the sending profile")
		return deny("sender domain is not assigned to the sending profile"), nil
	}

	usage, err := loadTenantUsage(ctx, input.TenantID, time.Now())
	if err != nil {
		return nil, err
	}
	if abuse := EvaluateAbuse(usage.QueuedCount, usage.BouncedCount, usage.ComplainedCount, profile.BounceThresholdPerMille, profile.ComplaintThresholdPerMille); abuse.Blocked {
		if err := pauseProfile(ctx, profile.ID, abuse.Reason); err != nil {
			g.Log().Warningf(ctx, "failed to pause sending profile %d after abuse threshold: %v", profile.ID, err)
		}
		kumo.RecordQuotaRejection(ctx, input.TenantID, profile.ID, abuse.Reason)
		return deny(abuse.Reason), nil
	}

	reservation, reason, err := reserveQuota(ctx, tenant, *profile, input.Count)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		kumo.RecordQuotaRejection(ctx, input.TenantID, profile.ID, reason)
		return deny(reason), nil
	}
	return &GuardResult{
		Allowed:     true,
		Profile:     *profile,
		Reservation: reservation,
	}, nil
}

func ReleaseReservation(ctx context.Context, reservation QuotaReservation) {
	for _, key := range []string{reservation.TenantDailyKey, reservation.ProfileDailyKey, reservation.ProfileHourKey} {
		if key == "" || reservation.Count <= 0 {
			continue
		}
		for i := int64(0); i < reservation.Count; i++ {
			_, _ = g.Redis().Decr(ctx, key)
		}
	}
}

func RecordQueued(ctx context.Context, tenantID, profileID int64, workflow string, count int64) error {
	if tenantID <= 0 || count <= 0 {
		return nil
	}
	date := time.Now().Format("2006-01-02")
	campaignCount := int64(0)
	apiCount := int64(0)
	switch strings.ToLower(strings.TrimSpace(workflow)) {
	case "api", "send_api":
		apiCount = count
	default:
		campaignCount = count
	}
	_, err := g.DB().Exec(ctx, `
		INSERT INTO tenant_usage_daily
			(tenant_id, date, queued_count, api_count, campaign_count, create_time, update_time)
		VALUES
			(?, ?, ?, ?, ?, EXTRACT(EPOCH FROM NOW()), EXTRACT(EPOCH FROM NOW()))
		ON CONFLICT (tenant_id, date)
		DO UPDATE SET
			queued_count = tenant_usage_daily.queued_count + EXCLUDED.queued_count,
			api_count = tenant_usage_daily.api_count + EXCLUDED.api_count,
			campaign_count = tenant_usage_daily.campaign_count + EXCLUDED.campaign_count,
			update_time = EXCLUDED.update_time
	`, tenantID, date, count, apiCount, campaignCount)
	return err
}

func SetTenantControl(ctx context.Context, input ControlInput) error {
	if input.TenantID <= 0 {
		return gerror.New("tenant is required")
	}
	status := normalizeTenantSendingStatus(input.Status)
	_, err := g.DB().Model("tenants").Ctx(ctx).
		Where("id", input.TenantID).
		Data(g.Map{
			"sending_status":         status,
			"sending_block_reason":   strings.TrimSpace(input.Reason),
			"sending_throttle_until": input.ThrottleUntil,
			"operator_kill_switch":   boolToSmallInt(input.OperatorKillSwitch),
			"sending_suspended_at":   suspendedAt(status),
			"update_time":            time.Now().Unix(),
		}).
		Update()
	if err == nil {
		kumo.RecordControlEvent(ctx, input.TenantID, 0, status, input.Reason)
	}
	return err
}

func SetProfileControl(ctx context.Context, input ControlInput) error {
	if input.TenantID <= 0 || input.ProfileID <= 0 {
		return gerror.New("tenant and profile are required")
	}
	status := normalizeProfileStatus(input.Status)
	_, err := g.DB().Model("tenant_sending_profiles").Ctx(ctx).
		Where("tenant_id", input.TenantID).
		Where("id", input.ProfileID).
		Data(g.Map{
			"status":               status,
			"paused_reason":        strings.TrimSpace(input.Reason),
			"throttle_until":       input.ThrottleUntil,
			"operator_kill_switch": boolToSmallInt(input.OperatorKillSwitch),
			"suspended_at":         suspendedAt(status),
			"update_time":          time.Now().Unix(),
		}).
		Update()
	if err == nil {
		kumo.RecordControlEvent(ctx, input.TenantID, input.ProfileID, status, input.Reason)
	}
	return err
}

func CheckProfileReadiness(ctx context.Context, profile Profile, resolver DNSResolver) ProfileReadiness {
	if resolver == nil {
		resolver = netResolver{}
	}
	readiness := ProfileReadiness{Ready: true, Checks: []ReadinessCheck{}, Domains: []DomainReadiness{}}
	if profile.KumoPoolName == "" && profile.KumoPoolID == 0 {
		readiness.add(false, "kumo_pool", "Kumo egress pool is required")
	}
	if strings.TrimSpace(profile.EgressMode) == "" {
		readiness.add(false, "egress_mode", "Egress mode is required")
	} else if !IsAllowedProductionEgress(profile.EgressMode, profile.EgressProvider) {
		readiness.add(false, "egress_mode", "DigitalOcean direct-to-MX is not production-ready; use KumoProxy, external Kumo node, provider HTTP API, or provider SMTP 2525")
	}
	status := kumo.GetStatus()
	if !status.Connected {
		readiness.add(false, "kumo_injection", "KumoMTA injection endpoint is not currently healthy")
	}
	if status.WebhookLastSeenAt == 0 {
		readiness.add(false, "kumo_webhook", "KumoMTA webhook receiver has not accepted events yet")
	}
	for _, domain := range profile.Domains {
		domainReadiness := CheckDomainReadiness(ctx, DomainReadinessInput{
			Domain:       domain,
			DKIMSelector: profile.DKIMSelector,
			EgressMode:   profile.EgressMode,
			Provider:     profile.EgressProvider,
		}, resolver)
		if !domainReadiness.Ready {
			readiness.Ready = false
		}
		readiness.Domains = append(readiness.Domains, domainReadiness)
	}
	if len(profile.Domains) == 0 {
		readiness.add(false, "sender_domains", "At least one sender domain is required")
	}
	return readiness
}

type DomainReadinessInput struct {
	Domain       string
	DKIMSelector string
	EgressMode   string
	Provider     string
}

func CheckDomainReadiness(ctx context.Context, input DomainReadinessInput, resolver DNSResolver) DomainReadiness {
	domain := strings.ToLower(strings.TrimSpace(input.Domain))
	result := DomainReadiness{Domain: domain, Ready: true, Checks: []ReadinessCheck{}}
	if domain == "" {
		result.add(false, "domain", "Domain is required")
		return result
	}
	txt, err := resolver.LookupTXT(ctx, domain)
	spfOK := err == nil && hasTXTPrefix(txt, "v=spf1")
	result.add(spfOK, "spf", statusMessage(spfOK, "SPF record found", "SPF record is missing"))
	selector := strings.TrimSpace(input.DKIMSelector)
	if selector == "" {
		result.add(false, "dkim", "DKIM selector is required")
	} else {
		dkimTXT, dkimErr := resolver.LookupTXT(ctx, selector+"._domainkey."+domain)
		dkimOK := dkimErr == nil && hasTXTPrefix(dkimTXT, "v=DKIM1")
		result.add(dkimOK, "dkim", statusMessage(dkimOK, "Kumo DKIM selector found", "Kumo DKIM selector is missing"))
	}
	dmarcTXT, dmarcErr := resolver.LookupTXT(ctx, "_dmarc."+domain)
	dmarcOK := dmarcErr == nil && hasTXTPrefix(dmarcTXT, "v=DMARC1")
	result.add(dmarcOK, "dmarc", statusMessage(dmarcOK, "DMARC record found", "DMARC record is missing"))
	_, mxErr := resolver.LookupMX(ctx, domain)
	mxOK := mxErr == nil
	result.info(mxOK, "mx", statusMessage(mxOK, "MX/mailbox readiness record found", "MX is not configured; mailbox readiness is separate from Kumo outbound readiness"))
	egressOK := IsAllowedProductionEgress(input.EgressMode, input.Provider)
	result.add(egressOK, "ptr_ehlo", statusMessage(egressOK, "PTR/EHLO alignment is handled by the selected external egress path", "PTR/EHLO cannot be production-ready on blocked DigitalOcean direct-to-MX egress"))
	if !IsAllowedProductionEgress(input.EgressMode, input.Provider) {
		result.add(false, "delivery_egress", "Direct-to-MX from DigitalOcean is blocked for production")
	}
	return result
}

type GitdateReadiness struct {
	Ready  bool             `json:"ready"`
	Checks []ReadinessCheck `json:"checks"`
}

func CheckGitdateDNS(ctx context.Context, resolver DNSResolver) GitdateReadiness {
	if resolver == nil {
		resolver = netResolver{}
	}
	result := GitdateReadiness{Ready: true, Checks: []ReadinessCheck{}}
	checkHost := func(name, expected, label string) {
		values, err := resolver.LookupHost(ctx, name)
		ok := err == nil && containsString(values, expected)
		result.add(ok, label, statusMessage(ok, name+" resolves to "+expected, name+" does not resolve to "+expected))
	}
	checkHost("mail.gitdate.ink", "159.89.33.85", "mail_a")
	checkHost("email.gitdate.ink", "192.241.130.241", "email_a")
	emailSPF, err := resolver.LookupTXT(ctx, "email.gitdate.ink")
	ok := err == nil && hasTXTContaining(emailSPF, "v=spf1") && hasTXTContaining(emailSPF, "ip4:192.241.130.241")
	result.add(ok, "email_spf", statusMessage(ok, "email.gitdate.ink SPF includes 192.241.130.241", "email.gitdate.ink SPF is missing the Kumo control IP"))
	dkim, err := resolver.LookupTXT(ctx, "s1._domainkey.email.gitdate.ink")
	ok = err == nil && hasTXTPrefix(dkim, "v=DKIM1")
	result.add(ok, "email_dkim", statusMessage(ok, "email.gitdate.ink DKIM selector s1 exists", "email.gitdate.ink DKIM selector s1 is missing"))
	dmarc, err := resolver.LookupTXT(ctx, "_dmarc.email.gitdate.ink")
	ok = err == nil && hasTXTPrefix(dmarc, "v=DMARC1")
	result.add(ok, "email_dmarc", statusMessage(ok, "email.gitdate.ink DMARC exists", "email.gitdate.ink DMARC is missing"))
	return result
}

func EvaluateQuota(limit, used, requested int64) quotaDecision {
	if requested <= 0 {
		requested = 1
	}
	if limit <= 0 {
		return quotaDecision{Allowed: true, UsedAfter: used + requested, Limit: limit}
	}
	usedAfter := used + requested
	if usedAfter > limit {
		return quotaDecision{Allowed: false, UsedAfter: usedAfter, Limit: limit, Reason: fmt.Sprintf("quota exceeded: %d of %d", usedAfter, limit)}
	}
	return quotaDecision{Allowed: true, UsedAfter: usedAfter, Limit: limit}
}

func EvaluateAbuse(queued, bounced, complained int64, bounceThresholdPerMille, complaintThresholdPerMille int) abuseDecision {
	if queued <= 0 {
		return abuseDecision{}
	}
	if bounceThresholdPerMille <= 0 {
		bounceThresholdPerMille = 100
	}
	if complaintThresholdPerMille <= 0 {
		complaintThresholdPerMille = 1
	}
	if bounced*1000 >= queued*int64(bounceThresholdPerMille) {
		return abuseDecision{Blocked: true, Reason: "bounce threshold exceeded"}
	}
	if complained*1000 >= queued*int64(complaintThresholdPerMille) {
		return abuseDecision{Blocked: true, Reason: "complaint threshold exceeded"}
	}
	return abuseDecision{}
}

func IsAllowedProductionEgress(mode, provider string) bool {
	mode = NormalizeEgressMode(mode)
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch mode {
	case EgressModeExternalKumoProxy, EgressModeExternalKumoNode, EgressModeProviderHTTPAPI, EgressModeProviderSMTP2525:
		return true
	case EgressModeDirectToMX:
		return provider != "" && provider != EgressProviderDigitalOcean
	default:
		return false
	}
}

func NormalizeEgressMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "kumoproxy", EgressModeExternalKumoProxy:
		return EgressModeExternalKumoProxy
	case "external", "external_node", EgressModeExternalKumoNode:
		return EgressModeExternalKumoNode
	case "http_api", "provider_api", EgressModeProviderHTTPAPI:
		return EgressModeProviderHTTPAPI
	case "smtp_2525", "submission_2525", EgressModeProviderSMTP2525:
		return EgressModeProviderSMTP2525
	case "direct", "direct_mx", EgressModeDirectToMX:
		return EgressModeDirectToMX
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

type dbProfile struct {
	ID                         int64  `json:"id"`
	TenantID                   int64  `json:"tenant_id"`
	Name                       string `json:"name"`
	DefaultFromDomain          string `json:"default_from_domain"`
	KumoPoolID                 int64  `json:"kumo_pool_id"`
	KumoPoolName               string `json:"kumo_pool_name"`
	EgressMode                 string `json:"egress_mode"`
	EgressProvider             string `json:"egress_provider"`
	DKIMSelector               string `json:"dkim_selector"`
	DailyQuota                 int64  `json:"daily_quota"`
	HourlyQuota                int64  `json:"hourly_quota"`
	WarmupEnabled              int    `json:"warmup_enabled"`
	Status                     string `json:"status"`
	PausedReason               string `json:"paused_reason"`
	ThrottleUntil              int64  `json:"throttle_until"`
	SuspendedAt                int64  `json:"suspended_at"`
	OperatorKillSwitch         int    `json:"operator_kill_switch"`
	BounceThresholdPerMille    int    `json:"bounce_threshold_per_mille"`
	ComplaintThresholdPerMille int    `json:"complaint_threshold_per_mille"`
	CreateTime                 int64  `json:"create_time"`
	UpdateTime                 int64  `json:"update_time"`
}

func (p dbProfile) toProfile() Profile {
	return Profile{
		ID:                         p.ID,
		TenantID:                   p.TenantID,
		Name:                       p.Name,
		DefaultFromDomain:          p.DefaultFromDomain,
		KumoPoolID:                 p.KumoPoolID,
		KumoPoolName:               p.KumoPoolName,
		EgressMode:                 p.EgressMode,
		EgressProvider:             p.EgressProvider,
		DKIMSelector:               p.DKIMSelector,
		DailyQuota:                 p.DailyQuota,
		HourlyQuota:                p.HourlyQuota,
		WarmupEnabled:              p.WarmupEnabled == 1,
		Status:                     p.Status,
		PausedReason:               p.PausedReason,
		ThrottleUntil:              p.ThrottleUntil,
		SuspendedAt:                p.SuspendedAt,
		OperatorKillSwitch:         p.OperatorKillSwitch == 1,
		BounceThresholdPerMille:    p.BounceThresholdPerMille,
		ComplaintThresholdPerMille: p.ComplaintThresholdPerMille,
		CreateTime:                 p.CreateTime,
		UpdateTime:                 p.UpdateTime,
	}
}

type poolRow struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type tenantControl struct {
	ID                         int64  `json:"id"`
	Status                     string `json:"status"`
	DailyQuota                 int64  `json:"daily_quota"`
	SendingStatus              string `json:"sending_status"`
	SendingBlockReason         string `json:"sending_block_reason"`
	SendingThrottleUntil       int64  `json:"sending_throttle_until"`
	OperatorKillSwitch         int    `json:"operator_kill_switch"`
	BounceThresholdPerMille    int    `json:"bounce_threshold_per_mille"`
	ComplaintThresholdPerMille int    `json:"complaint_threshold_per_mille"`
}

func (t tenantControl) blockReason(now int64) string {
	if strings.ToLower(strings.TrimSpace(t.Status)) != "active" {
		return "tenant is not active"
	}
	if t.OperatorKillSwitch == 1 {
		return nonEmpty(t.SendingBlockReason, "tenant sending is blocked by operator")
	}
	switch strings.ToLower(strings.TrimSpace(t.SendingStatus)) {
	case "", TenantSendingStatusActive:
		return ""
	case TenantSendingStatusThrottled:
		if t.SendingThrottleUntil > now {
			return nonEmpty(t.SendingBlockReason, "tenant sending is throttled")
		}
		return ""
	case TenantSendingStatusPaused, TenantSendingStatusSuspended:
		return nonEmpty(t.SendingBlockReason, "tenant sending is "+t.SendingStatus)
	default:
		return nonEmpty(t.SendingBlockReason, "tenant sending is blocked")
	}
}

type usageRow struct {
	QueuedCount     int64 `json:"queued_count"`
	BouncedCount    int64 `json:"bounced_count"`
	ComplainedCount int64 `json:"complained_count"`
}

func resolvePool(ctx context.Context, tenantID int64, poolID int64, poolName string, profileStatus string) (poolRow, error) {
	poolName = strings.TrimSpace(poolName)
	if poolID == 0 && poolName == "" {
		var tenant struct {
			DefaultKumoPool string `json:"default_kumo_pool"`
		}
		_ = g.DB().Model("tenants").Ctx(ctx).Fields("default_kumo_pool").Where("id", tenantID).Scan(&tenant)
		poolName = tenant.DefaultKumoPool
	}
	if poolID == 0 && poolName == "" {
		if requiresReady(profileStatus) {
			return poolRow{}, gerror.New("Kumo egress pool is required")
		}
		return poolRow{}, nil
	}
	var row poolRow
	model := g.DB().Model("kumo_egress_pools").Ctx(ctx)
	if poolID > 0 {
		model = model.Where("id", poolID)
	} else {
		model = model.Where("name", poolName)
	}
	if err := model.Scan(&row); err != nil {
		return poolRow{}, err
	}
	if row.ID == 0 {
		return poolRow{}, gerror.New("Kumo egress pool not found")
	}
	if strings.ToLower(row.Status) != "active" && strings.ToLower(row.Status) != "enabled" {
		return poolRow{}, gerror.New("Kumo egress pool is not active")
	}
	allowed, err := poolAllowedForTenant(ctx, tenantID, row.ID, row.Name)
	if err != nil {
		return poolRow{}, err
	}
	if !allowed {
		return poolRow{}, gerror.New("Kumo egress pool is not allowed for this tenant")
	}
	return row, nil
}

func poolAllowedForTenant(ctx context.Context, tenantID, poolID int64, poolName string) (bool, error) {
	count, err := g.DB().Model("tenant_allowed_kumo_pools").Ctx(ctx).Where("tenant_id", tenantID).Count()
	if err != nil {
		return false, err
	}
	if count == 0 {
		return true, nil
	}
	allowed, err := g.DB().Model("tenant_allowed_kumo_pools").Ctx(ctx).
		Where("tenant_id", tenantID).
		Where("pool_id", poolID).
		Exist()
	if err != nil {
		return false, err
	}
	if allowed {
		return true, nil
	}
	var tenant struct {
		DefaultKumoPool string `json:"default_kumo_pool"`
	}
	_ = g.DB().Model("tenants").Ctx(ctx).Fields("default_kumo_pool").Where("id", tenantID).Scan(&tenant)
	return strings.TrimSpace(tenant.DefaultKumoPool) != "" && tenant.DefaultKumoPool == poolName, nil
}

func verifyDomainsBelongToTenant(ctx context.Context, tenantID int64, domains []string) error {
	for _, domain := range domains {
		var row struct {
			Domain   string `json:"domain"`
			TenantID int64  `json:"tenant_id"`
			Active   int    `json:"active"`
		}
		if err := g.DB().Model("domain").Ctx(ctx).Fields("domain, tenant_id, active").Where("domain", domain).Scan(&row); err != nil {
			return err
		}
		if row.Domain == "" {
			return fmt.Errorf("sender domain %s does not exist", domain)
		}
		if row.TenantID != tenantID {
			return fmt.Errorf("sender domain %s does not belong to this tenant", domain)
		}
		if row.Active != 1 {
			return fmt.Errorf("sender domain %s is not active", domain)
		}
	}
	return nil
}

func replaceProfileDomains(ctx context.Context, profileID int64, domains []string) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model("tenant_sending_profile_domains").Ctx(ctx).Where("profile_id", profileID).Delete(); err != nil {
			return err
		}
		now := time.Now().Unix()
		for _, domain := range domains {
			if _, err := tx.Model("tenant_sending_profile_domains").Ctx(ctx).Data(g.Map{
				"profile_id":  profileID,
				"domain_name": domain,
				"create_time": now,
				"update_time": now,
			}).InsertIgnore(); err != nil {
				return err
			}
		}
		return nil
	})
}

func loadProfileDomains(ctx context.Context, profileID int64) []string {
	rows := make([]struct {
		DomainName string `json:"domain_name"`
	}, 0)
	_ = g.DB().Model("tenant_sending_profile_domains").Ctx(ctx).
		Fields("domain_name").
		Where("profile_id", profileID).
		Order("domain_name ASC").
		Scan(&rows)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.DomainName != "" {
			out = append(out, strings.ToLower(row.DomainName))
		}
	}
	return out
}

func resolveSendProfile(ctx context.Context, tenantID, profileID int64) (*Profile, error) {
	if profileID > 0 {
		return Load(ctx, tenantID, profileID)
	}
	profiles, err := List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if profile.Status == ProfileStatusReady || profile.Status == ProfileStatusWarming {
			return &profile, nil
		}
	}
	return nil, gerror.New("tenant sending profile is required for KumoMTA")
}

func loadTenantControl(ctx context.Context, tenantID int64) (tenantControl, error) {
	var row tenantControl
	err := g.DB().Model("tenants").Ctx(ctx).
		Fields("id, status, daily_quota, sending_status, sending_block_reason, sending_throttle_until, operator_kill_switch, bounce_threshold_per_mille, complaint_threshold_per_mille").
		Where("id", tenantID).
		Scan(&row)
	return row, err
}

func loadTenantUsage(ctx context.Context, tenantID int64, now time.Time) (usageRow, error) {
	var row usageRow
	err := g.DB().Model("tenant_usage_daily").Ctx(ctx).
		Fields("queued_count, bounced_count, complained_count").
		Where("tenant_id", tenantID).
		Where("date", now.Format("2006-01-02")).
		Scan(&row)
	return row, err
}

func reserveQuota(ctx context.Context, tenant tenantControl, profile Profile, count int64) (QuotaReservation, string, error) {
	reservation := QuotaReservation{Count: count}
	today := time.Now().Format("20060102")
	hour := time.Now().Format("2006010215")
	usage, err := loadTenantUsage(ctx, tenant.ID, time.Now())
	if err != nil {
		return reservation, "", err
	}
	reservation.TenantDailyKey = fmt.Sprintf("tenant:quota:daily:%d:%s", tenant.ID, today)
	reason, err := reserveCounter(ctx, reservation.TenantDailyKey, tenant.DailyQuota, usage.QueuedCount, count)
	if err != nil || reason != "" {
		return reservation, reason, err
	}
	reservation.ProfileDailyKey = fmt.Sprintf("tenant:profile:quota:daily:%d:%d:%s", tenant.ID, profile.ID, today)
	reason, err = reserveCounter(ctx, reservation.ProfileDailyKey, profile.DailyQuota, 0, count)
	if err != nil || reason != "" {
		ReleaseReservation(ctx, QuotaReservation{TenantDailyKey: reservation.TenantDailyKey, Count: count})
		return reservation, reason, err
	}
	reservation.ProfileHourKey = fmt.Sprintf("tenant:profile:quota:hour:%d:%d:%s", tenant.ID, profile.ID, hour)
	reason, err = reserveCounter(ctx, reservation.ProfileHourKey, profile.HourlyQuota, 0, count)
	if err != nil || reason != "" {
		ReleaseReservation(ctx, QuotaReservation{TenantDailyKey: reservation.TenantDailyKey, ProfileDailyKey: reservation.ProfileDailyKey, Count: count})
		return reservation, reason, err
	}
	return reservation, "", nil
}

func reserveCounter(ctx context.Context, key string, limit, seed, count int64) (string, error) {
	if limit <= 0 {
		return "", nil
	}
	val, err := g.Redis().Get(ctx, key)
	if err != nil {
		return "", err
	}
	if val.IsNil() {
		if err := g.Redis().SetEX(ctx, key, seed, int64(quotaCounterTTL.Seconds())); err != nil {
			return "", err
		}
	}
	for i := int64(0); i < count; i++ {
		next, err := g.Redis().Incr(ctx, key)
		if err != nil {
			return "", err
		}
		if next > limit {
			_, _ = g.Redis().Decr(ctx, key)
			return fmt.Sprintf("quota exceeded: %d of %d", next, limit), nil
		}
	}
	_, _ = g.Redis().Expire(ctx, key, int64(quotaCounterTTL.Seconds()))
	return "", nil
}

func pauseProfile(ctx context.Context, profileID int64, reason string) error {
	_, err := g.DB().Model("tenant_sending_profiles").Ctx(ctx).
		Where("id", profileID).
		Data(g.Map{
			"status":        ProfileStatusPaused,
			"paused_reason": reason,
			"update_time":   time.Now().Unix(),
		}).
		Update()
	return err
}

func deny(reason string) *GuardResult {
	return &GuardResult{Allowed: false, Reason: reason}
}

func profileBlockReason(profile Profile, now int64) string {
	if profile.OperatorKillSwitch {
		return nonEmpty(profile.PausedReason, "sending profile is blocked by operator")
	}
	if profile.ThrottleUntil > now {
		return nonEmpty(profile.PausedReason, "sending profile is throttled")
	}
	switch normalizeProfileStatus(profile.Status) {
	case ProfileStatusReady, ProfileStatusWarming:
		return ""
	case ProfileStatusPaused, ProfileStatusSuspended:
		return nonEmpty(profile.PausedReason, "sending profile is "+profile.Status)
	default:
		return "sending profile is not ready"
	}
}

func normalizeDomains(domains []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	return out
}

func normalizeProfileStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", ProfileStatusDraft:
		return ProfileStatusDraft
	case "active", ProfileStatusReady:
		return ProfileStatusReady
	case ProfileStatusWarming:
		return ProfileStatusWarming
	case ProfileStatusPaused:
		return ProfileStatusPaused
	case ProfileStatusSuspended:
		return ProfileStatusSuspended
	default:
		return ProfileStatusDraft
	}
}

func normalizeTenantSendingStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", TenantSendingStatusActive:
		return TenantSendingStatusActive
	case TenantSendingStatusPaused:
		return TenantSendingStatusPaused
	case TenantSendingStatusSuspended:
		return TenantSendingStatusSuspended
	case TenantSendingStatusThrottled:
		return TenantSendingStatusThrottled
	default:
		return TenantSendingStatusPaused
	}
}

func requiresReady(status string) bool {
	switch normalizeProfileStatus(status) {
	case ProfileStatusReady, ProfileStatusWarming:
		return true
	default:
		return false
	}
}

func normalizeThreshold(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func boolToSmallInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func suspendedAt(status string) int64 {
	if strings.ToLower(strings.TrimSpace(status)) == ProfileStatusSuspended || strings.ToLower(strings.TrimSpace(status)) == TenantSendingStatusSuspended {
		return time.Now().Unix()
	}
	return 0
}

func senderDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[1]))
}

func containsString(values []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == needle {
			return true
		}
	}
	return false
}

func hasTXTPrefix(values []string, prefix string) bool {
	prefix = strings.ToLower(prefix)
	for _, value := range values {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), prefix) {
			return true
		}
	}
	return false
}

func hasTXTContaining(values []string, needle string) bool {
	needle = strings.ToLower(needle)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func statusMessage(ok bool, pass, fail string) string {
	if ok {
		return pass
	}
	return fail
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func (r *ProfileReadiness) add(ok bool, name, message string) {
	r.Checks = append(r.Checks, ReadinessCheck{Name: name, Ready: ok, Message: message})
	if !ok {
		r.Ready = false
	}
}

func (r *DomainReadiness) add(ok bool, name, message string) {
	r.Checks = append(r.Checks, ReadinessCheck{Name: name, Ready: ok, Message: message})
	if !ok {
		r.Ready = false
	}
}

func (r *DomainReadiness) info(ok bool, name, message string) {
	r.Checks = append(r.Checks, ReadinessCheck{Name: name, Ready: ok, Message: message})
}

func (r *GitdateReadiness) add(ok bool, name, message string) {
	r.Checks = append(r.Checks, ReadinessCheck{Name: name, Ready: ok, Message: message})
	if !ok {
		r.Ready = false
	}
}

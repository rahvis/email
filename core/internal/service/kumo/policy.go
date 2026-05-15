package kumo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"billionmail-core/internal/service/domains"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	configVersionStatusPreview          = "preview"
	configVersionStatusDryRun           = "dry_run"
	configVersionStatusValidationFailed = "validation_failed"
	configVersionStatusBlocked          = "blocked"
	configVersionStatusDeployed         = "deployed"
)

type policyTenant struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Status          string `json:"status"`
	DefaultKumoPool string `json:"default_kumo_pool"`
}

type policyPool struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type policyNode struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	MetricsURL string `json:"metrics_url"`
	Status     string `json:"status"`
}

type policySource struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	SourceAddress string `json:"source_address"`
	EHLODomain    string `json:"ehlo_domain"`
	NodeID        int64  `json:"node_id"`
	Status        string `json:"status"`
	WarmupStatus  string `json:"warmup_status"`
}

type policyPoolSource struct {
	PoolID   int64  `json:"pool_id"`
	PoolName string `json:"pool_name"`
	SourceID int64  `json:"source_id"`
	Source   string `json:"source"`
	Weight   int    `json:"weight"`
	Status   string `json:"status"`
}

type policyProfile struct {
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
	WarmupEnabled              int      `json:"warmup_enabled"`
	Status                     string   `json:"status"`
	BounceThresholdPerMille    int      `json:"bounce_threshold_per_mille"`
	ComplaintThresholdPerMille int      `json:"complaint_threshold_per_mille"`
	Domains                    []string `json:"domains"`
}

type policyState struct {
	Config         *PublicConfig
	WebhookBaseURL string
	Tenants        []policyTenant
	Pools          []policyPool
	Nodes          []policyNode
	Sources        []policySource
	PoolSources    []policyPoolSource
	Profiles       []policyProfile
}

func GenerateConfigPreview(ctx context.Context, generatedBy int64) (*ConfigPreview, error) {
	state, err := loadPolicyState(ctx)
	if err != nil {
		return nil, err
	}
	preview := buildConfigPreviewFromState(state, generatedBy)
	if err := storeConfigVersion(ctx, preview, configVersionStatusPreview, "", false, 0, ""); err != nil {
		return nil, err
	}
	return preview, nil
}

func DeployConfig(ctx context.Context, input DeployConfigInput, generatedBy int64) (*DeployConfigResult, error) {
	version := strings.TrimSpace(input.Version)
	preview, err := loadConfigPreviewVersion(ctx, version, generatedBy)
	if err != nil {
		return nil, err
	}
	if len(preview.ValidationErrors) > 0 {
		_ = storeConfigVersion(ctx, preview, configVersionStatusValidationFailed, strings.Join(preview.ValidationErrors, "; "), input.DryRun, 0, "")
		RecordRuntimeAlert(RuntimeAlert{
			Type:     "kumo_config_deploy_failed",
			Severity: alertSeverityCritical,
			Message:  "KumoMTA policy deploy validation failed for version " + preview.Version,
		})
		return &DeployConfigResult{
			Version:          preview.Version,
			Status:           configVersionStatusValidationFailed,
			DryRun:           input.DryRun,
			Message:          "KumoMTA policy validation failed",
			Warnings:         preview.Warnings,
			ValidationErrors: preview.ValidationErrors,
		}, gerror.New("KumoMTA policy validation failed")
	}
	rollbackVersion, _ := latestDeployedConfigVersion(ctx)
	if input.DryRun {
		if err := storeConfigVersion(ctx, preview, configVersionStatusDryRun, "", true, 0, rollbackVersion); err != nil {
			return nil, err
		}
		return &DeployConfigResult{
			Version:         preview.Version,
			Status:          configVersionStatusDryRun,
			DryRun:          true,
			RollbackVersion: rollbackVersion,
			Message:         "Dry run validated; managed deployment remains disabled for v1 manual rollout",
			Warnings:        preview.Warnings,
		}, nil
	}

	err = gerror.New("managed KumoMTA deployment is disabled; use config preview and manual deployment or dry_run")
	_ = storeConfigVersion(ctx, preview, configVersionStatusBlocked, err.Error(), false, 0, rollbackVersion)
	RecordRuntimeAlert(RuntimeAlert{
		Type:     "kumo_config_deploy_blocked",
		Severity: alertSeverityWarning,
		Message:  "KumoMTA managed policy deploy blocked for version " + preview.Version,
	})
	return &DeployConfigResult{
		Version:         preview.Version,
		Status:          configVersionStatusBlocked,
		DryRun:          false,
		RollbackVersion: rollbackVersion,
		Message:         err.Error(),
		Warnings:        preview.Warnings,
	}, err
}

func loadPolicyState(ctx context.Context) (policyState, error) {
	cfg, err := GetPublicConfig(ctx)
	if err != nil {
		return policyState{}, err
	}
	state := policyState{
		Config:         cfg,
		WebhookBaseURL: domains.GetBaseURL(),
	}
	if err := g.DB().Model("tenants").Ctx(ctx).
		Fields("id, name, slug, status, default_kumo_pool").
		Order("id ASC").
		Scan(&state.Tenants); err != nil {
		return state, err
	}
	if err := g.DB().Model("kumo_nodes").Ctx(ctx).
		Fields("id, name, base_url, metrics_url, status").
		Order("name ASC, id ASC").
		Scan(&state.Nodes); err != nil {
		return state, err
	}
	if err := g.DB().Model("kumo_egress_pools").Ctx(ctx).
		Fields("id, name, description, status").
		Order("name ASC, id ASC").
		Scan(&state.Pools); err != nil {
		return state, err
	}
	if err := g.DB().Model("kumo_egress_sources").Ctx(ctx).
		Fields("id, name, source_address, ehlo_domain, node_id, status, warmup_status").
		Order("name ASC, id ASC").
		Scan(&state.Sources); err != nil {
		return state, err
	}
	if err := g.DB().Model("kumo_egress_pool_sources ps").Ctx(ctx).
		LeftJoin("kumo_egress_pools p", "p.id = ps.pool_id").
		LeftJoin("kumo_egress_sources s", "s.id = ps.source_id").
		Fields("ps.pool_id, COALESCE(p.name, '') as pool_name, ps.source_id, COALESCE(s.name, '') as source, ps.weight, ps.status").
		Order("p.name ASC, s.name ASC, ps.id ASC").
		Scan(&state.PoolSources); err != nil {
		return state, err
	}
	if err := g.DB().Model("tenant_sending_profiles").Ctx(ctx).
		Fields("id, tenant_id, name, default_from_domain, kumo_pool_id, kumo_pool_name, egress_mode, egress_provider, dkim_selector, daily_quota, hourly_quota, warmup_enabled, status, bounce_threshold_per_mille, complaint_threshold_per_mille").
		Order("tenant_id ASC, id ASC").
		Scan(&state.Profiles); err != nil {
		return state, err
	}
	for i := range state.Profiles {
		state.Profiles[i].Domains = loadPolicyProfileDomains(ctx, state.Profiles[i].ID)
	}
	return state, nil
}

func buildConfigPreviewFromState(state policyState, generatedBy int64) *ConfigPreview {
	sortPolicyState(&state)
	files := []PolicyFile{
		buildTenantPoolsFile(state),
		buildEgressSourcesFile(state),
		buildDKIMFile(state),
		buildTrafficClassesFile(state),
		buildWebhooksFile(state),
	}
	warnings, validationErrors := validatePolicyState(state, files)
	version := policyVersion(files, warnings, validationErrors)
	return &ConfigPreview{
		Version:          version,
		GeneratedAt:      time.Now().Unix(),
		GeneratedBy:      generatedBy,
		Files:            files,
		Warnings:         warnings,
		ValidationErrors: validationErrors,
	}
}

func buildTenantPoolsFile(state policyState) PolicyFile {
	var b strings.Builder
	b.WriteString("-- Generated by BillionMail. Manual deployment preview only.\n")
	b.WriteString("return {\n")
	b.WriteString("  tenants = {\n")
	for _, tenant := range state.Tenants {
		b.WriteString(fmt.Sprintf("    [%d] = { slug = %q, name = %q, status = %q, default_pool = %q },\n",
			tenant.ID, tenant.Slug, tenant.Name, tenant.Status, tenant.DefaultKumoPool))
	}
	b.WriteString("  },\n")
	b.WriteString("  profiles = {\n")
	for _, profile := range state.Profiles {
		b.WriteString(fmt.Sprintf("    [%d] = { tenant_id = %d, name = %q, status = %q, pool = %q, egress_mode = %q, egress_provider = %q, domains = {%s} },\n",
			profile.ID, profile.TenantID, profile.Name, profile.Status, profile.KumoPoolName, profile.EgressMode, profile.EgressProvider, quotedList(profile.Domains)))
	}
	b.WriteString("  },\n")
	b.WriteString("}\n")
	return newPolicyFile("policy/tenant_pools.lua", b.String())
}

func buildEgressSourcesFile(state policyState) PolicyFile {
	var b strings.Builder
	b.WriteString("-- Generated by BillionMail. Define pools/sources on KumoMTA manually for v1.\n")
	b.WriteString("return {\n")
	b.WriteString("  nodes = {\n")
	for _, node := range state.Nodes {
		b.WriteString(fmt.Sprintf("    [%d] = { name = %q, base_url = %q, metrics_url = %q, status = %q },\n",
			node.ID, node.Name, node.BaseURL, node.MetricsURL, node.Status))
	}
	b.WriteString("  },\n")
	b.WriteString("  sources = {\n")
	for _, source := range state.Sources {
		b.WriteString(fmt.Sprintf("    [%d] = { name = %q, source_address = %q, ehlo_domain = %q, node_id = %d, status = %q, warmup_status = %q },\n",
			source.ID, source.Name, source.SourceAddress, source.EHLODomain, source.NodeID, source.Status, source.WarmupStatus))
	}
	b.WriteString("  },\n")
	b.WriteString("  pools = {\n")
	for _, pool := range state.Pools {
		b.WriteString(fmt.Sprintf("    [%d] = { name = %q, status = %q, description = %q },\n", pool.ID, pool.Name, pool.Status, pool.Description))
	}
	b.WriteString("  },\n")
	b.WriteString("  pool_sources = {\n")
	for _, link := range state.PoolSources {
		b.WriteString(fmt.Sprintf("    { pool_id = %d, pool = %q, source_id = %d, source = %q, weight = %d, status = %q },\n",
			link.PoolID, link.PoolName, link.SourceID, link.Source, link.Weight, link.Status))
	}
	b.WriteString("  },\n")
	b.WriteString("}\n")
	return newPolicyFile("policy/egress_sources.lua", b.String())
}

func buildDKIMFile(state policyState) PolicyFile {
	var b strings.Builder
	b.WriteString("-- Generated by BillionMail. DKIM private keys are intentionally not exported here.\n")
	b.WriteString("return {\n")
	b.WriteString("  signing_domains = {\n")
	for _, profile := range state.Profiles {
		for _, domain := range profile.Domains {
			b.WriteString(fmt.Sprintf("    { tenant_id = %d, profile_id = %d, domain = %q, selector = %q, pool = %q, key_ref = %q },\n",
				profile.TenantID, profile.ID, domain, profile.DKIMSelector, profile.KumoPoolName, "billionmail://"+domain+"/"+profile.DKIMSelector))
		}
	}
	b.WriteString("  },\n")
	b.WriteString("}\n")
	return newPolicyFile("policy/dkim.lua", b.String())
}

func buildTrafficClassesFile(state policyState) PolicyFile {
	var b strings.Builder
	b.WriteString("-- Generated by BillionMail. Product quotas stay in BillionMail; Kumo handles delivery-plane shaping.\n")
	b.WriteString("return {\n")
	b.WriteString("  traffic_classes = {\n")
	for _, profile := range state.Profiles {
		className := fmt.Sprintf("tenant_%d_profile_%d", profile.TenantID, profile.ID)
		b.WriteString(fmt.Sprintf("    [%q] = { tenant_id = %d, profile_id = %d, pool = %q, daily_quota = %d, hourly_quota = %d, warmup = %s, bounce_threshold_per_mille = %d, complaint_threshold_per_mille = %d },\n",
			className, profile.TenantID, profile.ID, profile.KumoPoolName, profile.DailyQuota, profile.HourlyQuota, luaBool(profile.WarmupEnabled == 1), profile.BounceThresholdPerMille, profile.ComplaintThresholdPerMille))
	}
	b.WriteString("  },\n")
	b.WriteString("}\n")
	return newPolicyFile("policy/traffic_classes.lua", b.String())
}

func buildWebhooksFile(state policyState) PolicyFile {
	var b strings.Builder
	webhookURL := webhookURLForPolicy(state)
	b.WriteString("-- Generated by BillionMail. Webhook secrets are never exported.\n")
	b.WriteString("return {\n")
	b.WriteString(fmt.Sprintf("  webhook_url = %q,\n", webhookURL))
	b.WriteString("  correlation_headers = {\n")
	for _, header := range []string{
		"X-BM-Tenant-ID",
		"X-BM-Campaign-ID",
		"X-BM-Task-ID",
		"X-BM-Recipient-ID",
		"X-BM-Api-ID",
		"X-BM-Api-Log-ID",
		"X-BM-Message-ID",
		"X-BM-Sending-Profile-ID",
		"X-BM-Engine",
	} {
		b.WriteString(fmt.Sprintf("    %q,\n", header))
	}
	b.WriteString("  },\n")
	b.WriteString("  auth = { mode = \"hmac_or_bearer\", secret_ref = \"billionmail:kumo_webhook_secret\" },\n")
	b.WriteString("}\n")
	return newPolicyFile("policy/webhooks.lua", b.String())
}

func webhookURLForPolicy(state policyState) string {
	baseURL := strings.TrimRight(strings.TrimSpace(state.WebhookBaseURL), "/")
	if baseURL == "" {
		return "https://mail.gitdate.ink/api/kumo/events"
	}
	return baseURL + "/api/kumo/events"
}

func validatePolicyState(state policyState, files []PolicyFile) ([]string, []string) {
	warnings := []string{}
	errors := []string{}
	if state.Config == nil || !state.Config.Enabled {
		warnings = append(warnings, "KumoMTA is not enabled in BillionMail config")
	}
	if state.Config != nil && !state.Config.HasWebhookSecret {
		warnings = append(warnings, "KumoMTA webhook secret is not configured")
	}
	if strings.TrimSpace(state.WebhookBaseURL) == "" {
		warnings = append(warnings, "KumoMTA webhook URL fell back to https://mail.gitdate.ink/api/kumo/events because no reverse proxy base URL is configured")
	}
	poolByName := map[string]policyPool{}
	for _, pool := range state.Pools {
		poolByName[pool.Name] = pool
	}
	for _, profile := range state.Profiles {
		if profile.Status != "ready" && profile.Status != "warming" {
			continue
		}
		if strings.TrimSpace(profile.KumoPoolName) == "" {
			errors = append(errors, fmt.Sprintf("tenant_%d profile_%d has no Kumo pool", profile.TenantID, profile.ID))
		} else if pool, ok := poolByName[profile.KumoPoolName]; ok && pool.Status != "active" && pool.Status != "enabled" {
			errors = append(errors, fmt.Sprintf("tenant_%d profile_%d uses inactive Kumo pool %s", profile.TenantID, profile.ID, profile.KumoPoolName))
		}
		if strings.TrimSpace(profile.DKIMSelector) == "" {
			warnings = append(warnings, fmt.Sprintf("tenant_%d profile_%d has no DKIM selector", profile.TenantID, profile.ID))
		}
		if len(profile.Domains) == 0 {
			errors = append(errors, fmt.Sprintf("tenant_%d profile_%d has no sender domains", profile.TenantID, profile.ID))
		}
		if strings.EqualFold(profile.EgressMode, "direct_to_mx") && strings.EqualFold(profile.EgressProvider, "digitalocean") {
			errors = append(errors, fmt.Sprintf("tenant_%d profile_%d uses blocked DigitalOcean direct-to-MX egress", profile.TenantID, profile.ID))
		}
	}
	for _, file := range files {
		lower := strings.ToLower(file.Content)
		for _, forbidden := range []string{"auth_secret", "webhook_secret =", "begin rsa private key", "begin private key"} {
			if strings.Contains(lower, forbidden) {
				errors = append(errors, fmt.Sprintf("%s contains forbidden secret material marker %q", file.Path, forbidden))
			}
		}
	}
	sort.Strings(warnings)
	sort.Strings(errors)
	return warnings, errors
}

func loadPolicyProfileDomains(ctx context.Context, profileID int64) []string {
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
		if strings.TrimSpace(row.DomainName) != "" {
			out = append(out, strings.ToLower(strings.TrimSpace(row.DomainName)))
		}
	}
	return out
}

func storeConfigVersion(ctx context.Context, preview *ConfigPreview, status string, errorMessage string, dryRun bool, deployedAt int64, rollbackVersion string) error {
	if preview == nil {
		return gerror.New("config preview is required")
	}
	body, err := json.Marshal(preview)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err = g.DB().Model("kumo_config_versions").Ctx(ctx).
		Data(g.Map{
			"version":                 preview.Version,
			"generated_by_account_id": preview.GeneratedBy,
			"status":                  status,
			"preview":                 string(body),
			"deployed_at":             deployedAt,
			"error":                   errorMessage,
			"rollback_version":        rollbackVersion,
			"dry_run":                 boolToSmallInt(dryRun),
			"create_time":             now,
			"update_time":             now,
		}).
		OnConflict("version").
		OnDuplicate(g.Map{
			"generated_by_account_id": preview.GeneratedBy,
			"status":                  status,
			"preview":                 string(body),
			"deployed_at":             deployedAt,
			"error":                   errorMessage,
			"rollback_version":        rollbackVersion,
			"dry_run":                 boolToSmallInt(dryRun),
			"update_time":             now,
		}).
		Save()
	return err
}

func loadConfigPreviewVersion(ctx context.Context, version string, generatedBy int64) (*ConfigPreview, error) {
	if strings.TrimSpace(version) == "" {
		return GenerateConfigPreview(ctx, generatedBy)
	}
	val, err := g.DB().Model("kumo_config_versions").Ctx(ctx).Where("version", version).Value("preview")
	if err != nil {
		return nil, err
	}
	if val == nil || strings.TrimSpace(val.String()) == "" {
		return nil, gerror.New("KumoMTA config preview version not found")
	}
	var preview ConfigPreview
	if err := json.Unmarshal([]byte(val.String()), &preview); err != nil {
		return nil, err
	}
	return &preview, nil
}

func latestDeployedConfigVersion(ctx context.Context) (string, error) {
	val, err := g.DB().Model("kumo_config_versions").Ctx(ctx).
		Where("status", configVersionStatusDeployed).
		OrderDesc("deployed_at").
		Value("version")
	if err != nil || val == nil {
		return "", err
	}
	return val.String(), nil
}

func sortPolicyState(state *policyState) {
	sort.Slice(state.Tenants, func(i, j int) bool { return state.Tenants[i].ID < state.Tenants[j].ID })
	sort.Slice(state.Pools, func(i, j int) bool { return state.Pools[i].Name < state.Pools[j].Name })
	sort.Slice(state.Nodes, func(i, j int) bool { return state.Nodes[i].Name < state.Nodes[j].Name })
	sort.Slice(state.Sources, func(i, j int) bool { return state.Sources[i].Name < state.Sources[j].Name })
	sort.Slice(state.PoolSources, func(i, j int) bool {
		if state.PoolSources[i].PoolName == state.PoolSources[j].PoolName {
			return state.PoolSources[i].Source < state.PoolSources[j].Source
		}
		return state.PoolSources[i].PoolName < state.PoolSources[j].PoolName
	})
	sort.Slice(state.Profiles, func(i, j int) bool {
		if state.Profiles[i].TenantID == state.Profiles[j].TenantID {
			return state.Profiles[i].ID < state.Profiles[j].ID
		}
		return state.Profiles[i].TenantID < state.Profiles[j].TenantID
	})
	for i := range state.Profiles {
		sort.Strings(state.Profiles[i].Domains)
	}
}

func newPolicyFile(path, content string) PolicyFile {
	sum := sha256.Sum256([]byte(content))
	return PolicyFile{Path: path, Content: content, SHA256: hex.EncodeToString(sum[:])}
}

func policyVersion(files []PolicyFile, warnings, validationErrors []string) string {
	var b strings.Builder
	for _, file := range files {
		b.WriteString(file.Path)
		b.WriteString("\n")
		b.WriteString(file.SHA256)
		b.WriteString("\n")
	}
	for _, warning := range warnings {
		b.WriteString("warning:")
		b.WriteString(warning)
		b.WriteString("\n")
	}
	for _, validationError := range validationErrors {
		b.WriteString("error:")
		b.WriteString(validationError)
		b.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "policy-" + hex.EncodeToString(sum[:])[:16]
}

func quotedList(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%q", value))
	}
	return strings.Join(parts, ", ")
}

func luaBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func boolToSmallInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

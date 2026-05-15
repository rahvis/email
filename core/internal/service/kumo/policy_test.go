package kumo

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicyPreviewNoTenantsIsDeterministic(t *testing.T) {
	state := policyState{Config: &PublicConfig{Enabled: true, HasWebhookSecret: true}}

	first := buildConfigPreviewFromState(state, 7)
	second := buildConfigPreviewFromState(state, 7)

	require.Equal(t, first.Version, second.Version)
	require.Equal(t, first.Files, second.Files)
	require.Empty(t, first.ValidationErrors)
	require.Len(t, first.Files, 5)
}

func TestPolicyPreviewSharedPoolTenant(t *testing.T) {
	state := policyState{
		Config:         &PublicConfig{Enabled: true, HasWebhookSecret: true},
		WebhookBaseURL: "https://mail.example.com",
		Tenants: []policyTenant{
			{ID: 42, Name: "Acme", Slug: "acme", Status: "active", DefaultKumoPool: "shared-a"},
		},
		Pools: []policyPool{{ID: 1, Name: "shared-a", Status: "active"}},
		Profiles: []policyProfile{
			{ID: 7, TenantID: 42, Name: "Marketing", KumoPoolName: "shared-a", EgressMode: "external_kumoproxy", DKIMSelector: "bm1", Status: "ready", Domains: []string{"example.com"}},
		},
	}

	preview := buildConfigPreviewFromState(state, 1)

	require.Empty(t, preview.ValidationErrors)
	tenantPools := findPolicyFile(t, preview, "policy/tenant_pools.lua")
	require.Contains(t, tenantPools.Content, "tenant_id = 42")
	require.Contains(t, tenantPools.Content, "pool = \"shared-a\"")
	dkim := findPolicyFile(t, preview, "policy/dkim.lua")
	require.Contains(t, dkim.Content, "selector = \"bm1\"")
	require.NotContains(t, strings.ToLower(dkim.Content), "begin private key")
	webhooks := findPolicyFile(t, preview, "policy/webhooks.lua")
	require.Contains(t, webhooks.Content, "https://mail.example.com/api/kumo/events")
}

func TestPolicyPreviewWarnsWhenWebhookURLFallsBackToGitdate(t *testing.T) {
	state := policyState{Config: &PublicConfig{Enabled: true, HasWebhookSecret: true}}

	preview := buildConfigPreviewFromState(state, 1)

	webhooks := findPolicyFile(t, preview, "policy/webhooks.lua")
	require.Contains(t, webhooks.Content, "https://mail.gitdate.ink/api/kumo/events")
	require.Contains(t, preview.Warnings, "KumoMTA webhook URL fell back to https://mail.gitdate.ink/api/kumo/events because no reverse proxy base URL is configured")
}

func TestPolicyPreviewDedicatedPoolTenant(t *testing.T) {
	state := policyState{
		Config:      &PublicConfig{Enabled: true, HasWebhookSecret: true},
		Tenants:     []policyTenant{{ID: 77, Name: "Enterprise", Slug: "enterprise", Status: "active", DefaultKumoPool: "dedicated-17"}},
		Pools:       []policyPool{{ID: 17, Name: "dedicated-17", Status: "active"}},
		Nodes:       []policyNode{{ID: 1, Name: "kumo-node-1", BaseURL: "https://email.example.com", MetricsURL: "https://email.example.com/metrics", Status: "healthy"}},
		Sources:     []policySource{{ID: 31, Name: "source-31", SourceAddress: "203.0.113.31", EHLODomain: "mta.example.com", NodeID: 1, Status: "active"}},
		PoolSources: []policyPoolSource{{PoolID: 17, PoolName: "dedicated-17", SourceID: 31, Source: "source-31", Weight: 100, Status: "active"}},
		Profiles: []policyProfile{
			{ID: 11, TenantID: 77, Name: "Dedicated", KumoPoolName: "dedicated-17", EgressMode: "external_kumo_node", EgressProvider: "aws", DKIMSelector: "bm1", Status: "ready", Domains: []string{"brand.example"}},
		},
	}

	preview := buildConfigPreviewFromState(state, 1)

	require.Empty(t, preview.ValidationErrors)
	egress := findPolicyFile(t, preview, "policy/egress_sources.lua")
	require.Contains(t, egress.Content, "dedicated-17")
	require.Contains(t, egress.Content, "203.0.113.31")
	require.Contains(t, egress.Content, "mta.example.com")
}

func TestPolicyPreviewWarnsWhenDKIMSelectorMissing(t *testing.T) {
	state := policyState{
		Config: &PublicConfig{Enabled: true, HasWebhookSecret: true},
		Pools:  []policyPool{{ID: 1, Name: "shared-a", Status: "active"}},
		Profiles: []policyProfile{
			{ID: 7, TenantID: 42, Name: "Marketing", KumoPoolName: "shared-a", EgressMode: "external_kumoproxy", Status: "ready", Domains: []string{"example.com"}},
		},
	}

	preview := buildConfigPreviewFromState(state, 1)

	require.Empty(t, preview.ValidationErrors)
	require.Contains(t, preview.Warnings, "tenant_42 profile_7 has no DKIM selector")
}

func TestPolicyValidationBlocksDigitalOceanDirectToMX(t *testing.T) {
	state := policyState{
		Config: &PublicConfig{Enabled: true, HasWebhookSecret: true},
		Pools:  []policyPool{{ID: 1, Name: "shared-a", Status: "active"}},
		Profiles: []policyProfile{
			{ID: 7, TenantID: 42, Name: "Marketing", KumoPoolName: "shared-a", EgressMode: "direct_to_mx", EgressProvider: "digitalocean", DKIMSelector: "bm1", Status: "ready", Domains: []string{"example.com"}},
		},
	}

	preview := buildConfigPreviewFromState(state, 1)

	require.NotEmpty(t, preview.ValidationErrors)
	require.Contains(t, preview.ValidationErrors[0], "DigitalOcean direct-to-MX")
}

func findPolicyFile(t *testing.T, preview *ConfigPreview, path string) PolicyFile {
	t.Helper()
	for _, file := range preview.Files {
		if file.Path == path {
			return file
		}
	}
	require.Failf(t, "policy file not found", path)
	return PolicyFile{}
}

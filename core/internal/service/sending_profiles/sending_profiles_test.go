package sending_profiles

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeResolver struct {
	hosts map[string][]string
	txt   map[string][]string
	mx    map[string][]*net.MX
}

func (f fakeResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	if values, ok := f.hosts[host]; ok {
		return values, nil
	}
	return nil, errors.New("host not found")
}

func (f fakeResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if values, ok := f.txt[name]; ok {
		return values, nil
	}
	return nil, errors.New("txt not found")
}

func (f fakeResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	if values, ok := f.mx[name]; ok {
		return values, nil
	}
	return nil, errors.New("mx not found")
}

func (f fakeResolver) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	return nil, errors.New("ptr not configured")
}

func TestEvaluateQuotaAllowsUnlimitedAndBlocksOverLimit(t *testing.T) {
	require.True(t, EvaluateQuota(0, 100, 500).Allowed)

	allowed := EvaluateQuota(1000, 998, 2)
	require.True(t, allowed.Allowed)
	require.Equal(t, int64(1000), allowed.UsedAfter)

	blocked := EvaluateQuota(1000, 999, 2)
	require.False(t, blocked.Allowed)
	require.Contains(t, blocked.Reason, "quota exceeded")
}

func TestEvaluateAbuseBlocksOnlyExceededThresholds(t *testing.T) {
	require.False(t, EvaluateAbuse(1000, 99, 0, 100, 1).Blocked)

	bounceBlocked := EvaluateAbuse(1000, 100, 0, 100, 1)
	require.True(t, bounceBlocked.Blocked)
	require.Contains(t, bounceBlocked.Reason, "bounce")

	complaintBlocked := EvaluateAbuse(1000, 0, 1, 100, 1)
	require.True(t, complaintBlocked.Blocked)
	require.Contains(t, complaintBlocked.Reason, "complaint")
}

func TestDigitalOceanDirectToMXIsNotProductionReady(t *testing.T) {
	require.False(t, IsAllowedProductionEgress(EgressModeDirectToMX, EgressProviderDigitalOcean))
	require.True(t, IsAllowedProductionEgress(EgressModeExternalKumoProxy, EgressProviderDigitalOcean))
	require.True(t, IsAllowedProductionEgress(EgressModeProviderSMTP2525, "sendgrid"))
	require.True(t, IsAllowedProductionEgress(EgressModeProviderHTTPAPI, "mailgun"))
}

func TestGitdateDNSReadinessWithExpectedRecords(t *testing.T) {
	resolver := fakeResolver{
		hosts: map[string][]string{
			"mail.gitdate.ink":  {"159.89.33.85"},
			"email.gitdate.ink": {"192.241.130.241"},
		},
		txt: map[string][]string{
			"email.gitdate.ink":               {"v=spf1 ip4:192.241.130.241 ~all"},
			"s1._domainkey.email.gitdate.ink": {"v=DKIM1; k=rsa; p=MIIB..."},
			"_dmarc.email.gitdate.ink":        {"v=DMARC1; p=none; rua=mailto:info@gitdate.ink"},
		},
	}

	readiness := CheckGitdateDNS(context.Background(), resolver)
	require.True(t, readiness.Ready)
	require.Len(t, readiness.Checks, 5)
}

func TestDomainReadinessRequiresSPFDKIMDMARCAndAllowedEgress(t *testing.T) {
	resolver := fakeResolver{
		txt: map[string][]string{
			"example.com":                {"v=spf1 include:example.net -all"},
			"bm1._domainkey.example.com": {"v=DKIM1; k=rsa; p=MIIB..."},
			"_dmarc.example.com":         {"v=DMARC1; p=none"},
		},
		mx: map[string][]*net.MX{
			"example.com": []*net.MX{{Host: "mail.example.com.", Pref: 10}},
		},
	}

	ready := CheckDomainReadiness(context.Background(), DomainReadinessInput{
		Domain:       "example.com",
		DKIMSelector: "bm1",
		EgressMode:   EgressModeExternalKumoNode,
		Provider:     "aws",
	}, resolver)
	require.True(t, ready.Ready)

	blocked := CheckDomainReadiness(context.Background(), DomainReadinessInput{
		Domain:       "example.com",
		DKIMSelector: "bm1",
		EgressMode:   EgressModeDirectToMX,
		Provider:     EgressProviderDigitalOcean,
	}, resolver)
	require.False(t, blocked.Ready)
}

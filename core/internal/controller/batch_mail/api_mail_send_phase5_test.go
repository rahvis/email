package batch_mail

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPITenantHeaderAllowed(t *testing.T) {
	require.True(t, apiTenantHeaderAllowed(42, ""))
	require.True(t, apiTenantHeaderAllowed(42, "42"))
	require.False(t, apiTenantHeaderAllowed(42, "88"))
	require.False(t, apiTenantHeaderAllowed(42, "not-a-number"))
}

func TestAPIGroupTenantAllowed(t *testing.T) {
	require.True(t, apiGroupTenantAllowed(42, 42, 7))
	require.False(t, apiGroupTenantAllowed(42, 88, 7))
	require.False(t, apiGroupTenantAllowed(0, 42, 7))
	require.False(t, apiGroupTenantAllowed(42, 42, 0))
}

func TestValidateAPISenderRejectsDifferentDomain(t *testing.T) {
	require.True(t, apiSenderDomainAllowed("support@example.com", "alerts@example.com"))
	require.False(t, apiSenderDomainAllowed("support@example.com", "alerts@other.example"))
}

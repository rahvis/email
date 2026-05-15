package tenants

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSlugify(t *testing.T) {
	assert.Equal(t, "acme-marketing", Slugify("Acme Marketing"))
	assert.Equal(t, "northwind-labs", Slugify(" Northwind Labs! "))
	assert.Equal(t, "workspace", Slugify("!!!"))
}

func TestRolePermissions(t *testing.T) {
	assert.Contains(t, PermissionsForRole(RoleOwner), "billing:write")
	assert.Contains(t, PermissionsForRole(RoleAdmin), "domains:write")
	assert.Contains(t, PermissionsForRole(RoleMarketer), "campaigns:write")
	assert.Contains(t, PermissionsForRole(RoleDeveloper), "api_keys:write")
	assert.Contains(t, PermissionsForRole(RoleOperator), "platform:write")
	assert.Contains(t, PermissionsForRole("unknown"), "analytics:read")
}

func TestRoleFromRBAC(t *testing.T) {
	assert.Equal(t, RoleOwner, RoleFromRBAC([]string{"admin"}))
	assert.Equal(t, RoleOwner, RoleFromRBAC([]string{"operator"}))
	assert.Equal(t, RoleAdmin, RoleFromRBAC([]string{"user"}))
}

func TestCanAccessTenantID(t *testing.T) {
	current := &Context{TenantID: 42}
	assert.True(t, CanAccessTenantID(current, 42))
	assert.False(t, CanAccessTenantID(current, 88))
	assert.False(t, CanAccessTenantID(nil, 42))
}

func TestRequireTenantIDWithoutRequest(t *testing.T) {
	tenantID, err := RequireTenantID(context.Background())
	assert.Zero(t, tenantID)
	assert.Error(t, err)
}

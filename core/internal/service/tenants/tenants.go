package tenants

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/gconv"
)

const (
	StatusActive       = "active"
	StatusPendingSetup = "pending_setup"

	RoleOwner     = "owner"
	RoleAdmin     = "admin"
	RoleMarketer  = "marketer"
	RoleDeveloper = "developer"
	RoleOperator  = "operator"
)

type Context struct {
	TenantID             int64    `json:"tenant_id"`
	TenantName           string   `json:"tenant_name"`
	TenantSlug           string   `json:"tenant_slug"`
	Role                 string   `json:"role"`
	Permissions          []string `json:"permissions"`
	Plan                 string   `json:"plan"`
	DailyQuota           int64    `json:"daily_quota"`
	DailyUsed            int64    `json:"daily_used"`
	Status               string   `json:"status"`
	IsOperator           bool     `json:"is_operator"`
	SendingStatus        string   `json:"sending_status"`
	SendingBlockReason   string   `json:"sending_block_reason"`
	SendingThrottleUntil int64    `json:"sending_throttle_until"`
}

type Membership struct {
	TenantID int64  `json:"tenant_id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	Plan     string `json:"plan"`
}

type Tenant struct {
	ID              int64  `json:"tenant_id"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Status          string `json:"status"`
	PlanID          int64  `json:"plan_id"`
	DailyQuota      int64  `json:"daily_quota"`
	MonthlyQuota    int64  `json:"monthly_quota"`
	DefaultKumoPool string `json:"default_kumo_pool"`
	CreateTime      int64  `json:"create_time"`
	UpdateTime      int64  `json:"update_time"`
}

type CreateInput struct {
	Name            string
	OwnerEmail      string
	PlanID          int64
	DailyQuota      int64
	MonthlyQuota    int64
	DefaultKumoPool string
	Status          string
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = slugRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "workspace"
	}
	return slug
}

func PermissionsForRole(role string) []string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case RoleOwner:
		return []string{
			"billing:write", "members:write", "domains:write", "sending_profiles:write",
			"campaigns:write", "api_keys:write", "analytics:read", "suppression:write",
		}
	case RoleAdmin:
		return []string{
			"members:write", "domains:write", "sending_profiles:write",
			"campaigns:write", "api_keys:write", "analytics:read", "suppression:write",
		}
	case RoleMarketer:
		return []string{"contacts:write", "templates:write", "campaigns:write", "analytics:read"}
	case RoleDeveloper:
		return []string{"api_keys:write", "api_templates:write", "api_logs:read", "delivery_events:read"}
	case RoleOperator:
		return []string{"platform:write", "tenants:write", "kumo:write", "abuse:write", "analytics:read"}
	default:
		return []string{"analytics:read"}
	}
}

func RoleFromRBAC(roleNames []string) string {
	for _, role := range roleNames {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "admin", RoleOperator:
			return RoleOwner
		}
	}
	return RoleAdmin
}

func IsOperatorRoleNames(roleNames []string) bool {
	for _, role := range roleNames {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "admin", RoleOperator:
			return true
		}
	}
	return false
}

func CanAccessTenantID(ctx *Context, recordTenantID int64) bool {
	return ctx != nil && ctx.TenantID > 0 && ctx.TenantID == recordTenantID
}

func Current(ctx context.Context) *Context {
	req := g.RequestFromCtx(ctx)
	if req == nil {
		return nil
	}
	tenantID := req.GetCtxVar("tenant_id").Int64()
	if tenantID <= 0 {
		return nil
	}
	return &Context{
		TenantID:             tenantID,
		TenantName:           req.GetCtxVar("tenant_name").String(),
		TenantSlug:           req.GetCtxVar("tenant_slug").String(),
		Role:                 req.GetCtxVar("tenant_role").String(),
		Permissions:          gconv.Strings(req.GetCtxVar("tenant_permissions").Val()),
		Plan:                 req.GetCtxVar("tenant_plan").String(),
		DailyQuota:           req.GetCtxVar("tenant_daily_quota").Int64(),
		DailyUsed:            req.GetCtxVar("tenant_daily_used").Int64(),
		Status:               req.GetCtxVar("tenant_status").String(),
		IsOperator:           req.GetCtxVar("is_operator").Bool(),
		SendingStatus:        req.GetCtxVar("tenant_sending_status").String(),
		SendingBlockReason:   req.GetCtxVar("tenant_sending_block_reason").String(),
		SendingThrottleUntil: req.GetCtxVar("tenant_sending_throttle_until").Int64(),
	}
}

func CurrentTenantID(ctx context.Context) int64 {
	if current := Current(ctx); current != nil {
		return current.TenantID
	}
	return 0
}

func RequireTenantID(ctx context.Context) (int64, error) {
	tenantID := CurrentTenantID(ctx)
	if tenantID <= 0 {
		return 0, gerror.New("tenant context is required")
	}
	return tenantID, nil
}

func ScopeModel(ctx context.Context, model *gdb.Model, column string) *gdb.Model {
	if column == "" {
		column = "tenant_id"
	}
	if tenantID := CurrentTenantID(ctx); tenantID > 0 {
		return model.Where(column, tenantID)
	}
	return model
}

func SetRequestContext(r *ghttp.Request, current *Context) {
	if r == nil || current == nil {
		return
	}
	r.SetCtxVar("tenant_id", current.TenantID)
	r.SetCtxVar("tenant_name", current.TenantName)
	r.SetCtxVar("tenant_slug", current.TenantSlug)
	r.SetCtxVar("tenant_role", current.Role)
	r.SetCtxVar("tenant_permissions", current.Permissions)
	r.SetCtxVar("tenant_plan", current.Plan)
	r.SetCtxVar("tenant_daily_quota", current.DailyQuota)
	r.SetCtxVar("tenant_daily_used", current.DailyUsed)
	r.SetCtxVar("tenant_status", current.Status)
	r.SetCtxVar("is_operator", current.IsOperator)
	r.SetCtxVar("tenant_sending_status", current.SendingStatus)
	r.SetCtxVar("tenant_sending_block_reason", current.SendingBlockReason)
	r.SetCtxVar("tenant_sending_throttle_until", current.SendingThrottleUntil)
}

func ListForAccount(ctx context.Context, accountID int64) ([]Membership, error) {
	memberships := make([]Membership, 0)
	err := g.DB().Model("tenant_members tm").
		Ctx(ctx).
		LeftJoin("tenants t", "t.id = tm.tenant_id").
		LeftJoin("tenant_plans p", "p.id = t.plan_id").
		Fields("tm.tenant_id, t.name, t.slug, tm.role, t.status, COALESCE(p.name, '') as plan").
		Where("tm.account_id", accountID).
		Where("tm.status", StatusActive).
		Order("tm.id ASC").
		Scan(&memberships)
	return memberships, err
}

func ResolveForAccount(ctx context.Context, accountID int64, requestedTenantID int64, roleNames []string) (*Context, error) {
	if accountID <= 0 {
		return nil, gerror.New("account is required")
	}
	memberships, err := ListForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		defaultTenantID, ensureErr := EnsureDefaultTenant(ctx)
		if ensureErr != nil {
			return nil, ensureErr
		}
		if _, ensureErr = g.DB().Model("tenant_members").Ctx(ctx).InsertIgnore(g.Map{
			"tenant_id":   defaultTenantID,
			"account_id":  accountID,
			"role":        RoleFromRBAC(roleNames),
			"status":      StatusActive,
			"create_time": time.Now().Unix(),
			"update_time": time.Now().Unix(),
		}); ensureErr != nil {
			return nil, ensureErr
		}
		memberships, err = ListForAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
	}

	selected := Membership{}
	if requestedTenantID > 0 {
		for _, membership := range memberships {
			if membership.TenantID == requestedTenantID {
				selected = membership
				break
			}
		}
		if selected.TenantID == 0 {
			return nil, gerror.New("tenant membership not found")
		}
	} else if len(memberships) > 0 {
		selected = memberships[0]
	}
	if selected.TenantID == 0 {
		return nil, gerror.New("tenant context is required")
	}

	var row struct {
		TenantID             int64  `json:"tenant_id"`
		Name                 string `json:"name"`
		Slug                 string `json:"slug"`
		Status               string `json:"status"`
		Role                 string `json:"role"`
		Plan                 string `json:"plan"`
		DailyQuota           int64  `json:"daily_quota"`
		SendingStatus        string `json:"sending_status"`
		SendingBlockReason   string `json:"sending_block_reason"`
		SendingThrottleUntil int64  `json:"sending_throttle_until"`
	}
	err = g.DB().Model("tenants t").
		Ctx(ctx).
		LeftJoin("tenant_members tm", "tm.tenant_id = t.id").
		LeftJoin("tenant_plans p", "p.id = t.plan_id").
		Fields("t.id as tenant_id, t.name, t.slug, t.status, tm.role, COALESCE(p.name, '') as plan, COALESCE(t.daily_quota, 0) as daily_quota, COALESCE(t.sending_status, 'active') as sending_status, COALESCE(t.sending_block_reason, '') as sending_block_reason, COALESCE(t.sending_throttle_until, 0) as sending_throttle_until").
		Where("t.id", selected.TenantID).
		Where("tm.account_id", accountID).
		Where("tm.status", StatusActive).
		Scan(&row)
	if err != nil {
		return nil, err
	}
	if row.TenantID == 0 {
		return nil, gerror.New("tenant membership not found")
	}

	dailyUsed := dailyUsage(ctx, row.TenantID)
	return &Context{
		TenantID:             row.TenantID,
		TenantName:           row.Name,
		TenantSlug:           row.Slug,
		Role:                 row.Role,
		Permissions:          PermissionsForRole(row.Role),
		Plan:                 row.Plan,
		DailyQuota:           row.DailyQuota,
		DailyUsed:            dailyUsed,
		Status:               row.Status,
		IsOperator:           IsOperatorRoleNames(roleNames),
		SendingStatus:        row.SendingStatus,
		SendingBlockReason:   row.SendingBlockReason,
		SendingThrottleUntil: row.SendingThrottleUntil,
	}, nil
}

func dailyUsage(ctx context.Context, tenantID int64) int64 {
	var row struct {
		QueuedCount int64 `json:"queued_count"`
	}
	today := time.Now().Format("2006-01-02")
	_ = g.DB().Model("tenant_usage_daily").
		Ctx(ctx).
		Fields("queued_count").
		Where("tenant_id", tenantID).
		Where("date", today).
		Scan(&row)
	return row.QueuedCount
}

func EnsureDefaultTenant(ctx context.Context) (int64, error) {
	now := time.Now().Unix()
	var tenantID int64
	value, err := g.DB().Model("tenants").Ctx(ctx).Where("slug", "default-workspace").Value("id")
	if err == nil && value != nil {
		return value.Int64(), nil
	}
	planID, err := ensureDefaultPlan(ctx)
	if err != nil {
		return 0, err
	}
	result, err := g.DB().Model("tenants").Ctx(ctx).InsertIgnore(g.Map{
		"name":          "Default Workspace",
		"slug":          "default-workspace",
		"status":        StatusActive,
		"plan_id":       planID,
		"daily_quota":   0,
		"monthly_quota": 0,
		"create_time":   now,
		"update_time":   now,
	})
	if err != nil {
		return 0, err
	}
	tenantID, _ = result.LastInsertId()
	if tenantID > 0 {
		return tenantID, nil
	}
	value, err = g.DB().Model("tenants").Ctx(ctx).Where("slug", "default-workspace").Value("id")
	if err != nil || value == nil {
		return 0, fmt.Errorf("failed to create default tenant: %w", err)
	}
	return value.Int64(), nil
}

func ensureDefaultPlan(ctx context.Context) (int64, error) {
	value, err := g.DB().Model("tenant_plans").Ctx(ctx).Where("name", "Default").Value("id")
	if err == nil && value != nil {
		return value.Int64(), nil
	}
	now := time.Now().Unix()
	result, err := g.DB().Model("tenant_plans").Ctx(ctx).InsertIgnore(g.Map{
		"name":                 "Default",
		"daily_send_limit":     0,
		"monthly_send_limit":   0,
		"max_domains":          0,
		"max_contacts":         0,
		"dedicated_ip_allowed": 0,
		"create_time":          now,
		"update_time":          now,
	})
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()
	if id > 0 {
		return id, nil
	}
	value, err = g.DB().Model("tenant_plans").Ctx(ctx).Where("name", "Default").Value("id")
	if err != nil || value == nil {
		return 0, fmt.Errorf("failed to create default tenant plan: %w", err)
	}
	return value.Int64(), nil
}

func Create(ctx context.Context, input CreateInput) (*Tenant, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, gerror.New("tenant name is required")
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = StatusPendingSetup
	}
	slug := Slugify(name)
	baseSlug := slug
	for i := 2; ; i++ {
		exists, err := g.DB().Model("tenants").Ctx(ctx).Where("slug", slug).Count()
		if err != nil {
			return nil, err
		}
		if exists == 0 {
			break
		}
		slug = fmt.Sprintf("%s-%d", baseSlug, i)
	}
	now := time.Now().Unix()
	result, err := g.DB().Model("tenants").Ctx(ctx).Insert(g.Map{
		"name":              name,
		"slug":              slug,
		"status":            status,
		"plan_id":           input.PlanID,
		"daily_quota":       input.DailyQuota,
		"monthly_quota":     input.MonthlyQuota,
		"default_kumo_pool": input.DefaultKumoPool,
		"create_time":       now,
		"update_time":       now,
	})
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	if ownerEmail := strings.TrimSpace(input.OwnerEmail); ownerEmail != "" {
		var account struct {
			AccountId int64 `json:"account_id"`
		}
		_ = g.DB().Model("account").Ctx(ctx).Fields("account_id").Where("email", ownerEmail).Scan(&account)
		if account.AccountId > 0 {
			_, _ = g.DB().Model("tenant_members").Ctx(ctx).InsertIgnore(g.Map{
				"tenant_id":   id,
				"account_id":  account.AccountId,
				"role":        RoleOwner,
				"status":      StatusActive,
				"create_time": now,
				"update_time": now,
			})
		}
	}
	return &Tenant{
		ID:              id,
		Name:            name,
		Slug:            slug,
		Status:          status,
		PlanID:          input.PlanID,
		DailyQuota:      input.DailyQuota,
		MonthlyQuota:    input.MonthlyQuota,
		DefaultKumoPool: input.DefaultKumoPool,
		CreateTime:      now,
		UpdateTime:      now,
	}, nil
}

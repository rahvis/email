package tenants

import (
	"billionmail-core/api/tenants/v1"
	"billionmail-core/internal/service/rbac"
	"billionmail-core/internal/service/sending_profiles"
	service_tenants "billionmail-core/internal/service/tenants"
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) Current(ctx context.Context, req *v1.CurrentReq) (res *v1.CurrentRes, err error) {
	res = &v1.CurrentRes{}
	current := service_tenants.Current(ctx)
	if current == nil {
		return nil, gerror.New("tenant context is required")
	}
	res.Data = toAPIContext(current)
	res.SetSuccess("Get current tenant successfully")
	return res, nil
}

func (c *ControllerV1) List(ctx context.Context, req *v1.ListReq) (res *v1.ListRes, err error) {
	res = &v1.ListRes{}
	accountID := currentAccountID(ctx)
	list, err := service_tenants.ListForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	res.Data.Tenants = make([]v1.TenantMembership, 0, len(list))
	for _, item := range list {
		res.Data.Tenants = append(res.Data.Tenants, v1.TenantMembership{
			TenantID: item.TenantID,
			Name:     item.Name,
			Slug:     item.Slug,
			Role:     item.Role,
			Status:   item.Status,
			Plan:     item.Plan,
		})
	}
	res.SetSuccess("Get tenants successfully")
	return res, nil
}

func (c *ControllerV1) Switch(ctx context.Context, req *v1.SwitchReq) (res *v1.SwitchRes, err error) {
	res = &v1.SwitchRes{}
	accountID := currentAccountID(ctx)
	roleNames := rbac.GetCurrentRoles(ctx)
	current, err := service_tenants.ResolveForAccount(ctx, accountID, req.TenantID, roleNames)
	if err != nil {
		return nil, err
	}
	if err = g.RequestFromCtx(ctx).Session.Set("active_tenant_id", req.TenantID); err != nil {
		return nil, err
	}
	service_tenants.SetRequestContext(g.RequestFromCtx(ctx), current)
	res.Data = toAPIContext(current)
	res.SetSuccess("Switch tenant successfully")
	return res, nil
}

func (c *ControllerV1) Create(ctx context.Context, req *v1.CreateReq) (res *v1.CreateRes, err error) {
	res = &v1.CreateRes{}
	current := service_tenants.Current(ctx)
	if current == nil || !current.IsOperator {
		return nil, gerror.New("operator access is required")
	}
	tenant, err := service_tenants.Create(ctx, service_tenants.CreateInput{
		Name:            req.Name,
		OwnerEmail:      req.OwnerEmail,
		PlanID:          req.PlanID,
		DailyQuota:      req.DailyQuota,
		MonthlyQuota:    req.MonthlyQuota,
		DefaultKumoPool: req.DefaultKumoPool,
		Status:          req.Status,
	})
	if err != nil {
		return nil, err
	}
	res.Data = v1.Tenant{
		TenantID:        tenant.ID,
		Name:            tenant.Name,
		Slug:            tenant.Slug,
		Status:          tenant.Status,
		PlanID:          tenant.PlanID,
		DailyQuota:      tenant.DailyQuota,
		MonthlyQuota:    tenant.MonthlyQuota,
		DefaultKumoPool: tenant.DefaultKumoPool,
		CreateTime:      tenant.CreateTime,
		UpdateTime:      tenant.UpdateTime,
	}
	res.SetSuccess("Create tenant successfully")
	return res, nil
}

func (c *ControllerV1) ListSendingProfiles(ctx context.Context, req *v1.ListSendingProfilesReq) (res *v1.ListSendingProfilesRes, err error) {
	res = &v1.ListSendingProfilesRes{}
	current := service_tenants.Current(ctx)
	if !canManageTenantSending(current, req.ID) {
		return nil, gerror.New("tenant sending profile access is required")
	}
	profiles, err := sending_profiles.List(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	res.Data.Profiles = make([]v1.SendingProfile, 0, len(profiles))
	for _, profile := range profiles {
		res.Data.Profiles = append(res.Data.Profiles, toAPISendingProfile(profile))
	}
	res.SetSuccess("Get sending profiles successfully")
	return res, nil
}

func (c *ControllerV1) SendingProfile(ctx context.Context, req *v1.SendingProfileReq) (res *v1.SendingProfileRes, err error) {
	res = &v1.SendingProfileRes{}
	current := service_tenants.Current(ctx)
	if !canManageTenantSending(current, req.ID) {
		return nil, gerror.New("tenant sending profile access is required")
	}
	result, err := sending_profiles.Upsert(ctx, sending_profiles.UpsertInput{
		ProfileID:                  req.ProfileID,
		TenantID:                   req.ID,
		Name:                       req.Name,
		SenderDomains:              req.SenderDomains,
		DefaultFromDomain:          req.DefaultFromDomain,
		KumoPoolID:                 req.KumoPoolID,
		KumoPoolName:               req.KumoPoolName,
		EgressMode:                 req.EgressMode,
		EgressProvider:             req.EgressProvider,
		DKIMSelector:               req.DKIMSelector,
		DailyQuota:                 req.DailyQuota,
		HourlyQuota:                req.HourlyQuota,
		WarmupEnabled:              req.WarmupEnabled,
		Status:                     req.Status,
		BounceThresholdPerMille:    req.BounceThresholdPerMille,
		ComplaintThresholdPerMille: req.ComplaintThresholdPerMille,
	})
	if err != nil {
		return nil, err
	}
	res.Data.Profile = toAPISendingProfile(result.Profile)
	res.Data.Readiness = toAPIProfileReadiness(result.Readiness)
	res.SetSuccess("Save sending profile successfully")
	return res, nil
}

func (c *ControllerV1) SendingControl(ctx context.Context, req *v1.SendingControlReq) (res *v1.SendingControlRes, err error) {
	res = &v1.SendingControlRes{}
	current := service_tenants.Current(ctx)
	if current == nil || !current.IsOperator {
		return nil, gerror.New("operator access is required")
	}
	input := sending_profiles.ControlInput{
		TenantID:           req.ID,
		ProfileID:          req.ProfileID,
		Status:             req.Status,
		Reason:             req.Reason,
		ThrottleUntil:      req.ThrottleUntil,
		OperatorKillSwitch: req.OperatorKillSwitch,
	}
	if req.ProfileID > 0 {
		err = sending_profiles.SetProfileControl(ctx, input)
	} else {
		err = sending_profiles.SetTenantControl(ctx, input)
	}
	if err != nil {
		return nil, err
	}
	res.SetSuccess("Update sending control successfully")
	return res, nil
}

func currentAccountID(ctx context.Context) int64 {
	req := g.RequestFromCtx(ctx)
	if req == nil {
		return 0
	}
	return req.GetCtxVar("accountId").Int64()
}

func toAPIContext(current *service_tenants.Context) v1.TenantContext {
	return v1.TenantContext{
		TenantID:             current.TenantID,
		TenantName:           current.TenantName,
		TenantSlug:           current.TenantSlug,
		Role:                 current.Role,
		Permissions:          current.Permissions,
		Plan:                 current.Plan,
		DailyQuota:           current.DailyQuota,
		DailyUsed:            current.DailyUsed,
		Status:               current.Status,
		IsOperator:           current.IsOperator,
		SendingStatus:        current.SendingStatus,
		SendingBlockReason:   current.SendingBlockReason,
		SendingThrottleUntil: current.SendingThrottleUntil,
	}
}

func canManageTenantSending(current *service_tenants.Context, tenantID int64) bool {
	if current == nil {
		return false
	}
	if current.IsOperator {
		return true
	}
	if current.TenantID != tenantID {
		return false
	}
	switch strings.ToLower(current.Role) {
	case service_tenants.RoleOwner, service_tenants.RoleAdmin:
		return true
	default:
		return false
	}
}

func toAPISendingProfile(profile sending_profiles.Profile) v1.SendingProfile {
	return v1.SendingProfile{
		ID:                         profile.ID,
		TenantID:                   profile.TenantID,
		Name:                       profile.Name,
		DefaultFromDomain:          profile.DefaultFromDomain,
		KumoPoolID:                 profile.KumoPoolID,
		KumoPoolName:               profile.KumoPoolName,
		EgressMode:                 profile.EgressMode,
		EgressProvider:             profile.EgressProvider,
		DKIMSelector:               profile.DKIMSelector,
		DailyQuota:                 profile.DailyQuota,
		HourlyQuota:                profile.HourlyQuota,
		WarmupEnabled:              profile.WarmupEnabled,
		Status:                     profile.Status,
		PausedReason:               profile.PausedReason,
		ThrottleUntil:              profile.ThrottleUntil,
		OperatorKillSwitch:         profile.OperatorKillSwitch,
		BounceThresholdPerMille:    profile.BounceThresholdPerMille,
		ComplaintThresholdPerMille: profile.ComplaintThresholdPerMille,
		Domains:                    profile.Domains,
		CreateTime:                 profile.CreateTime,
		UpdateTime:                 profile.UpdateTime,
	}
}

func toAPIProfileReadiness(readiness sending_profiles.ProfileReadiness) v1.ProfileReadiness {
	out := v1.ProfileReadiness{Ready: readiness.Ready}
	out.Checks = make([]v1.ReadinessCheck, 0, len(readiness.Checks))
	for _, check := range readiness.Checks {
		out.Checks = append(out.Checks, v1.ReadinessCheck{Name: check.Name, Ready: check.Ready, Message: check.Message})
	}
	out.Domains = make([]v1.DomainReadiness, 0, len(readiness.Domains))
	for _, domain := range readiness.Domains {
		item := v1.DomainReadiness{Domain: domain.Domain, Ready: domain.Ready, Checks: make([]v1.ReadinessCheck, 0, len(domain.Checks))}
		for _, check := range domain.Checks {
			item.Checks = append(item.Checks, v1.ReadinessCheck{Name: check.Name, Ready: check.Ready, Message: check.Message})
		}
		out.Domains = append(out.Domains, item)
	}
	return out
}

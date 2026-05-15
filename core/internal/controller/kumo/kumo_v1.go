package kumo

import (
	"context"

	v1 "billionmail-core/api/kumo/v1"
	kumoService "billionmail-core/internal/service/kumo"
	"billionmail-core/internal/service/public"
	serviceTenants "billionmail-core/internal/service/tenants"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) GetConfig(ctx context.Context, req *v1.GetConfigReq) (res *v1.GetConfigRes, err error) {
	res = &v1.GetConfigRes{}
	if err := requireKumoOperator(ctx); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Operator access is required")))
		return res, nil
	}
	config, err := kumoService.GetPublicConfig(ctx)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to load KumoMTA configuration: {}", err.Error())))
		return res, nil
	}
	res.Data = config
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return res, nil
}

func (c *ControllerV1) SaveConfig(ctx context.Context, req *v1.SaveConfigReq) (res *v1.SaveConfigRes, err error) {
	res = &v1.SaveConfigRes{}
	if err := requireKumoOperator(ctx); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Operator access is required")))
		return res, nil
	}
	config, err := kumoService.UpdateConfig(ctx, kumoService.UpdateConfigInput{
		Enabled:          req.Enabled,
		CampaignsEnabled: req.CampaignsEnabled,
		APIEnabled:       req.APIEnabled,
		BaseURL:          req.BaseURL,
		InjectPath:       req.InjectPath,
		MetricsURL:       req.MetricsURL,
		TLSVerify:        req.TLSVerify,
		AuthMode:         req.AuthMode,
		AuthSecret:       req.AuthSecret,
		WebhookSecret:    req.WebhookSecret,
		TimeoutMS:        req.TimeoutMS,
		DefaultPool:      req.DefaultPool,
	})
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to save KumoMTA configuration: {}", err.Error())))
		return res, nil
	}
	_ = public.WriteLog(ctx, public.LogParams{
		Type: "kumo",
		Log:  "KumoMTA configuration updated",
		Data: config,
	})
	res.Data = config
	res.SetSuccess(public.LangCtx(ctx, "KumoMTA configuration saved"))
	return res, nil
}

func (c *ControllerV1) TestConnection(ctx context.Context, req *v1.TestConnectionReq) (res *v1.TestConnectionRes, err error) {
	res = &v1.TestConnectionRes{}
	if err := requireKumoOperator(ctx); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Operator access is required")))
		return res, nil
	}
	result, err := kumoService.TestConnection(ctx, kumoService.TestConnectionInput{
		BaseURL:    req.BaseURL,
		InjectPath: req.InjectPath,
		MetricsURL: req.MetricsURL,
		AuthMode:   req.AuthMode,
		AuthSecret: req.AuthSecret,
		TLSVerify:  req.TLSVerify,
		TimeoutMS:  req.TimeoutMS,
	})
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "KumoMTA connection test failed: {}", err.Error())))
		return res, nil
	}
	res.Data = result
	if result.OK {
		res.SetSuccess(public.LangCtx(ctx, "KumoMTA reachable"))
	} else {
		res.SetSuccess(result.Message)
	}
	return res, nil
}

func (c *ControllerV1) GetStatus(ctx context.Context, req *v1.GetStatusReq) (res *v1.GetStatusRes, err error) {
	res = &v1.GetStatusRes{}
	if err := requireKumoOperator(ctx); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Operator access is required")))
		return res, nil
	}
	res.Data = kumoService.GetStatus()
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return res, nil
}

func (c *ControllerV1) GetMetrics(ctx context.Context, req *v1.GetMetricsReq) (res *v1.GetMetricsRes, err error) {
	res = &v1.GetMetricsRes{}
	if err := requireKumoOperator(ctx); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Operator access is required")))
		return res, nil
	}
	res.Data = kumoService.GetMetricsSnapshot()
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return res, nil
}

func (c *ControllerV1) GetRuntime(ctx context.Context, req *v1.GetRuntimeReq) (res *v1.GetRuntimeRes, err error) {
	res = &v1.GetRuntimeRes{}
	current := serviceTenants.Current(ctx)
	operatorView := current != nil && current.IsOperator
	tenantID := int64(0)
	if !operatorView {
		if current == nil || current.TenantID <= 0 {
			res.SetError(gerror.New(public.LangCtx(ctx, "Tenant context is required")))
			return res, nil
		}
		tenantID = current.TenantID
	}
	snapshot, err := kumoService.GetRuntimeSnapshot(ctx, tenantID, operatorView)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to load KumoMTA runtime snapshot: {}", err.Error())))
		return res, nil
	}
	res.Data = snapshot
	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return res, nil
}

func (c *ControllerV1) Events(ctx context.Context, req *v1.EventsReq) (res *v1.EventsRes, err error) {
	res = &v1.EventsRes{}
	httpReq := g.RequestFromCtx(ctx)
	if httpReq == nil {
		res.SetError(gerror.New("request context unavailable"))
		return res, nil
	}

	result, err := kumoService.IngestWebhook(ctx, httpReq)
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "KumoMTA webhook rejected: {}", err.Error())))
		return res, nil
	}
	res.Data = result
	res.SetSuccess(public.LangCtx(ctx, "KumoMTA webhook accepted"))
	return res, nil
}

func (c *ControllerV1) PreviewConfig(ctx context.Context, req *v1.PreviewConfigReq) (res *v1.PreviewConfigRes, err error) {
	res = &v1.PreviewConfigRes{}
	if err := requireKumoOperator(ctx); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Operator access is required")))
		return res, nil
	}
	preview, err := kumoService.GenerateConfigPreview(ctx, public.GetCurrentAccountId(ctx))
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to generate KumoMTA policy preview: {}", err.Error())))
		return res, nil
	}
	_ = public.WriteLog(ctx, public.LogParams{
		Type: "kumo",
		Log:  "KumoMTA policy preview generated",
		Data: g.Map{
			"version":           preview.Version,
			"files":             len(preview.Files),
			"warnings":          preview.Warnings,
			"validation_errors": preview.ValidationErrors,
		},
	})
	res.Data = preview
	res.SetSuccess(public.LangCtx(ctx, "KumoMTA policy preview generated"))
	return res, nil
}

func (c *ControllerV1) DeployConfig(ctx context.Context, req *v1.DeployConfigReq) (res *v1.DeployConfigRes, err error) {
	res = &v1.DeployConfigRes{}
	if err := requireKumoOperator(ctx); err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Operator access is required")))
		return res, nil
	}
	result, err := kumoService.DeployConfig(ctx, kumoService.DeployConfigInput{
		Version: req.Version,
		DryRun:  req.DryRun,
	}, public.GetCurrentAccountId(ctx))
	_ = public.WriteLog(ctx, public.LogParams{
		Type: "kumo",
		Log:  "KumoMTA policy deploy attempt",
		Data: g.Map{
			"version":   req.Version,
			"dry_run":   req.DryRun,
			"result":    result,
			"has_error": err != nil,
		},
	})
	res.Data = result
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "KumoMTA policy deploy blocked: {}", err.Error())))
		return res, nil
	}
	res.SetSuccess(public.LangCtx(ctx, "KumoMTA policy dry run completed"))
	return res, nil
}

func requireKumoOperator(ctx context.Context) error {
	current := serviceTenants.Current(ctx)
	if current == nil || !current.IsOperator {
		return gerror.New("operator access is required")
	}
	return nil
}

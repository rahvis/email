package batch_mail

import (
	"billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/consts"
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/tenants"
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) ApiTemplatesDelete(ctx context.Context, req *v1.ApiTemplatesDeleteReq) (res *v1.ApiTemplatesDeleteRes, err error) {
	res = &v1.ApiTemplatesDeleteRes{}
	tenantID, err := tenants.RequireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	// verify if API exists
	count, err := g.DB().Model("api_templates").Where("id", req.ID).Where("tenant_id", tenantID).Count()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, gerror.New(public.LangCtx(ctx, "API does not exist"))
	}

	// start transaction
	var apiTemplate entity.ApiTemplates
	_ = g.DB().Model("api_templates").Where("id", req.ID).Where("tenant_id", tenantID).Scan(&apiTemplate)
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// delete API related send logs
		_, err = tx.Model("api_mail_logs").Where("api_id", req.ID).Where("tenant_id", tenantID).Delete()
		if err != nil {
			return err
		}

		// delete API template
		_, err = tx.Model("api_templates").Where("id", req.ID).Where("tenant_id", tenantID).Delete()
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.SendAPI,
		Log:  "Delete API template :" + apiTemplate.ApiName + " successfully",
		Data: apiTemplate,
	})

	res.SetSuccess(public.LangCtx(ctx, "Delete API successfully"))
	return res, nil
}

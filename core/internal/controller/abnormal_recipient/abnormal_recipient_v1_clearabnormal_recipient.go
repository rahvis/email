package abnormal_recipient

import (
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/tenants"
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"billionmail-core/api/abnormal_recipient/v1"
)

func (c *ControllerV1) ClearabnormalRecipient(ctx context.Context, req *v1.ClearabnormalRecipientReq) (res *v1.ClearabnormalRecipientRes, err error) {
	res = &v1.ClearabnormalRecipientRes{}
	_, err = tenants.ScopeModel(ctx, g.DB().Model("abnormal_recipient"), "tenant_id").Where("1=1").Delete()
	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Clearing failed: {}", err.Error())))
		return
	}
	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.AbnormalRecipient,
		Log:  "Clearing the abnormal email box was successful",
	})

	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return
}

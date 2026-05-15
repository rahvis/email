package batch_mail

import (
	"billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/consts"
	"billionmail-core/internal/model/entity"
	service_batch_mail "billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/tenants"
	"context"
	"net"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) ApiTemplatesUpdate(ctx context.Context, req *v1.ApiTemplatesUpdateReq) (res *v1.ApiTemplatesUpdateRes, err error) {
	res = &v1.ApiTemplatesUpdateRes{}
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
	var current entity.ApiTemplates
	if err = g.DB().Model("api_templates").Where("id", req.ID).Where("tenant_id", tenantID).Scan(&current); err != nil {
		return nil, err
	}

	// verify if template exists
	count, err = g.DB().Model("email_templates").Where("id", req.TemplateId).Where("tenant_id", tenantID).Count()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, gerror.New(public.LangCtx(ctx, "Email template does not exist"))
	}

	tx, err := g.DB().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// current time
	now := time.Now().Unix()

	updateMap := g.Map{
		"api_name":           req.ApiName,
		"template_id":        req.TemplateId,
		"subject":            req.Subject,
		"addresser":          req.Addresser,
		"full_name":          req.FullName,
		"unsubscribe":        req.Unsubscribe,
		"track_open":         req.TrackOpen,
		"track_click":        req.TrackClick,
		"delivery_engine":    service_batch_mail.NormalizeAPIDeliveryEngineForAPI(req.DeliveryEngine),
		"sending_profile_id": req.SendingProfileId,
		"active":             req.Active,
		"expire_time":        req.ExpireTime,
		"update_time":        now,
		"group_id":           req.GroupId,
	}
	if req.ResetKey {
		apiKey, keyErr := generateApiKey()
		if keyErr != nil {
			return nil, keyErr
		}
		updateMap["api_key"] = apiKey
		updateMap["api_key_hash"] = hashAPIKey(apiKey)
		updateMap["last_key_update_time"] = now
	}

	_, err = tx.Model("api_templates").
		Where("id", req.ID).
		Where("tenant_id", tenantID).
		Update(updateMap)

	if err != nil {
		return nil, err
	}

	// ip
	_, err = tx.Model("api_ip_whitelist").
		Where("api_id", req.ID).
		Where("tenant_id", tenantID).
		Delete()
	if err != nil {
		g.Log().Errorf(ctx, "[API Update] Failed to delete IP whitelist: %v", err)
		return nil, err
	}

	if len(req.IpWhitelist) > 0 {
		for _, ip := range req.IpWhitelist {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				g.Log().Errorf(ctx, "[API Templates Update] Empty IP address")
				continue
			}

			if net.ParseIP(ip) == nil {
				g.Log().Errorf(ctx, "[API Templates Update] Invalid IP address: %s", ip)
				continue
			}
			_, err = tx.Model("api_ip_whitelist").Insert(g.Map{
				"api_id":      req.ID,
				"tenant_id":   current.TenantId,
				"ip":          strings.TrimSpace(ip),
				"create_time": now,
			})
			if err != nil {
				return nil, err
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	updateMap["ip_whitelist"] = req.IpWhitelist
	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.SendAPI,
		Log:  "Update API template :" + req.ApiName + " successfully",
		Data: updateMap,
	})

	res.SetSuccess(public.LangCtx(ctx, "Update API successfully"))
	return res, nil
}

package batch_mail

import (
	"billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/kumo"
	"billionmail-core/internal/service/outbound"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/tenants"
	"context"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) ApiTemplatesList(ctx context.Context, req *v1.ApiTemplatesListReq) (res *v1.ApiTemplatesListRes, err error) {
	res = &v1.ApiTemplatesListRes{}
	tenantID, err := tenants.RequireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	// build query conditions
	model := g.DB().Model("api_templates").Safe().Where("tenant_id", tenantID)

	// add api_name fuzzy search
	if req.Keyword != "" {
		model = model.WhereLike("api_name", "%"+req.Keyword+"%")
	}
	// active filter
	if req.Active != -1 {
		model = model.Where("active", req.Active)
	}

	//if req.StartTime > 0 && req.EndTime <= 0 {
	//	req.EndTime = int(time.Now().Unix())
	//}
	//if req.StartTime > 0 {
	//	model = model.WhereGTE("create_time", req.StartTime)
	//}
	//if req.EndTime > 0 {
	//	model = model.WhereLTE("create_time", req.EndTime)
	//}
	// get total
	total, err := model.Count()
	if err != nil {
		return nil, err
	}

	// query by page
	var list []*v1.ApiTemplatesInfo
	err = model.Page(req.Page, req.PageSize).
		OrderDesc("id").
		Scan(&list)
	if err != nil {
		return nil, err
	}

	for _, item := range list {
		// build base query
		query := g.DB().Model("api_mail_logs aml")

		query = query.LeftJoin("mailstat_message_ids mi", "aml.message_id=mi.message_id")
		query = query.LeftJoin("mailstat_send_mails sm", "mi.postfix_message_id=sm.postfix_message_id")
		query = query.Where("aml.api_id", item.Id)
		query = query.Where("aml.tenant_id", tenantID)
		query = query.Where("aml.status", 2)
		query = query.Where("(aml.engine IS NULL OR aml.engine <> ?)", outbound.EngineKumoMTA)
		//if req.StartTime > 0 {
		//	query.Where("sm.log_time_millis > ?", req.StartTime*1000-1)
		//}
		//
		//if req.EndTime > 0 {
		//	query.Where("sm.log_time_millis < ?", req.EndTime*1000+1)
		//}

		// count各项数据
		query.Fields(
			"count(*) as sends",
			"coalesce(sum(case when sm.status='sent' and sm.dsn like '2.%' then 1 else 0 end), 0) as delivered",
			"coalesce(sum(case when sm.status='bounced' then 1 else 0 end), 0) as bounced",
		)

		result, err := query.One()
		if err != nil {
			// g.Log().Error(ctx, "Stats API failed to send data:", err)
			continue
		}

		postfixSends := result["sends"].Int()
		postfixDelivered := result["delivered"].Int()
		postfixBounced := result["bounced"].Int()

		kumoQuery := g.DB().Model("api_mail_logs").
			Where("api_id", item.Id).
			Where("tenant_id", tenantID).
			Where("engine", outbound.EngineKumoMTA)
		if req.StartTime > 0 {
			kumoQuery = kumoQuery.WhereGTE("create_time", req.StartTime)
		}
		if req.EndTime > 0 {
			kumoQuery = kumoQuery.WhereLTE("create_time", req.EndTime)
		}
		kumoQuery.Fields(
			"coalesce(sum(case when injection_status='queued' then 1 else 0 end), 0) as queued",
			"coalesce(sum(case when delivery_status='delivered' then 1 else 0 end), 0) as delivered",
			"coalesce(sum(case when delivery_status='deferred' then 1 else 0 end), 0) as deferred",
			"coalesce(sum(case when delivery_status='bounced' then 1 else 0 end), 0) as bounced",
			"coalesce(sum(case when delivery_status='expired' then 1 else 0 end), 0) as expired",
			"coalesce(sum(case when delivery_status='complained' then 1 else 0 end), 0) as complained",
		)
		kumoResult, _ := kumoQuery.One()
		item.QueuedCount = kumoResult["queued"].Int()
		item.DeliveredCount = kumoResult["delivered"].Int()
		item.DeferredCount = kumoResult["deferred"].Int()
		item.BouncedCount = kumoResult["bounced"].Int()
		item.ExpiredCount = kumoResult["expired"].Int()
		item.ComplainedCount = kumoResult["complained"].Int()

		item.SendCount = postfixSends + item.QueuedCount
		item.SuccessCount = postfixDelivered + item.DeliveredCount
		item.FailCount = postfixBounced + item.BouncedCount + item.ExpiredCount + item.ComplainedCount

		// count opened, clicked (use campaign_id directly)
		apiCampaignId := item.Id + 1000000000
		openedCount, _ := g.DB().Model("mailstat_opened").
			Where("campaign_id", apiCampaignId).
			WhereGTE("log_time_millis", req.StartTime*1000).
			WhereLTE("log_time_millis", req.EndTime*1000).
			Fields("count(distinct postfix_message_id) as opened").
			Value()
		clickedCount, _ := g.DB().Model("mailstat_clicked").
			Where("campaign_id", apiCampaignId).
			WhereGTE("log_time_millis", req.StartTime*1000).
			WhereLTE("log_time_millis", req.EndTime*1000).
			Fields("count(distinct postfix_message_id) as clicked").
			Value()

		if item.SendCount > 0 {
			item.DeliveryRate = public.Round(float64(item.SuccessCount)/float64(item.SendCount)*100, 2)
			item.BounceRate = public.Round(float64(item.FailCount)/float64(item.SendCount)*100, 2)
			item.OpenRate = public.Round(float64(openedCount.Int())/float64(item.SendCount)*100, 2)
			item.ClickRate = public.Round(float64(clickedCount.Int())/float64(item.SendCount)*100, 2)
		} else {
			item.DeliveryRate = 0
			item.BounceRate = 0
			item.OpenRate = 0
			item.ClickRate = 0
		}

		// count unsubscribe
		//recipients := []string{}
		//_, err = g.DB().Model("api_mail_logs").Where("api_id", item.Id).Fields("recipient").Array(&recipients)
		//unsubscribeCount := 0
		//if len(recipients) > 0 {
		//	unsubscribeCount, _ = g.DB().Model("bm_contacts").
		//		Where("email", recipients).
		//		Where("active", 0).
		//		WhereGTE("create_time", item.CreateTime).
		//		Count()
		//}
		//item.UnsubscribeCount = unsubscribeCount

		// get IP whitelist
		var ipRows []struct{ Ip string }
		err = g.DB().Model("api_ip_whitelist").
			Where("api_id", item.Id).
			Where("tenant_id", tenantID).
			Fields("ip").
			Scan(&ipRows)

		if err != nil {
			g.Log().Error(ctx, "Failed to get IP whitelist:", err)
			continue
		}
		ips := make([]string, 0, len(ipRows))
		for _, row := range ipRows {
			ips = append(ips, row.Ip)
		}
		item.IpWhitelist = ips
		item.ServerAddresser = domains.GetBaseURL() + "/api/batch_mail/api/send"
		if item.DeliveryEngine == outbound.EngineKumoMTA || item.DeliveryEngine == "tenant_default" {
			status := kumo.GetStatus()
			item.WebhookLastSeenAt = status.WebhookLastSeenAt
			item.WebhookLagSeconds = status.WebhookLagSeconds
			item.WebhookHealthy = status.WebhookLastSeenAt > 0 && status.WebhookLagSeconds < 300
		}
	}

	res.Data.Total = total
	res.Data.List = list
	res.SetSuccess(public.LangCtx(ctx, "Get API list successfully"))
	return res, nil
}

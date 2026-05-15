package batch_mail

import (
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/tenants"
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"time"

	"billionmail-core/api/batch_mail/v1"
)

func (c *ControllerV1) TaskOverview(ctx context.Context, req *v1.TaskOverviewReq) (res *v1.TaskOverviewRes, err error) {
	res = &v1.TaskOverviewRes{}
	tenantID, err := tenants.RequireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	startTime := req.StartTime
	endTime := req.EndTime
	if startTime > 0 && endTime < 0 {
		endTime = int(time.Now().Unix())
	}

	query := g.DB().Model("mailstat_send_mails sm").
		LeftJoin("mailstat_message_ids mi", "sm.postfix_message_id=mi.postfix_message_id").
		LeftJoin("recipient_info ri", "mi.message_id=ri.message_id").
		Where("ri.tenant_id", tenantID)
	if startTime > 0 {
		query = query.Where("sm.log_time_millis > ?", startTime*1000-1)
	}
	if endTime > 0 {
		query = query.Where("sm.log_time_millis < ?", endTime*1000+1)
	}
	query = query.Fields(
		"count(*) as sends",
		"coalesce(sum(case when sm.status='sent' and sm.dsn like '2.%' then 1 else 0 end), 0) as delivered",
		"coalesce(sum(case when sm.status='bounced' then 1 else 0 end), 0) as bounced",
	)
	result, err := query.One()
	if err != nil {
		return nil, err
	}
	sends := result["sends"].Int()
	delivered := result["delivered"].Int()
	bounced := result["bounced"].Int()
	deferred := 0
	expired := 0
	complained := 0
	queued := 0

	kumoQuery := g.DB().Model("recipient_info").Where("tenant_id", tenantID).Where("engine", "kumomta")
	if startTime > 0 {
		kumoQuery = kumoQuery.Where("sent_time >= ?", startTime)
	}
	if endTime > 0 {
		kumoQuery = kumoQuery.Where("sent_time <= ?", endTime)
	}
	kumoResult, kumoErr := kumoQuery.Fields(
		"coalesce(sum(case when injection_status='queued' then 1 else 0 end), 0) as queued",
		"coalesce(sum(case when delivery_status='delivered' then 1 else 0 end), 0) as delivered",
		"coalesce(sum(case when delivery_status='deferred' then 1 else 0 end), 0) as deferred",
		"coalesce(sum(case when delivery_status='bounced' then 1 else 0 end), 0) as bounced",
		"coalesce(sum(case when delivery_status='expired' then 1 else 0 end), 0) as expired",
		"coalesce(sum(case when delivery_status='complained' then 1 else 0 end), 0) as complained",
	).One()
	if kumoErr == nil {
		queued = kumoResult["queued"].Int()
		sends += queued
		delivered += kumoResult["delivered"].Int()
		deferred = kumoResult["deferred"].Int()
		bounced += kumoResult["bounced"].Int()
		expired = kumoResult["expired"].Int()
		complained = kumoResult["complained"].Int()
	} else {
		g.Log().Warningf(ctx, "failed to load KumoMTA campaign overview counters: %v", kumoErr)
	}

	openedCount, _ := g.DB().Model("mailstat_opened").
		Where("tenant_id", tenantID).
		WhereGTE("log_time_millis", startTime*1000).
		WhereLTE("log_time_millis", endTime*1000).
		Fields("count(distinct postfix_message_id) as opened").
		Value()
	clickedCount, _ := g.DB().Model("mailstat_clicked").
		Where("tenant_id", tenantID).
		WhereGTE("log_time_millis", startTime*1000).
		WhereLTE("log_time_millis", endTime*1000).
		Fields("count(distinct postfix_message_id) as clicked").
		Value()

	var deliveryRate, bounceRate, openRate, clickRate float64
	if sends > 0 {
		deliveryRate = public.Round(float64(delivered)/float64(sends)*100, 2)
		bounceRate = public.Round(float64(bounced)/float64(sends)*100, 2)
		openRate = public.Round(float64(openedCount.Int())/float64(sends)*100, 2)
		clickRate = public.Round(float64(clickedCount.Int())/float64(sends)*100, 2)
	}

	res.Data.Sends = sends
	res.Data.Queued = queued
	res.Data.Delivered = delivered
	res.Data.Deferred = deferred
	res.Data.Bounced = bounced
	res.Data.Expired = expired
	res.Data.Complained = complained
	res.Data.Opened = openedCount.Int()
	res.Data.Clicked = clickedCount.Int()
	res.Data.DeliveryRate = deliveryRate
	res.Data.BounceRate = bounceRate
	res.Data.OpenRate = openRate
	res.Data.ClickRate = clickRate

	res.SetSuccess(public.LangCtx(ctx, "Success"))
	return res, nil
}

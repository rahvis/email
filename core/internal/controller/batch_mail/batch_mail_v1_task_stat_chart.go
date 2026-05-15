package batch_mail

import (
	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/public"
	"context"
	"fmt"
	"github.com/gogf/gf/v2/util/gconv"

	"github.com/gogf/gf/v2/errors/gerror"

	"billionmail-core/api/batch_mail/v1"
)

func (c *ControllerV1) TaskStatChart(ctx context.Context, req *v1.TaskStatChartReq) (res *v1.TaskStatChartRes, err error) {
	res = &v1.TaskStatChartRes{}

	taskInfo, err := batch_mail.GetTaskInfo(ctx, int(req.TaskId))
	if err != nil {
		res.Code = 500
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to get task information: {}", err.Error())))
		return
	}

	if taskInfo == nil || taskInfo.Id == 0 {
		res.Code = 404
		res.SetError(gerror.New(public.LangCtx(ctx, "Task not found: {}", req.TaskId)))
		return
	}

	// reuse the maillog_stat service to get the overview
	overview := maillog_stat.NewOverview()
	overviewMap := overview.Overview(req.TaskId, req.Domain, req.StartTime, req.EndTime)
	mergeKumoTaskLifecycleIntoOverview(ctx, overviewMap, req.TaskId)

	err = gconv.Struct(overviewMap, &res.Data)

	if err != nil {
		err = fmt.Errorf("failed to convert overview data: %v", err)
		return
	}

	//statService := batch_mail.NewTaskStatService()
	//
	//chartData := statService.GetTaskStatChart(req.TaskId, req.Domain, req.StartTime, req.EndTime)
	//
	//res.Data.Dashboard = chartData["dashboard"]
	//res.Data.MailProviders = chartData["mail_providers"]
	//res.Data.SendMailChart = chartData["send_mail_chart"]
	//res.Data.BounceRateChart = chartData["bounce_rate_chart"]
	//res.Data.OpenRateChart = chartData["open_rate_chart"]
	//res.Data.ClickRateChart = chartData["click_rate_chart"]

	res.SetSuccess(public.LangCtx(ctx, "Task statistics retrieved successfully"))
	return
}

func mergeKumoTaskLifecycleIntoOverview(ctx context.Context, overviewMap map[string]interface{}, taskID int64) {
	if overviewMap == nil || taskID <= 0 {
		return
	}
	dashboard, ok := overviewMap["dashboard"].(map[string]interface{})
	if !ok {
		return
	}

	kumoStats := batch_mail.GetTaskKumoLifecycleStats(ctx, taskID)
	queued := kumoStats["queued"]
	deliveredKumo := kumoStats["delivered"]
	deferred := kumoStats["deferred"]
	bouncedKumo := kumoStats["bounced"]
	expired := kumoStats["expired"]
	complained := kumoStats["complained"]

	sends := gconv.Int(dashboard["sends"]) + queued
	delivered := gconv.Int(dashboard["delivered"]) + deliveredKumo
	bounced := gconv.Int(dashboard["bounced"]) + bouncedKumo
	opened := gconv.Int(dashboard["opened"])
	clicked := gconv.Int(dashboard["clicked"])

	dashboard["sends"] = sends
	dashboard["queued"] = queued
	dashboard["delivered"] = delivered
	dashboard["deferred"] = deferred
	dashboard["bounced"] = bounced
	dashboard["expired"] = expired
	dashboard["complained"] = complained

	if sends > 0 {
		dashboard["delivery_rate"] = public.Round(float64(delivered)/float64(sends)*100, 2)
		dashboard["bounce_rate"] = public.Round(float64(bounced)/float64(sends)*100, 2)
	} else {
		dashboard["delivery_rate"] = 0
		dashboard["bounce_rate"] = 0
	}
	if delivered > 0 {
		dashboard["open_rate"] = public.Round(float64(opened)/float64(delivered)*100, 2)
		dashboard["click_rate"] = public.Round(float64(clicked)/float64(delivered)*100, 2)
	} else {
		dashboard["open_rate"] = 0
		dashboard["click_rate"] = 0
	}

	if sendChart, ok := overviewMap["send_mail_chart"].(map[string]interface{}); ok {
		if sendDashboard, ok := sendChart["dashboard"].(map[string]interface{}); ok {
			failed := bounced + deferred + expired + complained
			sendDashboard["sends"] = sends
			sendDashboard["delivered"] = delivered
			sendDashboard["failed"] = failed
			if sends > 0 {
				sendDashboard["delivery_rate"] = public.Round(float64(delivered)/float64(sends)*100, 2)
				sendDashboard["failure_rate"] = public.Round(float64(failed)/float64(sends)*100, 2)
			} else {
				sendDashboard["delivery_rate"] = 0
				sendDashboard["failure_rate"] = 0
			}
		}
	}
}

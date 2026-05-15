package kumo

import (
	"context"

	"billionmail-core/api/kumo/v1"
)

type IKumoV1 interface {
	GetConfig(ctx context.Context, req *v1.GetConfigReq) (res *v1.GetConfigRes, err error)
	SaveConfig(ctx context.Context, req *v1.SaveConfigReq) (res *v1.SaveConfigRes, err error)
	TestConnection(ctx context.Context, req *v1.TestConnectionReq) (res *v1.TestConnectionRes, err error)
	GetStatus(ctx context.Context, req *v1.GetStatusReq) (res *v1.GetStatusRes, err error)
	GetMetrics(ctx context.Context, req *v1.GetMetricsReq) (res *v1.GetMetricsRes, err error)
	GetRuntime(ctx context.Context, req *v1.GetRuntimeReq) (res *v1.GetRuntimeRes, err error)
	Events(ctx context.Context, req *v1.EventsReq) (res *v1.EventsRes, err error)
	PreviewConfig(ctx context.Context, req *v1.PreviewConfigReq) (res *v1.PreviewConfigRes, err error)
	DeployConfig(ctx context.Context, req *v1.DeployConfigReq) (res *v1.DeployConfigRes, err error)
}

package tenants

import (
	"context"

	"billionmail-core/api/tenants/v1"
)

type ITenantsV1 interface {
	Current(ctx context.Context, req *v1.CurrentReq) (res *v1.CurrentRes, err error)
	List(ctx context.Context, req *v1.ListReq) (res *v1.ListRes, err error)
	Switch(ctx context.Context, req *v1.SwitchReq) (res *v1.SwitchRes, err error)
	Create(ctx context.Context, req *v1.CreateReq) (res *v1.CreateRes, err error)
	ListSendingProfiles(ctx context.Context, req *v1.ListSendingProfilesReq) (res *v1.ListSendingProfilesRes, err error)
	SendingProfile(ctx context.Context, req *v1.SendingProfileReq) (res *v1.SendingProfileRes, err error)
	SendingControl(ctx context.Context, req *v1.SendingControlReq) (res *v1.SendingControlRes, err error)
}

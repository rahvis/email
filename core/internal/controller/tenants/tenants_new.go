package tenants

import api "billionmail-core/api/tenants"

type ControllerV1 struct{}

func NewV1() api.ITenantsV1 {
	return &ControllerV1{}
}

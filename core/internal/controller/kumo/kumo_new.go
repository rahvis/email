package kumo

import api "billionmail-core/api/kumo"

type ControllerV1 struct{}

func NewV1() api.IKumoV1 {
	return &ControllerV1{}
}

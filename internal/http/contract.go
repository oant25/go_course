//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package transport

import (
	"context"

	api "github.com/oant25/go_course/internal/generated"
)

type createHandler interface {
	CreateTrip(context.Context, api.CreateTripRequestObject) (api.CreateTripResponseObject, error)
}
type getHandler interface {
	GetTrip(context.Context, api.GetTripRequestObject) (api.GetTripResponseObject, error)
}
type finishHandler interface {
	FinishTrip(context.Context, api.FinishTripRequestObject) (api.FinishTripResponseObject, error)
}
type healthHandler interface {
	Health(context.Context, api.HealthRequestObject) (api.HealthResponseObject, error)
}
type readyHandler interface {
	Ready(context.Context, api.ReadyRequestObject) (api.ReadyResponseObject, error)
}

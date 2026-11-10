package transport

import (
	"context"

	api "github.com/oant25/go_course/internal/generated"
)

type Server struct {
	create createHandler
	get    getHandler
	finish finishHandler
	health healthHandler
	ready  readyHandler
}

var _ api.StrictServerInterface = (*Server)(nil)

func NewServer(create createHandler, get getHandler, finish finishHandler, health healthHandler, ready readyHandler) *Server {
	return &Server{create: create, get: get, finish: finish, health: health, ready: ready}
}

func (s *Server) CreateTrip(ctx context.Context, req api.CreateTripRequestObject) (api.CreateTripResponseObject, error) {
	return s.create.CreateTrip(ctx, req)
}

func (s *Server) GetTrip(ctx context.Context, req api.GetTripRequestObject) (api.GetTripResponseObject, error) {
	return s.get.GetTrip(ctx, req)
}

func (s *Server) FinishTrip(ctx context.Context, req api.FinishTripRequestObject) (api.FinishTripResponseObject, error) {
	return s.finish.FinishTrip(ctx, req)
}

func (s *Server) Health(ctx context.Context, req api.HealthRequestObject) (api.HealthResponseObject, error) {
	return s.health.Health(ctx, req)
}

func (s *Server) Ready(ctx context.Context, req api.ReadyRequestObject) (api.ReadyResponseObject, error) {
	return s.ready.Ready(ctx, req)
}

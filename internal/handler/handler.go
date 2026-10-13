package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/service"
	"github.com/oant25/go_course/internal/trip"
)

type tripService interface {
	Create(ctx context.Context, in service.CreateInput) (trip.Trip, error)
	Get(ctx context.Context, id uuid.UUID) (trip.Trip, error)
	Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}

type pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	trips        tripService
	db           pinger
	readyTimeout time.Duration
}

var _ api.StrictServerInterface = (*Handler)(nil)

func New(trips tripService, db pinger, readyTimeout time.Duration) *Handler {
	return &Handler{
		trips:        trips,
		db:           db,
		readyTimeout: readyTimeout,
	}
}

func (h *Handler) Health(_ context.Context, _ api.HealthRequestObject) (api.HealthResponseObject, error) {
	return api.Health200JSONResponse{Status: api.Ok}, nil
}

func (h *Handler) Ready(ctx context.Context, _ api.ReadyRequestObject) (api.ReadyResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, h.readyTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		slog.Warn("readiness check failed", "error", err)
		return api.Ready503JSONResponse{Status: api.Unavailable}, nil
	}
	return api.Ready200JSONResponse{Status: api.Ok}, nil
}

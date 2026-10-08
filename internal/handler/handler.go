package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/service"
)

type pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	trips        *service.Trips
	db           pinger
	readyTimeout time.Duration
}

var _ api.ServerInterface = (*Handler)(nil)

func New(trips *service.Trips, db pinger, readyTimeout time.Duration) *Handler {
	return &Handler{
		trips:        trips,
		db:           db,
		readyTimeout: readyTimeout,
	}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.readyTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		slog.Warn("readiness check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}

	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

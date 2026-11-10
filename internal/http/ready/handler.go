package ready

import (
	"context"
	"time"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/requeststate"
)

type Handler struct {
	db      pinger
	timeout time.Duration
}

func New(db pinger, timeout time.Duration) *Handler {
	return &Handler{db: db, timeout: timeout}
}

func (h *Handler) Ready(ctx context.Context, _ api.ReadyRequestObject) (api.ReadyResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		requeststate.Fail(ctx, 503, "database_unavailable", err, nil)
		return api.Ready503JSONResponse{Status: api.Unavailable}, nil
	}
	return api.Ready200JSONResponse{Status: api.Ok}, nil
}

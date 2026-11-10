package get

import (
	"context"
	"fmt"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/tripconvert"
)

type Handler struct{ usecase usecase }

func New(usecase usecase) *Handler { return &Handler{usecase: usecase} }
func (h *Handler) GetTrip(ctx context.Context, req api.GetTripRequestObject) (api.GetTripResponseObject, error) {
	value, err := h.usecase.Execute(ctx, req.TripId)
	if err != nil {
		return nil, fmt.Errorf("get trip request: %w", err)
	}
	return api.GetTrip200JSONResponse(tripconvert.ToAPI(value)), nil
}

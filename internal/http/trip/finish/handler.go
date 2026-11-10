package finish

import (
	"context"
	"fmt"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/tripconvert"
)

type Handler struct{ usecase usecase }

func New(usecase usecase) *Handler { return &Handler{usecase: usecase} }
func (h *Handler) FinishTrip(ctx context.Context, req api.FinishTripRequestObject) (api.FinishTripResponseObject, error) {
	value, err := h.usecase.Execute(ctx, req.TripId)
	if err != nil {
		return nil, fmt.Errorf("finish trip request: %w", err)
	}
	return api.FinishTrip200JSONResponse(tripconvert.ToAPI(value)), nil
}

package create

import (
	"context"
	"fmt"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/domainerror"
	"github.com/oant25/go_course/internal/http/tripconvert"
)

type Handler struct{ usecase usecase }

func New(usecase usecase) *Handler { return &Handler{usecase: usecase} }
func (h *Handler) CreateTrip(ctx context.Context, req api.CreateTripRequestObject) (api.CreateTripResponseObject, error) {
	if req.Body == nil {
		return nil, domainerror.ErrInvalidRequest
	}
	value, err := h.usecase.Execute(ctx, toInput(*req.Body))
	if err != nil {
		return nil, fmt.Errorf("create trip request: %w", err)
	}
	location := tripconvert.Path(value.Snapshot().ID)
	return api.CreateTrip201JSONResponse{Body: tripconvert.ToAPI(value), Headers: api.CreateTrip201ResponseHeaders{Location: &location}}, nil
}

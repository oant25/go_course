package health

import (
	"context"

	api "github.com/oant25/go_course/internal/generated"
)

type Handler struct{}

func New() *Handler { return &Handler{} }

//nolint:unparam // Возвращаемая ошибка предусмотрена сгенерированным HTTP-контрактом.
func (*Handler) Health(context.Context, api.HealthRequestObject) (api.HealthResponseObject, error) {
	return api.Health200JSONResponse{Status: api.Ok}, nil
}

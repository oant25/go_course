package tripconvert

import (
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"

	"github.com/oant25/go_course/internal/domain/trip"
	api "github.com/oant25/go_course/internal/generated"
)

func Path(id uuid.UUID) string { return "/api/v1/trips/" + id.String() }

func ToAPI(value trip.Trip) api.Trip {
	t := value.Snapshot()
	finishedAt := nullable.NewNullNullable[time.Time]()
	if t.FinishedAt != nil {
		finishedAt = nullable.NewNullableWithValue(t.FinishedAt.UTC())
	}

	return api.Trip{
		Id:       t.ID,
		UserId:   t.UserID,
		DriverId: t.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  t.Start.Latitude,
			Longitude: t.Start.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  t.End.Latitude,
			Longitude: t.End.Longitude,
		},
		Price:          t.Price,
		Status:         api.TripStatus(t.Status),
		StartedAt:      t.StartedAt.UTC(),
		FinishedAt:     finishedAt,
		LastPositionAt: nullable.NewNullNullable[time.Time](),
	}
}

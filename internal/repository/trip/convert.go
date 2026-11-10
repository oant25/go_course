package trip

import (
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/oant25/go_course/internal/domain/trip"
)

func scanTrip(row pgx.Row) (trip.Trip, error) {
	var t trip.State
	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.Start.Latitude, &t.Start.Longitude, &t.End.Latitude, &t.End.Longitude,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return trip.Trip{}, fmt.Errorf("scan trip: %w", err)
	}
	return trip.Restore(t)
}

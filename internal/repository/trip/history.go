package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/oant25/go_course/internal/domain/trip"
)

func (r *Repository) AddStatusChange(ctx context.Context, tripID uuid.UUID, from *trip.Status, to trip.Status, at time.Time) error {
	query, args, err := psql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "changed_at").
		Values(tripID, from, to, at).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert status change: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := r.executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert status change: %w", err)
	}
	return nil
}

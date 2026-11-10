package trip

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/infra/postgres/transaction"
)

const (
	pgUniqueViolation      = "23505"
	activeTripPerDriverIdx = "trips_one_active_per_driver_idx"
)

func (r *Repository) Create(ctx context.Context, value trip.Trip) (trip.Trip, error) {
	t := value.Snapshot()
	query, args, err := psql.Insert("trips").
		Columns(tripColumns...).
		Values(
			t.ID, t.UserID, t.DriverID,
			t.Start.Latitude, t.Start.Longitude, t.End.Latitude, t.End.Longitude,
			t.Price, t.Status, t.StartedAt, t.FinishedAt, t.CreatedAt, t.UpdatedAt,
		).
		Suffix("RETURNING " + strings.Join(tripColumns, ", ")).
		ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build insert trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	created, err := scanTrip(r.executor(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == activeTripPerDriverIdx {
			return trip.Trip{}, trip.ErrDriverBusy
		}
		return trip.Trip{}, fmt.Errorf("insert trip: %w", err)
	}
	return created, nil
}

func (r *Repository) SaveFinished(ctx context.Context, value trip.Trip) (trip.Trip, error) {
	if _, ok := transaction.FromContext(ctx); !ok {
		return trip.Trip{}, errors.New("saving finished trip requires a transaction")
	}
	t := value.Snapshot()
	if t.Status != trip.StatusCompleted || t.FinishedAt == nil {
		return trip.Trip{}, fmt.Errorf("save finished trip: %w", trip.ErrInvalidState)
	}
	query, args, err := psql.Update("trips").
		Set(statusColumn, t.Status).
		Set("finished_at", t.FinishedAt).
		Set("updated_at", t.UpdatedAt).
		Where(sq.Eq{"id": t.ID, statusColumn: trip.StatusActive}).
		Suffix("RETURNING " + strings.Join(tripColumns, ", ")).ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build finish trip: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	finished, err := scanTrip(r.executor(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, fmt.Errorf("locked trip changed before update: %w", trip.ErrInvalidState)
	}
	if err != nil {
		return trip.Trip{}, fmt.Errorf("save finished trip: %w", err)
	}
	return finished, nil
}

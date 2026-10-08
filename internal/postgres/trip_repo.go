package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oant25/go_course/internal/trip"
)

const (
	pgUniqueViolation      = "23505"
	activeTripPerDriverIdx = "trips_one_active_per_driver_idx"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", "status", "started_at", "finished_at", "created_at", "updated_at",
}

type TripRepo struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripRepo(pool *pgxpool.Pool, queryTimeout time.Duration) *TripRepo {
	return &TripRepo{pool: pool, queryTimeout: queryTimeout}
}

func (r *TripRepo) Create(ctx context.Context, t trip.Trip) (trip.Trip, error) {
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

	created, err := scanTrip(executorFrom(ctx, r.pool).QueryRow(ctx, query, args...))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == activeTripPerDriverIdx {
			return trip.Trip{}, trip.ErrDriverBusy
		}
		return trip.Trip{}, fmt.Errorf("insert trip: %w", err)
	}
	return created, nil
}

func (r *TripRepo) GetByID(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	query, args, err := psql.Select(tripColumns...).
		From("trips").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	t, err := scanTrip(executorFrom(ctx, r.pool).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, trip.ErrNotFound
	}
	if err != nil {
		return trip.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return t, nil
}

func (r *TripRepo) Finish(ctx context.Context, id uuid.UUID, at time.Time) (trip.Trip, error) {
	query, args, err := psql.Update("trips").
		Set("status", trip.StatusCompleted).
		Set("finished_at", at).
		Set("updated_at", at).
		Where(sq.Eq{"id": id, "status": trip.StatusActive}).
		Suffix("RETURNING " + strings.Join(tripColumns, ", ")).
		ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build finish trip: %w", err)
	}

	qctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	finished, err := scanTrip(executorFrom(qctx, r.pool).QueryRow(qctx, query, args...))
	if err == nil {
		return finished, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	if _, err := r.GetByID(ctx, id); err != nil {
		return trip.Trip{}, err
	}
	return trip.Trip{}, trip.ErrAlreadyCompleted
}

func (r *TripRepo) AddStatusChange(ctx context.Context, tripID uuid.UUID, from *trip.Status, to trip.Status, at time.Time) error {
	query, args, err := psql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "changed_at").
		Values(tripID, from, to, at).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert status change: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := executorFrom(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert status change: %w", err)
	}
	return nil
}

func scanTrip(row pgx.Row) (trip.Trip, error) {
	var t trip.Trip
	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.Start.Latitude, &t.Start.Longitude, &t.End.Latitude, &t.End.Longitude,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}

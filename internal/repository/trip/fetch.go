package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/infra/postgres/transaction"
)

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return r.fetch(ctx, id, false)
}

func (r *Repository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	if _, ok := transaction.FromContext(ctx); !ok {
		return trip.Trip{}, errors.New("locking trip read requires a transaction")
	}
	return r.fetch(ctx, id, true)
}

func (r *Repository) fetch(ctx context.Context, id uuid.UUID, forUpdate bool) (trip.Trip, error) {
	builder := psql.Select(tripColumns...).From("trips").Where(sq.Eq{"id": id})
	if forUpdate {
		builder = builder.Suffix("FOR UPDATE")
	}
	query, args, err := builder.ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	t, err := scanTrip(r.executor(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, trip.ErrTripNotFound
	}
	if err != nil {
		return trip.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return t, nil
}

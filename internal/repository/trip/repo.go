package trip

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/oant25/go_course/internal/infra/postgres/transaction"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", statusColumn, "started_at", "finished_at", "created_at", "updated_at",
}

type Repository struct {
	pool         executor
	queryTimeout time.Duration
}

func New(pool executor, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}

func (r *Repository) executor(ctx context.Context) executor {
	if tx, ok := transaction.FromContext(ctx); ok {
		return tx
	}
	return r.pool
}

const (
	statusColumn = "status"
)

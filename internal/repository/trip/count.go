package trip

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	"github.com/oant25/go_course/internal/domain/trip"
)

func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	query, args, err := psql.Select("COUNT(*)").From("trips").Where(sq.Eq{statusColumn: trip.StatusActive}).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build active trip count: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var count int64
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
}

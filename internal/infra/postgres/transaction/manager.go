package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNestedTransaction = errors.New("nested transactions are not supported")

type contextKey struct{}

type Manager struct {
	pool         beginner
	queryTimeout time.Duration
}

func New(pool beginner, queryTimeout time.Duration) *Manager {
	return &Manager{pool: pool, queryTimeout: queryTimeout}
}

func FromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(contextKey{}).(pgx.Tx)
	return tx, ok
}

func (m *Manager) Do(ctx context.Context, fn func(context.Context) error) (err error) {
	if _, ok := FromContext(ctx); ok {
		return ErrNestedTransaction
	}
	beginCtx, cancelBegin := context.WithTimeout(ctx, m.queryTimeout)
	tx, err := m.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	cancelBegin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.queryTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanupCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()
	if err := fn(context.WithValue(ctx, contextKey{}, tx)); err != nil {
		return fmt.Errorf("transaction operation: %w", err)
	}
	commitCtx, cancelCommit := context.WithTimeout(ctx, m.queryTimeout)
	defer cancelCommit()
	if err := tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return nil
}

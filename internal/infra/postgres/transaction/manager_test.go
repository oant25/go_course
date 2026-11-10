package transaction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/mock/gomock"

	"github.com/oant25/go_course/internal/infra/postgres/transaction"
)

func TestBeginErrorDoesNotInvokeCallback(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	pool := NewMockbeginner(ctrl)
	ctx := context.Background()
	failure := errors.New("cannot start transaction")
	pool.EXPECT().BeginTx(gomock.Any(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted}).DoAndReturn(func(beginCtx context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
		if _, ok := beginCtx.Deadline(); !ok {
			t.Fatal("begin has no deadline")
		}
		return nil, failure
	})
	err := transaction.New(pool, time.Second).Do(ctx, func(context.Context) error {
		t.Fatal("callback ran after failed begin")
		return nil
	})
	if !errors.Is(err, failure) || err == failure {
		t.Fatalf("expected wrapped begin error, got %v", err)
	}
	if _, ok := transaction.FromContext(ctx); ok {
		t.Fatal("transaction leaked into parent context")
	}
}

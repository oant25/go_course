//go:build integration

package trip_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/infra/postgres/transaction"
	triprepo "github.com/oant25/go_course/internal/repository/trip"
)

func TestHistoryConstraintFailureRollsBackCreateIntegration(t *testing.T) {
	pool := database(t)
	repo := triprepo.New(pool, testTimeout)
	tx := transaction.New(pool, testTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	in := input()
	at := time.Now().UTC()
	value, err := trip.New(uuid.New(), trip.Details{UserID: in.UserID, DriverID: in.DriverID, Start: in.Start, End: in.End, Price: in.Price}, at)
	if err != nil {
		t.Fatal(err)
	}
	id := value.Snapshot().ID
	err = tx.Do(ctx, func(ctx context.Context) error {
		if _, err := repo.Create(ctx, value); err != nil {
			return err
		}
		return repo.AddStatusChange(ctx, id, nil, trip.Status("invalid"), at)
	})
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
		t.Fatalf("expected check constraint failure: %v", err)
	}
	if _, err := repo.GetByID(ctx, id); !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("trip survived rollback: %v", err)
	}
	var history int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id=$1", id).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 0 {
		t.Fatalf("history survived rollback: %d", history)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool unusable after rollback: %v", err)
	}
}

func TestPrimaryKeyConflictIsNotDriverBusyIntegration(t *testing.T) {
	pool := database(t)
	repo := triprepo.New(pool, testTimeout)
	trips := newService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	created, err := trips.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	original := created.Snapshot()
	original.DriverID = uuid.New()
	duplicate, err := trip.Restore(original)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Create(ctx, duplicate)
	var postgresError *pgconn.PgError
	if err == nil || errors.Is(err, trip.ErrDriverBusy) || errors.Is(err, trip.ErrTripNotFound) || !errors.As(err, &postgresError) || postgresError.Code != "23505" {
		t.Fatalf("wrong classification of primary key conflict: %v", err)
	}
	fetched, err := repo.GetByID(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Snapshot().DriverID != created.Snapshot().DriverID {
		t.Fatal("failed insert changed original trip")
	}
}

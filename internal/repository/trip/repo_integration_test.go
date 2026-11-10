//go:build integration

package trip_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	metricnoop "go.opentelemetry.io/otel/metric/noop"

	"github.com/oant25/go_course/internal/domain/trip"
	transport "github.com/oant25/go_course/internal/http"
	"github.com/oant25/go_course/internal/http/health"
	"github.com/oant25/go_course/internal/http/ready"
	createhttp "github.com/oant25/go_course/internal/http/trip/create"
	finishhttp "github.com/oant25/go_course/internal/http/trip/finish"
	gethttp "github.com/oant25/go_course/internal/http/trip/get"
	"github.com/oant25/go_course/internal/infra/clock"
	"github.com/oant25/go_course/internal/infra/identifier"
	"github.com/oant25/go_course/internal/infra/postgres/transaction"
	"github.com/oant25/go_course/internal/infra/telemetry"
	"github.com/oant25/go_course/internal/model"
	triprepo "github.com/oant25/go_course/internal/repository/trip"
	"github.com/oant25/go_course/internal/usecase/trip/create"
	"github.com/oant25/go_course/internal/usecase/trip/finish"
	"github.com/oant25/go_course/internal/usecase/trip/get"
)

const testTimeout = 10 * time.Second

func database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal("connect to integration PostgreSQL failed")
	}
	schema := "lr2_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), testTimeout)
		defer done()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA IF EXISTS "+quoted+" CASCADE"); err != nil {
			t.Errorf("cleanup test schema: %v", err)
		}
		if err := admin.Close(cleanup); err != nil {
			t.Errorf("close test admin: %v", err)
		}
	})
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid integration DATABASE_URL")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("create integration pool failed")
	}
	t.Cleanup(pool.Close)
	for _, name := range []string{"00001_create_trips.sql", "00002_create_trip_status_history.sql"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		up, _, found := strings.Cut(string(data), "-- +goose Down")
		if !found {
			t.Fatalf("migration %s has no Down section", name)
		}
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return pool
}

func input() model.CreateTripInput {
	return model.CreateTripInput{
		UserID: uuid.New(), DriverID: uuid.New(),
		Start: trip.Point{Latitude: 55.75, Longitude: 37.61},
		End:   trip.Point{Latitude: 59.93, Longitude: 30.31}, Price: 1450,
	}
}

type scenarios struct {
	create *create.Usecase
	get    *get.Usecase
	finish *finish.Usecase
}

func newService(pool *pgxpool.Pool) scenarios {
	events, err := telemetry.NewBusiness(metricnoop.NewMeterProvider(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		panic(err)
	}
	repo := triprepo.New(pool, testTimeout)
	tx := transaction.New(pool, testTimeout)
	return scenarios{create.New(repo, tx, clock.System{}, identifier.UUID{}, events), get.New(repo), finish.New(repo, tx, clock.System{}, events)}
}

func (s scenarios) Create(ctx context.Context, in model.CreateTripInput) (trip.Trip, error) {
	return s.create.Execute(ctx, in)
}

func (s scenarios) Get(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return s.get.Execute(ctx, id)
}

func (s scenarios) Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return s.finish.Execute(ctx, id)
}

func newRouter(pool *pgxpool.Pool, trips scenarios) (http.Handler, error) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	server := transport.NewServer(createhttp.New(trips.create), gethttp.New(trips.get), finishhttp.New(trips.finish), health.New(), ready.New(pool, testTimeout))
	return transport.NewRouter(server, logger, 1<<20)
}

func TestTripLifecycleIntegration(t *testing.T) {
	pool := database(t)
	trips := newService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	in := input()
	created, err := trips.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	initial := created.Snapshot()
	if initial.ID == uuid.Nil || initial.Status != trip.StatusActive || initial.FinishedAt != nil || initial.Price != in.Price {
		t.Fatalf("unexpected created trip: %+v", initial)
	}
	if _, err := trips.Create(ctx, in); !errors.Is(err, trip.ErrDriverBusy) {
		t.Fatalf("busy driver: %v", err)
	}
	finished, err := trips.Finish(ctx, initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	state := finished.Snapshot()
	if state.Status != trip.StatusCompleted || state.FinishedAt == nil || !state.UpdatedAt.Equal(*state.FinishedAt) ||
		!state.StartedAt.Equal(initial.StartedAt) || !state.CreatedAt.Equal(initial.CreatedAt) {
		t.Fatalf("unexpected finished trip: %+v", state)
	}
	if _, err := trips.Finish(ctx, initial.ID); !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("second finish: %v", err)
	}
	fetched, err := trips.Get(ctx, initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !fetched.Snapshot().FinishedAt.Equal(*state.FinishedAt) {
		t.Fatal("second finish changed timestamp")
	}
	if _, err := trips.Get(ctx, uuid.New()); !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("missing trip: %v", err)
	}
	if _, err := trips.Finish(ctx, uuid.New()); !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("finish missing trip: %v", err)
	}
	var history int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id=$1", initial.ID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 2 {
		t.Fatalf("history rows = %d, want 2", history)
	}
	if _, err := trips.Create(ctx, in); err != nil {
		t.Fatalf("driver should be free after finish: %v", err)
	}
}

func TestConcurrentFinishIntegration(t *testing.T) {
	pool := database(t)
	trips := newService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	created, err := trips.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := trips.Finish(ctx, created.Snapshot().ID)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, trip.ErrTripCompleted):
			conflicted++
		default:
			t.Fatalf("unexpected finish error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("success=%d conflict=%d, want 1/1", succeeded, conflicted)
	}
	var completed int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id=$1 AND to_status=$2", created.Snapshot().ID, trip.StatusCompleted).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != 1 {
		t.Fatalf("completion history rows = %d, want 1", completed)
	}
}

func TestDomainTransitionRollbackIntegration(t *testing.T) {
	pool := database(t)
	repo := triprepo.New(pool, testTimeout)
	tx := transaction.New(pool, testTimeout)
	trips := newService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	created, err := trips.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	id := created.Snapshot().ID
	if _, err := repo.GetByIDForUpdate(ctx, id); err == nil {
		t.Fatal("locking read without transaction succeeded")
	}
	err = tx.Do(ctx, func(ctx context.Context) error {
		locked, err := repo.GetByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		at := time.Now().UTC()
		if err := locked.Finish(at); err != nil {
			return err
		}
		if _, err := repo.SaveFinished(ctx, locked); err != nil {
			return err
		}
		from := trip.StatusActive
		return repo.AddStatusChange(ctx, id, &from, trip.Status("invalid"), at)
	})
	if err == nil {
		t.Fatal("invalid history insert succeeded")
	}
	fetched, err := trips.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	state := fetched.Snapshot()
	if state.Status != trip.StatusActive || state.FinishedAt != nil || !state.UpdatedAt.Equal(created.Snapshot().UpdatedAt) {
		t.Fatalf("rollback did not restore trip: %+v", state)
	}
	var history int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id=$1", id).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 1 {
		t.Fatalf("history after rollback = %d, want 1", history)
	}
}

func TestHTTPDomainErrorsIntegration(t *testing.T) {
	pool := database(t)
	trips := newService(pool)
	router, err := newRouter(pool, trips)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	created, err := trips.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/trips/" + created.Snapshot().ID.String()
	for _, tc := range []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, path, http.StatusOK, ""},
		{http.MethodPost, path + "/finish", http.StatusOK, ""},
		{http.MethodPost, path + "/finish", http.StatusConflict, "trip_completed"},
		{http.MethodGet, "/api/v1/trips/" + uuid.NewString(), http.StatusNotFound, "trip_not_found"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil).WithContext(ctx)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Fatalf("%s %s: status %d, want %d", tc.method, tc.path, response.Code, tc.status)
		}
		if tc.code != "" && !strings.Contains(response.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatalf("missing problem code %s: %s", tc.code, response.Body.String())
		}
	}
}

func TestCorruptStoredTripReturnsInternalErrorIntegration(t *testing.T) {
	pool := database(t)
	trips := newService(pool)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	created, err := trips.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	id := created.Snapshot().ID
	if _, err := pool.Exec(ctx, "UPDATE trips SET user_id=$1 WHERE id=$2", uuid.Nil, id); err != nil {
		t.Fatal(err)
	}
	if _, err := trips.Get(ctx, id); !errors.Is(err, trip.ErrInvalidState) {
		t.Fatalf("corrupt state = %v", err)
	}
	router, err := newRouter(pool, trips)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil).WithContext(ctx))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"code":"internal_error"`) || strings.Contains(body, "user_id") || strings.Contains(body, "stored trip") {
		t.Fatalf("unsafe internal response: %s", body)
	}
}

func TestBothInsertsRollbackIntegration(t *testing.T) {
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
	forced := errors.New("abort after both inserts")
	err = tx.Do(ctx, func(ctx context.Context) error {
		if _, err := repo.Create(ctx, value); err != nil {
			return err
		}
		if err := repo.AddStatusChange(ctx, id, nil, trip.StatusActive, at); err != nil {
			return err
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("transaction error = %v", err)
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
}

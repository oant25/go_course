//go:build integration

package trip_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationRoundTripIntegration(t *testing.T) {
	pool := database(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	names := []string{"00001_create_trips.sql", "00002_create_trip_status_history.sql"}
	up := make([]string, 0, len(names))
	down := make([]string, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		before, after, ok := strings.Cut(string(data), "-- +goose Down")
		if !ok {
			t.Fatalf("no Down in %s", name)
		}
		up = append(up, before)
		down = append(down, after)
	}
	for i := len(down) - 1; i >= 0; i-- {
		if _, err := pool.Exec(ctx, down[i]); err != nil {
			t.Fatalf("down %s: %v", names[i], err)
		}
	}
	var remaining int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname=current_schema() AND tablename IN ('trips','trip_status_history')").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("tables survived down: %d", remaining)
	}
	for i, sql := range up {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("up %s: %v", names[i], err)
		}
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname=current_schema() AND tablename IN ('trips','trip_status_history')").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("tables after up: %d", remaining)
	}
	if _, err := newService(pool).Create(ctx, input()); err != nil {
		t.Fatalf("reapplied schema unusable: %v", err)
	}
}

package trip_test

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/oant25/go_course/internal/domain/trip"
)

func validDetails() trip.Details {
	return trip.Details{
		UserID:   uuid.MustParse("10000000-0000-0000-0000-000000000001"),
		DriverID: uuid.MustParse("20000000-0000-0000-0000-000000000002"),
		Start:    trip.Point{Latitude: 55.75, Longitude: 37.61},
		End:      trip.Point{Latitude: 59.93, Longitude: 30.31},
		Price:    1450,
	}
}

func exampleTrip(t *testing.T) trip.Trip {
	t.Helper()
	value, err := trip.New(uuid.MustParse("30000000-0000-0000-0000-000000000003"), validDetails(),
		time.Date(2026, time.November, 10, 12, 30, 0, 0, time.FixedZone("MSK", 3*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestNew(t *testing.T) {
	t.Parallel()
	value := exampleTrip(t)
	got := value.Snapshot()
	details := validDetails()
	at := time.Date(2026, time.November, 10, 9, 30, 0, 0, time.UTC)
	want := trip.State{
		ID:     uuid.MustParse("30000000-0000-0000-0000-000000000003"),
		UserID: details.UserID, DriverID: details.DriverID,
		Start: details.Start, End: details.End, Price: details.Price,
		Status: trip.StatusActive, StartedAt: at, CreatedAt: at, UpdatedAt: at,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("new trip = %+v, want %+v", got, want)
	}
}

func TestDetailsValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*trip.Details)
		field  string
	}{
		{"valid", func(_ *trip.Details) {}, ""},
		{"zero price", func(d *trip.Details) { d.Price = 0 }, ""},
		{"maximum price", func(d *trip.Details) { d.Price = math.MaxInt64 }, ""},
		{"same user and driver", func(d *trip.Details) { d.DriverID = d.UserID }, ""},
		{"nil user", func(d *trip.Details) { d.UserID = uuid.Nil }, "user_id"},
		{"nil driver", func(d *trip.Details) { d.DriverID = uuid.Nil }, "driver_id"},
		{"negative price", func(d *trip.Details) { d.Price = -1 }, "price"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			details := validDetails()
			tc.change(&details)
			assertValidation(t, details.Validate(), tc.field)
			value, err := trip.New(uuid.MustParse("30000000-0000-0000-0000-000000000003"), details, time.Time{})
			assertValidation(t, err, tc.field)
			if tc.field != "" && !reflect.DeepEqual(value, trip.Trip{}) {
				t.Fatal("invalid input returned a usable trip")
			}
		})
	}
}

func TestCoordinateValidation(t *testing.T) {
	t.Parallel()
	fields := []struct {
		name  string
		limit float64
		set   func(*trip.Details, float64)
	}{
		{"start_point.latitude", 90, func(d *trip.Details, v float64) { d.Start.Latitude = v }},
		{"start_point.longitude", 180, func(d *trip.Details, v float64) { d.Start.Longitude = v }},
		{"end_point.latitude", 90, func(d *trip.Details, v float64) { d.End.Latitude = v }},
		{"end_point.longitude", 180, func(d *trip.Details, v float64) { d.End.Longitude = v }},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			for _, value := range []float64{-field.limit, 0, field.limit} {
				details := validDetails()
				field.set(&details, value)
				assertValidation(t, details.Validate(), "")
			}
			for _, value := range []float64{
				math.Nextafter(-field.limit, math.Inf(-1)), math.Nextafter(field.limit, math.Inf(1)),
				math.NaN(), math.Inf(-1), math.Inf(1),
			} {
				details := validDetails()
				field.set(&details, value)
				assertValidation(t, details.Validate(), field.name)
			}
		})
	}
}

func assertValidation(t *testing.T, err error, field string) {
	t.Helper()
	if field == "" {
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		return
	}
	var validation *trip.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error %v is not ValidationError", err)
	}
	if validation.Field != field || validation.Reason == "" {
		t.Fatalf("validation = %+v, want field %s and a reason", validation, field)
	}
}

func TestNewRejectsNilID(t *testing.T) {
	t.Parallel()
	_, err := trip.New(uuid.Nil, validDetails(), time.Time{})
	assertValidation(t, err, "id")
}

func TestFinish(t *testing.T) {
	t.Parallel()
	value := exampleTrip(t)
	before := value.Snapshot()
	at := before.StartedAt.Add(40 * time.Minute).In(time.FixedZone("MSK", 3*60*60))
	if err := value.Finish(at); err != nil {
		t.Fatal(err)
	}
	want := before
	utc := at.UTC()
	want.Status, want.FinishedAt, want.UpdatedAt = trip.StatusCompleted, &utc, utc
	if got := value.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("finished trip = %+v, want %+v", got, want)
	}
	if err := value.Finish(at.Add(time.Hour)); !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("second finish = %v, want ErrTripCompleted", err)
	}
	if got := value.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatal("second finish mutated the trip")
	}
}

func TestFinishRejectsZeroValue(t *testing.T) {
	t.Parallel()
	var value trip.Trip
	if err := value.Finish(time.Now()); !errors.Is(err, trip.ErrInvalidState) {
		t.Fatalf("finish zero trip = %v, want ErrInvalidState", err)
	}
	if !reflect.DeepEqual(value, trip.Trip{}) {
		t.Fatal("invalid state was mutated")
	}
}

func TestRestore(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*trip.State)
		field  string
	}{
		{"active", func(_ *trip.State) {}, ""},
		{"completed", func(s *trip.State) { s.Status = trip.StatusCompleted; at := s.UpdatedAt; s.FinishedAt = &at }, ""},
		{"nil id", func(s *trip.State) { s.ID = uuid.Nil }, "id"},
		{"nil user", func(s *trip.State) { s.UserID = uuid.Nil }, "user_id"},
		{"nil driver", func(s *trip.State) { s.DriverID = uuid.Nil }, "driver_id"},
		{"negative price", func(s *trip.State) { s.Price = -1 }, "price"},
		{"invalid coordinate", func(s *trip.State) { s.End.Latitude = math.NaN() }, "end_point.latitude"},
		{"unknown status", func(s *trip.State) { s.Status = "cancelled" }, "status"},
		{"active with finish time", func(s *trip.State) { at := s.UpdatedAt; s.FinishedAt = &at }, "finished_at"},
		{"completed without finish time", func(s *trip.State) { s.Status = trip.StatusCompleted }, "finished_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := exampleTrip(t).Snapshot()
			tc.change(&state)
			value, err := trip.Restore(state)
			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(value.Snapshot(), state) {
					t.Fatal("restoration changed valid data")
				}
				return
			}
			wrapped := fmt.Errorf("read database row: %w", err)
			if !errors.Is(wrapped, trip.ErrInvalidState) {
				t.Fatalf("error = %v, want invalid state", wrapped)
			}
			var corrupt *trip.StateError
			if !errors.As(wrapped, &corrupt) {
				t.Fatalf("error = %v, want StateError", wrapped)
			}
			assertValidation(t, wrapped, tc.field)
			if errors.Is(wrapped, trip.ErrTripCompleted) {
				t.Fatal("corrupt data became a business conflict")
			}
			if !reflect.DeepEqual(value, trip.Trip{}) {
				t.Fatal("restore returned corrupt state")
			}
		})
	}
}

func TestSnapshotAndRestoreOwnTheirData(t *testing.T) {
	t.Parallel()
	value := exampleTrip(t)
	if err := value.Finish(value.Snapshot().StartedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	want := value.Snapshot()
	snapshot := value.Snapshot()
	snapshot.Status = trip.StatusActive
	snapshot.Price = -1
	*snapshot.FinishedAt = time.Time{}
	if !reflect.DeepEqual(value.Snapshot(), want) {
		t.Fatal("mutating snapshot changed original")
	}

	input := value.Snapshot()
	local := input.StartedAt.In(time.FixedZone("MSK", 3*60*60))
	input.StartedAt, input.CreatedAt, input.UpdatedAt = local, local, local
	finished := input.FinishedAt.In(local.Location())
	input.FinishedAt = &finished
	restored, err := trip.Restore(input)
	if err != nil {
		t.Fatal(err)
	}
	got := restored.Snapshot()
	if got.StartedAt.Location() != time.UTC || got.CreatedAt.Location() != time.UTC ||
		got.UpdatedAt.Location() != time.UTC || got.FinishedAt.Location() != time.UTC {
		t.Fatal("restored timestamps were not normalized to UTC")
	}
	*input.FinishedAt = time.Time{}
	if !reflect.DeepEqual(restored.Snapshot(), got) {
		t.Fatal("restore retained caller-owned pointer")
	}
}

func TestValidationMessageDoesNotIncludeValue(t *testing.T) {
	t.Parallel()
	details := validDetails()
	details.Price = -123456789
	err := details.Validate()
	if err == nil || err.Error() != "price: must be non-negative" {
		t.Fatalf("unsafe or unexpected validation message: %v", err)
	}
}

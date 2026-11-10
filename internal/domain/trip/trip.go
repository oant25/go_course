package trip

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Point struct {
	Latitude  float64
	Longitude float64
}

type State struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    Point
	End      Point
	Price    int64

	Status     Status
	StartedAt  time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Trip struct {
	state State
}

func New(id uuid.UUID, details Details, at time.Time) (Trip, error) {
	if id == uuid.Nil {
		return Trip{}, &ValidationError{Field: "id", Reason: nonEmptyUUIDReason}
	}
	if err := details.Validate(); err != nil {
		return Trip{}, fmt.Errorf("validate trip: %w", err)
	}
	at = at.UTC()
	return Trip{state: State{
		ID: id, UserID: details.UserID, DriverID: details.DriverID,
		Start: details.Start, End: details.End, Price: details.Price,
		Status: StatusActive, StartedAt: at, CreatedAt: at, UpdatedAt: at,
	}}, nil
}

func Restore(state State) (Trip, error) {
	if err := state.validate(); err != nil {
		return Trip{}, &StateError{cause: err}
	}
	state.StartedAt = state.StartedAt.UTC()
	state.CreatedAt = state.CreatedAt.UTC()
	state.UpdatedAt = state.UpdatedAt.UTC()
	state.FinishedAt = copyTime(state.FinishedAt)
	return Trip{state: state}, nil
}

func (t Trip) Snapshot() State {
	state := t.state
	state.FinishedAt = copyTime(state.FinishedAt)
	return state
}

func (t *Trip) Finish(at time.Time) error {
	if err := t.state.validate(); err != nil {
		return &StateError{cause: err}
	}
	if t.state.Status != StatusActive {
		return ErrTripCompleted
	}
	at = at.UTC()
	t.state.Status = StatusCompleted
	t.state.FinishedAt = &at
	t.state.UpdatedAt = at
	return nil
}

func (s State) validate() error {
	if s.ID == uuid.Nil {
		return &ValidationError{Field: "id", Reason: nonEmptyUUIDReason}
	}
	if err := (Details{UserID: s.UserID, DriverID: s.DriverID, Start: s.Start, End: s.End, Price: s.Price}).Validate(); err != nil {
		return err
	}
	switch s.Status {
	case StatusActive:
		if s.FinishedAt != nil {
			return &ValidationError{Field: "finished_at", Reason: "must be absent for an active trip"}
		}
	case StatusCompleted:
		if s.FinishedAt == nil {
			return &ValidationError{Field: "finished_at", Reason: "is required for a completed trip"}
		}
	default:
		return &ValidationError{Field: "status", Reason: "must be active or completed"}
	}
	return nil
}

func copyTime(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	copied := at.UTC()
	return &copied
}

const (
	nonEmptyUUIDReason = "must be a non-empty UUID"
)

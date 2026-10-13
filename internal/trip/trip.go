package trip

import (
	"errors"
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

type Trip struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	DriverID uuid.UUID

	Start Point
	End   Point

	Price int64

	Status     Status
	StartedAt  time.Time
	FinishedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	ErrNotFound         = errors.New("trip not found")
	ErrAlreadyCompleted = errors.New("trip already completed")
	ErrDriverBusy       = errors.New("driver already has an active trip")
)

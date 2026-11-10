package model

import (
	"github.com/google/uuid"

	"github.com/oant25/go_course/internal/domain/trip"
)

type CreateTripInput struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    trip.Point
	End      trip.Point
	Price    int64
}

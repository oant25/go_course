//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/oant25/go_course/internal/domain/trip"
)

type tripRepository interface {
	Create(context.Context, trip.Trip) (trip.Trip, error)
	AddStatusChange(context.Context, uuid.UUID, *trip.Status, trip.Status, time.Time) error
}

type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type (
	clock       interface{ Now() time.Time }
	idGenerator interface{ New() uuid.UUID }
)

type recorder interface {
	TripCreated(context.Context, uuid.UUID)
}

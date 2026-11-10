//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/oant25/go_course/internal/domain/trip"
)

type usecase interface {
	Execute(context.Context, uuid.UUID) (trip.Trip, error)
}

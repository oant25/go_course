//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package create

import (
	"context"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/model"
)

type usecase interface {
	Execute(context.Context, model.CreateTripInput) (trip.Trip, error)
}

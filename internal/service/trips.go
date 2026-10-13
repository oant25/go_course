package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/oant25/go_course/internal/postgres"
	"github.com/oant25/go_course/internal/trip"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type Trips struct {
	repo *postgres.TripRepo
	tx   TxManager
}

func NewTrips(repo *postgres.TripRepo, tx TxManager) *Trips {
	return &Trips{repo: repo, tx: tx}
}

type CreateInput struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    trip.Point
	End      trip.Point
	Price    int64
}

func (s *Trips) Create(ctx context.Context, in CreateInput) (trip.Trip, error) {
	now := time.Now().UTC()
	t := trip.Trip{
		ID:        uuid.New(),
		UserID:    in.UserID,
		DriverID:  in.DriverID,
		Start:     in.Start,
		End:       in.End,
		Price:     in.Price,
		Status:    trip.StatusActive,
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	var created trip.Trip
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		created, err = s.repo.Create(ctx, t)
		if err != nil {
			return err
		}
		return s.repo.AddStatusChange(ctx, created.ID, nil, trip.StatusActive, now)
	})
	if err != nil {
		return trip.Trip{}, err
	}
	return created, nil
}

func (s *Trips) Get(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Trips) Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	now := time.Now().UTC()
	from := trip.StatusActive

	var finished trip.Trip
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		finished, err = s.repo.Finish(ctx, id, now)
		if err != nil {
			return err
		}
		return s.repo.AddStatusChange(ctx, id, &from, trip.StatusCompleted, now)
	})
	if err != nil {
		return trip.Trip{}, err
	}
	return finished, nil
}

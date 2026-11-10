package create

import (
	"context"
	"fmt"

	"github.com/oant25/go_course/internal/domain/trip"
	"github.com/oant25/go_course/internal/model"
)

type Usecase struct {
	events recorder
	repo   tripRepository
	tx     txManager
	clock  clock
	ids    idGenerator
}

func New(repo tripRepository, tx txManager, clock clock, ids idGenerator, events recorder) *Usecase {
	return &Usecase{repo: repo, tx: tx, clock: clock, ids: ids, events: events}
}

func (u *Usecase) Execute(ctx context.Context, in model.CreateTripInput) (trip.Trip, error) {
	details := trip.Details{UserID: in.UserID, DriverID: in.DriverID, Start: in.Start, End: in.End, Price: in.Price}
	if err := details.Validate(); err != nil {
		return trip.Trip{}, fmt.Errorf("validate create input: %w", err)
	}
	now := u.clock.Now().UTC()
	value, err := trip.New(u.ids.New(), details, now)
	if err != nil {
		return trip.Trip{}, fmt.Errorf("construct trip: %w", err)
	}
	var created trip.Trip
	if err := u.tx.Do(ctx, func(txCtx context.Context) error {
		var err error
		created, err = u.repo.Create(txCtx, value)
		if err != nil {
			return fmt.Errorf("insert trip: %w", err)
		}
		if err := u.repo.AddStatusChange(txCtx, created.Snapshot().ID, nil, trip.StatusActive, now); err != nil {
			return fmt.Errorf("record trip creation: %w", err)
		}
		return nil
	}); err != nil {
		return trip.Trip{}, fmt.Errorf("create trip transaction: %w", err)
	}
	u.events.TripCreated(ctx, created.Snapshot().ID)
	return created, nil
}

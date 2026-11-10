package finish

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/oant25/go_course/internal/domain/trip"
)

type Usecase struct {
	events recorder
	repo   tripRepository
	tx     txManager
	clock  clock
}

func New(repo tripRepository, tx txManager, clock clock, events recorder) *Usecase {
	return &Usecase{repo: repo, tx: tx, clock: clock, events: events}
}

func (u *Usecase) Execute(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	var finished trip.Trip
	if err := u.tx.Do(ctx, func(txCtx context.Context) error {
		value, err := u.repo.GetByIDForUpdate(txCtx, id)
		if err != nil {
			return fmt.Errorf("load trip to finish: %w", err)
		}
		now := u.clock.Now().UTC()
		if err := value.Finish(now); err != nil {
			return fmt.Errorf("finish trip: %w", err)
		}
		finished, err = u.repo.SaveFinished(txCtx, value)
		if err != nil {
			return fmt.Errorf("save finished trip: %w", err)
		}
		from := trip.StatusActive
		if err := u.repo.AddStatusChange(txCtx, id, &from, trip.StatusCompleted, now); err != nil {
			return fmt.Errorf("record trip completion: %w", err)
		}
		return nil
	}); err != nil {
		return trip.Trip{}, fmt.Errorf("finish trip transaction: %w", err)
	}
	u.events.TripCompleted(ctx, finished.Snapshot().ID)
	return finished, nil
}

package finish

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
)

type Usecase struct {
	repository tripRepository
	tx         txManager
	events     events
}

func New(repository tripRepository, tx txManager, events events) *Usecase {
	return &Usecase{repository: repository, tx: tx, events: events}
}

func (service *Usecase) Execute(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	var trip domain.Trip
	err := service.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		trip, err = service.repository.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		previousStatus := trip.Status
		if err := trip.Finish(time.Now()); err != nil {
			return err
		}
		if err := service.repository.Complete(ctx, id, *trip.FinishedAt); err != nil {
			return err
		}
		return service.repository.AddStatusChange(ctx, id, &previousStatus, trip.Status, domain.ReasonCompleted, *trip.FinishedAt)
	})
	if err != nil {
		return domain.Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	service.events.Completed(ctx, trip.Id)
	return trip, nil
}

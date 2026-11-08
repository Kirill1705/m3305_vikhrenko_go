package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
)

type Usecase struct{ repository tripRepository }

func New(repository tripRepository) *Usecase { return &Usecase{repository: repository} }
func (service *Usecase) Execute(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	trip, err := service.repository.Get(ctx, id)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return trip, nil
}

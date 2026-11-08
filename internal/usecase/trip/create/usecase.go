package create

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/model"
)

type Usecase struct {
	repository     tripRepository
	tx             txManager
	events         events
	idempotencyTTL time.Duration
}

func New(repository tripRepository, tx txManager, events events, idempotencyTTL time.Duration) *Usecase {
	return &Usecase{repository: repository, tx: tx, events: events, idempotencyTTL: idempotencyTTL}
}

func (service *Usecase) Execute(ctx context.Context, data domain.TripData, key *uuid.UUID) (model.CreateResult, error) {
	if err := domain.Validate(data); err != nil {
		return model.CreateResult{}, err
	}
	var requestHash [sha256.Size]byte
	if key != nil {
		canonicalData := data
		if canonicalData.StartPoint.Latitude == 0 {
			canonicalData.StartPoint.Latitude = 0
		}
		if canonicalData.StartPoint.Longitude == 0 {
			canonicalData.StartPoint.Longitude = 0
		}
		if canonicalData.EndPoint.Latitude == 0 {
			canonicalData.EndPoint.Latitude = 0
		}
		if canonicalData.EndPoint.Longitude == 0 {
			canonicalData.EndPoint.Longitude = 0
		}
		canonicalRequest, err := json.Marshal(canonicalData)
		if err != nil {
			return model.CreateResult{}, fmt.Errorf("encode idempotency request: %w", err)
		}
		requestHash = sha256.Sum256(canonicalRequest)
	}
	var trip domain.Trip
	var replayed bool
	err := service.tx.Do(ctx, func(ctx context.Context) error {
		if key != nil {
			response, claimed, err := service.repository.ClaimIdempotency(ctx, *key, requestHash[:], service.idempotencyTTL)
			if err != nil {
				return err
			}
			if !claimed {
				if err := json.Unmarshal(response, &trip); err != nil {
					return fmt.Errorf("decode idempotency response: %w", err)
				}
				replayed = true
				return nil
			}
		}
		var err error
		trip, err = service.create(ctx, data)
		if err != nil {
			return err
		}
		if key != nil {
			response, err := json.Marshal(trip)
			if err != nil {
				return fmt.Errorf("encode idempotency response: %w", err)
			}
			return service.repository.SaveIdempotencyResponse(ctx, *key, response, service.idempotencyTTL)
		}
		return nil
	})
	if err != nil {
		return model.CreateResult{}, fmt.Errorf("create trip: %w", err)
	}
	if !replayed {
		service.events.Created(ctx, trip.Id)
	}
	return model.CreateResult{Trip: trip, Replayed: replayed}, nil
}

func (service *Usecase) create(ctx context.Context, data domain.TripData) (domain.Trip, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("generate trip id: %w", err)
	}
	trip := domain.Trip{
		Id:         id,
		UserId:     data.UserId,
		DriverId:   data.DriverId,
		StartPoint: data.StartPoint,
		EndPoint:   data.EndPoint,
		Price:      data.Price,
		Status:     domain.StatusActive,
		StartedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := service.repository.Insert(ctx, trip); err != nil {
		return domain.Trip{}, err
	}
	if err := service.repository.AddStatusChange(ctx, trip.Id, nil, trip.Status, domain.ReasonCreated, trip.StartedAt); err != nil {
		return domain.Trip{}, err
	}
	return trip, nil
}

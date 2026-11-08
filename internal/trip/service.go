package trip

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Kirill1705/m3305_vikhrenko_go/api"
	"github.com/google/uuid"
)

type TxManager interface {
	Do(context.Context, func(context.Context) error) error
}

type Service struct {
	repository     *Repository
	tx             TxManager
	idempotencyTTL time.Duration
}

func NewService(repository *Repository, tx TxManager, idempotencyTTL time.Duration) *Service {
	return &Service{repository: repository, tx: tx, idempotencyTTL: idempotencyTTL}
}

func (service *Service) Create(ctx context.Context, data api.TripData, key *uuid.UUID) (api.Trip, bool, error) {
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
			return api.Trip{}, false, fmt.Errorf("encode idempotency request: %w", err)
		}
		requestHash = sha256.Sum256(canonicalRequest)
	}
	var trip api.Trip
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
		return api.Trip{}, false, fmt.Errorf("create trip: %w", err)
	}
	return trip, replayed, nil
}

func (service *Service) create(ctx context.Context, data api.TripData) (api.Trip, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return api.Trip{}, fmt.Errorf("generate trip id: %w", err)
	}
	trip := api.Trip{
		Id:         id,
		UserId:     data.UserId,
		DriverId:   data.DriverId,
		StartPoint: data.StartPoint,
		EndPoint:   data.EndPoint,
		Price:      data.Price,
		Status:     api.TripStatus("active"),
		StartedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}
	trip.FinishedAt.SetNull()
	trip.LastPositionAt.SetNull()
	if err := service.repository.Insert(ctx, trip); err != nil {
		return api.Trip{}, err
	}
	if err := service.repository.AddStatusChange(ctx, trip.Id, nil, trip.Status, "Trip created", trip.StartedAt); err != nil {
		return api.Trip{}, err
	}
	return trip, nil
}

func (service *Service) Get(ctx context.Context, id uuid.UUID) (api.Trip, error) {
	trip, err := service.repository.Get(ctx, id)
	if err != nil {
		return api.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return trip, nil
}

func (service *Service) Finish(ctx context.Context, id uuid.UUID) (api.Trip, error) {
	var trip api.Trip
	err := service.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		trip, err = service.repository.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if trip.Status == api.TripStatus("completed") {
			return ErrTripCompleted
		}
		finishedAt := time.Now().UTC().Truncate(time.Microsecond)
		if err := service.repository.Complete(ctx, id, finishedAt); err != nil {
			return err
		}
		previousStatus := trip.Status
		trip.Status = api.TripStatus("completed")
		trip.FinishedAt.Set(finishedAt)
		return service.repository.AddStatusChange(ctx, id, &previousStatus, trip.Status, "Trip completed", finishedAt)
	})
	if err != nil {
		return api.Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	return trip, nil
}

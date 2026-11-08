package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/usecase/trip/create"
)

const conflictCase = "claim conflict"

const (
	caseDriverBusy = "driver busy"
	caseReplay     = "replay"
	caseInvalid    = "invalid"
)

func TestCreate(t *testing.T) {
	for _, name := range []string{"success", "key success", caseDriverBusy, "history failure", caseReplay, conflictCase, caseInvalid} {
		t.Run(name, func(t *testing.T) {
			controller := gomock.NewController(t)
			repo := NewMocktripRepository(controller)
			tx := NewMocktxManager(controller)
			events := NewMockevents(controller)
			usecase := create.New(repo, tx, events, time.Hour)
			data := domain.TripData{UserId: uuid.New(), DriverId: uuid.New(), Price: 1450}
			var key *uuid.UUID
			var expected error
			if name == caseInvalid {
				data.Price = -1
			} else {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
			}
			if name == "key success" {
				value := uuid.New()
				key = &value
				repo.EXPECT().ClaimIdempotency(gomock.Any(), value, gomock.Any(), time.Hour).Return(nil, true, nil)
				repo.EXPECT().SaveIdempotencyResponse(gomock.Any(), value, gomock.Any(), time.Hour).Return(nil)
			}
			if name == caseReplay || name == conflictCase {
				value := uuid.New()
				key = &value
				stored, _ := json.Marshal(domain.Trip{Id: value, Status: domain.StatusActive})
				if name == conflictCase {
					expected = domain.ErrIdempotencyConflict
				}
				repo.EXPECT().ClaimIdempotency(gomock.Any(), value, gomock.Any(), time.Hour).Return(stored, false, expected)
			} else if name != caseInvalid {
				if name == caseDriverBusy {
					expected = domain.ErrDriverBusy
				}
				repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, value domain.Trip) error {
					if value.Id == uuid.Nil || value.Status != domain.StatusActive || value.UserId != data.UserId || value.StartedAt.IsZero() {
						t.Fatalf("incorrect trip: %+v", value)
					}
					if name == caseDriverBusy {
						return domain.ErrDriverBusy
					}
					return nil
				})
				if name != caseDriverBusy {
					var historyError error
					if name == "history failure" {
						historyError = errors.New("history unavailable")
						expected = historyError
					}
					repo.EXPECT().AddStatusChange(gomock.Any(), gomock.Any(), nil, domain.StatusActive, domain.ReasonCreated, gomock.Any()).Return(historyError)
					if historyError == nil {
						events.EXPECT().Created(gomock.Any(), gomock.Any())
					}
				}
			}
			result, err := usecase.Execute(context.Background(), data, key)
			if name == caseInvalid {
				var invalid *domain.ValidationError
				if !errors.As(err, &invalid) {
					t.Fatalf("expected validation error: %v", err)
				}
				return
			}
			if !errors.Is(err, expected) {
				t.Fatalf("got %v, want %v", err, expected)
			}
			if expected == nil && (result.Trip.Id == uuid.Nil || result.Replayed != (name == caseReplay)) {
				t.Fatalf("incorrect result: %+v", result)
			}
		})
	}
}

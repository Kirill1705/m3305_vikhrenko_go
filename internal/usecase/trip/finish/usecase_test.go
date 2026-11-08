package finish_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/usecase/trip/finish"
)

const updateFailureCase = "update failure"

func TestFinish(t *testing.T) {
	for _, name := range []string{"success", "completed", "missing", updateFailureCase} {
		t.Run(name, func(t *testing.T) {
			controller := gomock.NewController(t)
			repo := NewMocktripRepository(controller)
			tx := NewMocktxManager(controller)
			events := NewMockevents(controller)
			value := domain.Trip{Id: uuid.New(), Status: domain.StatusActive}
			var expected error
			if name == "completed" {
				value.Status = domain.StatusCompleted
				expected = domain.ErrTripCompleted
			}
			var fetchError error
			if name == "missing" {
				fetchError = domain.ErrTripNotFound
				expected = fetchError
			}
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
			repo.EXPECT().GetForUpdate(gomock.Any(), value.Id).Return(value, fetchError)
			if name == "success" || name == updateFailureCase {
				if name == updateFailureCase {
					expected = errors.New("update failed")
				}
				repo.EXPECT().Complete(gomock.Any(), value.Id, gomock.Any()).DoAndReturn(func(_ context.Context, _ uuid.UUID, at time.Time) error {
					if at.IsZero() {
						t.Fatal("missing finish timestamp")
					}
					return expected
				})
				if expected == nil {
					repo.EXPECT().AddStatusChange(gomock.Any(), value.Id, gomock.Any(), domain.StatusCompleted, domain.ReasonCompleted, gomock.Any()).Return(nil)
					events.EXPECT().Completed(gomock.Any(), value.Id)
				}
			}
			result, err := finish.New(repo, tx, events).Execute(context.Background(), value.Id)
			if !errors.Is(err, expected) {
				t.Fatalf("got %v, want %v", err, expected)
			}
			if err == nil && (result.Status != domain.StatusCompleted || result.FinishedAt == nil) {
				t.Fatalf("incorrect completed trip: %+v", result)
			}
		})
	}
}

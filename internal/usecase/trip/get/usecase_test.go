package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/usecase/trip/get"
)

func TestGet(t *testing.T) {
	for _, expected := range []error{nil, domain.ErrTripNotFound, errors.New("database unavailable")} {
		controller := gomock.NewController(t)
		repo := NewMocktripRepository(controller)
		id := uuid.New()
		repo.EXPECT().Get(gomock.Any(), id).Return(domain.Trip{Id: id}, expected)
		result, err := get.New(repo).Execute(context.Background(), id)
		if !errors.Is(err, expected) {
			t.Fatalf("got %v, want %v", err, expected)
		}
		if expected == nil && result.Id != id {
			t.Fatal("wrong trip")
		}
	}
}

package get_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/trip/get"
)

func TestHandler(t *testing.T) {
	for _, failure := range []error{nil, domain.ErrTripNotFound} {
		mock := NewMockusecase(gomock.NewController(t))
		id := uuid.New()
		mock.EXPECT().Execute(gomock.Any(), id).Return(domain.Trip{Id: id}, failure)
		writer := httptest.NewRecorder()
		get.New(mock).Handle(writer, httptest.NewRequest(http.MethodGet, "/", nil), id)
		status := http.StatusOK
		if failure != nil {
			status = http.StatusNotFound
		}
		if writer.Code != status {
			t.Fatalf("got %d: %s", writer.Code, writer.Body.String())
		}
		if failure != nil && writer.Header().Get("Content-Type") != "application/problem+json" {
			t.Fatal("incorrect content type")
		}
	}
}

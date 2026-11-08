package domainerror_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&domain.ValidationError{Field: "price", Reason: "negative"}, http.StatusBadRequest, domainerror.InvalidRequest},
		{domain.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{domain.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{domain.ErrDriverBusy, http.StatusConflict, "driver_busy"},
		{domain.ErrIdempotencyConflict, http.StatusConflict, "idempotency_conflict"},
		{errors.New("private database password"), http.StatusInternalServerError, domainerror.InternalError},
	}
	for _, test := range cases {
		status, body := domainerror.Problem(fmt.Errorf("wrapped: %w", test.err))
		if status != test.status || int(body.Status) != status || body.Code != test.code {
			t.Fatalf("incorrect mapping: %d %+v", status, body)
		}
		if status == http.StatusInternalServerError && (strings.Contains(body.Title, "private") || body.Detail == nil || strings.Contains(*body.Detail, "private")) {
			t.Fatal("private details leaked")
		}
	}
}

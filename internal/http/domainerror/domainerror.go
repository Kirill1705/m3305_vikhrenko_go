package domainerror

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kirill1705/m3305_vikhrenko_go/api"
	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
)

const (
	InvalidRequest = "invalid_request"
	InternalError  = "internal_error"
	ProblemBaseURI = "https://tripgo.example/problems/"
)

func Problem(err error) (int, api.Problem) {
	status := http.StatusInternalServerError
	code := InternalError
	title := "Internal Server Error"
	detail := "Internal server error"
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		status = http.StatusBadRequest
		code = InvalidRequest
		title = "Invalid request"
		detail = validation.Error()
	case errors.Is(err, domain.ErrTripNotFound):
		status = http.StatusNotFound
		code = "trip_not_found"
		title = "Trip not found"
		detail = "Trip was not found"
	case errors.Is(err, domain.ErrTripCompleted):
		status = http.StatusConflict
		code = "trip_completed"
		title = "Trip completed"
		detail = "Operation is not allowed for a completed trip"
	case errors.Is(err, domain.ErrDriverBusy):
		status = http.StatusConflict
		code = "driver_busy"
		title = "Driver busy"
		detail = "Driver already has an active trip"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		status = http.StatusConflict
		code = "idempotency_conflict"
		title = "Idempotency conflict"
		detail = "Idempotency-Key was already used with a different request body"
	}
	return status, api.Problem{Type: ProblemBaseURI + strings.ReplaceAll(code, "_", "-"), Title: title, Status: int32(status), Code: code, Detail: &detail}
}

package response

import (
	"encoding/json"
	"net/http"

	"github.com/Kirill1705/m3305_vikhrenko_go/api"
	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/domainerror"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/requestinfo"
)

const (
	JSONType    = "application/json"
	ProblemType = "application/problem+json"
)

func JSON(w http.ResponseWriter, r *http.Request, status int, contentType string, value any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		if info := requestinfo.From(r.Context()); info != nil {
			info.Err = err
		}
	}
}

func Error(w http.ResponseWriter, r *http.Request, err error) {
	status, body := domainerror.Problem(err)
	if info := requestinfo.From(r.Context()); info != nil {
		info.Err = err
		info.ErrorType = body.Code
	}
	instance := r.URL.Path
	body.Instance = &instance
	JSON(w, r, status, ProblemType, body)
}

func Invalid(w http.ResponseWriter, r *http.Request) {
	Error(w, r, &domain.ValidationError{Field: "request", Reason: "validation failed"})
}

func Trip(value domain.Trip) api.Trip {
	result := api.Trip{Id: value.Id, UserId: value.UserId, DriverId: value.DriverId, StartPoint: api.Coordinates{Latitude: value.StartPoint.Latitude, Longitude: value.StartPoint.Longitude}, EndPoint: api.Coordinates{Latitude: value.EndPoint.Latitude, Longitude: value.EndPoint.Longitude}, Price: value.Price, Status: api.TripStatus(value.Status), StartedAt: value.StartedAt}
	result.FinishedAt.SetNull()
	if value.FinishedAt != nil {
		result.FinishedAt.Set(*value.FinishedAt)
	}
	result.LastPositionAt.SetNull()
	if value.LastPositionAt != nil {
		result.LastPositionAt.Set(*value.LastPositionAt)
	}
	return result
}

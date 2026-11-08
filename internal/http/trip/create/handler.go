package create

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Kirill1705/m3305_vikhrenko_go/api"
	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/response"
)

type Handler struct{ usecase usecase }

func New(usecase usecase) *Handler { return &Handler{usecase: usecase} }
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var input api.CreateTripJSONRequestBody
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.Invalid(w, r)
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		response.Invalid(w, r)
		return
	}
	data := domain.TripData{DriverId: input.DriverId, UserId: input.UserId, Price: input.Price, StartPoint: domain.Coordinates{Latitude: input.StartPoint.Latitude, Longitude: input.StartPoint.Longitude}, EndPoint: domain.Coordinates{Latitude: input.EndPoint.Latitude, Longitude: input.EndPoint.Longitude}}
	if err := domain.Validate(data); err != nil {
		response.Error(w, r, err)
		return
	}
	result, err := h.usecase.Execute(r.Context(), data, params.IdempotencyKey)
	if err != nil {
		response.Error(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+result.Trip.Id.String())
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, response.JSONType, response.Trip(result.Trip))
}

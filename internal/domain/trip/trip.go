package trip

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	ReasonCreated          = "Trip created"
	ReasonCompleted        = "Trip completed"
)

type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type TripData struct {
	DriverId   uuid.UUID   `json:"driver_id"`
	EndPoint   Coordinates `json:"end_point"`
	Price      int64       `json:"price"`
	StartPoint Coordinates `json:"start_point"`
	UserId     uuid.UUID   `json:"user_id"`
}

type Trip struct {
	DriverId       uuid.UUID   `json:"driver_id"`
	EndPoint       Coordinates `json:"end_point"`
	FinishedAt     *time.Time  `json:"finished_at"`
	Id             uuid.UUID   `json:"id"`
	LastPositionAt *time.Time  `json:"last_position_at"`
	Price          int64       `json:"price"`
	StartPoint     Coordinates `json:"start_point"`
	StartedAt      time.Time   `json:"started_at"`
	Status         Status      `json:"status"`
	UserId         uuid.UUID   `json:"user_id"`
}

func (trip *Trip) Finish(at time.Time) error {
	if trip.Status != StatusActive {
		return ErrTripCompleted
	}
	at = at.UTC().Truncate(time.Microsecond)
	trip.Status = StatusCompleted
	trip.FinishedAt = &at
	return nil
}

func Validate(data TripData) error {
	if data.UserId == uuid.Nil {
		return &ValidationError{Field: "user_id", Reason: "must be a nonzero UUID"}
	}
	if data.DriverId == uuid.Nil {
		return &ValidationError{Field: "driver_id", Reason: "must be a nonzero UUID"}
	}
	if data.Price < 0 {
		return &ValidationError{Field: "price", Reason: "must be nonnegative"}
	}
	if err := validateCoordinates("start_point", data.StartPoint); err != nil {
		return err
	}
	return validateCoordinates("end_point", data.EndPoint)
}

func validateCoordinates(field string, point Coordinates) error {
	if math.IsNaN(point.Latitude) || math.IsInf(point.Latitude, 0) || point.Latitude < -90 || point.Latitude > 90 {
		return &ValidationError{Field: field + ".latitude", Reason: "must be between -90 and 90"}
	}
	if math.IsNaN(point.Longitude) || math.IsInf(point.Longitude, 0) || point.Longitude < -180 || point.Longitude > 180 {
		return &ValidationError{Field: field + ".longitude", Reason: "must be between -180 and 180"}
	}
	return nil
}

type ValidationError struct {
	Field  string
	Reason string
}

func (err *ValidationError) Error() string { return fmt.Sprintf("%s: %s", err.Field, err.Reason) }

package trip_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
)

func TestValidation(t *testing.T) {
	valid := trip.TripData{UserId: uuid.New(), DriverId: uuid.New()}
	cases := []struct {
		name   string
		change func(*trip.TripData)
		field  string
	}{
		{"valid", func(_ *trip.TripData) {}, ""},
		{"user", func(data *trip.TripData) { data.UserId = uuid.Nil }, "user_id"},
		{"driver", func(data *trip.TripData) { data.DriverId = uuid.Nil }, "driver_id"},
		{"price", func(data *trip.TripData) { data.Price = -1 }, "price"},
		{"start latitude", func(data *trip.TripData) { data.StartPoint.Latitude = 91 }, "start_point.latitude"},
		{"start longitude", func(data *trip.TripData) { data.StartPoint.Longitude = -181 }, "start_point.longitude"},
		{"end latitude", func(data *trip.TripData) { data.EndPoint.Latitude = math.NaN() }, "end_point.latitude"},
		{"end longitude", func(data *trip.TripData) { data.EndPoint.Longitude = math.Inf(1) }, "end_point.longitude"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			data := valid
			test.change(&data)
			err := trip.Validate(data)
			if test.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var invalid *trip.ValidationError
			if !errors.As(err, &invalid) || invalid.Field != test.field || invalid.Error() == "" {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestFinish(t *testing.T) {
	value := trip.Trip{Status: trip.StatusActive}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := value.Finish(at); err != nil {
		t.Fatal(err)
	}
	if value.Status != trip.StatusCompleted || value.FinishedAt == nil || !value.FinishedAt.Equal(at) {
		t.Fatalf("incorrect transition: %+v", value)
	}
	if err := value.Finish(at.Add(time.Hour)); !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("second finish: %v", err)
	}
	if !value.FinishedAt.Equal(at) {
		t.Fatal("second finish overwrote timestamp")
	}
}

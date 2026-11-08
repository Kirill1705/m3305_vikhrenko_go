package model

import "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"

type CreateResult struct {
	Trip     trip.Trip
	Replayed bool
}

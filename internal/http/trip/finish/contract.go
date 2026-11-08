//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
)

type usecase interface {
	Execute(context.Context, uuid.UUID) (domain.Trip, error)
}

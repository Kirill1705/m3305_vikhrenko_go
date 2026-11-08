//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
)

type tripRepository interface {
	GetForUpdate(context.Context, uuid.UUID) (domain.Trip, error)
	Complete(context.Context, uuid.UUID, time.Time) error
	AddStatusChange(context.Context, uuid.UUID, *domain.Status, domain.Status, string, time.Time) error
}
type txManager interface {
	Do(context.Context, func(context.Context) error) error
}
type events interface {
	Completed(context.Context, uuid.UUID)
}

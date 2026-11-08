//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
)

type tripRepository interface {
	Insert(context.Context, domain.Trip) error
	AddStatusChange(context.Context, uuid.UUID, *domain.Status, domain.Status, string, time.Time) error
	ClaimIdempotency(context.Context, uuid.UUID, []byte, time.Duration) ([]byte, bool, error)
	SaveIdempotencyResponse(context.Context, uuid.UUID, []byte, time.Duration) error
}
type txManager interface {
	Do(context.Context, func(context.Context) error) error
}
type events interface {
	Created(context.Context, uuid.UUID)
}

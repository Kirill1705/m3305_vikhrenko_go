package trip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Kirill1705/m3305_vikhrenko_go/internal/database"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (repository *Repository) ClaimIdempotency(ctx context.Context, key uuid.UUID, requestHash []byte, ttl time.Duration) ([]byte, bool, error) {
	executor := database.FromContext(ctx)
	if executor == nil {
		return nil, false, errors.New("claim idempotency key requires a transaction")
	}
	query, args, err := sq.Insert("trip_idempotency_keys").
		Columns("key", "request_hash", "response", "expires_at").
		Values(key, requestHash, nil, sq.Expr("clock_timestamp() + make_interval(secs => ?)", ttl.Seconds())).
		Suffix("ON CONFLICT (key) DO UPDATE SET request_hash = EXCLUDED.request_hash, response = NULL, expires_at = EXCLUDED.expires_at WHERE trip_idempotency_keys.expires_at <= clock_timestamp() RETURNING key").
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, false, fmt.Errorf("build claim idempotency query: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, repository.queryTimeout)
	defer cancel()
	var claimedKey uuid.UUID
	err = executor.QueryRow(ctx, query, args...).Scan(&claimedKey)
	if err == nil {
		return nil, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("claim idempotency key: %w", err)
	}
	query, args, err = sq.Select("request_hash", "response").
		From("trip_idempotency_keys").Where(sq.Eq{"key": key}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, false, fmt.Errorf("build read idempotency query: %w", err)
	}
	var storedHash, response []byte
	if err := executor.QueryRow(ctx, query, args...).Scan(&storedHash, &response); err != nil {
		return nil, false, fmt.Errorf("read idempotency response: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash) {
		return nil, false, ErrIdempotencyConflict
	}
	if len(response) == 0 {
		return nil, false, errors.New("idempotency response is missing")
	}
	return response, false, nil
}

func (repository *Repository) SaveIdempotencyResponse(ctx context.Context, key uuid.UUID, response []byte, ttl time.Duration) error {
	executor := database.FromContext(ctx)
	if executor == nil {
		return errors.New("save idempotency response requires a transaction")
	}
	query, args, err := sq.Update("trip_idempotency_keys").
		Set("response", string(response)).
		Set("expires_at", sq.Expr("clock_timestamp() + make_interval(secs => ?)", ttl.Seconds())).
		Where(sq.Eq{"key": key, "response": nil}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build save idempotency response query: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, repository.queryTimeout)
	defer cancel()
	result, err := executor.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("save idempotency response: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("idempotency reservation is missing or already completed")
	}
	return nil
}

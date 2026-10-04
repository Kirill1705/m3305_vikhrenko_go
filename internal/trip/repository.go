package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Kirill1705/m3305_vikhrenko_go/api"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/database"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}

func (repository *Repository) executor(ctx context.Context) database.DBTX {
	if tx := database.FromContext(ctx); tx != nil {
		return tx
	}
	return repository.pool
}

func (repository *Repository) Insert(ctx context.Context, trip api.Trip) error {
	query, args, err := sq.Insert("trips").
		Columns("id", "user_id", "driver_id", "start_latitude", "start_longitude",
			"end_latitude", "end_longitude", "price", "status", "started_at", "created_at", "updated_at").
		Values(trip.Id, trip.UserId, trip.DriverId, trip.StartPoint.Latitude, trip.StartPoint.Longitude,
			trip.EndPoint.Latitude, trip.EndPoint.Longitude, trip.Price, trip.Status,
			trip.StartedAt, trip.StartedAt, trip.StartedAt).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip query: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, repository.queryTimeout)
	defer cancel()
	if _, err := repository.executor(ctx).Exec(ctx, query, args...); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.ConstraintName == "trips_active_driver_uniq" {
			return ErrDriverBusy
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

func (repository *Repository) Get(ctx context.Context, id uuid.UUID) (api.Trip, error) {
	return repository.get(ctx, id, false)
}

func (repository *Repository) GetForUpdate(ctx context.Context, id uuid.UUID) (api.Trip, error) {
	if database.FromContext(ctx) == nil {
		return api.Trip{}, errors.New("get trip for update requires a transaction")
	}
	return repository.get(ctx, id, true)
}

func (repository *Repository) get(ctx context.Context, id uuid.UUID, forUpdate bool) (api.Trip, error) {
	query := sq.Select("id", "user_id", "driver_id", "start_latitude", "start_longitude",
		"end_latitude", "end_longitude", "price", "status", "started_at", "finished_at").
		From("trips").Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar)
	if forUpdate {
		query = query.Suffix("FOR UPDATE")
	}
	sql, args, err := query.ToSql()
	if err != nil {
		return api.Trip{}, fmt.Errorf("build select trip query: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, repository.queryTimeout)
	defer cancel()
	var trip api.Trip
	var finishedAt *time.Time
	err = repository.executor(ctx).QueryRow(ctx, sql, args...).Scan(
		&trip.Id, &trip.UserId, &trip.DriverId,
		&trip.StartPoint.Latitude, &trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude, &trip.EndPoint.Longitude,
		&trip.Price, &trip.Status, &trip.StartedAt, &finishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.Trip{}, ErrTripNotFound
	}
	if err != nil {
		return api.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	trip.StartedAt = trip.StartedAt.UTC()
	trip.FinishedAt.SetNull()
	if finishedAt != nil {
		trip.FinishedAt.Set(finishedAt.UTC())
	}
	trip.LastPositionAt.SetNull()
	return trip, nil
}

func (repository *Repository) Complete(ctx context.Context, id uuid.UUID, finishedAt time.Time) error {
	query, args, err := sq.Update("trips").
		Set("status", api.TripStatus("completed")).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(sq.Eq{"id": id, "status": "active"}).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build complete trip query: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, repository.queryTimeout)
	defer cancel()
	result, err := repository.executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("complete trip: %w", err)
	}
	if result.RowsAffected() == 0 {
		if _, err := repository.Get(ctx, id); err != nil {
			return err
		}
		return ErrTripCompleted
	}
	return nil
}

func (repository *Repository) AddStatusChange(ctx context.Context, id uuid.UUID, from *api.TripStatus, to api.TripStatus, reason string, changedAt time.Time) error {
	query, args, err := sq.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(id, from, to, reason, changedAt).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build insert status history query: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, repository.queryTimeout)
	defer cancel()
	if _, err := repository.executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}
	return nil
}

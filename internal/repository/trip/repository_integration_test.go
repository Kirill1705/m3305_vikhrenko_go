//go:build integration

package trip_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/infra/postgres/transaction"
	repository "github.com/Kirill1705/m3305_vikhrenko_go/internal/repository/trip"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "tripgo_test_" + uuid.New().String()[:8]
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db := sql.OpenDB(stdlib.GetPoolConnector(pool))
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../../../migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestRepositoryTransaction(t *testing.T) {
	pool := testPool(t)
	repo := repository.NewRepository(pool, time.Second)
	tx := transaction.NewTransactionManager(pool, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	value := domain.Trip{Id: uuid.New(), UserId: uuid.New(), DriverId: uuid.New(), Status: domain.StatusActive, Price: 1450, StartedAt: time.Now().UTC().Truncate(time.Microsecond)}
	failure := errors.New("abort both inserts")
	err := tx.Do(ctx, func(ctx context.Context) error {
		if err := repo.Insert(ctx, value); err != nil {
			return err
		}
		if err := repo.AddStatusChange(ctx, value.Id, nil, value.Status, domain.ReasonCreated, value.StartedAt); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("incorrect transaction result: %v", err)
	}
	if _, err := repo.Get(ctx, value.Id); !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("trip was not rolled back: %v", err)
	}
	var history int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id=$1", value.Id).Scan(&history); err != nil || history != 0 {
		t.Fatalf("history was not rolled back: %d %v", history, err)
	}
	if err := tx.Do(ctx, func(ctx context.Context) error {
		if err := repo.Insert(ctx, value); err != nil {
			return err
		}
		return repo.AddStatusChange(ctx, value.Id, nil, value.Status, domain.ReasonCreated, value.StartedAt)
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Get(ctx, value.Id)
	if err != nil || stored.Id != value.Id || stored.Price != value.Price || !stored.StartedAt.Equal(value.StartedAt) {
		t.Fatalf("incorrect stored trip: %+v %v", stored, err)
	}
	other := value
	other.Id = uuid.New()
	if err := repo.Insert(ctx, other); !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("expected driver busy: %v", err)
	}
	if _, err := repo.GetForUpdate(ctx, value.Id); err == nil {
		t.Fatal("unlocked read was accepted")
	}
	if err := tx.Do(ctx, func(ctx context.Context) error {
		current, err := repo.GetForUpdate(ctx, value.Id)
		if err != nil {
			return err
		}
		if err := current.Finish(time.Now()); err != nil {
			return err
		}
		if err := repo.Complete(ctx, current.Id, *current.FinishedAt); err != nil {
			return err
		}
		previous := domain.StatusActive
		return repo.AddStatusChange(ctx, current.Id, &previous, current.Status, domain.ReasonCompleted, *current.FinishedAt)
	}); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.Get(ctx, value.Id)
	if err != nil || stored.Status != domain.StatusCompleted || stored.FinishedAt == nil {
		t.Fatalf("finish not committed: %+v %v", stored, err)
	}
}

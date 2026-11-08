package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kirill1705/m3305_vikhrenko_go/internal/config"
	transport "github.com/Kirill1705/m3305_vikhrenko_go/internal/http"
	createhandler "github.com/Kirill1705/m3305_vikhrenko_go/internal/http/trip/create"
	finishhandler "github.com/Kirill1705/m3305_vikhrenko_go/internal/http/trip/finish"
	gethandler "github.com/Kirill1705/m3305_vikhrenko_go/internal/http/trip/get"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/infra/postgres"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/infra/postgres/transaction"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/infra/telemetry"
	repository "github.com/Kirill1705/m3305_vikhrenko_go/internal/repository/trip"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/usecase/trip/create"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/usecase/trip/finish"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/usecase/trip/get"
)

func New(ctx context.Context, cfg config.Config) (_ *Application, err error) {
	signals, err := telemetry.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var pool *pgxpool.Pool
	defer func() {
		if err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
			defer cancel()
			err = errors.Join(err, signals.Shutdown(cleanupCtx))
			if pool != nil {
				pool.Close()
			}
		}
	}()
	poolConfig, err := postgres.PoolConfig(cfg, signals.Traces, signals.Metrics)
	if err != nil {
		return nil, err
	}
	connectCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	pool, err = pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	if err = postgres.Ping(connectCtx, pool); err != nil {
		return nil, err
	}
	repo := repository.NewRepository(pool, cfg.DatabaseQueryTimeout)
	tx := transaction.NewTransactionManager(pool, cfg.DatabaseQueryTimeout)
	createUsecase := create.New(repo, tx, signals, cfg.IdempotencyTTL)
	getUsecase := get.New(repo)
	finishUsecase := finish.New(repo, tx, signals)
	handler, err := transport.NewHandler(transport.Dependencies{Create: createhandler.New(createUsecase), Get: gethandler.New(getUsecase), Finish: finishhandler.New(finishUsecase), Readiness: pool, Logger: signals.Logger, TracerProvider: signals.Traces, MeterProvider: signals.Metrics}, cfg.DatabaseQueryTimeout, cfg.HTTPMaxBodyBytes)
	if err != nil {
		return nil, err
	}
	if err = signals.ObserveActive(repo.CountActive); err != nil {
		return nil, fmt.Errorf("register active trips gauge: %w", err)
	}
	requestCtx, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadTimeout: cfg.HTTPReadTimeout, ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout, WriteTimeout: cfg.HTTPWriteTimeout, IdleTimeout: cfg.HTTPIdleTimeout, ErrorLog: slog.NewLogLogger(signals.Logger.Handler(), slog.LevelError), BaseContext: func(net.Listener) context.Context { return requestCtx }}
	return &Application{server: server, pool: pool, telemetry: signals, cfg: cfg, cancelRequests: cancelRequests}, nil
}

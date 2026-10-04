package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kirill1705/m3305_vikhrenko_go/internal/config"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/database"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/httpapi"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/trip"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "trip-service")
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})).With("service", "trip-service")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	err = run(ctx, cfg, logger)
	stop()
	if err != nil {
		logger.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := database.NewPool(ctx, cfg)
	if err != nil {
		return err
	}

	repository := trip.NewRepository(pool, cfg.DatabaseQueryTimeout)
	txManager := database.NewTransactionManager(pool, cfg.DatabaseQueryTimeout)
	service := trip.NewService(repository, txManager, cfg.IdempotencyTTL)
	handler, err := httpapi.NewHandler(service, pool, logger, cfg.DatabaseQueryTimeout, cfg.HTTPMaxBodyBytes)
	if err != nil {
		pool.Close()
		return fmt.Errorf("create HTTP handler: %w", err)
	}

	requestCtx, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRequests()
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		BaseContext: func(net.Listener) context.Context {
			return requestCtx
		},
	}
	listenerConfig := net.ListenConfig{}
	listener, err := listenerConfig.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		pool.Close()
		return fmt.Errorf("listen HTTP: %w", err)
	}

	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	logger.Info("service started", "http_addr", listener.Addr().String())

	var serveError error
	select {
	case <-ctx.Done():
		logger.Info("stopping service")
	case err := <-serveErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			serveError = fmt.Errorf("serve HTTP: %w", err)
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancelShutdown()
	shutdownError := shutdown(shutdownCtx, server, pool, cancelRequests)
	if err := errors.Join(serveError, shutdownError); err != nil {
		return err
	}
	logger.Info("service stopped")
	return nil
}

func shutdown(ctx context.Context, server *http.Server, pool *pgxpool.Pool, cancelRequests context.CancelFunc) error {
	var shutdownError error
	if err := server.Shutdown(ctx); err != nil {
		cancelRequests()
		closeError := server.Close()
		shutdownError = errors.Join(fmt.Errorf("shutdown HTTP: %w", err), closeError)
	}
	cancelRequests()

	poolClosed := make(chan struct{})
	go func() {
		pool.Close()
		close(poolClosed)
	}()
	select {
	case <-poolClosed:
		if err := ctx.Err(); err != nil {
			return errors.Join(shutdownError, fmt.Errorf("SHUTDOWN_TIMEOUT exceeded: %w", err))
		}
		return shutdownError
	case <-ctx.Done():
		return errors.Join(shutdownError, fmt.Errorf("SHUTDOWN_TIMEOUT exceeded: %w", ctx.Err()))
	}
}

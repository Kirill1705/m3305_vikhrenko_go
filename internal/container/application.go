package container

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kirill1705/m3305_vikhrenko_go/internal/config"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/infra/telemetry"
)

type Application struct {
	server         *http.Server
	pool           *pgxpool.Pool
	telemetry      *telemetry.Telemetry
	cfg            config.Config
	cancelRequests context.CancelFunc
}

func (app *Application) Run(ctx context.Context) error {
	listenerConfig := net.ListenConfig{}
	listener, err := listenerConfig.Listen(ctx, "tcp", app.cfg.HTTPAddr)
	if err != nil {
		return errors.Join(fmt.Errorf("listen HTTP: %w", err), app.stop(ctx))
	}

	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- app.server.Serve(listener)
	}()
	app.telemetry.Logger.Info("service started", "http_addr", listener.Addr().String())

	var serveError error
	select {
	case <-ctx.Done():
		app.telemetry.Logger.Info("stopping service")
	case err := <-serveErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			serveError = fmt.Errorf("serve HTTP: %w", err)
		}
	}

	shutdownError := app.stop(ctx)
	if err := errors.Join(serveError, shutdownError); err != nil {
		return err
	}
	app.telemetry.Logger.Info("service stopped")
	return nil
}

func (app *Application) stop(parent context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), app.cfg.ShutdownTimeout)
	defer cancel()
	var result error
	if err := app.server.Shutdown(ctx); err != nil {
		app.cancelRequests()
		result = errors.Join(fmt.Errorf("shutdown HTTP: %w", err), app.server.Close())
	}
	app.cancelRequests()
	if err := app.telemetry.Shutdown(ctx); err != nil {
		app.telemetry.Logger.WarnContext(ctx, "telemetry shutdown failed", "error", err)
	}
	closed := make(chan struct{})
	go func() { app.pool.Close(); close(closed) }()
	select {
	case <-closed:
		if err := ctx.Err(); err != nil {
			return errors.Join(result, fmt.Errorf("SHUTDOWN_TIMEOUT exceeded: %w", err))
		}
	case <-ctx.Done():
		return errors.Join(result, fmt.Errorf("SHUTDOWN_TIMEOUT exceeded: %w", ctx.Err()))
	}
	return result
}

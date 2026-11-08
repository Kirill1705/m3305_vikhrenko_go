package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kirill1705/m3305_vikhrenko_go/api"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/trip"
)

type Handler struct {
	service      *trip.Service
	pool         *pgxpool.Pool
	logger       *slog.Logger
	queryTimeout time.Duration
}

var _ api.ServerInterface = (*Handler)(nil)

func NewHandler(service *trip.Service, pool *pgxpool.Pool, logger *slog.Logger, queryTimeout time.Duration, maxBodyBytes int64) (http.Handler, error) {
	h := &Handler{service: service, pool: pool, logger: logger, queryTimeout: queryTimeout}
	spec, err := api.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI: %w", err)
	}
	spec.Servers = nil
	validator, err := legacy.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("create request validator: %w", err)
	}

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					h.logger.ErrorContext(r.Context(), "request panicked", "panic", recovered)
					h.problem(w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error")
				}
			}()
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
			route, params, err := validator.FindRoute(r)
			if err == nil {
				input := &openapi3filter.RequestValidationInput{Request: r, PathParams: params, Route: route}
				if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
					h.invalid(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		h.problem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "Route was not found")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		h.problem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "Method is not allowed for this route")
	})
	return api.HandlerWithOptions(h, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			h.invalid(w, r)
		},
	}), nil
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var input api.CreateTripJSONRequestBody
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		h.invalid(w, r)
		return
	}
	if decoder.Decode(new(any)) != io.EOF || input.UserId == uuid.Nil || input.DriverId == uuid.Nil {
		h.invalid(w, r)
		return
	}
	result, replayed, err := h.service.Create(r.Context(), input, params.IdempotencyKey)
	if err != nil {
		h.serviceError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+result.Id.String())
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	h.json(w, r, status, "application/json", result)
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	result, err := h.service.Get(r.Context(), tripID)
	if err != nil {
		h.serviceError(w, r, err)
		return
	}
	h.json(w, r, http.StatusOK, "application/json", result)
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(strings.TrimSpace(string(body))) != 0 {
		h.invalid(w, r)
		return
	}
	result, err := h.service.Finish(r.Context(), tripID)
	if err != nil {
		h.serviceError(w, r, err)
		return
	}
	h.json(w, r, http.StatusOK, "application/json", result)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.json(w, r, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		h.logger.WarnContext(ctx, "database readiness check failed", "error", err)
		h.json(w, r, http.StatusServiceUnavailable, "application/json", api.HealthResponse{Status: api.Unavailable})
		return
	}
	h.json(w, r, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}

func (h *Handler) serviceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, trip.ErrTripNotFound):
		h.problem(w, r, http.StatusNotFound, "trip_not_found", "Trip not found", "Trip was not found")
	case errors.Is(err, trip.ErrTripCompleted):
		h.problem(w, r, http.StatusConflict, "trip_completed", "Trip completed", "Operation is not allowed for a completed trip")
	case errors.Is(err, trip.ErrDriverBusy):
		h.problem(w, r, http.StatusConflict, "driver_busy", "Driver busy", "Driver already has an active trip")
	case errors.Is(err, trip.ErrIdempotencyConflict):
		h.problem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict", "Idempotency-Key was already used with a different request body")
	default:
		h.logger.ErrorContext(r.Context(), "trip operation failed", "error", err)
		h.problem(w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error", "Internal server error")
	}
}

func (h *Handler) invalid(w http.ResponseWriter, r *http.Request) {
	h.problem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request", "Request validation failed")
}

func (h *Handler) problem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	instance := r.URL.Path
	h.json(w, r, status, "application/problem+json", api.Problem{
		Type:  "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"),
		Title: title, Status: int32(status), Code: code, Detail: &detail, Instance: &instance,
	})
}

func (h *Handler) json(w http.ResponseWriter, r *http.Request, status int, contentType string, value any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.ErrorContext(r.Context(), "write response failed", "error", err)
	}
}

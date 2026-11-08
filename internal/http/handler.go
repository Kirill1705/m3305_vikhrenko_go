package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/Kirill1705/m3305_vikhrenko_go/api"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/domainerror"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/middleware"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/requestinfo"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/response"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/trip/create"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/trip/finish"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/trip/get"
)

type Dependencies struct {
	Create         *create.Handler
	Get            *get.Handler
	Finish         *finish.Handler
	Readiness      readiness
	Logger         *slog.Logger
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}
type Handler struct {
	dependencies Dependencies
	queryTimeout time.Duration
}

var _ api.ServerInterface = (*Handler)(nil)

func NewHandler(dependencies Dependencies, queryTimeout time.Duration, maxBodyBytes int64) (http.Handler, error) {
	h := &Handler{dependencies: dependencies, queryTimeout: queryTimeout}
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI: %w", err)
	}
	spec.Servers = nil
	validator, err := legacy.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("create request validator: %w", err)
	}
	metrics, err := middleware.NewMetrics(dependencies.MeterProvider.Meter("tripgo/http"))
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		routeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "Route was not found")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		routeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "Method is not allowed for this route")
	})
	handler := api.HandlerWithOptions(h, api.ChiServerOptions{BaseRouter: router, ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) { response.Invalid(w, r) }})
	next := handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		route, params, err := validator.FindRoute(r)
		if err == nil {
			input := &openapi3filter.RequestValidationInput{Request: r, PathParams: params, Route: route}
			if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
				response.Invalid(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
	handler = metrics.Middleware(handler)
	handler = middleware.Logging(dependencies.Logger)(handler)
	handler = middleware.Tracing(dependencies.TracerProvider)(handler)
	handler = middleware.Recover(dependencies.Logger, func(r *http.Request) string {
		route, _, err := validator.FindRoute(r)
		if err != nil {
			return "unmatched"
		}
		return route.Path
	})(handler)
	return handler, nil
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	h.dependencies.Create.Handle(w, r, params)
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	h.dependencies.Get.Handle(w, r, id)
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	h.dependencies.Finish.Handle(w, r, id)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, response.JSONType, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()
	if err := h.dependencies.Readiness.Ping(ctx); err != nil {
		info := requestinfo.From(r.Context())
		info.Err = err
		info.ErrorType = "database_unavailable"
		response.JSON(w, r, http.StatusServiceUnavailable, response.JSONType, api.HealthResponse{Status: api.Unavailable})
		return
	}
	response.JSON(w, r, http.StatusOK, response.JSONType, api.HealthResponse{Status: api.Ok})
}

func routeProblem(w http.ResponseWriter, r *http.Request, status int32, code, title, detail string) {
	instance := r.URL.Path
	response.JSON(w, r, int(status), response.ProblemType, api.Problem{Type: domainerror.ProblemBaseURI + strings.ReplaceAll(code, "_", "-"), Title: title, Status: status, Code: code, Detail: &detail, Instance: &instance})
}

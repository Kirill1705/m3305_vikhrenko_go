package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/Kirill1705/m3305_vikhrenko_go/api"
	domain "github.com/Kirill1705/m3305_vikhrenko_go/internal/domain/trip"
	transport "github.com/Kirill1705/m3305_vikhrenko_go/internal/http"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/trip/create"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/model"
)

const body = `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":59.9,"longitude":30.3},"end_point":{"latitude":59.8,"longitude":30.2},"price":1450}`

const busyCase = "busy"

const (
	caseReplay   = "replay"
	caseInternal = "internal"
	casePanic    = "panic"
)

func TestCreateHandler(t *testing.T) {
	for _, name := range []string{"success", caseReplay, busyCase, caseInternal, casePanic, "invalid", "unknown field", "missing price", "null latitude"} {
		t.Run(name, func(t *testing.T) {
			mock := NewMockusecase(gomock.NewController(t))
			status := http.StatusCreated
			payload := body
			switch name {
			case "invalid":
				payload = `{`
				status = http.StatusBadRequest
			case "unknown field":
				payload = strings.TrimSuffix(body, "}") + `,"extra":true}`
				status = http.StatusBadRequest
			case "missing price":
				payload = strings.ReplaceAll(body, `,"price":1450`, "")
				status = http.StatusBadRequest
			case "null latitude":
				payload = strings.ReplaceAll(body, "59.9", "null")
				status = http.StatusBadRequest
			case busyCase:
				status = http.StatusConflict
			case caseInternal, casePanic:
				status = http.StatusInternalServerError
			case caseReplay:
				status = http.StatusOK
			}
			if status != http.StatusBadRequest {
				mock.EXPECT().Execute(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, data domain.TripData, _ *uuid.UUID) (model.CreateResult, error) {
					if name == casePanic {
						panic("private database panic")
					}
					if name == busyCase {
						return model.CreateResult{}, domain.ErrDriverBusy
					}
					if name == caseInternal {
						return model.CreateResult{}, errors.New("private database error")
					}
					return model.CreateResult{Trip: domain.Trip{Id: uuid.New(), UserId: data.UserId, DriverId: data.DriverId, Price: data.Price, Status: domain.StatusActive}, Replayed: name == caseReplay}, nil
				})
			}
			handler, err := transport.NewHandler(transport.Dependencies{Create: create.New(mock), Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), TracerProvider: tracenoop.NewTracerProvider(), MeterProvider: noop.NewMeterProvider()}, time.Second, 1024)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			handler.ServeHTTP(writer, request)
			if writer.Code != status {
				t.Fatalf("got %d: %s", writer.Code, writer.Body.String())
			}
			if status < http.StatusBadRequest {
				var trip api.Trip
				if err := json.Unmarshal(writer.Body.Bytes(), &trip); err != nil || trip.Id == uuid.Nil || writer.Header().Get("Location") == "" {
					t.Fatalf("incorrect success: %s", writer.Body.String())
				}
			} else {
				var problem api.Problem
				if err := json.Unmarshal(writer.Body.Bytes(), &problem); err != nil || problem.Code == "" || writer.Header().Get("Content-Type") != "application/problem+json" {
					t.Fatalf("incorrect problem: %s", writer.Body.String())
				}
				if strings.Contains(writer.Body.String(), "private") {
					t.Fatal("private error exposed")
				}
			}
		})
	}
}

SHELL := /bin/sh
GO ?= go

DOCKER ?= docker
DOCKER_IMAGE ?= trip-service:lab1
DOCKER_CONTAINER ?= tripgo-lab1
DOCKER_STOP_TIMEOUT ?= 30
DOCKER_BUILD_NETWORK ?= host
DOCKER_ENV := HTTP_ADDR LOG_LEVEL SHUTDOWN_TIMEOUT HTTP_READ_TIMEOUT HTTP_READ_HEADER_TIMEOUT \
	HTTP_WRITE_TIMEOUT HTTP_IDLE_TIMEOUT HTTP_MAX_BODY_BYTES DATABASE_URL DATABASE_MAX_CONNS \
	DATABASE_MIN_CONNS DATABASE_MAX_CONN_LIFETIME DATABASE_CONNECT_TIMEOUT DATABASE_QUERY_TIMEOUT IDEMPOTENCY_TTL

ifneq ($(wildcard .env.example),)
include .env.example
endif
ifneq ($(wildcard .env),)
include .env
endif
export

.PHONY: generate migrate migrate-down migrate-status run build test docker-build docker-run

generate:
	$(GO) tool oapi-codegen --config api/oapi-codegen.yaml contracts/openapi/trip-service.openapi.yaml

migrate:
	@GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" $(GO) tool goose -dir migrations up

migrate-down:
	@GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" $(GO) tool goose -dir migrations down

migrate-status:
	@GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" $(GO) tool goose -dir migrations status

build:
	$(GO) build -o bin/trip-service ./cmd/trip-service

run: build
	./bin/trip-service

test:
	$(GO) test -race ./...

docker-build:
	$(DOCKER) build --network "$(DOCKER_BUILD_NETWORK)" -t "$(DOCKER_IMAGE)" .

docker-run: docker-build
	$(DOCKER) run --rm --name "$(DOCKER_CONTAINER)" --network host --stop-timeout "$(DOCKER_STOP_TIMEOUT)" $(foreach variable,$(DOCKER_ENV),--env $(variable)) "$(DOCKER_IMAGE)"

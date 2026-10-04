FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build

WORKDIR /src

ENV CGO_ENABLED=0 GOTOOLCHAIN=local

COPY go.mod go.sum ./
COPY api/ ./api/
COPY cmd/trip-service/ ./cmd/trip-service/
COPY internal/ ./internal/

RUN go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/trip-service ./cmd/trip-service

FROM scratch

COPY --from=build /out/trip-service /trip-service
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

USER 65532:65532

EXPOSE 8080
STOPSIGNAL SIGTERM

ENTRYPOINT ["/trip-service"]

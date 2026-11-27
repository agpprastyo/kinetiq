# Multi-stage Dockerfile for Kinetiq Go API & Worker
FROM golang:alpine AS builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/kinetiq-api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/kinetiq-worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/kinetiq ./cmd/kinetiq

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/kinetiq-api /app/kinetiq-api
COPY --from=builder /out/kinetiq-worker /app/kinetiq-worker
COPY --from=builder /out/kinetiq /app/kinetiq

EXPOSE 8080
CMD ["/app/kinetiq-api"]

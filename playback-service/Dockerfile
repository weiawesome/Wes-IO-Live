FROM golang:1.24.3-alpine AS builder

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY playback-service/ ./playback-service/

# Build (GOWORK=off: use go.mod only, not go.work)
WORKDIR /app/playback-service
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /playback-service ./cmd/main.go

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /playback-service .
COPY --from=builder /app/playback-service/config ./config

EXPOSE 8087

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8087/health || exit 1

CMD ["./playback-service"]

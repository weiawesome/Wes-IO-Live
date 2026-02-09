FROM golang:1.24.3-alpine AS builder

RUN apk add --no-cache gcc musl-dev pkgconf librdkafka-dev

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY media-service/ ./media-service/

# Build (GOWORK=off: use go.mod only, not go.work - Docker only copies needed modules)
# CGO required: pkg/pubsub uses confluent-kafka-go (librdkafka)
WORKDIR /app/media-service
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o /media-service ./cmd/main.go

FROM alpine:3.19

# Install FFmpeg, librdkafka and other dependencies
RUN apk --no-cache add ca-certificates tzdata ffmpeg librdkafka

WORKDIR /app

COPY --from=builder /media-service .
COPY --from=builder /app/media-service/config ./config

# Create HLS output directory
RUN mkdir -p /app/hls

EXPOSE 8085

CMD ["./media-service"]

FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY media-service/ ./media-service/

# Build
WORKDIR /app/media-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /media-service ./cmd/main.go

FROM alpine:3.19

# Install FFmpeg and other dependencies
RUN apk --no-cache add ca-certificates tzdata ffmpeg

WORKDIR /app

COPY --from=builder /media-service .
COPY --from=builder /app/media-service/config ./config

# Create HLS output directory
RUN mkdir -p /app/hls

EXPOSE 8085

CMD ["./media-service"]

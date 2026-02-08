# Build stage
# Build from parent directory: docker build -t playback-service -f playback-service/Dockerfile .
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy pkg module first (for caching)
COPY pkg/ ./pkg/

# Copy playback-service module
COPY playback-service/go.mod playback-service/go.sum* ./playback-service/

# Download dependencies
WORKDIR /app/playback-service
RUN go mod download

# Copy source code
WORKDIR /app
COPY playback-service/ ./playback-service/

# Build the binary
WORKDIR /app/playback-service
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o playback-service ./cmd

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/playback-service/playback-service .
COPY --from=builder /app/playback-service/config/config.yaml ./config/

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

# Expose port
EXPOSE 8087

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8087/health || exit 1

# Run the binary
CMD ["./playback-service"]

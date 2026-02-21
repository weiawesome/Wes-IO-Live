FROM golang:1.24.3-alpine AS builder

RUN apk add --no-cache gcc musl-dev pkgconf librdkafka-dev sqlite-dev

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY proto/ ./proto/
COPY social-graph-service/ ./social-graph-service/

# Build (GOWORK=off: use go.mod only, not go.work - Docker only copies needed modules)
# CGO required: confluent-kafka-go (librdkafka), go-sqlite3 (use system lib to avoid musl pread64/pwrite64)
WORKDIR /app/social-graph-service
ENV GOWORK=off
RUN go mod download
ENV CGO_CFLAGS="-DUSE_LIBSQLITE3"
ENV CGO_LDFLAGS="-lsqlite3"
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o /social-graph-service ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata librdkafka sqlite-libs

WORKDIR /app

COPY --from=builder /social-graph-service .
COPY --from=builder /app/social-graph-service/config ./config

EXPOSE 8095

CMD ["./social-graph-service"]

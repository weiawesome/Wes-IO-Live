FROM golang:1.24.3-alpine AS builder

RUN apk add --no-cache gcc musl-dev pkgconf librdkafka-dev

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY proto/ ./proto/
COPY presence-service/ ./presence-service/

# Build (GOWORK=off: use go.mod only, not go.work - Docker only copies needed modules)
# CGO required: confluent-kafka-go (librdkafka)
WORKDIR /app/presence-service
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o /presence-service ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata librdkafka

WORKDIR /app

COPY --from=builder /presence-service .
COPY --from=builder /app/presence-service/config ./config

EXPOSE 8092

CMD ["./presence-service"]

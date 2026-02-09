FROM golang:1.24.3-alpine AS builder

RUN apk add --no-cache gcc musl-dev pkgconf librdkafka-dev

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY proto/ ./proto/
COPY chat-service/ ./chat-service/

# Build (GOWORK=off: use go.mod only, not go.work - Docker only copies needed modules)
WORKDIR /app/chat-service
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o /chat-service ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata librdkafka

WORKDIR /app

COPY --from=builder /chat-service .
COPY --from=builder /app/chat-service/config ./config

EXPOSE 8088 50052

CMD ["./chat-service"]

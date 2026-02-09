FROM golang:1.24.3-alpine AS builder

WORKDIR /app

# Copy workspace files (chat-history-service is standalone, no pkg/proto)
COPY go.work go.work.sum* ./
COPY chat-history-service/ ./chat-history-service/

# Build (GOWORK=off: use go.mod only, not go.work)
WORKDIR /app/chat-history-service
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /chat-history-service ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /chat-history-service .
COPY --from=builder /app/chat-history-service/config ./config

EXPOSE 8091

CMD ["./chat-history-service"]

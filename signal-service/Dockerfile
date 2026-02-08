FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY proto/ ./proto/
COPY signal-service/ ./signal-service/

# Build
WORKDIR /app/signal-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /signal-service ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /signal-service .
COPY --from=builder /app/signal-service/config ./config

EXPOSE 8084

CMD ["./signal-service"]

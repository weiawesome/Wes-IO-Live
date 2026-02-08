FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY proto/ ./proto/
COPY room-service/ ./room-service/

# Build
WORKDIR /app/room-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /room-service ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /room-service .
COPY --from=builder /app/room-service/config ./config

EXPOSE 8083

CMD ["./room-service"]

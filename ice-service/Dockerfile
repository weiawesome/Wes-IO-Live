FROM golang:1.24.3-alpine AS builder

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY ice-service/ ./ice-service/

# Build (GOWORK=off: use go.mod only, not go.work - Docker only copies needed modules)
WORKDIR /app/ice-service
ENV GOWORK=off
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /ice-service ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /ice-service .
COPY --from=builder /app/ice-service/config ./config

EXPOSE 8086

CMD ["./ice-service"]

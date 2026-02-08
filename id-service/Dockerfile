FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY proto/ ./proto/
COPY id-service/ ./id-service/

# Build
WORKDIR /app/id-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /id-service ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /id-service .
COPY --from=builder /app/id-service/config ./config

EXPOSE 8090 50053

CMD ["./id-service"]

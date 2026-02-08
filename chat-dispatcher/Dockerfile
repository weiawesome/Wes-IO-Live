FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev pkgconf librdkafka-dev

WORKDIR /app

# Copy workspace files
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY proto/ ./proto/
COPY chat-dispatcher/ ./chat-dispatcher/

# Build
WORKDIR /app/chat-dispatcher
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o /chat-dispatcher ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata librdkafka

WORKDIR /app

COPY --from=builder /chat-dispatcher .
COPY --from=builder /app/chat-dispatcher/config ./config

EXPOSE 8089

CMD ["./chat-dispatcher"]

FROM golang:1.26-alpine AS builder

RUN apk --no-cache add git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o messenger-app ./cmd/messenger-app

FROM alpine:3.21

RUN apk --no-cache add ca-certificates curl

WORKDIR /app
COPY --from=builder /app/messenger-app .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

CMD ["./messenger-app"]

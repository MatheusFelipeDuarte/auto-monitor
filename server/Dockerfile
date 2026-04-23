# Builder stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the server
RUN CGO_ENABLED=1 GOOS=linux go build -o monitor-server ./server/main.go

# Final stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates libc6-compat

WORKDIR /root/

# Copy binary
COPY --from=builder /app/monitor-server .
# Copy .env if needed (though using env vars in docker-compose is better)
# COPY --from=builder /app/server/.env .

# Ports
EXPOSE 50051 8080

CMD ["./monitor-server"]

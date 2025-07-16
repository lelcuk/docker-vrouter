# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy source code
COPY . .

# Download dependencies and build
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o router ./cmd/main.go

# Final stage
FROM alpine:latest

# Install required packages for network management
RUN apk --no-cache add ca-certificates iproute2 bridge-utils

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/router .

# Create directories
RUN mkdir -p /var/lib/docker-router /etc/router

# Set executable permissions
RUN chmod +x router

# Default command
CMD ["./router"]
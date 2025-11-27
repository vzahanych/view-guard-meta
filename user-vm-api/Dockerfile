# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy proto module first (needed for go mod download)
COPY proto/ ./proto/

# Copy go mod files (from user-vm-api directory)
COPY user-vm-api/go.mod user-vm-api/go.sum ./user-vm-api/

# Change to user-vm-api directory for go mod operations
WORKDIR /build/user-vm-api

# Download dependencies
RUN go mod download

# Copy source code (from user-vm-api directory)
COPY user-vm-api/ ./

# Build
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.version=$VERSION -X main.buildTime=$BUILD_TIME -X main.gitCommit=$GIT_COMMIT" \
    -o user-vm-api ./cmd/server

# Runtime stage
FROM alpine:latest

# Install runtime dependencies including WireGuard tools so we can inspect and manage wg interfaces
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    wireguard-tools \
    iproute2

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/user-vm-api/user-vm-api .

# Create data directory
RUN mkdir -p /app/data

# Expose ports
EXPOSE 51820/udp
EXPOSE 8080

ENTRYPOINT ["./user-vm-api"]


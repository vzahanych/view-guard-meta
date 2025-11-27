# Frontend build stage
FROM node:20-alpine AS frontend-builder

WORKDIR /build

# Copy frontend files
COPY edge/orchestrator/internal/web/frontend/package.json edge/orchestrator/internal/web/frontend/package-lock.json* ./edge/orchestrator/internal/web/frontend/
WORKDIR /build/edge/orchestrator/internal/web/frontend
# Install dependencies (npm install works with or without package-lock.json)
RUN npm install

# Copy frontend source
COPY edge/orchestrator/internal/web/frontend/ ./

# Build frontend (outputs to ../static)
RUN npm run build

# Go build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Copy go mod files from orchestrator
COPY edge/orchestrator/go.mod edge/orchestrator/go.sum ./
# Copy crypto/go for replace directive
COPY crypto/go ./crypto/go
# Copy proto for replace directive (if needed)
COPY proto ./proto

# Copy orchestrator source code
COPY edge/orchestrator/ ./edge/orchestrator/

# Copy built frontend from frontend-builder (vite outputs to ../static)
COPY --from=frontend-builder /build/edge/orchestrator/internal/web/static ./edge/orchestrator/internal/web/static

# Set working directory for orchestrator
WORKDIR /build/edge/orchestrator

# Download dependencies (now that all files are in place)
RUN go mod download

# Build the orchestrator
RUN CGO_ENABLED=1 go build -o orchestrator .

# Runtime stage
FROM alpine:latest

# Install runtime dependencies (including WireGuard tools for tunnel client)
RUN apk add --no-cache \
    ca-certificates \
    ffmpeg \
    sqlite \
    v4l-utils \
    wget \
    bash \
    coreutils \
    wireguard-tools \
    iproute2 \
    && rm -rf /var/cache/apk/*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/edge/orchestrator/orchestrator .

# Create data directories
RUN mkdir -p /app/data/clips /app/data/snapshots /app/data/db /app/config

# Create default config directory (config will be mounted in docker-compose)
RUN mkdir -p /app/config

# Expose health check port and web UI port
EXPOSE 8080 8081

# Run the orchestrator
CMD ["./orchestrator", "-config", "/app/config/config.yaml"]


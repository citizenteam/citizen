# Multi-stage build for optimal image size and security

# Build stage
FROM golang:1.23-alpine AS builder

# Add metadata
LABEL stage=builder
LABEL description="Build stage for Citizen Backend"

# Install build dependencies
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    upx

# Create non-root user for build process
RUN adduser -D -g '' builder

WORKDIR /app

# Copy dependency files first for better caching
COPY --chown=builder:builder backend/go.mod backend/go.sum ./

# Download dependencies as builder user
USER builder
RUN go mod download && go mod verify

# Copy source code
COPY --chown=builder:builder backend/ ./

# Ensure go.mod and go.sum are synchronized
USER root
RUN go mod tidy

# Build with optimizations
ARG BUILD_VERSION=development
ARG BUILD_TIME
ARG GIT_COMMIT=unknown

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Build the application with build info
RUN go build \
    -ldflags="-w -s -X 'main.Version=${BUILD_VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}' -extldflags '-static'" \
    -a -installsuffix cgo \
    -trimpath \
    -o main ./cmd/server

# Compress binary (optional, can be disabled for faster builds)
ARG COMPRESS_BINARY=true
RUN if [ "$COMPRESS_BINARY" = "true" ]; then upx --best --lzma main; fi

# Runtime stage - distroless for maximum security
FROM gcr.io/distroless/static-debian12:nonroot AS runtime-distroless

WORKDIR /app

# Copy ca-certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy application binary and migrations
COPY --from=builder --chown=nonroot:nonroot /app/main ./
COPY --from=builder --chown=nonroot:nonroot /app/migrations ./migrations

# Set timezone
ENV TZ=UTC

EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/main", "-health-check"] || exit 1

USER nonroot:nonroot

CMD ["./main"]

# Alternative runtime stage - Alpine with tools (for debugging/development)
FROM alpine:3.19 AS runtime-alpine

# Install runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    curl \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Create necessary directories
RUN mkdir -p /app /home/appuser && \
    chmod 700 /home/appuser && \
    chown -R appuser:appgroup /app /home/appuser

WORKDIR /app

# Copy application files
COPY --from=builder --chown=appuser:appgroup /app/main ./
COPY --from=builder --chown=appuser:appgroup /app/migrations ./migrations

# Set timezone
ENV TZ=UTC

EXPOSE 3000

# Health check with curl
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

# Install runtime tools and su-exec for user switching
RUN apk add --no-cache curl su-exec

# Copy entrypoint script that drops privileges to appuser
COPY docker/dockerfiles/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Set the entrypoint
ENTRYPOINT ["/entrypoint.sh"]
CMD ["./main"]

# Use Alpine runtime for production
FROM runtime-alpine AS final

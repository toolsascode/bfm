# ============================================================================
# Stage 1: Build BfM Go Binaries
# ============================================================================
FROM golang:1.25.4-alpine AS go-builder

LABEL org.opencontainers.image.description="BfM - Backend for Migrations"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.source="https://github.com/toolsascode/bfm"
LABEL org.opencontainers.image.authors="Carlos Junior <carlosrfjunior@gmail.com>"
LABEL org.opencontainers.image.vendor="toolsascode"
LABEL org.opencontainers.image.url="https://github.com/toolsascode/bfm"

WORKDIR /build

# Install tooling needed to generate gRPC code
RUN apk add --no-cache bash=5.2.37-r0 protobuf-dev=29.4-r0 protoc=29.4-r0

# Install Go protobuf plugins (pinned versions for reproducibility)
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

# Copy bfm go mod files
COPY api/go.mod api/go.sum ./

# Download dependencies
RUN go mod download

# Copy bfm source code
COPY api/ .

# Generate protobuf bindings for the migration service
WORKDIR /build/internal/api/protobuf
RUN ./generate.sh
WORKDIR /build

# Build the application binaries
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bfm-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bfm-worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bfm-cli ./cmd/cli

# ============================================================================
# Stage 2: Build FfM Frontend
# ============================================================================
FROM node:24.11.1-alpine AS frontend-builder

WORKDIR /build

# Copy package files and npm config
COPY ffm/package*.json ffm/.npmrc* ./

# Install dependencies
RUN npm ci

# Copy source code
COPY ffm/ .

# Build the application
RUN npm run build

# ============================================================================
# Stage 3: Production Runtime
# ============================================================================
FROM alpine:3.19

# Install required tools
RUN apk --no-cache add \
    ca-certificates=20250911-r0 \
    tzdata=2025b-r0 \
    bash=5.2.21-r0 \
    sed=4.9-r2 \
    curl=8.14.1-r2 \
    && rm -rf /var/cache/apk/*

WORKDIR /app

# Create bin directory for binaries
RUN mkdir -p /app/bin

# Copy binaries from go-builder
COPY --from=go-builder /build/bfm-server /app/bin/bfm-server
COPY --from=go-builder /build/bfm-worker /app/bin/bfm-worker
COPY --from=go-builder /build/bfm-cli /app/bin/bfm-cli

# Copy frontend build from frontend-builder
COPY --from=frontend-builder /build/dist /app/frontend

# Copy startup script
COPY shared/start.sh /app/start.sh
RUN chmod +x /app/start.sh

# Create non-root user
RUN addgroup -g 1000 bfm && \
    adduser -D -u 1000 -G bfm bfm && \
    chown -R bfm:bfm /app

USER bfm

# Expose ports
# 7070: BfM HTTP API (serves both API and frontend)
# 9090: BfM gRPC API (direct access)
EXPOSE 7070 9090

# Use startup script as entrypoint
ENTRYPOINT ["/app/start.sh"]


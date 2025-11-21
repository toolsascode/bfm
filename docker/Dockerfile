# ============================================================================
# Stage 1: Build BfM Go Binaries
# ============================================================================
FROM golang:1.24-alpine AS go-builder

WORKDIR /build

# Install tooling needed to generate gRPC code
RUN apk add --no-cache bash protobuf-dev protoc

# Install Go protobuf plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Copy bfm go mod files
COPY api/go.mod api/go.sum ./

# Download dependencies
RUN go mod download

# Copy bfm source code
COPY api/ .

# Generate protobuf bindings for the migration service
RUN cd internal/api/protobuf && ./generate.sh

# Build the application binaries
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bfm-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bfm-worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bfm-cli ./cmd/cli

# ============================================================================
# Stage 2: Build FfM Frontend
# ============================================================================
FROM node:20-alpine AS frontend-builder

WORKDIR /build

# Copy package files
COPY ffm/package*.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY ffm/ .

# Build the application
RUN npm run build

# ============================================================================
# Stage 3: Production Runtime
# ============================================================================
FROM alpine:latest

# Install required tools
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    bash \
    sed \
    curl \
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


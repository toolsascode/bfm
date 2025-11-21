# Build stage
FROM golang:1.25.4-alpine AS builder

WORKDIR /build

# Install tooling needed to generate gRPC code
RUN apk add --no-cache bash protobuf-dev

# Copy bfm go mod files
COPY go.mod go.sum ./

# Download dependencies (this will use the replace directive to find sfm)
RUN go mod download

# Copy bfm source code
COPY . .

# Generate protobuf bindings for the migration service
RUN cd internal/api/protobuf && ./generate.sh

# Build the application (server and worker)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bfm-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bfm-worker ./cmd/worker

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/bfm-server /app/bfm-server
COPY --from=builder /build/bfm-worker /app/bfm-worker

# Create non-root user
RUN addgroup -g 1000 bfm && \
    adduser -D -u 1000 -G bfm bfm && \
    chown -R bfm:bfm /app

USER bfm

EXPOSE 7070 9090

CMD ["./bfm-server"]

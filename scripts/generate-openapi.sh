#!/bin/bash
# Generate OpenAPI 3.1.1 specification using swag
# Usage: ./scripts/generate-openapi.sh

set -e

cd "$(dirname "$0")/.."

# Check if swag is installed
if ! command -v swag &> /dev/null; then
    echo "Error: swag is not installed" >&2
    echo "Install it with: go install github.com/swaggo/swag/cmd/swag@latest" >&2
    exit 1
fi

echo "Generating OpenAPI specification..."

# Change to api directory
# cd api

# Generate OpenAPI spec using swag
# -g: Go file path with swagger general API Info
# --output: output directory for generated files
# --outputTypes: only generate yaml and json (not docs.go)
# --parseInternal: parse internal dependencies
# --parseDependency: parse external dependencies
# --parseDepth: parse depth for dependencies
swag init -g ./cmd/server/main.go \
    --output ./api/docs \
    --parseInternal \
    --parseDepth 1 \
    --dir ./api

swag fmt --dir ./api/docs

# Copy swagger.yaml to the package directory for embedding
cp ./api/docs/swagger.yaml ./api/internal/api/http/swagger.yaml
echo "Copied swagger.yaml to package directory for embedding"

#!/bin/bash
# Validate OpenAPI 3.2 specification
# Usage: ./scripts/validate-openapi.sh [openapi-file]

set -e

OPENAPI_FILE="${1:-api/internal/api/http/openapi.yaml}"

if [ ! -f "$OPENAPI_FILE" ]; then
    echo "Error: OpenAPI file not found: $OPENAPI_FILE" >&2
    exit 1
fi

# Check if openapi-spec-validator is installed
if ! command -v openapi-spec-validator &> /dev/null; then
    echo "Error: openapi-spec-validator is not installed" >&2
    echo "Install it with: pip install openapi-spec-validator" >&2
    exit 1
fi

echo "Validating OpenAPI specification: $OPENAPI_FILE"
openapi-spec-validator "$OPENAPI_FILE"

if [ $? -eq 0 ]; then
    echo "✓ OpenAPI specification is valid"
    exit 0
else
    echo "✗ OpenAPI specification validation failed" >&2
    exit 1
fi

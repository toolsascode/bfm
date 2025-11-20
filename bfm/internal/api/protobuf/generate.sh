#!/bin/bash
# Generate Protobuf and gRPC code from .proto file

set -e

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "Error: protoc is not installed"
    echo "Install it from: https://grpc.io/docs/protoc-installation/"
    exit 1
fi

# Check if Go plugins are installed
if ! command -v protoc-gen-go &> /dev/null; then
    echo "Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "Installing protoc-gen-go-grpc..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

# Get the directory of this script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Generate code
echo "Generating Protobuf code..."
protoc \
    --proto_path=$SCRIPT_DIR \
    --go_out=$SCRIPT_DIR \
    --go_opt=paths=source_relative \
    --go-grpc_out=$SCRIPT_DIR \
    --go-grpc_opt=paths=source_relative \
    $SCRIPT_DIR/migration.proto

echo "Protobuf code generated successfully!"


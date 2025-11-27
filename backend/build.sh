#!/bin/bash
# 构建 Go 后端

set -e

echo "Downloading dependencies..."
go mod tidy

echo "Building..."
CGO_ENABLED=1 go build -ldflags="-s -w" -o vte .

echo "Done! Binary: ./vte"

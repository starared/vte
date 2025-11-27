#!/bin/bash
# 构建 Go 后端

set -e

echo "Downloading dependencies..."
go mod tidy

echo "Building..."
if [[ "$OSTYPE" == "darwin"* ]] || [[ "$OSTYPE" == "linux"* ]]; then
    CGO_ENABLED=1 go build -ldflags="-s -w" -o vte .
else
    go build -ldflags="-s -w" -o vte.exe .
fi

echo "Done! Binary: ./vte"

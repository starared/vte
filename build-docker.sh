#!/bin/bash
# 手动发布 Docker 镜像到 Docker Hub
# 用法: ./publish-docker.sh [版本号]
# 例如: ./publish-docker.sh 1.0.0

set -e

IMAGE_NAME="rtyedfty/vte"
VERSION=${1:-latest}

echo "=========================================="
echo "  发布 VTE Docker 镜像"
echo "  镜像: $IMAGE_NAME:$VERSION"
echo "=========================================="

# 登录 Docker Hub (如果还没登录)
echo "检查 Docker Hub 登录状态..."
docker login

# 构建多架构镜像
echo "构建镜像 (amd64 + arm64)..."
docker buildx create --use --name vte-builder 2>/dev/null || true
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -f Dockerfile \
  -t $IMAGE_NAME:$VERSION \
  -t $IMAGE_NAME:latest \
  --push \
  .

echo "=========================================="
echo "  发布成功!"
echo "  docker pull $IMAGE_NAME:$VERSION"
echo "=========================================="

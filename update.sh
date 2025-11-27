#!/bin/bash
echo "========================================"
echo "  VTE 更新脚本"
echo "========================================"
echo

echo "[1/3] 拉取最新镜像..."
docker pull rtyedfty/vte:latest

echo "[2/3] 停止旧容器..."
docker stop vte 2>/dev/null || true
docker rm vte 2>/dev/null || true

echo "[3/3] 启动新容器..."
docker run -d \
  --name vte \
  -p 8050:8050 \
  -v vte-data:/app/backend/data \
  --restart unless-stopped \
  rtyedfty/vte:latest

echo
echo "========================================"
echo "  更新完成！"
echo "  访问: http://127.0.0.1:8050"
echo "========================================"

#!/bin/bash

echo "========================================"
echo "VTE 更新脚本"
echo "========================================"
echo

echo "[1/3] 拉取最新镜像..."
docker pull rtyedfty/vte:latest
if [ $? -ne 0 ]; then
    echo "错误: 拉取镜像失败"
    exit 1
fi

echo
echo "[2/3] 停止并删除旧容器..."
docker stop vte 2>/dev/null
sleep 2
docker rm vte 2>/dev/null

echo
echo "[3/3] 启动新容器（数据会保留）..."
docker run -d \
  --name vte \
  -p 8050:8050 \
  -v vte-data:/app/data \
  --restart unless-stopped \
  rtyedfty/vte:latest

if [ $? -ne 0 ]; then
    echo "错误: 启动容器失败"
    exit 1
fi

echo
echo "========================================"
echo "更新完成！"
echo "访问: http://localhost:8050"
echo "========================================"

@echo off
chcp 65001 >nul
REM 手动发布 Docker 镜像到 Docker Hub
REM 用法: publish-docker.bat [版本号]
REM 例如: publish-docker.bat 1.0.0

set IMAGE_NAME=rtyedfty/vte
set VERSION=%1
if "%VERSION%"=="" set VERSION=latest

echo ==========================================
echo   发布 VTE Docker 镜像
echo   镜像: %IMAGE_NAME%:%VERSION%
echo ==========================================

REM 登录 Docker Hub
echo 检查 Docker Hub 登录状态...
docker login

REM 构建镜像
echo 构建镜像...
docker buildx create --use --name vte-builder 2>nul
docker buildx build --platform linux/amd64,linux/arm64 -f Dockerfile -t %IMAGE_NAME%:%VERSION% -t %IMAGE_NAME%:latest --push .

echo ==========================================
echo   发布成功!
echo   docker pull %IMAGE_NAME%:%VERSION%
echo ==========================================
pause

@echo off
echo ========================================
echo VTE 更新脚本
echo ========================================
echo.

echo [1/3] 拉取最新镜像...
docker pull rtyedfty/vte:latest
if errorlevel 1 (
    echo 错误: 拉取镜像失败
    pause
    exit /b 1
)

echo.
echo [2/3] 停止并删除旧容器...
docker stop vte 2>nul
timeout /t 2 /nobreak >nul
docker rm vte 2>nul

echo.
echo [3/3] 启动新容器（数据会保留）...
docker run -d ^
  --name vte ^
  -p 8050:8050 ^
  -v vte-data:/app/data ^
  -e TZ=Asia/Shanghai ^
  --restart unless-stopped ^
  rtyedfty/vte:latest

if errorlevel 1 (
    echo 错误: 启动容器失败
    pause
    exit /b 1
)

echo.
echo ========================================
echo 更新完成！
echo 访问: http://localhost:8050
echo ========================================
pause

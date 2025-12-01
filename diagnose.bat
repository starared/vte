@echo off
chcp 65001 >nul
echo ========================================
echo   VTE 环境诊断工具
echo ========================================
echo.

echo [检查 1/5] 检查 Go 环境...
where go >nul 2>nul
if %errorlevel% neq 0 (
    echo ❌ 未安装 Go
    echo    请访问 https://go.dev/dl/ 下载安装
) else (
    go version
    echo ✅ Go 已安装
)
echo.

echo [检查 2/5] 检查 Node.js 环境...
where node >nul 2>nul
if %errorlevel% neq 0 (
    echo ❌ 未安装 Node.js
    echo    请访问 https://nodejs.org/ 下载安装
) else (
    node --version
    echo ✅ Node.js 已安装
)
echo.

echo [检查 3/5] 检查 npm...
where npm >nul 2>nul
if %errorlevel% neq 0 (
    echo ❌ 未安装 npm
) else (
    npm --version
    echo ✅ npm 已安装
)
echo.

echo [检查 4/5] 检查项目目录结构...
if exist "frontend" (
    echo ✅ frontend 目录存在
) else (
    echo ❌ 找不到 frontend 目录
)

if exist "backend" (
    echo ✅ backend 目录存在
) else (
    echo ❌ 找不到 backend 目录
)

if exist "frontend\node_modules" (
    echo ✅ 前端依赖已安装
) else (
    echo ⚠️  前端依赖未安装，首次运行 start.bat 会自动安装
)

if exist "frontend\dist\index.html" (
    echo ✅ 前端已构建
) else (
    echo ⚠️  前端未构建，首次运行 start.bat 会自动构建
)

if exist "backend\vte.exe" (
    echo ✅ 后端已构建
) else (
    echo ⚠️  后端未构建，首次运行 start.bat 会自动构建
)
echo.

echo [检查 5/5] 检查端口占用 (8050)...
netstat -ano | findstr :8050 >nul
if %errorlevel% equ 0 (
    echo ⚠️  端口 8050 已被占用
    echo    占用端口的进程:
    netstat -ano | findstr :8050
    echo.
    echo    如需关闭，请运行: taskkill /PID [进程ID] /F
) else (
    echo ✅ 端口 8050 可用
)
echo.

echo ========================================
echo   诊断完成
echo ========================================
echo.
echo 如果所有检查都通过，请运行 start.bat 启动服务
echo 如果遇到问题，请查看 TROUBLESHOOTING.zh-CN.md
echo.
pause

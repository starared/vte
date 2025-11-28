@echo off
chcp 65001 >nul
echo ========================================
echo   VTE - Multi-backend LLM API Gateway
echo ========================================
echo.

:: 检查 Go 是否安装
where go >nul 2>nul
if %errorlevel% neq 0 (
    echo [提示] 未检测到 Go，正在自动安装...
    winget install GoLang.Go --silent --accept-package-agreements --accept-source-agreements
    if %errorlevel% neq 0 (
        echo [错误] 自动安装失败，请手动安装 Go 1.21+
        echo 下载地址: https://go.dev/dl/
        pause
        exit /b 1
    )
    echo [提示] Go 安装完成，请关闭此窗口并重新运行 start.bat
    pause
    exit /b 0
)

:: 检查 Node.js 是否安装
where node >nul 2>nul
if %errorlevel% neq 0 (
    echo [提示] 未检测到 Node.js，正在自动安装...
    winget install OpenJS.NodeJS.LTS --silent --accept-package-agreements --accept-source-agreements
    if %errorlevel% neq 0 (
        echo [错误] 自动安装失败，请手动安装 Node.js 18+
        echo 下载地址: https://nodejs.org/
        pause
        exit /b 1
    )
    echo [提示] Node.js 安装完成，请关闭此窗口并重新运行 start.bat
    pause
    exit /b 0
)

:: 检查前端依赖和构建
echo [1/3] 检查前端...
cd frontend

:: 始终检查并安装/更新依赖
if not exist node_modules (
    echo 安装前端依赖...
    call npm install
    if %errorlevel% neq 0 (
        echo [错误] 前端依赖安装失败
        cd ..
        pause
        exit /b 1
    )
) else (
    echo 更新前端依赖...
    call npm install
    if %errorlevel% neq 0 (
        echo [警告] 依赖更新失败，尝试继续...
    )
)

:: 检查是否需要构建
if not exist "dist\index.html" (
    echo 构建前端...
    call npm run build
    if %errorlevel% neq 0 (
        echo [错误] 前端构建失败
        cd ..
        pause
        exit /b 1
    )
) else (
    echo 前端已构建
)
cd ..

:: 检查后端是否需要构建
if not exist "backend\vte.exe" (
    echo [2/3] 构建后端...
    cd backend
    echo 检查 Go 依赖...
    go mod download
    go mod tidy
    echo 编译后端...
    go build -o vte.exe .
    if %errorlevel% neq 0 (
        echo [错误] 后端编译失败
        cd ..
        pause
        exit /b 1
    )
    cd ..
) else (
    echo [2/3] 后端已构建，跳过
)

:: 启动
echo [3/3] 启动服务...
echo.
echo ========================================
echo   VTE 已启动
echo   访问地址: http://127.0.0.1:8050
echo   默认账号: admin / admin123
echo   按 Ctrl+C 停止服务
echo ========================================
echo.

cd backend
vte.exe

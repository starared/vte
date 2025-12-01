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
        echo.
        echo 如需帮助，请运行 diagnose.bat 检查环境配置
        echo 或查看 TROUBLESHOOTING.zh-CN.md 获取详细解决方案
        echo.
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
        echo.
        echo 如需帮助，请运行 diagnose.bat 检查环境配置
        echo 或查看 TROUBLESHOOTING.zh-CN.md 获取详细解决方案
        echo.
        pause
        exit /b 1
    )
    echo [提示] Node.js 安装完成，请关闭此窗口并重新运行 start.bat
    pause
    exit /b 0
)

:: 检查前端目录是否存在
if not exist "frontend" (
    echo [错误] 找不到 frontend 目录
    echo 请确保在项目根目录下运行此脚本
    echo.
    echo 如需帮助，请运行 diagnose.bat 检查环境配置
    echo.
    pause
    exit /b 1
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
        echo.
        echo 可能的原因：
        echo   - 网络连接问题
        echo   - npm 配置问题
        echo.
        echo 解决方案：
        echo   1. 检查网络连接
        echo   2. 尝试使用国内镜像: npm config set registry https://registry.npmmirror.com
        echo   3. 查看 TROUBLESHOOTING.zh-CN.md 获取更多帮助
        echo.
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
        echo.
        echo 可能的原因：
        echo   - 依赖安装不完整
        echo   - Node.js 版本过低
        echo.
        echo 解决方案：
        echo   1. 删除 node_modules 目录后重试
        echo   2. 确保 Node.js 版本 18 或更高
        echo   3. 查看 TROUBLESHOOTING.zh-CN.md 获取更多帮助
        echo.
        cd ..
        pause
        exit /b 1
    )
) else (
    echo 前端已构建
)
cd ..

:: 检查后端目录是否存在
if not exist "backend" (
    echo [错误] 找不到 backend 目录
    echo 请确保在项目根目录下运行此脚本
    echo.
    echo 如需帮助，请运行 diagnose.bat 检查环境配置
    echo.
    pause
    exit /b 1
)

:: 检查后端是否需要构建
if not exist "backend\vte.exe" (
    echo [2/3] 构建后端...
    cd backend
    echo 检查 Go 依赖...
    go mod download
    if %errorlevel% neq 0 (
        echo [错误] Go 依赖下载失败
        echo.
        echo 可能的原因：
        echo   - 网络连接问题
        echo   - Go 代理配置问题
        echo.
        echo 解决方案：
        echo   1. 设置 Go 代理: go env -w GOPROXY=https://goproxy.cn,direct
        echo   2. 查看 TROUBLESHOOTING.zh-CN.md 获取更多帮助
        echo.
        cd ..
        pause
        exit /b 1
    )
    go mod tidy
    echo 编译后端...
    go build -o vte.exe .
    if %errorlevel% neq 0 (
        echo [错误] 后端编译失败
        echo.
        echo 可能的原因：
        echo   - Go 版本过低（需要 1.21+）
        echo   - 依赖包下载不完整
        echo.
        echo 解决方案：
        echo   1. 检查 Go 版本: go version
        echo   2. 重新下载依赖: go mod download
        echo   3. 查看 TROUBLESHOOTING.zh-CN.md 获取更多帮助
        echo.
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

:: 程序退出后的处理
if %errorlevel% neq 0 (
    echo.
    echo ========================================
    echo [错误] 程序异常退出，错误代码: %errorlevel%
    echo ========================================
    echo.
    echo 可能的原因：
    echo   - 端口 8050 已被占用
    echo   - 配置文件损坏
    echo   - 权限不足
    echo.
    echo 解决方案：
    echo   1. 运行 diagnose.bat 检查环境
    echo   2. 检查是否有其他程序占用 8050 端口
    echo   3. 查看 TROUBLESHOOTING.zh-CN.md 获取更多帮助
    echo.
    pause
)

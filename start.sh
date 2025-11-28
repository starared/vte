#!/bin/bash
# VTE - Multi-backend LLM API Gateway

set -e

echo "========================================"
echo "  VTE - Multi-backend LLM API Gateway"
echo "========================================"
echo

# 检测系统类型
detect_os() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo "macos"
    elif [[ -f /etc/debian_version ]]; then
        echo "debian"
    elif [[ -f /etc/redhat-release ]]; then
        echo "redhat"
    else
        echo "unknown"
    fi
}

OS_TYPE=$(detect_os)

# 检查 Go
if ! command -v go &> /dev/null; then
    echo "[提示] 未检测到 Go，正在自动安装..."
    case $OS_TYPE in
        macos)
            if command -v brew &> /dev/null; then
                brew install go
            else
                echo "[错误] 请先安装 Homebrew 或手动安装 Go: https://go.dev/dl/"
                exit 1
            fi
            ;;
        debian)
            sudo apt update && sudo apt install -y golang-go
            ;;
        redhat)
            sudo yum install -y golang || sudo dnf install -y golang
            ;;
        *)
            echo "[错误] 无法自动安装，请手动安装 Go 1.21+: https://go.dev/dl/"
            exit 1
            ;;
    esac
    echo "[提示] Go 安装完成"
fi

# 检查 Node.js
if ! command -v node &> /dev/null; then
    echo "[提示] 未检测到 Node.js，正在自动安装..."
    case $OS_TYPE in
        macos)
            if command -v brew &> /dev/null; then
                brew install node
            else
                echo "[错误] 请先安装 Homebrew 或手动安装 Node.js: https://nodejs.org/"
                exit 1
            fi
            ;;
        debian)
            sudo apt update && sudo apt install -y nodejs npm
            ;;
        redhat)
            sudo yum install -y nodejs npm || sudo dnf install -y nodejs npm
            ;;
        *)
            echo "[错误] 无法自动安装，请手动安装 Node.js 18+: https://nodejs.org/"
            exit 1
            ;;
    esac
    echo "[提示] Node.js 安装完成"
fi

# 检查前端依赖和构建
echo "[1/3] 检查前端..."
cd frontend

# 始终检查并安装/更新依赖
if [ ! -d "node_modules" ]; then
    echo "安装前端依赖..."
    npm install
    if [ $? -ne 0 ]; then
        echo "[错误] 前端依赖安装失败"
        cd ..
        exit 1
    fi
else
    echo "更新前端依赖..."
    npm install
    if [ $? -ne 0 ]; then
        echo "[警告] 依赖更新失败，尝试继续..."
    fi
fi

# 检查是否需要构建
if [ ! -f "dist/index.html" ]; then
    echo "构建前端..."
    npm run build
    if [ $? -ne 0 ]; then
        echo "[错误] 前端构建失败"
        cd ..
        exit 1
    fi
else
    echo "前端已构建"
fi
cd ..

# 检查后端是否需要构建
if [ ! -f "backend/vte" ]; then
    echo "[2/3] 构建后端..."
    cd backend
    echo "检查 Go 依赖..."
    go mod download
    go mod tidy
    echo "编译后端..."
    go build -o vte .
    if [ $? -ne 0 ]; then
        echo "[错误] 后端编译失败"
        cd ..
        exit 1
    fi
    cd ..
else
    echo "[2/3] 后端已构建，跳过"
fi

# 启动
echo "[3/3] 启动服务..."
echo
echo "========================================"
echo "  VTE 已启动"
echo "  访问地址: http://127.0.0.1:8050"
echo "  默认账号: admin / admin123"
echo "  按 Ctrl+C 停止服务"
echo "========================================"
echo

cd backend
./vte

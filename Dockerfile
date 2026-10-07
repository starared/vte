# VTE - Go 版本 Dockerfile
# 内存占用 ~10-20MB，镜像体积 ~30MB

# ========== 前端构建 ==========
FROM node:22-alpine AS frontend

WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ .
RUN npm run build

# ========== Go 后端构建 ==========
FROM golang:1.24-alpine AS builder

WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
COPY VERSION /app/VERSION
# 版本号从 VERSION 文件注入，保证二进制和镜像版本一致
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X vte/internal/handlers.Version=$(cat /app/VERSION)" -o vte .

# ========== 最终镜像 ==========
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 复制 Go 二进制
COPY --from=builder /app/backend/vte ./

# 复制前端构建产物
COPY --from=frontend /app/frontend/dist ./frontend/dist

# 复制版本文件
COPY VERSION ./

# 创建数据目录
RUN mkdir -p /app/data

# 环境变量
ENV DATABASE_PATH=/app/data/gateway.db
ENV HOST=0.0.0.0
ENV PORT=8050
ENV TZ=Asia/Shanghai

EXPOSE 8050

VOLUME ["/app/data"]

CMD ["./vte"]

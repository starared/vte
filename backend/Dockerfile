# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o vte .

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/vte .

# 创建数据目录
RUN mkdir -p /app/data

# 环境变量
ENV DATABASE_PATH=/app/data/gateway.db
ENV HOST=0.0.0.0
ENV PORT=8050

EXPOSE 8050

VOLUME ["/app/data"]

CMD ["./vte"]

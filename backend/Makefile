.PHONY: build run clean docker

# 构建
build:
	go mod tidy
	CGO_ENABLED=1 go build -ldflags="-s -w" -o vte .

# 运行
run: build
	./vte

# 清理
clean:
	rm -f vte vte.exe
	rm -rf data/

# Windows 构建
build-windows:
	go mod tidy
	set CGO_ENABLED=1 && go build -ldflags="-s -w" -o vte.exe .

# Docker 构建
docker:
	docker build -t vte-go -f Dockerfile .

# 交叉编译 Linux (需要交叉编译工具链)
build-linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o vte-linux .

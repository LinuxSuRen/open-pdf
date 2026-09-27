BINARY := bin/open-pdf
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PKG := github.com/linuxsuren/open-pdf

.PHONY: all build web test test-race lint run fixtures docker-build docker-run clean help

all: lint test build ## 代码检查 + 测试 + 构建

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

web: ## 构建前端（需要 Node 20+；产物 web/dist 会嵌入二进制）
	cd web && npm ci --no-audit --no-fund && npm run build

build: ## 编译后端二进制（嵌入 web/dist）
	CGO_ENABLED=0 go build -trimpath \
		-ldflags "-s -w -X ${PKG}/internal/buildinfo.Version=$(VERSION)" \
		-o $(BINARY) ./cmd/open-pdf

test: ## 运行全部测试
	go test ./...

test-race: ## 带 race 检测运行全部测试
	go test -race -count=1 ./...

lint: ## go vet + gofmt 检查
	go vet ./...
	@test -z "$$(gofmt -l cmd internal web/embed.go)" || (gofmt -l cmd internal web/embed.go && exit 1)

run: build ## 本地启动服务（默认 :8080）
	./$(BINARY)

fixtures: ## 生成示例文件到 build/fixtures
	go run ./cmd/mkfixtures build/fixtures

docker-build: ## 构建容器镜像
	docker build -t open-pdf:$(VERSION) .

docker-run: ## 运行容器镜像（端口 8080）
	docker run --rm -p 8080:8080 --tmpfs /tmp open-pdf:$(VERSION)

clean: ## 清理构建产物
	rm -rf bin build

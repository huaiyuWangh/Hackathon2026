GO  ?= go
BIN := bin/bridge

# cgo 音频依赖（portaudio/opus）：MacPorts 的 pkg-config 看不到 Homebrew 路径，显式补上
export PKG_CONFIG_PATH := /usr/local/lib/pkgconfig:/opt/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)

.PHONY: build run test cover vet lint clean

build:
	$(GO) build -o $(BIN) ./cmd/bridge

run: build
	$(BIN)

test:
	$(GO) test -race ./...

# 覆盖率汇总（与 CI 的 coverage.txt 同格式）
cover:
	$(GO) test -coverprofile=coverage.txt -covermode=atomic ./...
	$(GO) tool cover -func=coverage.txt | tail -1
	@echo "明细: go tool cover -html=coverage.txt"

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin coverage.txt

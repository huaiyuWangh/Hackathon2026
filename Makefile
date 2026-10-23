GO  ?= go
BIN := bin/bridge

# cgo 音频依赖（portaudio/opus）：MacPorts 的 pkg-config 看不到 Homebrew 路径，显式补上
export PKG_CONFIG_PATH := /usr/local/lib/pkgconfig:/opt/local/lib/pkgconfig$(if $(PKG_CONFIG_PATH),:$(PKG_CONFIG_PATH),)

.PHONY: build run test vet clean

build:
	$(GO) build -o $(BIN) ./cmd/bridge

run: build
	$(BIN)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

clean:
	rm -rf bin

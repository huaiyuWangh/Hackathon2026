GO  ?= go
BIN := bin/bridge

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

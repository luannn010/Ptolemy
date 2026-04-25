APP_NAME=workerd
BIN_DIR=bin

run:
	go run ./cmd/workerd

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP_NAME) ./cmd/workerd
	
build-mcp:
	mkdir -p bin
	go build -o bin/ptolemy-mcp ./cmd/ptolemy-mcp

test:
	go test ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy
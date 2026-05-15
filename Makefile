APP_NAME=workerd
BIN_DIR=bin

run:
	go run ./cmd/workerd

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/workerd ./cmd/workerd
	go build -o $(BIN_DIR)/ptolemy ./cmd/ptolemy
	go build -o $(BIN_DIR)/ptolemy-mcp ./cmd/ptolemy-mcp
	go build -o $(BIN_DIR)/ptolemy-task-runner ./cmd/ptolemy-task-runner

test:
	go test -p 1 ./...

test-integration:
	go test -tags=integration ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

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

# Windows-only voice catcher.
# voice-native: uses the built-in Windows System.Speech listener (no extra deps).
voice-native:
	mkdir -p $(BIN_DIR)
	GOOS=windows go build -o $(BIN_DIR)/ptolemy-voice.exe ./cmd/ptolemy-voice

# voice: uses the Vosk offline listener (needs PortAudio + a Vosk model via VOSK_MODEL_PATH).
voice:
	mkdir -p $(BIN_DIR)
	GOOS=windows go build -tags vosk -o $(BIN_DIR)/ptolemy-voice.exe ./cmd/ptolemy-voice

test:
	go test -p 1 ./...

test-integration:
	go test -tags=integration ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

.PHONY: build run clean help install test

BINARY_NAME=anomaly-detector
MAIN_PACKAGE=./

help:
	@echo "Anomaly Detection Agent - Make Targets"
	@echo "======================================"
	@echo "  make build       - Build the binary for current OS"
	@echo "  make build-linux - Cross-compile for Linux"
	@echo "  make run         - Run the agent (requires sudo)"
	@echo "  make install     - Install to /usr/local/bin (requires sudo)"
	@echo "  make clean       - Remove build artifacts"
	@echo "  make test        - Run basic tests"
	@echo "  make help        - Show this help message"

build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Done. Binary: ./$(BINARY_NAME)"

build-linux:
	@echo "Building $(BINARY_NAME) for Linux (amd64)..."
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux $(MAIN_PACKAGE)
	@echo "Done. Binary: ./$(BINARY_NAME)-linux"

run: build
	@echo "Running agent (requires sudo)..."
	sudo ./$(BINARY_NAME) -verbose

install: build
	@echo "Installing to /usr/local/bin..."
	sudo cp $(BINARY_NAME) /usr/local/bin/
	@echo "Installed. Run with: sudo anomaly-detector"

clean:
	@echo "Cleaning up..."
	go clean
	rm -f $(BINARY_NAME) $(BINARY_NAME)-linux
	@echo "Done."

test:
	@echo "Running Go tests..."
	go test -v -race ./...

fmt:
	@echo "Formatting code..."
	go fmt ./...

lint:
	@echo "Running linter..."
	go vet ./...

deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod verify

.PHONY: deps fmt lint

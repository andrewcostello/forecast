.PHONY: build test install clean help

# Variables
BINARY_NAME=forecast
GO=go
GOFLAGS=-v

# Build
build:
	$(GO) build $(GOFLAGS) -o $(BINARY_NAME) cmd/forecast/main.go

# Install to $GOPATH/bin
install:
	$(GO) install $(GOFLAGS) cmd/forecast/main.go

# Run tests
test:
	$(GO) test ./... -v

# Run tests with coverage
test-coverage:
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out

# Download dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

# Format code
fmt:
	$(GO) fmt ./...

# Lint
lint:
	golangci-lint run

# Run
run:
	$(GO) run cmd/forecast/main.go

# Help
help:
	@echo "Available targets:"
	@echo "  build          - Build the binary"
	@echo "  install        - Install to \$$GOPATH/bin"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  clean          - Remove build artifacts"
	@echo "  deps           - Download dependencies"
	@echo "  fmt            - Format code"
	@echo "  lint           - Run linter"
	@echo "  run            - Run without building"

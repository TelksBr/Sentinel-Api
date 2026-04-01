# API V2 - Makefile para build Linux (desenvolvido no Windows)

BINARY_NAME=api-v2
BUILD_DIR=build
VERSION?=1.0.0

# Build para Linux x64
.PHONY: build-x64
build-x64:
	@echo "Building for Linux x64..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)_x64 ./cmd/api

# Build para Linux ARM64
.PHONY: build-arm64
build-arm64:
	@echo "Building for Linux ARM64..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)_arm64 ./cmd/api

# Build para todas as plataformas Linux
.PHONY: build-all
build-all: build-x64 build-arm64
	@echo "All Linux builds completed!"

# Build estático (sem dependências externas)
.PHONY: build-static
build-static:
	@echo "Building static binary for Linux x64..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME)_static ./cmd/api

# Limpar builds
.PHONY: clean
clean:
	@echo "Cleaning build directory..."
	@rm -rf $(BUILD_DIR)

# Instalar dependências
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	@go mod tidy
	@go mod download

# Executar em modo desenvolvimento
.PHONY: run
run:
	@echo "Running in development mode..."
	@go run ./cmd/api

# Testes
.PHONY: test
test:
	@echo "Running tests..."
	@go test ./...

# Formatar código
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Lint
.PHONY: lint
lint:
	@echo "Running linter..."
	@golangci-lint run

# Help
.PHONY: help
help:
	@echo "Available commands:"
	@echo "  build-x64     - Build for Linux x64"
	@echo "  build-arm64   - Build for Linux ARM64"
	@echo "  build-all     - Build for all Linux platforms"
	@echo "  build-static  - Build static binary for Linux"
	@echo "  clean         - Clean build directory"
	@echo "  deps          - Install dependencies"
	@echo "  run           - Run in development mode"
	@echo "  test          - Run tests"
	@echo "  fmt           - Format code"
	@echo "  lint          - Run linter"
	@echo "  help          - Show this help"

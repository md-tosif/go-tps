.PHONY: build run clean test fmt install help

# Binary name
BINARY_NAME=go-tps

# Build the project
build:
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) .
	@echo "Build complete: $(BINARY_NAME)"

# Run the project
run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(BINARY_NAME)

# Run with custom RPC
run-local: build
	@echo "Running $(BINARY_NAME) with local RPC..."
	@RPC_URL=http://localhost:8545 ./$(BINARY_NAME)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -f $(BINARY_NAME)-*
	@rm -f *.db
	@rm -f mnemonics.txt
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Install dependencies
install:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies installed"

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 .
	@GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 .
	@GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 .
	@GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME)-windows-amd64.exe .
	@echo "Multi-platform build complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  make build      - Build the project"
	@echo "  make run        - Build and run the project"
	@echo "  make run-local  - Build and run with local RPC"
	@echo "  make clean      - Remove build artifacts and database"
	@echo "  make test       - Run tests"
	@echo "  make fmt        - Format code"
	@echo "  make install    - Install/update dependencies"
	@echo "  make build-all  - Build for multiple platforms"
	@echo "  make help       - Show this help message"

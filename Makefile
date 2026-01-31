.PHONY: all clean quick test help install release

# Build configuration
BINARY_NAME=terminal-ai
VERSION=$(shell git describe --tags --abbrev=0 2>/dev/null || echo "1.0.0")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_DIR=build

# Go build flags
LDFLAGS=-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)

# Default target
all:
	@echo "Building all platforms..."
	@./build/build-all.sh

# Quick build for current platform
quick:
	@echo "Quick build for current platform..."
	@./build/quick-build.sh

# Build specific platforms
linux-amd64:
	@mkdir -p $(BUILD_DIR)/linux-amd64
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/linux-amd64/$(BINARY_NAME) .
	@cp ui.html .env.example README.md $(BUILD_DIR)/linux-amd64/ 2>/dev/null || true
	@cd $(BUILD_DIR)/linux-amd64 && tar -czf ../$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz * && cd ../..
	@echo "✓ linux-amd64 built"

linux-arm64:
	@mkdir -p $(BUILD_DIR)/linux-arm64
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/linux-arm64/$(BINARY_NAME) .
	@cp ui.html .env.example README.md $(BUILD_DIR)/linux-arm64/ 2>/dev/null || true
	@cd $(BUILD_DIR)/linux-arm64 && tar -czf ../$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz * && cd ../..
	@echo "✓ linux-arm64 built"

linux-arm:
	@mkdir -p $(BUILD_DIR)/linux-arm
	@GOOS=linux GOARCH=arm CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/linux-arm/$(BINARY_NAME) .
	@cp ui.html .env.example README.md $(BUILD_DIR)/linux-arm/ 2>/dev/null || true
	@cd $(BUILD_DIR)/linux-arm && tar -czf ../$(BINARY_NAME)-$(VERSION)-linux-arm.tar.gz * && cd ../..
	@echo "✓ linux-arm built"

windows-amd64:
	@mkdir -p $(BUILD_DIR)/windows-amd64
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/windows-amd64/$(BINARY_NAME).exe .
	@cp ui.html .env.example README.md $(BUILD_DIR)/windows-amd64/ 2>/dev/null || true
	@cd $(BUILD_DIR)/windows-amd64 && zip -r ../$(BINARY_NAME)-$(VERSION)-windows-amd64.zip * && cd ../..
	@echo "✓ windows-amd64 built"

darwin-amd64:
	@mkdir -p $(BUILD_DIR)/darwin-amd64
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/darwin-amd64/$(BINARY_NAME) .
	@cp ui.html .env.example README.md $(BUILD_DIR)/darwin-amd64/ 2>/dev/null || true
	@cd $(BUILD_DIR)/darwin-amd64 && tar -czf ../$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz * && cd ../..
	@echo "✓ darwin-amd64 built"

darwin-arm64:
	@mkdir -p $(BUILD_DIR)/darwin-arm64
	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(BUILD_DIR)/darwin-arm64/$(BINARY_NAME) .
	@cp ui.html .env.example README.md $(BUILD_DIR)/darwin-arm64/ 2>/dev/null || true
	@cd $(BUILD_DIR)/darwin-arm64 && tar -czf ../$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz * && cd ../..
	@echo "✓ darwin-arm64 built"

# Build for Raspberry Pi (ARM)
rpi: linux-arm64 linux-arm
	@echo "✓ Raspberry Pi builds complete"

# Build all Linux platforms
linux: linux-amd64 linux-arm64 linux-arm
	@echo "✓ All Linux builds complete"

# Build all macOS platforms
macos: darwin-amd64 darwin-arm64
	@echo "✓ All macOS builds complete"

# Build all Windows platforms
windows: windows-amd64
	@echo "✓ All Windows builds complete"

# Test the binary
test:
	@echo "Running tests..."
	@go test -v ./...

# Clean build directory
clean:
	@echo "Cleaning build directory..."
	@rm -rf $(BUILD_DIR)
	@echo "✓ Clean complete"

# Install locally
install:
	@echo "Installing $(BINARY_NAME)..."
	@go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .
	@sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "✓ Installed to /usr/local/bin/$(BINARY_NAME)"

# Create release artifacts
release: all
	@echo "Release artifacts ready in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/*.tar.gz $(BUILD_DIR)/*.zip 2>/dev/null || true

# Show help
help:
	@echo "Terminal AI CLI - Build Targets"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all           - Build all platforms (default)"
	@echo "  quick         - Quick build for current platform"
	@echo "  linux         - Build all Linux platforms"
	@echo "  linux-amd64   - Build Linux x86_64"
	@echo "  linux-arm64   - Build Linux ARM64"
	@echo "  linux-arm     - Build Linux ARM (v7)"
	@echo "  windows       - Build all Windows platforms"
	@echo "  windows-amd64 - Build Windows x86_64"
	@echo "  macos         - Build all macOS platforms"
	@echo "  darwin-amd64  - Build macOS Intel"
	@echo "  darwin-arm64  - Build macOS Apple Silicon"
	@echo "  rpi           - Build Raspberry Pi (ARM64, ARM)"
	@echo "  test          - Run tests"
	@echo "  clean         - Clean build directory"
	@echo "  install       - Install to /usr/local/bin"
	@echo "  release       - Create release artifacts"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make quick           # Quick build"
	@echo "  make linux-amd64     # Build for Linux x86_64"
	@echo "  make windows         # Build all Windows platforms"
	@echo "  make rpi             # Build for Raspberry Pi"
	@echo "  make install         # Install locally"

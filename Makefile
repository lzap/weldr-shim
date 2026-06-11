.PHONY: all build clean install test fmt lint vet check smoke-test

# Build settings
BINARY_NAME = weldr-shim
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

# Paths
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
SOCKETDIR ?= /run/weldr
CACHEDIR ?= /var/cache/weldr-shim

all: build

# Build the binary
build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

# Start the service
start: build
	sudo ./$(BINARY_NAME)

# Install binary
install: build
	install -D -m 0755 $(BINARY_NAME) $(DESTDIR)$(BINDIR)/$(BINARY_NAME)

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf vendor/

# Format code
fmt:
	go fmt ./...

# Run linters
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed, skipping"; exit 0; }
	golangci-lint run

# Run go vet
vet:
	go vet ./...

# Check: format, vet, and lint
check: fmt vet lint

# Run smoke test
smoke-test:
	@echo "Running smoke tests..."
	./smoke-test.sh

# Help
help:
	@echo "Available targets:"
	@echo "  all         - Build the binary (default)"
	@echo "  build       - Build the binary"
	@echo "  install     - Install binary to $(BINDIR)"
	@echo "  clean       - Remove build artifacts"
	@echo "  fmt         - Format Go code"
	@echo "  lint        - Run golangci-lint (if installed)"
	@echo "  vet         - Run go vet"
	@echo "  check       - Run fmt, vet, and lint"
	@echo "  smoke-test  - Run integration smoke test"
	@echo ""
	@echo "Variables:"
	@echo "  PREFIX      - Installation prefix (default: /usr/local)"
	@echo "  BINDIR      - Binary installation directory (default: PREFIX/bin)"
	@echo "  VERSION     - Version string (default: git describe)"

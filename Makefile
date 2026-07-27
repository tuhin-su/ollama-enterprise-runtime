.PHONY: all build install uninstall update clean help

# Target installation directory
PREFIX ?= /usr/local
INSTALL_BIN_DIR = $(PREFIX)/bin
INSTALL_LIB_DIR = $(PREFIX)/lib/ollama

# CGO compilation variables for LanceDB CGO bindings
export CGO_CFLAGS := -I$(shell pwd)/include
export CGO_LDFLAGS := $(shell pwd)/lib/linux_amd64/liblancedb_go.a -lm -ldl -lpthread

# Number of parallel compilation jobs
PARALLEL_JOBS ?= $(shell nproc 2>/dev/null || echo 8)

all: build

help:
	@echo "Ollama Master Makefile"
	@echo "Available commands:"
	@echo "  make build      - Configure and build Ollama & native llama-server payload"
	@echo "  make install    - Install built ollama binary and native libraries to system (may require sudo)"
	@echo "  make uninstall  - Remove installed ollama binary and libraries from system (may require sudo)"
	@echo "  make update     - Pull the latest changes from repository and rebuild"
	@echo "  make clean      - Clean cmake build artifacts"

lancedb-bindings:
	@echo ">>> Ensuring LanceDB CGO native bindings are present..."
	@if [ ! -f "include/lancedb.h" ] || [ ! -f "lib/linux_amd64/liblancedb_go.a" ]; then \
		echo ">>> Downloading LanceDB native bindings..."; \
		LANCE_MOD_DIR=$$(go list -m -f '{{.Dir}}' github.com/lancedb/lancedb-go 2>/dev/null || true); \
		if [ -z "$$LANCE_MOD_DIR" ]; then \
			echo ">>> Resolving lancedb-go dependency..."; \
			go get github.com/lancedb/lancedb-go@v0.1.2; \
			LANCE_MOD_DIR=$$(go list -m -f '{{.Dir}}' github.com/lancedb/lancedb-go); \
		fi; \
		DOWNLOAD_SCRIPT="$$LANCE_MOD_DIR/scripts/download-artifacts.sh"; \
		if [ -f "$$DOWNLOAD_SCRIPT" ]; then \
			bash "$$DOWNLOAD_SCRIPT" v0.1.2; \
		else \
			echo "Error: Could not locate lancedb-go download script at $$DOWNLOAD_SCRIPT"; \
			exit 1; \
		fi; \
	else \
		echo ">>> LanceDB native bindings already present."; \
	fi

build: lancedb-bindings
	@echo ">>> Configuring superbuild..."
	cmake -B build -DCMAKE_INSTALL_PREFIX=$(PREFIX) .
	@echo ">>> Compiling native payload runner (llama-server)..."
	cmake --build build --parallel $(PARALLEL_JOBS) --target ollama-llama-server-local
	@echo ">>> Compiling Ollama Go binary..."
	export CGO_CFLAGS="-I$$(pwd)/include" && \
	export CGO_LDFLAGS="$$(pwd)/lib/linux_amd64/liblancedb_go.a -lm" && \
	go build -o ollama .
	@echo ">>> Build completed successfully! You can run ./ollama serve or run 'make install'."

install:
	@echo ">>> Installing Ollama to $(PREFIX)..."
	@if [ -d "build" ]; then \
		cmake --install build; \
		echo ">>> Install completed successfully."; \
	else \
		echo "Error: Build directory not found. Please run 'make build' first."; \
		exit 1; \
	fi

uninstall:
	@echo ">>> Uninstalling Ollama from $(PREFIX)..."
	rm -f $(INSTALL_BIN_DIR)/ollama
	rm -rf $(INSTALL_LIB_DIR)
	@echo ">>> Uninstall completed successfully."

update:
	@echo ">>> Updating repository..."
	git pull
	@echo ">>> Rebuilding..."
	$(MAKE) build

clean:
	@echo ">>> Cleaning build artifacts..."
	rm -rf build
	rm -f ollama
	@echo ">>> Clean completed."

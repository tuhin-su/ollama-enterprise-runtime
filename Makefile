.PHONY: all build install uninstall update clean help

# Target installation directory
PREFIX ?= /usr/local
INSTALL_BIN_DIR = $(PREFIX)/bin
INSTALL_LIB_DIR = $(PREFIX)/lib/loom

# CGO compilation variables for LanceDB CGO bindings
export CGO_CFLAGS := -I$(shell pwd)/include
export CGO_LDFLAGS := $(shell pwd)/lib/linux_amd64/liblancedb_go.a -lm -ldl -lpthread

# Number of parallel compilation jobs
PARALLEL_JOBS ?= $(shell nproc 2>/dev/null || echo 8)

all: build

help:
	@echo "Loom Master Makefile"
	@echo "Available commands:"
	@echo "  make build      - Configure and build Loom & native llama-server payload"
	@echo "  make install    - Install built loom binary and native libraries to system (may require sudo)"
	@echo "  make uninstall  - Remove installed loom binary and libraries from system (may require sudo)"
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
	@echo ">>> Checking if GPU exists..."
	@if command -v nvidia-smi >/dev/null 2>&1 && nvidia-smi >/dev/null 2>&1; then \
		echo ">>> NVIDIA GPU detected! Configuring with CUDA v13 backend..."; \
		cmake -B build -DCMAKE_INSTALL_PREFIX=$(PREFIX) -DLOOM_LLAMA_BACKENDS=cuda_v13 .; \
	else \
		echo ">>> No GPU detected. Configuring CPU-only build..."; \
		cmake -B build -DCMAKE_INSTALL_PREFIX=$(PREFIX) .; \
	fi
	@echo ">>> Compiling payloads and binary..."
	cmake --build build --parallel $(PARALLEL_JOBS)
	@if [ -f build/loom ]; then cp build/loom .; fi
	@echo ">>> Build completed successfully! You can run ./loom serve or run 'make install'."

install:
	@echo ">>> Installing Loom to $(PREFIX)..."
	@if [ -d "build" ]; then \
		cmake --install build; \
		echo ">>> Install completed successfully."; \
	else \
		echo "Error: Build directory not found. Please run 'make build' first."; \
		exit 1; \
	fi

uninstall:
	@echo ">>> Uninstalling Loom from $(PREFIX)..."
	rm -f $(INSTALL_BIN_DIR)/loom
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
	rm -f loom
	@echo ">>> Clean completed."

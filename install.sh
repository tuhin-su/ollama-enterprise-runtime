#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== Loom Build & Install Script ===${NC}"

# Detect go environment
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go compiler is not installed or not in PATH.${NC}"
    exit 1
fi

# Detect platform operating system
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [[ "$OS" != "linux" ]]; then
    echo -e "${YELLOW}Warning: Only Linux platform is officially tested with the static library linker paths in this script.${NC}"
fi

# 1. Download LanceDB CGO native bindings if they don't exist
if [[ ! -f "include/lancedb.h" || ! -f "lib/linux_amd64/liblancedb_go.a" ]]; then
    echo -e "${BLUE}Downloading LanceDB CGO native bindings...${NC}"
    
    # Locate the download script in go mod cache
    LANCE_MOD_DIR=$(go list -m -f '{{.Dir}}' github.com/lancedb/lancedb-go 2>/dev/null || true)
    if [[ -z "$LANCE_MOD_DIR" ]]; then
        echo -e "${BLUE}Resolving lancedb-go dependency...${NC}"
        go get github.com/lancedb/lancedb-go@v0.1.2
        LANCE_MOD_DIR=$(go list -m -f '{{.Dir}}' github.com/lancedb/lancedb-go)
    fi
    
    DOWNLOAD_SCRIPT="$LANCE_MOD_DIR/scripts/download-artifacts.sh"
    if [[ -f "$DOWNLOAD_SCRIPT" ]]; then
        bash "$DOWNLOAD_SCRIPT" v0.1.2
    else
        echo -e "${RED}Error: Could not locate lancedb-go download script at: $DOWNLOAD_SCRIPT${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}LanceDB native artifacts already present.${NC}"
fi

ROOT_DIR=$(pwd)
FORCE_BUILD=false
for arg in "$@"; do
    if [[ "$arg" == "--build" ]]; then
        FORCE_BUILD=true
    fi
done

if [[ "$FORCE_BUILD" == "true" ]] || [[ ! -f "loom" ]]; then
    # 2. Build Loom binary
    echo -e "${BLUE}Building Loom with LanceDB storage engine...${NC}"

    export CGO_CFLAGS="-I$ROOT_DIR/include"
    export CGO_LDFLAGS="$ROOT_DIR/lib/linux_amd64/liblancedb_go.a -lm -ldl -lpthread"

    go build -o loom .

    echo -e "${GREEN}Build successful: Binary compiled at $ROOT_DIR/loom${NC}"
else
    echo -e "${GREEN}Loom binary already exists. Skipping build. (Use --build to force)${NC}"
fi

# 3. Install Loom binary
INSTALL_DIR="/usr/local/bin"

echo -e "${BLUE}Installing loom binary to $INSTALL_DIR...${NC}"
if [ -w "$INSTALL_DIR" ]; then
    rm -f "$INSTALL_DIR/loom"
    cp loom "$INSTALL_DIR/loom"
    if [ -d "$ROOT_DIR/build/lib/loom" ]; then
        mkdir -p /usr/local/lib
        rm -rf "/usr/local/lib/loom"
        cp -r "$ROOT_DIR/build/lib/loom" "/usr/local/lib/loom"
    fi
    echo -e "${GREEN}Success! loom has been installed to $INSTALL_DIR/loom${NC}"
else
    echo -e "${YELLOW}Notice: Write permission denied to $INSTALL_DIR. Trying with sudo...${NC}"
    if command -v sudo &> /dev/null; then
        sudo rm -f "$INSTALL_DIR/loom"
        sudo cp loom "$INSTALL_DIR/loom"
        if [ -d "$ROOT_DIR/build/lib/loom" ]; then
            sudo mkdir -p /usr/local/lib
            sudo rm -rf "/usr/local/lib/loom"
            sudo cp -r "$ROOT_DIR/build/lib/loom" "/usr/local/lib/loom"
        fi
        echo -e "${GREEN}Success! loom has been installed to $INSTALL_DIR/loom using sudo.${NC}"
    else
        echo -e "${RED}Error: Could not write to $INSTALL_DIR and 'sudo' is not available.${NC}"
        echo -e "${YELLOW}Please manually copy the 'loom' binary and 'build/lib/loom' payloads.${NC}"
    fi
fi

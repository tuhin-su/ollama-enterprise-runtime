#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== Ollama Build & Install Script ===${NC}"

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

# 2. Build Ollama binary
echo -e "${BLUE}Building Ollama with LanceDB storage engine...${NC}"
ROOT_DIR=$(pwd)

export CGO_CFLAGS="-I$ROOT_DIR/include"
export CGO_LDFLAGS="$ROOT_DIR/lib/linux_amd64/liblancedb_go.a -lm -ldl -lpthread"

go build -o ollama .

echo -e "${GREEN}Build successful: Binary compiled at $ROOT_DIR/ollama${NC}"

# 3. Install Ollama binary
INSTALL_DIR="/usr/local/bin"

echo -e "${BLUE}Installing ollama binary to $INSTALL_DIR...${NC}"
if [ -w "$INSTALL_DIR" ]; then
    cp ollama "$INSTALL_DIR/ollama"
    echo -e "${GREEN}Success! ollama has been installed to $INSTALL_DIR/ollama${NC}"
else
    echo -e "${YELLOW}Notice: Write permission denied to $INSTALL_DIR. Trying with sudo...${NC}"
    if command -v sudo &> /dev/null; then
        sudo cp ollama "$INSTALL_DIR/ollama"
        echo -e "${GREEN}Success! ollama has been installed to $INSTALL_DIR/ollama using sudo.${NC}"
    else
        echo -e "${RED}Error: Could not write to $INSTALL_DIR and 'sudo' is not available.${NC}"
        echo -e "${YELLOW}Please manually copy the 'ollama' binary to your path, e.g.:${NC}"
        echo -e "  cp ollama ~/.local/bin/ollama"
    fi
fi

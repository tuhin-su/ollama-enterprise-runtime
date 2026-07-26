#!/usr/bin/env bash
# install.sh — Universal installer for tuhin-su/ollama-master
# Supports: Linux (amd64, arm64, arm), macOS (amd64, arm64)
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/tuhin-su/ollama-master/main/install.sh | sh
#   VERSION=v1.2.3 sh install.sh          # pin a specific version
#   INSTALL_DIR=/usr/local/bin sh install.sh

set -euo pipefail

# ─────────────────────────────────────────────
# Config
# ─────────────────────────────────────────────
REPO="tuhin-su/ollama-master"
BINARY="ollama"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# ─────────────────────────────────────────────
# Helpers
# ─────────────────────────────────────────────
info()    { printf '\033[0;32m[info]\033[0m  %s\n' "$*"; }
warn()    { printf '\033[0;33m[warn]\033[0m  %s\n' "$*" >&2; }
error()   { printf '\033[0;31m[error]\033[0m %s\n' "$*" >&2; exit 1; }
success() { printf '\033[0;34m[done]\033[0m  %s\n' "$*"; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || error "Required command not found: $1. Please install it and retry."
}

# ─────────────────────────────────────────────
# Detect OS
# ─────────────────────────────────────────────
detect_os() {
  local os
  os="$(uname -s)"
  case "$os" in
    Linux)  echo "linux" ;;
    Darwin) echo "darwin" ;;
    *)      error "Unsupported OS: $os. For Windows use install.ps1" ;;
  esac
}

# ─────────────────────────────────────────────
# Detect CPU architecture
# ─────────────────────────────────────────────
detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64 | amd64)          echo "amd64" ;;
    aarch64 | arm64)         echo "arm64" ;;
    armv7l | armv7 | armhf)  echo "arm"   ;;
    armv6l)                  echo "arm"   ;;
    *)                       error "Unsupported architecture: $arch" ;;
  esac
}

# ─────────────────────────────────────────────
# Resolve latest version tag from GitHub API
# ─────────────────────────────────────────────
latest_version() {
  local api_url="https://api.github.com/repos/${REPO}/releases/latest"
  local version

  if command -v curl >/dev/null 2>&1; then
    version="$(curl -fsSL "$api_url" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  elif command -v wget >/dev/null 2>&1; then
    version="$(wget -qO- "$api_url" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  else
    error "Neither curl nor wget found. Please install one and retry."
  fi

  [ -n "$version" ] || error "Could not determine latest release version. Check your internet connection."
  echo "$version"
}

# ─────────────────────────────────────────────
# Download a file (curl or wget)
# ─────────────────────────────────────────────
download() {
  local url="$1"
  local dest="$2"
  info "Downloading: $url"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --progress-bar "$url" -o "$dest"
  else
    wget -q --show-progress "$url" -O "$dest"
  fi
}

# ─────────────────────────────────────────────
# Verify SHA256 checksum if a .sha256 file exists
# ─────────────────────────────────────────────
verify_checksum() {
  local file="$1"
  local checksum_url="$2"
  local checksum_file="${file}.sha256"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$checksum_url" -o "$checksum_file" 2>/dev/null || { warn "No checksum file found, skipping verification."; return 0; }
  else
    wget -qO "$checksum_file" "$checksum_url" 2>/dev/null || { warn "No checksum file found, skipping verification."; return 0; }
  fi

  info "Verifying checksum..."
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$(dirname "$file")" && echo "$(cat "$checksum_file")  $(basename "$file")" | sha256sum -c -) \
      || error "Checksum verification failed! The download may be corrupt."
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$(dirname "$file")" && echo "$(cat "$checksum_file")  $(basename "$file")" | shasum -a 256 -c -) \
      || error "Checksum verification failed! The download may be corrupt."
  else
    warn "sha256sum/shasum not found, skipping checksum verification."
  fi
}

# ─────────────────────────────────────────────
# Install binary
# ─────────────────────────────────────────────
install_binary() {
  local src="$1"
  local install_path="${INSTALL_DIR}/${BINARY}"

  # Create install dir if needed
  if [ ! -d "$INSTALL_DIR" ]; then
    info "Creating $INSTALL_DIR ..."
    if [ "$(id -u)" -eq 0 ]; then
      mkdir -p "$INSTALL_DIR"
    else
      sudo mkdir -p "$INSTALL_DIR"
    fi
  fi

  info "Installing to $install_path ..."
  chmod +x "$src"
  if [ -w "$INSTALL_DIR" ]; then
    cp "$src" "$install_path"
  else
    sudo cp "$src" "$install_path"
  fi

  success "Installed $BINARY $(${install_path} --version 2>/dev/null || true)"
}

# ─────────────────────────────────────────────
# Post-install: systemd service (Linux only)
# ─────────────────────────────────────────────
install_systemd_service() {
  command -v systemctl >/dev/null 2>&1 || return 0
  [ "$(id -u)" -ne 0 ] && ! command -v sudo >/dev/null 2>&1 && return 0

  local service_file="/etc/systemd/system/ollama.service"
  info "Installing systemd service..."

  cat <<'EOF' | if [ "$(id -u)" -eq 0 ]; then tee "$service_file"; else sudo tee "$service_file"; fi > /dev/null
[Unit]
Description=Ollama AI Runtime
Documentation=https://github.com/tuhin-su/ollama-master
After=network-online.target

[Service]
ExecStart=/usr/local/bin/ollama serve
Restart=always
RestartSec=3
Environment="HOME=/root"
Environment="PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

[Install]
WantedBy=multi-user.target
EOF

  if [ "$(id -u)" -eq 0 ]; then
    systemctl daemon-reload
    systemctl enable ollama 2>/dev/null || true
    info "Service installed. Start with: systemctl start ollama"
  else
    sudo systemctl daemon-reload
    sudo systemctl enable ollama 2>/dev/null || true
    info "Service installed. Start with: sudo systemctl start ollama"
  fi
}

# ─────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────
main() {
  printf '\n'
  printf '  ██████╗ ██╗     ██╗      █████╗ ███╗   ███╗ █████╗ \n'
  printf ' ██╔═══██╗██║     ██║     ██╔══██╗████╗ ████║██╔══██╗\n'
  printf ' ██║   ██║██║     ██║     ███████║██╔████╔██║███████║\n'
  printf ' ██║   ██║██║     ██║     ██╔══██║██║╚██╔╝██║██╔══██║\n'
  printf ' ╚██████╔╝███████╗███████╗██║  ██║██║ ╚═╝ ██║██║  ██║\n'
  printf '  ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝╚═╝  ╚═╝\n'
  printf '\n'
  printf '  github.com/tuhin-su/ollama-master\n\n'

  local os arch version asset_name asset_url checksum_url bin_path

  os="$(detect_os)"
  arch="$(detect_arch)"
  version="${VERSION:-$(latest_version)}"
  version="${version#v}"   # strip leading 'v' for display; add back for URL

  info "OS:           $os"
  info "Architecture: $arch"
  info "Version:      v${version}"

  # Asset naming convention: ollama-<os>-<arch>[.tar.gz]
  # e.g. ollama-linux-amd64.tar.gz, ollama-darwin-arm64
  local base_url="https://github.com/${REPO}/releases/download/v${version}"

  case "$os" in
    linux)
      asset_name="ollama-linux-${arch}.tar.gz"
      asset_url="${base_url}/${asset_name}"
      checksum_url="${base_url}/${asset_name}.sha256"
      ;;
    darwin)
      asset_name="ollama-darwin-${arch}.tar.gz"
      asset_url="${base_url}/${asset_name}"
      checksum_url="${base_url}/${asset_name}.sha256"
      ;;
  esac

  local archive_path="${TMP_DIR}/${asset_name}"
  download "$asset_url" "$archive_path"
  verify_checksum "$archive_path" "$checksum_url"

  info "Extracting archive..."
  tar -xzf "$archive_path" -C "$TMP_DIR"

  # Find the binary — it may be in a subdirectory
  bin_path="$(find "$TMP_DIR" -type f -name "$BINARY" | head -1)"
  [ -n "$bin_path" ] || error "Binary '$BINARY' not found in archive."

  install_binary "$bin_path"

  if [ "$os" = "linux" ]; then
    install_systemd_service
  fi

  printf '\n'
  success "Ollama v${version} installed successfully!"
  printf '\n'
  printf '  Quick start:\n'
  printf '    ollama serve          # start the server\n'
  printf '    ollama run gemma4     # run a model\n'
  printf '\n'
  printf '  Docs: https://github.com/tuhin-su/ollama-master#readme\n\n'
}

main "$@"

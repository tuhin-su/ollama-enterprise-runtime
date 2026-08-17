# Installation

Install Loom from the [tuhin-su/loom-master](https://github.com/tuhin-su/loom-master)
release page. The installer auto-detects your OS and CPU architecture.

---

## Linux & macOS — One Command

```bash
curl -fsSL https://raw.githubusercontent.com/tuhin-su/loom-master/main/install.sh | sh
```

The script:
1. Detects your OS (`linux` / `darwin`) and architecture (`amd64`, `arm64`, `arm`)
2. Fetches the latest release from GitHub
3. Verifies the SHA256 checksum
4. Installs `loom` to `/usr/local/bin`
5. Optionally registers a **systemd service** on Linux (auto-start on boot)

### Options

```bash
# Pin a specific version
VERSION=v1.2.3 curl -fsSL .../install.sh | sh

# Custom install directory
INSTALL_DIR=$HOME/.local/bin curl -fsSL .../install.sh | sh
```

### Supported platforms

| OS | Architecture | Notes |
|----|-------------|-------|
| Linux | amd64 (x86\_64) | Most desktops and servers |
| Linux | arm64 (aarch64) | Raspberry Pi 4/5, ARM servers |
| Linux | arm (armv7) | Older Raspberry Pi, embedded |
| macOS | amd64 | Intel Mac |
| macOS | arm64 | Apple Silicon (M1/M2/M3/M4) |

---

## Windows — PowerShell

Run in **PowerShell** (Administrator recommended for service registration):

```powershell
irm https://raw.githubusercontent.com/tuhin-su/loom-master/main/install.ps1 | iex
```

The script:
1. Detects architecture (`amd64` or `arm64`)
2. Downloads and verifies the `.zip` release
3. Installs `loom.exe` to `%LOCALAPPDATA%\Programs\Loom`
4. Adds the install directory to your `PATH`
5. Registers and starts **LoomService** (Windows service, requires admin)

### Options

```powershell
# Pin a specific version
$env:VERSION = "v1.2.3"
irm .../install.ps1 | iex

# Custom install directory
irm .../install.ps1 | iex -Args "-InstallDir C:\Tools\Loom"
```

### Supported platforms

| OS | Architecture |
|----|-------------|
| Windows 10/11 | amd64 (x86\_64) |
| Windows 11 | arm64 |

---

## Manual Installation

Download the binary directly from the
[Releases page](https://github.com/tuhin-su/loom-master/releases).

| Platform | File |
|----------|------|
| Linux amd64 | `loom-linux-amd64.tar.gz` |
| Linux arm64 | `loom-linux-arm64.tar.gz` |
| Linux arm | `loom-linux-armv7.tar.gz` |
| Windows amd64 | `loom-windows-amd64.zip` |
| Windows arm64 | `loom-windows-arm64.zip` |

Each archive includes a `.sha256` checksum file. Verify before installing:

```bash
# Linux / macOS
sha256sum -c loom-linux-amd64.tar.gz.sha256

# Windows (PowerShell)
(Get-FileHash loom-windows-amd64.zip -Algorithm SHA256).Hash
```

---

## Update

Re-run the installer to update to the latest version — it will overwrite the
existing binary in-place.

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/tuhin-su/loom-master/main/install.sh | sh

# Windows
irm https://raw.githubusercontent.com/tuhin-su/loom-master/main/install.ps1 | iex
```

---

## Uninstall

### Linux / macOS

```bash
sudo rm /usr/local/bin/loom

# Remove systemd service (Linux only)
sudo systemctl stop loom
sudo systemctl disable loom
sudo rm /etc/systemd/system/loom.service
sudo systemctl daemon-reload
```

### Windows

```powershell
# Stop and remove service (if installed)
sc.exe stop LoomService
sc.exe delete LoomService

# Remove binary
Remove-Item "$env:LOCALAPPDATA\Programs\Loom\loom.exe"
```

### Remove memory database

```bash
rm ~/.loom/memory.db
```

---

## Building from Source

See [development.md](development.md) for full instructions.

```bash
git clone https://github.com/tuhin-su/loom-master.git
cd loom-master
go build .
./loom serve
```

#!/usr/bin/env bash
set -euo pipefail

REPO="${MISSIONBASE_CLI_REPO:-Quantum-Fire-Labs/missionbase-cli}"
INSTALL_DIR="${MISSIONBASE_INSTALL_DIR:-$HOME/.local/bin}"
BIN="$INSTALL_DIR/missionbase"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="missionbase-${os}-${arch}"
if [[ "$os" == "mingw"* || "$os" == "msys"* || "$os" == "cygwin"* ]]; then
  asset="missionbase-windows-${arch}.exe"
fi

api="https://api.github.com/repos/${REPO}/releases/latest"
url="$(curl -fsSL "$api" | grep -oE '"browser_download_url": "[^"]+' | cut -d'"' -f4 | grep "/${asset}$" | head -n1)"
if [[ -z "$url" ]]; then
  echo "Could not find release asset: $asset" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
tmp="$(mktemp)"
curl -fL "$url" -o "$tmp"
chmod +x "$tmp"
mv "$tmp" "$BIN"

echo "Installed missionbase to $BIN"
"$BIN" version

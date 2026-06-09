#!/usr/bin/env bash
set -euo pipefail

REPO="${MISSIONBASE_CLI_REPO:-Quantum-Fire-Labs/missionbase-cli}"
INSTALL_DIR="${MISSIONBASE_INSTALL_DIR:-$HOME/.local/bin}"
REQUESTED_BIN="${1:-all}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

case "$REQUESTED_BIN" in
  all) bins=(missionbase missionbase-agent) ;;
  missionbase|missionbase-agent) bins=("$REQUESTED_BIN") ;;
  *) echo "Usage: install.sh [all|missionbase|missionbase-agent]" >&2; exit 1 ;;
esac

api="https://api.github.com/repos/${REPO}/releases/latest"
release_json="$(curl -fsSL "$api")"
mkdir -p "$INSTALL_DIR"

for bin in "${bins[@]}"; do
  asset="${bin}-${os}-${arch}"
  if [[ "$os" == "mingw"* || "$os" == "msys"* || "$os" == "cygwin"* ]]; then
    asset="${bin}-windows-${arch}.exe"
  fi

  url="$(printf '%s' "$release_json" | grep -oE '"browser_download_url": "[^"]+' | cut -d'"' -f4 | grep "/${asset}$" | head -n1)"
  if [[ -z "$url" ]]; then
    echo "Could not find release asset: $asset" >&2
    exit 1
  fi

  tmp="$(mktemp)"
  curl -fL "$url" -o "$tmp"
  chmod +x "$tmp"
  mv "$tmp" "$INSTALL_DIR/$bin"
  echo "Installed $bin to $INSTALL_DIR/$bin"
  "$INSTALL_DIR/$bin" version
  echo
 done

#!/usr/bin/env bash
set -euo pipefail

REPO="melodyogonna/solai"
BIN_DIR="${HOME}/.local/bin"
BIN_NAME="solai"

# Resolve architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *)
    echo "error: unsupported architecture: $ARCH (supported: x86_64, aarch64)" >&2
    exit 1
    ;;
esac

ASSET="${BIN_NAME}-linux-${ARCH}"

# Resolve download URL — latest release
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Downloading ${ASSET} from ${REPO}..."
mkdir -p "$BIN_DIR"
curl -fsSL "$URL" -o "${BIN_DIR}/${BIN_NAME}"
chmod +x "${BIN_DIR}/${BIN_NAME}"

echo "Installed to ${BIN_DIR}/${BIN_NAME}"

# Advise on PATH if needed
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$BIN_DIR"; then
  echo ""
  echo "Add solai to your PATH by running:"
  echo ""
  echo "    echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
  echo ""
  echo "Or for zsh:"
  echo ""
  echo "    echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
fi

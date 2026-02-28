#!/usr/bin/env sh
# Angee installer — installs the angee CLI to /usr/local/bin
# Usage: curl https://angee.ai/install.sh | sh
set -e

ANGEE_VERSION=latest
REPO="fyltr/angee"
INSTALL_DIR="${ANGEE_INSTALL_DIR:-/usr/local/bin}"

# Detect OS and arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  *)
    echo "Unsupported OS: $OS"
    echo "On Windows, download from: https://github.com/${REPO}/releases"
    exit 1
    ;;
esac

# Resolve latest version if needed
if [ "$ANGEE_VERSION" = "latest" ]; then
  ANGEE_VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')"
fi

FILENAME="angee-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${ANGEE_VERSION}/${FILENAME}"

echo "Installing angee v${ANGEE_VERSION} (${OS}/${ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "${TMP}/${FILENAME}"
tar -xzf "${TMP}/${FILENAME}" -C "$TMP"

# Install
if [ -w "$INSTALL_DIR" ]; then
  cp "${TMP}/angee-${OS}-${ARCH}" "${INSTALL_DIR}/angee"
  chmod +x "${INSTALL_DIR}/angee"
else
  sudo cp "${TMP}/angee-${OS}-${ARCH}" "${INSTALL_DIR}/angee"
  sudo chmod +x "${INSTALL_DIR}/angee"
fi

echo ""
echo "  ✔ angee v${ANGEE_VERSION} installed to ${INSTALL_DIR}/angee"
echo ""
echo "  Get started:"
echo "    angee init"
echo "    angee up"
echo ""

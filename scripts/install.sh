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
  ANGEE_VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
    | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')" || true
fi

if [ -z "$ANGEE_VERSION" ]; then
  echo ""
  echo "  No releases found on GitHub. Building from source instead."
  echo ""

  # Check prerequisites
  if ! command -v go >/dev/null 2>&1; then
    echo "  ✗ Go is required to build from source."
    echo "    Install Go: https://go.dev/dl/"
    echo "    Or set ANGEE_VERSION to a specific release tag."
    exit 1
  fi

  echo "  Building angee CLI..."
  SRCDIR="$(mktemp -d)"
  trap 'rm -rf "$SRCDIR"' EXIT

  git clone --depth 1 "https://github.com/${REPO}.git" "$SRCDIR" 2>/dev/null || {
    echo "  ✗ Failed to clone repository. Install from local checkout instead:"
    echo "    make build-cli && sudo cp dist/angee /usr/local/bin/"
    exit 1
  }

  (cd "$SRCDIR" && go build -o angee ./cmd/angee/) || {
    echo "  ✗ Build failed."
    exit 1
  }

  if [ -w "$INSTALL_DIR" ]; then
    cp "${SRCDIR}/angee" "${INSTALL_DIR}/angee"
    chmod +x "${INSTALL_DIR}/angee"
  else
    sudo cp "${SRCDIR}/angee" "${INSTALL_DIR}/angee"
    sudo chmod +x "${INSTALL_DIR}/angee"
  fi

  echo ""
  echo "  ✔ angee (built from source) installed to ${INSTALL_DIR}/angee"
  echo ""
  echo "  Get started:"
  echo "    angee init"
  echo "    angee up"
  echo ""
  exit 0
fi

FILENAME="angee-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${ANGEE_VERSION}/${FILENAME}"

echo "Installing angee v${ANGEE_VERSION} (${OS}/${ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "${TMP}/${FILENAME}" || {
  echo ""
  echo "  ✗ Download failed: ${URL}"
  echo "    The release may not have a binary for ${OS}/${ARCH}."
  echo "    Build from source: git clone https://github.com/${REPO} && cd angee && make build-cli"
  exit 1
}
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

#!/usr/bin/env sh
# Installs portctl from a GitHub release. Usage:
#   curl -fsSL https://raw.githubusercontent.com/vikas0686/portctl/main/install.sh | sh
set -eu

REPO="vikas0686/portctl"
INSTALL_DIR="${PORTCTL_INSTALL_DIR:-/usr/local/bin}"
VERSION="${PORTCTL_VERSION:-latest}"

os=$(uname -s)
arch=$(uname -m)

case "$os" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *)
    echo "portctl: unsupported OS: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *)
    echo "portctl: unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

if [ "$VERSION" = "latest" ]; then
  url="https://github.com/${REPO}/releases/latest/download/portctl_${os}_${arch}.tar.gz"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/portctl_${os}_${arch}.tar.gz"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "portctl: downloading ${url}"
curl -fsSL "$url" -o "$tmp/portctl.tar.gz"
tar -xzf "$tmp/portctl.tar.gz" -C "$tmp"

if [ -w "$INSTALL_DIR" ]; then
  mv "$tmp/portctl" "$INSTALL_DIR/portctl"
else
  echo "portctl: ${INSTALL_DIR} needs sudo to write to"
  sudo mv "$tmp/portctl" "$INSTALL_DIR/portctl"
fi
chmod +x "$INSTALL_DIR/portctl"

echo "portctl: installed to ${INSTALL_DIR}/portctl"
echo "run: portctl ls"

#!/usr/bin/env sh
# Install the prebuilt acorn binary into ~/.local/bin (Linux/mac) or %USERPROFILE%\.acorn (Windows).
# Pipe into sh after publishing release binaries:  curl -sSL https://example/install.sh | sh
set -eu

REPO="yumlevi/acorn-cli"
VERSION="${ACORN_VERSION:-latest}"
BIN="acorn"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "unsupported arch: $arch"; exit 1 ;;
esac

url_base="https://github.com/$REPO/releases/$VERSION/download"
if [ "$VERSION" = "latest" ]; then
  url_base="https://github.com/$REPO/releases/latest/download"
fi
ext=""
[ "$os" = "windows" ] && ext=".exe"

dest_dir="$HOME/.local/bin"
mkdir -p "$dest_dir"
dest="$dest_dir/$BIN$ext"

url="$url_base/$BIN-$os-$arch$ext"
echo "→ fetching $url"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$url" -o "$dest"
elif command -v wget >/dev/null 2>&1; then
  wget -q "$url" -O "$dest"
else
  echo "need curl or wget"; exit 1
fi
chmod +x "$dest"
echo "✓ installed $dest"

case ":$PATH:" in
  *":$dest_dir:"*) ;;
  *) echo "warning: $dest_dir is not in \$PATH — add it to your shell rc" ;;
esac

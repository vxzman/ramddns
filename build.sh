#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-dev}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "")
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
# Normalize arch: x86_64 -> amd64, aarch64 -> arm64
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
esac

echo "Building aiolos ${VERSION} ..."
echo "Platform: ${OS}/${ARCH}"

LDFLAGS="-X aiolos/cmd/aiolos.version=${VERSION} -X aiolos/cmd/aiolos.commit=${COMMIT} -X aiolos/cmd/aiolos.buildDate=${BUILD_DATE}"

go build -ldflags "$LDFLAGS" -o build/aiolos ./cmd/aiolos

echo "Build successful: $(pwd)/build/aiolos"

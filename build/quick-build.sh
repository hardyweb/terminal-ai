#!/bin/bash

# Terminal AI CLI - Quick Build Script
# Builds for the current platform only

set -e

VERSION="1.0.0"
BUILD_DIR="build"
BINARY_NAME="terminal-ai"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "╔════════════════════════════════════════════════════╗"
echo "║     Terminal AI CLI - Quick Build                    ║"
echo "╚════════════════════════════════════════════════════╝"
echo ""

# Get version info
if git rev-parse HEAD &> /dev/null; then
    GIT_COMMIT=$(git rev-parse --short HEAD)
    GIT_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
    BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    VERSION="${GIT_TAG:-$VERSION}"
else
    GIT_COMMIT="unknown"
    BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
fi

# Detect current platform
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Linux*)     OS=linux;;
    Darwin*)    OS=darwin;;
    MINGW*)     OS=windows;;
    *)          OS=unknown;;
esac

case "$ARCH" in
    x86_64)     ARCH=amd64;;
    aarch64)    ARCH=arm64;;
    armv7l)     ARCH=arm;;
    i386)       ARCH=386;;
    *)          ARCH=unknown;;
esac

PLATFORM="${OS}-${ARCH}"
OUTPUT_NAME="${BINARY_NAME}"

if [ "${OS}" = "windows" ]; then
    OUTPUT_NAME="${BINARY_NAME}.exe"
fi

OUTPUT_DIR="${BUILD_DIR}/${PLATFORM}"
OUTPUT_PATH="${OUTPUT_DIR}/${OUTPUT_NAME}"

echo -e "${BLUE}Build Info:${NC}"
echo "  Version: ${VERSION}"
echo "  Platform: ${PLATFORM}"
echo "  Commit: ${GIT_COMMIT}"
echo "  Date: ${BUILD_DATE}"
echo ""

# Create build directory
mkdir -p "${OUTPUT_DIR}"

# Build
echo -e "${YELLOW}Building ${PLATFORM}...${NC}"
go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
    -trimpath \
    -o "${OUTPUT_PATH}" \
    .

if [ $? -eq 0 ]; then
    FILE_SIZE=$(du -h "${OUTPUT_PATH}" | cut -f1)
    echo -e "${GREEN}✓ Build successful: ${OUTPUT_PATH} (${FILE_SIZE})${NC}"
else
    echo -e "${RED}✗ Build failed${NC}"
    exit 1
fi

# Copy assets
if [ -f "ui.html" ]; then
    cp ui.html "${OUTPUT_DIR}/"
    echo -e "${GREEN}✓ Copied ui.html${NC}"
fi

if [ -f ".env.example" ]; then
    cp .env.example "${OUTPUT_DIR}/"
    echo -e "${GREEN}✓ Copied .env.example${NC}"
fi

# Create archive
echo ""
echo -e "${YELLOW}Creating archive...${NC}"
cd "${OUTPUT_DIR}"

if [ "${OS}" = "windows" ]; then
    ARCHIVE_NAME="${BINARY_NAME}-${VERSION}-${PLATFORM}.zip"
    zip -r "../${ARCHIVE_NAME}" * > /dev/null
else
    ARCHIVE_NAME="${BINARY_NAME}-${VERSION}-${PLATFORM}.tar.gz"
    tar -czf "../${ARCHIVE_NAME}" *
fi

cd ..
echo -e "${GREEN}✓ Created: ${BUILD_DIR}/${ARCHIVE_NAME}${NC}"

# Generate checksum
echo -e "${YELLOW}Generating checksum...${NC}"
sha256sum "${ARCHIVE_NAME}" > "${ARCHIVE_NAME}.sha256"
cd ..
echo -e "${GREEN}✓ Checksum: ${BUILD_DIR}/${ARCHIVE_NAME}.sha256${NC}"

echo ""
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}Quick build complete!${NC}"
echo ""
echo "Binary: ${OUTPUT_PATH}"
echo "Archive: ${BUILD_DIR}/${ARCHIVE_NAME}"
echo ""
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"

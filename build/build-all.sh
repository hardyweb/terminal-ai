#!/bin/bash

# Terminal AI CLI - Multi-Platform Build Script
# Builds binaries for Windows, Linux, macOS, Alpine, and other platforms

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

# Platform configurations
declare -A PLATFORMS=(
    ["linux-amd64"]="linux amd64"
    ["linux-arm64"]="linux arm64"
    ["linux-arm"]="linux arm"
    ["linux-386"]="linux 386"
    ["windows-amd64"]="windows amd64"
    ["windows-arm64"]="windows arm64"
    ["windows-386"]="windows 386"
    ["darwin-amd64"]="darwin amd64"
    ["darwin-arm64"]="darwin arm64"
    ["freebsd-amd64"]="freebsd amd64"
    ["openbsd-amd64"]="openbsd amd64"
)

echo "╔════════════════════════════════════════════════════╗"
echo "║   Terminal AI CLI - Multi-Platform Build Script      ║"
echo "╚════════════════════════════════════════════════════╝"
echo ""

# Clean build directory
echo -e "${YELLOW}Cleaning build directory...${NC}"
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"
echo -e "${GREEN}✓ Build directory cleaned${NC}"
echo ""

# Get version from git or use default
if git rev-parse HEAD &> /dev/null; then
    GIT_COMMIT=$(git rev-parse --short HEAD)
    GIT_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
    BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    VERSION="${GIT_TAG:-$VERSION}"
else
    GIT_COMMIT="unknown"
    GIT_TAG=""
    BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
fi

echo -e "${BLUE}Build Info:${NC}"
echo "  Version: ${VERSION}"
echo "  Commit: ${GIT_COMMIT}"
echo "  Date: ${BUILD_DATE}"
echo ""

# Build function
build_platform() {
    local platform=$1
    local goos=$2
    local goarch=$3
    local output_name="${BINARY_NAME}"
    
    if [ "${goos}" = "windows" ]; then
        output_name="${BINARY_NAME}.exe"
    fi
    
    local output_dir="${BUILD_DIR}/${platform}"
    local output_path="${output_dir}/${output_name}"
    
    mkdir -p "${output_dir}"
    
    echo -e "${YELLOW}Building ${platform}...${NC}"
    
    CGO_ENABLED=0 GOOS=${goos} GOARCH=${goarch} \
        go build \
        -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
        -trimpath \
        -o "${output_path}" \
        .
    
    if [ $? -eq 0 ]; then
        local file_size=$(du -h "${output_path}" | cut -f1)
        echo -e "${GREEN}✓ ${platform}: ${output_path} (${file_size})${NC}"
        return 0
    else
        echo -e "${RED}✗ ${platform} failed${NC}"
        return 1
    fi
}

# Copy required files
copy_assets() {
    local target_dir=$1
    echo -e "${YELLOW}Copying assets to ${target_dir}...${NC}"
    
    # Copy UI file
    if [ -f "ui.html" ]; then
        cp ui.html "${target_dir}/"
        echo -e "${GREEN}  ✓ ui.html${NC}"
    fi
    
    # Copy env example
    if [ -f ".env.example" ]; then
        cp .env.example "${target_dir}/"
        echo -e "${GREEN}  ✓ .env.example${NC}"
    fi
    
    # Copy README
    if [ -f "README.md" ]; then
        cp README.md "${target_dir}/"
        echo -e "${GREEN}  ✓ README.md${NC}"
    fi
}

# Create archive
create_archive() {
    local platform=$1
    local binary_name="${BINARY_NAME}"
    local target_dir="${BUILD_DIR}/${platform}"
    
    if [ "${platform}" == *"windows"* ]; then
        binary_name="${BINARY_NAME}.exe"
    fi
    
    # Copy assets
    copy_assets "${target_dir}"
    
    # Create archive
    if [ "${platform}" == *"windows"* ]; then
        local archive_name="${BUILD_DIR}/${BINARY_NAME}-${VERSION}-${platform}.zip"
        echo -e "${YELLOW}Creating ZIP archive...${NC}"
        cd "${target_dir}"
        zip -r "../$(basename ${archive_name})" * > /dev/null
        cd ..
        echo -e "${GREEN}✓ ${archive_name}${NC}"
    else
        local archive_name="${BUILD_DIR}/${BINARY_NAME}-${VERSION}-${platform}.tar.gz"
        echo -e "${YELLOW}Creating tar.gz archive...${NC}"
        cd "${target_dir}"
        tar -czf "../$(basename ${archive_name})" *
        cd ..
        echo -e "${GREEN}✓ ${archive_name}${NC}"
    fi
}

# Generate checksums
generate_checksums() {
    echo -e "${YELLOW}Generating checksums...${NC}"
    cd "${BUILD_DIR}"
    sha256sum *.tar.gz *.zip 2>/dev/null > SHA256SUMS.txt || true
    md5sum *.tar.gz *.zip 2>/dev/null > MD5SUMS.txt || true
    cd ..
    echo -e "${GREEN}✓ Checksums generated${NC}"
}

# Build all platforms
echo -e "${BLUE}Building all platforms...${NC}"
echo ""

FAILED_BUILDS=()
SUCCESS_BUILDS=()

for platform in "${!PLATFORMS[@]}"; do
    IFS=' ' read -r goos goarch <<< "${PLATFORMS[$platform]}"
    
    if build_platform "${platform}" "${goos}" "${goarch}"; then
        SUCCESS_BUILDS+=("${platform}")
    else
        FAILED_BUILDS+=("${platform}")
    fi
done

echo ""

# Create archives
echo -e "${BLUE}Creating release archives...${NC}"
echo ""

for platform in "${SUCCESS_BUILDS[@]}"; do
    create_archive "${platform}"
done

# Generate checksums
generate_checksums

# Summary
echo ""
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}Build Summary${NC}"
echo ""
echo -e "${GREEN}Successful builds (${#SUCCESS_BUILDS[@]}):${NC}"
for platform in "${SUCCESS_BUILDS[@]}"; do
    echo "  ✓ ${platform}"
done

if [ ${#FAILED_BUILDS[@]} -gt 0 ]; then
    echo ""
    echo -e "${RED}Failed builds (${#FAILED_BUILDS[@]}):${NC}"
    for platform in "${FAILED_BUILDS[@]}"; do
        echo "  ✗ ${platform}"
    done
fi

echo ""
echo -e "${BLUE}Output directory: ${BUILD_DIR}/${NC}"
echo ""

# List all artifacts
echo -e "${BLUE}Generated artifacts:${NC}"
ls -lh "${BUILD_DIR}/" | grep -E '\.(tar\.gz|zip|txt)$' | awk '{print "  " $9 " (" $5 ")"}'

echo ""
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"

# Exit with error if any build failed
if [ ${#FAILED_BUILDS[@]} -gt 0 ]; then
    exit 1
fi

echo -e "${GREEN}All builds completed successfully! 🚀${NC}"

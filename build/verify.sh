#!/bin/bash

# Terminal AI CLI - Build Verification Script
# Verifies build integrity by testing checksums

set -e

BUILD_DIR="build"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "╔════════════════════════════════════════════════════╗"
echo "║   Terminal AI CLI - Build Verification              ║"
echo "╚════════════════════════════════════════════════════╝"
echo ""

if [ ! -d "${BUILD_DIR}" ]; then
    echo -e "${RED}✗ Build directory not found: ${BUILD_DIR}${NC}"
    echo "Run ./build/build-all.sh or ./build/quick-build.sh first"
    exit 1
fi

cd "${BUILD_DIR}"

# Count files
TOTAL_FILES=$(ls -1 *.tar.gz *.zip 2>/dev/null | wc -l)
CHECKSUM_FILES=$(ls -1 *.sha256 2>/dev/null | wc -l)

echo -e "${BLUE}Found ${TOTAL_FILES} build artifacts${NC}"
echo -e "${BLUE}Found ${CHECKSUM_FILES} checksum files${NC}"
echo ""

# Verify SHA256
if [ -f "SHA256SUMS.txt" ]; then
    echo -e "${YELLOW}Verifying SHA256SUMS.txt...${NC}"
    if sha256sum -c SHA256SUMS.txt; then
        echo -e "${GREEN}✓ SHA256 verification passed${NC}"
    else
        echo -e "${RED}✗ SHA256 verification failed${NC}"
        exit 1
    fi
    echo ""
fi

# Verify individual checksums
if [ ${CHECKSUM_FILES} -gt 0 ]; then
    echo -e "${YELLOW}Verifying individual checksums...${NC}"
    FAILED=0
    
    for checksum_file in *.sha256; do
        echo -n "  Checking $(basename ${checksum_file} .sha256)... "
        if sha256sum -c "${checksum_file}" > /dev/null 2>&1; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
            FAILED=$((FAILED + 1))
        fi
    done
    
    if [ $FAILED -eq 0 ]; then
        echo -e "${GREEN}✓ All checksums verified${NC}"
    else
        echo -e "${RED}✗ ${FAILED} checksum(s) failed${NC}"
        exit 1
    fi
    echo ""
fi

# Display file info
echo -e "${BLUE}Build Artifacts:${NC}"
echo ""

printf "%-40s %10s %10s\n" "File" "Size" "Type"
printf "%-40s %10s %10s\n" "----" "----" "----"

for file in $(ls -1 *.tar.gz *.zip 2>/dev/null | sort); do
    size=$(ls -lh "$file" | awk '{print $5}')
    type=$(echo "$file" | grep -oE '\.(tar\.gz|zip)$')
    printf "%-40s %10s %10s\n" "$file" "$size" "$type"
done

echo ""
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}All builds verified successfully!${NC}"
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"

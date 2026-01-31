#!/bin/bash

# Terminal AI CLI Installation Script
# Version: 1.0
# Author: Hardy & Jafar

set -e

echo "╔════════════════════════════════════════════════════╗"
echo "║     Terminal AI CLI - Installation Script              ║"
echo "╚════════════════════════════════════════════════════╝"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running as root
if [ "$EUID" -eq 0 ]; then 
    echo -e "${RED}Don't run as root!${NC}"
    exit 1
fi

# Detect OS
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Linux*)     OS=linux;;
    Darwin*)    OS=macos;;
    MINGW*)     OS=windows;;
    *)          OS=unknown;;
esac

echo -e "${GREEN}Detected OS: ${OS} (${ARCH})${NC}"
echo ""

# Check Go installation
check_go() {
    if command -v go &> /dev/null; then
        GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
        echo -e "${GREEN}✓ Go found: ${GO_VERSION}${NC}"
        return 0
    else
        echo -e "${YELLOW}⚠ Go not found${NC}"
        return 1
    fi
}

# Install Go if not found
install_go() {
    echo -e "${YELLOW}Installing Go...${NC}"
    
    case "$OS" in
        linux)
            if [ "$ARCH" = "x86_64" ]; then
                GO_ARCH="amd64"
            elif [ "$ARCH" = "aarch64" ]; then
                GO_ARCH="arm64"
            elif [ "$ARCH" = "armv7l" ]; then
                GO_ARCH="armv7l"
            else
                echo -e "${RED}Unsupported architecture: ${ARCH}${NC}"
                exit 1
            fi
            
            GO_VERSION="1.21.6"
            wget "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
            sudo rm -rf /usr/local/go
            sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
            rm "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
            
            # Add to PATH
            if ! grep -q '/usr/local/go/bin' ~/.bashrc 2>/dev/null; then
                echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
            fi
            if ! grep -q '/usr/local/go/bin' ~/.zshrc 2>/dev/null; then
                echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.zshrc
            fi
            export PATH=$PATH:/usr/local/go/bin
            ;;
            
        macos)
            if [ "$ARCH" = "arm64" ]; then
                GO_ARCH="arm64"
            else
                GO_ARCH="amd64"
            fi
            
            if command -v brew &> /dev/null; then
                brew install go
            else
                echo -e "${RED}Homebrew not found. Please install Go manually.${NC}"
                exit 1
            fi
            ;;
            
        windows)
            echo -e "${RED}Please install Go manually from https://go.dev/dl/${NC}"
            exit 1
            ;;
    esac
    
    echo -e "${GREEN}✓ Go installed${NC}"
}

# Check and install Go
if ! check_go; then
    read -p "Install Go now? (y/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        install_go
    else
        echo -e "${RED}Go is required. Exiting.${NC}"
        exit 1
    fi
fi

# Clone or update repository
REPO_DIR="${HOME}/terminal-ai"
if [ -d "$REPO_DIR" ]; then
    echo -e "${YELLOW}Updating existing installation...${NC}"
    cd "$REPO_DIR"
    git pull
else
    echo -e "${YELLOW}Cloning repository...${NC}"
    git clone https://github.com/your-repo/terminal-ai.git "$REPO_DIR"
    cd "$REPO_DIR"
fi

echo ""

# Install dependencies
echo -e "${YELLOW}Installing dependencies...${NC}"
go mod tidy
echo -e "${GREEN}✓ Dependencies installed${NC}"

# Build
echo -e "${YELLOW}Building...${NC}"
go build -o terminal-ai main.go
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Build successful${NC}"
else
    echo -e "${RED}✗ Build failed${NC}"
    exit 1
fi

# Create config directory
CONFIG_DIR="${HOME}/.config/terminal-ai"
echo -e "${YELLOW}Creating config directory...${NC}"
mkdir -p "${CONFIG_DIR}/skills"
mkdir -p "${CONFIG_DIR}/users"
echo -e "${GREEN}✓ Config directory created: ${CONFIG_DIR}${NC}"

# Setup .env file
if [ ! -f "${CONFIG_DIR}/.env" ]; then
    echo -e "${YELLOW}Creating .env file...${NC}"
    cp .env.example "${CONFIG_DIR}/.env"
    echo -e "${GREEN}✓ .env created: ${CONFIG_DIR}/.env${NC}"
    echo -e "${YELLOW}⚠ Please edit ${CONFIG_DIR}/.env and add your API keys${NC}"
else
    echo -e "${GREEN}✓ .env already exists${NC}"
fi

# Install binary
INSTALL_DIR="/usr/local/bin"
if [ -w "$INSTALL_DIR" ]; then
    echo -e "${YELLOW}Installing to ${INSTALL_DIR}...${NC}"
    cp terminal-ai "$INSTALL_DIR/terminal-ai"
    chmod +x "$INSTALL_DIR/terminal-ai"
    echo -e "${GREEN}✓ Installed to ${INSTALL_DIR}/terminal-ai${NC}"
else
    echo -e "${YELLOW}⚠ No write permission to ${INSTALL_DIR}${NC}"
    echo -e "${YELLOW}Install with: sudo cp terminal-ai /usr/local/bin/${NC}"
fi

# Install web server binary
go build -o terminal-ai-web web.go 2>/dev/null || true

# Create default users
echo -e "${YELLOW}Setting up default users...${NC}"
echo -e "${YELLOW}⚠ Run 'terminal-ai user create' to add users${NC}"

# Verify installation
echo ""
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}Installation Complete!${NC}"
echo ""
echo "Binary: $(which terminal-ai || echo 'Not in PATH')"
echo "Config: ${CONFIG_DIR}"
echo ""
echo "Next steps:"
echo "  1. Edit ${CONFIG_DIR}/.env and add API keys"
echo "  2. Test: terminal-ai 'hello'"
echo "  3. Run web server: terminal-ai web-server"
echo ""
echo "Documentation: cd ${REPO_DIR} && ls docs/"
echo ""
echo -e "${GREEN}══════════════════════════════════════════════════════${NC}"

# Test installation
echo -e "${YELLOW}Testing installation...${NC}"
if command -v terminal-ai &> /dev/null; then
    terminal-ai --version 2>/dev/null || terminal-ai "hello" << EOF

EOF
    echo -e "${GREEN}✓ Installation verified${NC}"
else
    echo -e "${YELLOW}⚠ Binary not in PATH. Relogin or run:${NC}"
    echo "  export PATH=$PATH:/usr/local/bin"
fi

echo ""
echo -e "${GREEN}Happy AI Chatting! 🤖${NC}"

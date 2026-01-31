# Terminal AI CLI - Build Documentation

## Build Scripts

This project includes comprehensive build scripts for cross-platform compilation.

### Quick Start

```bash
# Quick build for current platform
./build/quick-build.sh

# Or using Make
make quick

# Build all platforms
./build/build-all.sh

# Or using Make
make all
```

### Build Scripts Location

- `build/build-all.sh` - Multi-platform build script
- `build/quick-build.sh` - Quick build for current platform
- `build/verify.sh` - Verify build checksums
- `Makefile` - Make targets for convenient building

### Supported Platforms

| Platform | Architecture | Binary Name |
|----------|-------------|-------------|
| Linux | amd64, arm64, arm, 386 | `terminal-ai` |
| Windows | amd64, arm64, 386 | `terminal-ai.exe` |
| macOS | amd64, arm64 | `terminal-ai` |
| FreeBSD | amd64 | `terminal-ai` |
| OpenBSD | amd64 | `terminal-ai` |

### Build Output

After running a build, you'll find:

```
build/
├── linux-amd64/
│   ├── terminal-ai          # Binary
│   ├── ui.html               # Web UI
│   ├── .env.example          # Environment template
│   └── README.md             # Documentation
├── SHA256SUMS.txt           # All checksums
└── MD5SUMS.txt              # All checksums
```

### Make Targets

```bash
make quick          # Quick build for current platform
make all            # Build all platforms
make linux          # Build all Linux platforms
make linux-amd64    # Build Linux x86_64
make windows        # Build all Windows platforms
make macos          # Build all macOS platforms
make rpi            # Build for Raspberry Pi
make test           # Run tests
make clean          # Clean build directory
make install        # Install to /usr/local/bin
make release        # Create release artifacts
make help           # Show help
```

### Manual Build

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o terminal-ai .

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o terminal-ai.exe .

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o terminal-ai .

# Raspberry Pi (ARM)
GOOS=linux GOARCH=arm64 go build -o terminal-ai .
```

### Alpine Linux

For Alpine Linux, use the standard Linux binary:

```bash
GOOS=linux GOARCH=amd64 go build -o terminal-ai .
```

If you encounter musl libc issues, either:
1. Build on Alpine directly
2. Install gcompat: `apk add gcompat`

### Verification

Verify build integrity:

```bash
# Using verify script
./build/verify.sh

# Manual verification
sha256sum -c build/SHA256SUMS.txt

# Single file
sha256sum -c build/terminal-ai-1.0.0-linux-amd64.tar.gz.sha256
```

### Release Workflow

1. Tag the release:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

2. Build all platforms:
   ```bash
   ./build/build-all.sh
   ```

3. Verify:
   ```bash
   ./build/verify.sh
   ```

4. Upload artifacts from `build/` to release

### Build Options

The build scripts use optimized flags:
- `-ldflags="-s -w"` - Strip debug info, reduce size
- `-trimpath` - Remove filesystem paths
- `CGO_ENABLED=0` - Static binaries
- Version info embedded

### Troubleshooting

**Build fails:**
- Ensure Go 1.21+ is installed
- Check platform support

**Binary too large:**
- Already optimized with build flags
- Optional: `upx --best --lzma terminal-ai`

**Permission denied:**
- Linux/macOS: `chmod +x terminal-ai`
- Windows: Unblock in Properties

**Alpine execution error:**
- Alpine uses musl libc
- Install gcompat: `apk add gcompat`

### See Also

- `build/README.md` - Detailed build documentation
- `Makefile` - All available build targets
- `docs/QUICKSTART.md` - Quick start guide

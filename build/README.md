# Wails v3 Build System Guide

This document describes how to build and package LiteRSS on Windows and macOS using the Wails v3 build system with Task runner.
Dependencies and release details are in [Build Requirements](../docs/BUILD_REQUIREMENTS.md).

## Prerequisites

- **Go**: the version in `go.mod`
- **Node.js 24**: [https://nodejs.org/](https://nodejs.org/)
- **Wails CLI v3**: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26`（与 `go.mod` 钉定版本一致）
- **Task**: [https://taskfile.dev/installation/](https://taskfile.dev/installation/)
- **Windows**: `choco install nsis -y`（只在打安装包时需要；Windows 构建不需要 CGO/MinGW）
- **macOS**: `xcode-select --install`

## Quick Start

```bash
# Development mode with hot reload (dev server on port 5173, configurable via WAILS_VITE_PORT)
task dev

# Production build for the current platform (output: build/bin/)
task build
task windows:build  # on Windows
task darwin:build   # on macOS

# Installers
task package
task windows:package  # NSIS installer
task darwin:package   # DMG
```

### Architecture-Specific Builds

```bash
task windows:build ARCH=amd64
task windows:build ARCH=arm64
task darwin:build ARCH=universal  # Intel + Apple Silicon
```

Build each platform on that platform. `task setup:docker` builds an image that cross-compiles Windows binaries from another OS when CGO is on; Windows builds do not need CGO, so plain Go cross-compilation is enough.

## Task Commands Reference

- `task build` / `task package` / `task run` / `task dev`
- `task windows:build`, `task windows:package`, `task windows:sign` (requires certificate)
- `task darwin:build`, `task darwin:package`, `task darwin:sign`, `task darwin:notarize` (require Apple credentials)
- `task common:install:frontend:deps`, `task common:build:frontend`, `task common:dev:frontend`
- `task common:generate:icons` - Generate platform icons from appicon.png
- `task common:update:build-assets` - Update build assets from config

## Configuration

- `build/config.yml`: application name, version, company, bundle identifier and dev mode settings.
- `build/windows/Taskfile.yml` and `build/darwin/Taskfile.yml`: per-platform build settings.
- The product name, publisher and version also live in the installer files listed in [Build Requirements](../docs/BUILD_REQUIREMENTS.md#安装包与发布); change them together.

### Signing

Windows: set `SIGN_CERTIFICATE` and `TIMESTAMP_SERVER` in `build/windows/Taskfile.yml`, then run `wails3 setup signing`.
macOS: set `SIGN_IDENTITY` and `NOTARIZATION_PROFILE` in `build/darwin/Taskfile.yml`.

## GitHub Actions

- **Release** (`release.yml`): push a `v*` tag, or run it from Actions → Release with a version such as `v0.1.0`.
  Builds Windows (AMD64, ARM64) and macOS (Universal).
- **Test** (`test.yml`): on push/PR to main; backend tests and build check on Windows and macOS.

## Troubleshooting

### CGO is disabled (macOS)

```bash
export CGO_ENABLED=1
task build
```

### Task not found

```bash
brew install go-task      # macOS
choco install go-task     # Windows
```

## Resources

- [Wails v3 Documentation](https://v3.wails.io/)
- [Task Documentation](https://taskfile.dev/)
- [Build Configuration Reference](./config.yml)
- [GitHub Actions Workflows](../.github/workflows/)

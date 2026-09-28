# Build Requirements

This document describes the system-level dependencies required for building LiteRSS on Windows and macOS. Linux is not a supported platform (ADR 0018).

## Toolchain versions

Use the Go version in `go.mod` and the Node version in `.github/workflows/test.yml` (currently Node 24). The frontend lockfile defines dependency engine constraints; Node 18 is insufficient. The local check scripts also require Python 3.8+ and `goimports` on PATH. Override their Python executable with `PYTHON` when needed.

## Wails 版本

`go.mod` 钉住 `github.com/wailsapp/wails/v3 v3.0.0-beta.26`。只为具体修复或阻断问题升级，PR 写明原因；每次升级重跑 [ADR 0011](adr/0011-桌面壳继续用-Wails-v3-钉版.md) 列出的检查项（隔离实例能启动、单实例生效、`WebviewUserDataPath` 可用、CSP 头被采用、长轮询不被截断、托盘 API 可用）。

`wails3` CLI 与库同版本安装，否则 `build/` 下的 Taskfile 与生成命令可能对不上：

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
```

## 开发构建与正式构建

身份常量（名字、UniqueID、数据目录、desktopapi 端口、自启值名、更新仓库）集中在 `internal/identity`，由构建标签选择：

- 带 `-tags production`（`task build` / `task package` 的正式构建）：正式身份，数据在用户配置目录下的 `LiteRSS`，端口 1236。
- 不带该标签（`go build`、`go test`、`wails3 dev` / `task build DEV=true`）：开发身份，数据在 exe 旁的 `data\`，端口 1235，独立 UniqueID。

所以直接 `go build` 出来的二进制永远是开发实例，不会碰已安装版本；规则见 AGENTS.md「开发实例与取证」。

## CGO

- **Windows 不需要 CGO**：Wails v3 通过纯 Go 调用 WebView2，`CGO_ENABLED=0 go build .` 可以构建；`build/windows/Taskfile.yml` 默认也关闭 CGO，不需要 MinGW。
- **macOS 需要 CGO**（Wails 的 AppKit 绑定），由 Xcode Command Line Tools 提供编译器。

## Platform-Specific Requirements

### Windows

只需要 Go 与 Node；打安装包时再装 NSIS（`choco install nsis -y`，或从 [nsis.sourceforge.io](https://nsis.sourceforge.io/) 安装）。不需要 MinGW。

正式构建由 `build/windows/Taskfile.yml` 加 `-ldflags "-H windowsgui"`，启动时不开控制台窗口。Windows 二进制不依赖额外运行库（WebView2 由系统提供）。

### macOS

Install Xcode Command Line Tools (if not already installed):

```bash
xcode-select --install
```

Wails creates the `LiteRSS.app` bundle during build; `build/darwin/Info.plist` supplies its metadata. The bundle is self-contained.

## Building with Wails

```bash
# Development build with hot reload
wails3 dev

# Production build (recommended: use Task)
task build

# Platform-specific build with Task
task windows:build
task darwin:build
```

Wails v3 uses `build/config.yml` for build configuration and Taskfile for platform-specific builds; the frontend is built via `frontend/package.json` scripts. Build each platform on that platform; GitHub Actions uses native runners.

## 安装包与发布

- 安装包的产品名、发布者、版本与安装目录在 `build/config.yml`、`build/windows/installer.nsi`、`build/windows/info.json`、
  `build/windows/nsis/wails_tools.nsh`、`build/darwin/Info.plist` 与 `build/darwin/Taskfile.yml` 里各写一份，改名或改版本时一起改；
  版本号还在 `internal/version/version.go` 与 `frontend/package.json`（`python scripts/verify.py versions` 核对这两处）。
- 应用图标的源在 `frontend/public/assets`：`logo.svg` 是大图（网页图标，也是 128 以上的图标帧与 `build/appicon.png`），
  `logo-<N>.svg` 是逐尺寸对齐像素格的小图（`.ico` 的 16–64 帧，顶栏按缩放比例取 16/20/24/32）。小图不能由大图缩出：150% 缩放下托盘与顶栏是 24 像素，
  缩出来的线条落在半个像素上会发糊。改图标后跑 `python scripts/render_icons.py` 重新生成 `build/windows/icon.ico`、`build/appicon.png`
  与 `build/darwin/icons.icns` 并提交；构建只由 `appicon.png` 生成 `.icns`，不再生成 `.ico`。Windows 的托盘与开发构建的窗口图标嵌的是这个 `.ico`。
- 安装目录、数据目录、UniqueID、自启值名与端口都与旧版 MrRSS 不同，新旧版可以并行安装；`internal/identity` 的测试核对身份常量与安装包配置。
- Windows 安装包由 NSIS 从 `build/windows/installer.nsi` 生成，按用户安装（`RequestExecutionLevel user`，不弹 UAC），默认装到 `%LOCALAPPDATA%\Programs\LiteRSS`，
  目录页可改；安装目录与卸载项记在 `HKCU`，卸载不删用户数据（`%APPDATA%\LiteRSS`）。应用内更新以 `/S /D=<当前目录>` 静默运行它（ADR 0019）：
  安装前最多等 30 秒让旧进程退出、释放 exe，静默安装后与完成页的「启动」都经 `explorer.exe` 启动应用，不要改回管理员安装或直接 `Exec` 应用；
  便携版是 exe 旁放 `portable.txt` 的 zip，数据在 exe 旁的 `data\`。macOS 发布 DMG（`build/darwin/create-dmg.sh`）。
- 推送 `v*` 标签或手动触发 `.github/workflows/release.yml` 时，在 Windows（amd64、arm64）与 macOS（universal）的原生 runner 上构建，
  发布说明取自 `CHANGELOG.md` 同版本一节。全部平台构建完后 `checksums` 任务下载本次发布的全部 `LiteRSS-*` 资产，生成并上传 `SHA256SUMS`：
  应用内更新只安装其中列出且校验相符的安装包；某个平台构建失败时它不在清单里，那个平台的用户会被引到发布页。
  重新跑某个平台的构建后要重跑 `checksums`（`--clobber` 覆盖旧清单）。`.github/workflows/pre-release-check.yml` 手动触发，只构建不发布，末尾同样对产物算一遍校验和。

## GitHub Actions

- `test.yml`：后端测试与编译检查在 Windows 与 macOS 上各跑一遍；前端 lint、单测、构建与脚本自测与平台无关。
- `release.yml` 与 `pre-release-check.yml`：见上节。

## Troubleshooting

### macOS: "CGO is disabled" Error

**Solution**: Enable CGO before building (Windows builds do not need it):

```bash
export CGO_ENABLED=1
wails3 build
```

### macOS: Missing Xcode Command Line Tools

**Solution**: Install Xcode Command Line Tools:

```bash
xcode-select --install
```

## Related Documentation

- [Architecture Overview](ARCHITECTURE.md)
- [Code Patterns](CODE_PATTERNS.md)
- [Testing Guide](TESTING.md)

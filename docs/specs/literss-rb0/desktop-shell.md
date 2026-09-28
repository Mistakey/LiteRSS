# 桌面壳选型（mrrss-280.1）

> 调研节点 mrrss-280.1 的结论，供 mrrss-280「MrRSS 重做规格」引用。调研日期 2026-09-26。
> 只是调研与推荐；最终写进规格的取舍由 forge:spec 定。

## 结论

**留在 Wails v3，把版本钉到调研时最新的 beta（`v3.0.0-beta.26` 或更新），并把壳压成一层薄适配：**

1. Wails 只允许出现在 `main.go`（加一个可选的 `shell` 小包）里。前端对 `@wailsio/runtime` 的唯一依赖
   （`frontend/src/utils/clipboard.ts` 的 `Clipboard.SetText`）改用 `navigator.clipboard`，前端就变成
   纯 HTTP `/api/*` 客户端，任何浏览器都能跑。
2. 壳只做四件事：窗口、单实例、在系统浏览器打开链接、窗口位置记忆。托盘、关闭到托盘和开机自启按新动线
   **建议删除**（理由见「桌面集成核对」）。
3. 给 agent 取证单开一条浏览器通道：同一套 Go 后端在回环地址上同时提供前端静态文件和 `/api/*`，
   使用独立的数据目录。agent 用 Chrome 或 Playwright 驱动界面，不再依赖 Wails 窗口能否启动。
   这条通道可以用 Wails v3 自带的 `-tags server` 构建，也可以复用现有 `internal/desktopapi` 监听器再挂上
   前端文件。**它只是开发与验收用的构建，不是部署形态，不与 ADR 0005 冲突**；规格里要把这个区分写清楚。

Wails v2 作为后备：壳已经压薄，改用 v2 基本只需要重写 `main.go`。只有在 v3 真出现阻断性缺陷时才退回 v2。
Tauri 和「本地服务 + 系统浏览器」不推荐作为主壳。

## 关键依据

### 1. 发布成熟度

| 选项 | 现状（一手来源） |
| --- | --- |
| Wails v3 | 2026-08-02 发布 Beta。官方原话："The desktop API is stable and teams are already using v3 in production, but you should test thoroughly before deploying"（[v3 Beta 博客](https://github.com/wailsapp/wails/blob/master/docs/mpress/content/blog/wails-v3-beta.md)）。此后每晚自动发一个 beta：beta.8 发布于 2026-08-12，beta.26 发布于 2026-09-25（`gh api repos/wailsapp/wails/releases`）。beta.26 修了「WebView2 进程失败后留下空白窗口」（PR #6002），和本应用在 Windows 上的场景直接相关。GA 之前还有 38 个发布闸门，其中 27 个是阻断项，截至 2026-08-18 一个都没通过（[#5844](https://github.com/wailsapp/wails/issues/5844)）；RC 和 GA 没有给日期。 |
| Wails v2 | 仍是官方稳定版，"will continue to receive fixes"（同一篇博客）。最近两版是 v2.13.0（2026-07-06）和 v2.14.0（2026-08-10）。从 v3 退回 v2 要重新移植："migration is a real port rather than a version number change"。 |
| Tauri v2 | 稳定版 `tauri-v2.11.6`（2026-09-19）；v3 已进入 alpha（`tauri-v3.0.0-alpha.2`，2026-09-21）。 |

判断：v3 的风险在于版本号每天都在变，不在于 API 会大改。把版本钉住、只在需要某个修复时才升级，就能控制这个风险。
退回 v2 虽然换来「stable」标签，但要重移植一遍，还要放弃 v3 已经在用的多窗口和托盘等 API。换来的稳定性
对一个只有一个窗口、单人自用的阅读器来说收益有限。

### 2. agent 会话能否启动并取证（重新定性 Common Issues 7）

本次调研查到 Common Issues 7「启动后停在 `Starting Wails v3...` 就退出，没有窗口，也没有回环 API」
**很可能不是 Wails 或 agent 环境的问题，而是单实例机制**：

- `main.go` 先把日志写成 `Starting Wails v3...`，然后才调用 `application.New`。`SingleInstance` 的
  `UniqueID` 固定为 `com.mrrss.app`（`main.go:244`），而且 Windows 上始终启用。
- Wails v3 beta.8 的 `pkg/application/application.go:206-216` 里，如果已有实例在运行，就
  `notifyFirstInstance()` 后 `os.Exit(ExitCode)`，不会创建窗口，也走不到后面的 `desktopapi.Start`。
  这和记录下来的现象一致。
- 取证时（2026-09-26）用户的 MrRSS 正在同一会话里运行（`MrRSS.exe` PID 36492，SessionId 1）。agent
  进程同样在 SessionId 1、`UserInteractive=True`，并且能通过回环 API 访问它：
  `curl http://127.0.0.1:1234/api/version` → `{"version":"1.3.28"}`，200。
- 开发构建和安装版共用同一个 `UniqueID`，也共用同一个 `%APPDATA%` 数据目录（`fileutil.GetDataDir`）。
  所以 agent 启动开发构建时只有两种结果：用户开着 MrRSS，就静默退出，还会把用户的窗口唤到前台；用户没开，
  就直接打开用户的真实数据库。而且 `main.go` 以截断模式打开 `debug.log`，一启动就会清掉用户的日志。

为了不影响用户正在运行的实例、数据和日志，本次没有实际启动第二个实例来复现。这个判断的依据是代码路径和
进程现场，还不是复现结果。它指向的结论是：取证能力取决于**开发实例能否与用户实例隔离**（独立的 UniqueID
和数据目录），而不取决于换哪种壳。换成 Tauri 或 v2，只要单实例和数据目录不隔离，照样会遇到同一个问题。
再加上第 3 点的浏览器通道，agent 就能完整取证界面和 API；原生窗口的观感仍按 Epic 的 Rulings 由用户验收。

### 3. Go 后端复用程度

| 选项 | 复用 |
| --- | --- |
| Wails v3（现状） | 全部复用。Wails 只在 `main.go` 里 import；`handlers` 通过 `BrowserOpener` 接口与 Wails 解耦（`internal/handlers/core/handler.go:19-25`）；API 走 `/api/*` HTTP，经 `AssetOptions.Handler/Middleware` 挂进 WebView。 |
| Wails v2 | 后端全部复用，但 `main.go` 要重写：用 `wails.Run(options)` 加 v2 的 `runtime` 包。v2 有 `SingleInstanceLock`（[v2 options 文档](https://github.com/wailsapp/wails/blob/master/website/docs/reference/options.mdx)），托盘是 v3 才有的（`docs/mpress/content/features/menus/systray.md` 只在 v3 文档里）。 |
| Tauri v2 | Go 后端只能作为 sidecar 独立进程：要按目标三元组给二进制命名，在 `externalBin` 里声明，在 capability 里授予 shell `spawn` 权限（[Tauri sidecar](https://v2.tauri.app/develop/sidecar/)）。进程的生命周期、端口协商、崩溃重启和单实例都要自己写一遍 Rust 胶水。项目会多出第三种语言和一条工具链。 |
| 本地服务 + 浏览器/WebView2 | 后端全部复用，`desktopapi` 已经有回环监听和跨源防护。但窗口、单实例、生命周期（浏览器标签页关了之后服务何时退出）都不再受控。另外，任何本地网页都能访问 localhost，只靠现有的 Origin 拦截不够，还要加 token。 |

### 4. 按新动线核对桌面集成

动线：未读列表 → 扫中文标题 → 点开 → 去浏览器细读/剪藏 → 已读。FreshRSS 在 NAS 上常驻抓取，客户端
打开时同步即可。

| 集成 | 动线需要吗 | 建议 |
| --- | --- | --- |
| 在系统浏览器打开链接 | 需要（「去浏览器」这一步） | 保留。v3 `app.Browser.OpenURL`，已经接在 `BrowserOpener` 上。 |
| 单实例 | 需要（双击图标应唤起已开窗口，避免两个进程同时写同一个 SQLite） | 保留，但开发构建要使用不同的 UniqueID（见第 2 点）。 |
| 窗口位置/大小记忆 | 需要，但很轻 | 保留。现有实现在 `main.go` 里写 `window_*` 设置；可简化成退出时保存一次。 |
| 托盘 / 关闭到托盘 | 不需要：动线不依赖后台常驻，同步由打开窗口时触发 | 建议删除（包括托盘菜单、`close_to_tray`、macOS 双击关闭的特判）。关窗口就退出进程。 |
| 开机自启 | 不需要：用户要读的时候才打开 | 建议删除 `startup_on_boot` 和 `internal/utils/startup.go`。 |
| 自动更新检查 | 不属于壳能力 | 交给「打包与发布」未定项；最多保留「有新版本时提示并跳到发布页」（与 feature-inventory U10 的倾向一致）。 |
| 剪贴板 | 前端唯一的 Wails runtime 调用 | 改用 `navigator.clipboard`，前端从此不依赖壳。 |

删掉托盘以后，`main.go` 里大约一半的窗口状态和托盘代码（`setupSystemTray`、`WindowClosing` 钩子、
`lastWindowState` 的并发恢复）都会消失。壳薄到这个程度，将来 v3 → v2 或 v3 → GA 的迁移成本都只有一个文件。

### 5. Windows 打包体积与构建链

| 选项 | 构建链 | 体积 |
| --- | --- | --- |
| Wails v3 | Go + Node + `wails3`/Task + NSIS。**Windows 上不需要 CGO**：v3 把 go-webview2 并入 `v3/internal/webview2`，通过纯 Go 调用 WebView2（[wailsapp/go-webview2](https://github.com/wailsapp/go-webview2)、[v3 跨平台构建](https://v3.wails.io/guides/build/cross-platform/)）。`docs/BUILD_REQUIREMENTS.md` 写的「Wails v3 requires CGO」「Windows 需要 mingw」在 Windows 上已经过时，应该更正。 | 本地构建 `build/bin/MrRSS.exe` 31 MB；旧版 NSIS 安装包约 12 MB（`MrRSS-1.3.23-windows-amd64-installer.exe`）。WebView2 运行时 Win11 自带；仓库还带了 bootstrapper。 |
| Wails v2 | 同上，Windows 同样不需要 CGO。 | 与 v3 相当。 |
| Tauri v2 | 需要额外安装 MSVC「Desktop development with C++」和 Rust MSVC 工具链；打 MSI 还要启用 VBSCRIPT 功能（[Tauri prerequisites](https://v2.tauri.app/start/prerequisites/)）。Go sidecar 要按三元组命名。 | Rust 壳本身很小，但 Go sidecar（约 25–30 MB）仍然占大头，总体积不会比 Wails 小。 |
| 本地服务 + 浏览器 | 只要 Go + Node。 | 最小，但没有「应用」的形态（没有独立窗口身份，只能依赖 Edge `--app` 模式）。 |

## 被否决的方案

- **Tauri v2**：后端一行都没省，却多了 Rust 工具链、sidecar 进程管理和第三种语言。体积没有优势，取证问题也
  照样存在。
- **本地服务 + 系统浏览器/WebView2 窗口作为主壳**：服务生命周期、单实例和回环安全都要自己重做，还失去独立
  窗口。它唯一的优点是 agent 能用浏览器取证，而推荐方案里的浏览器通道已经拿到了这个优点。
- **现在就退回 Wails v2**：要重新移植，换来的「stable」对这个场景价值有限。只作为壳压薄之后的后备。

## 规格需要接住的事项

- 开发/agent 构建与用户实例隔离：独立的 `UniqueID`、独立的数据目录（可复用 `portable.txt` 机制或加环境
  变量）、日志不截断用户文件。建议作为同一张图里的独立 Issue，同时更正 AGENTS.md Common Issues 7 的表述。
- 浏览器取证通道的形态（Wails `-tags server`，还是 desktopapi 挂前端文件），以及它与 ADR 0005「只做桌面
  应用」的边界写法。
- `docs/BUILD_REQUIREMENTS.md` 中 Windows CGO/mingw 的过时说法。
- Wails 版本钉在哪个 beta、升级节奏（例如只在需要某个修复时才升级，并在 PR 里注明）。

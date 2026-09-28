# AI Agent Guidelines for LiteRSS

项目共享指令只写在这里；`CLAUDE.md` 导入本文件，只放适配器信息。Git 分支与提交约定来自用户的全局指令，这里不重复。

## 项目边界

- 正在重做为 LiteRSS：桌面上的 FreshRSS 未读阅读器，只服务一条动线——未读列表 → 扫中文标题 → 点开（中文读正文 / 英文看 AI 摘要）
  → 值得细读或剪藏的去浏览器 → 以上已读。
- FreshRSS 是订阅、分类与已读状态的权威；本地只推用户明确做过的意图，从不把别处的已读改回未读。不做本地订阅（ADR 0002）。
- 隐私：无统计，不加载外部字体与脚本；凭据加密存储、日志脱敏；SQL 参数化；不信任的 HTML 与路径按不可信输入处理。
- 不再跟随上游；需要某个上游修复时手工移植，不合并（ADR 0017）。
- 依赖与可执行命令以 `go.mod`、`frontend/package.json`、lockfile 与 Taskfile 为准，工具链细节以它们为准。

## 重做进行中

- Epic `literss-rb0`；目标形态以 [spec](docs/specs/literss-rb0/spec.md) 为准，同目录是各规划节点的一手依据。本仓库其余文档只描述已经落地的代码。
- 旧实现只通过标签 `legacy-final` 对照：`git show legacy-final:<路径>`，或 `git worktree add <仓库旁目录> legacy-final` 检出只读副本。
  主干不留旧代码目录；旧版已知 bug 与安全漏洞不热修。
- forge:finish 删除 spec 时一并删除本节。

## 开发实例与取证

- 已安装版本的单实例 ID（旧版 `com.mrrss.app`、新版 `io.github.mistakey.literss`）在 agent 会话里一律不得用于启动：用户实例在运行时，
  新进程只会通知它然后在 `Starting Wails v3...` 后退出；没在运行时，它会打开用户的真实数据。带 `-tags production` 的构建就是这种构建。
- agent 只启动开发构建：不带 `production` 标签的 `go build -o <scratch 目录>\LiteRSS.exe .`。它自带独立 UniqueID、exe 旁的 `data\`
  （数据、日志与 WebView2 用户数据）和 1235 端口（`internal/identity`），能在用户实例运行时正常启动；二进制放在 scratch 目录，不放进仓库。
  只连假 FreshRSS，不填真实账号，不打开「开机时启动」（它写的是用户真实的 `HKCU\...\Run`，值名 `LiteRSS-dev`）。结束时只 `Stop-Process` 自己启动的 PID。
- 探活：`curl --noproxy "*" http://127.0.0.1:1235/api/version`。本机设了 `HTTP_PROXY`，不加 `--noproxy` 回环请求也会走代理。
- UI 通过浏览器取证通道 `http://127.0.0.1:1235/` 驱动（入口只写 `127.0.0.1`：从 `localhost` 打开时页面自身的 CSS/JS 会被回环防护拒成 403），
  用 `chrome-headless-shell` 与 `scripts/browser-forensics.mjs`，步骤见 [Testing](docs/TESTING.md#浏览器取证)。
- 原生窗口观感、托盘、自启与真实账号行为由用户在自建构建上验收，自动化检查不能代替。

## 按任务阅读

适用的指令读一次，之后只读任务需要的部分；范围变化或证据与所读矛盾时再读。链接不是改动前的必读清单。

| 任务 | 参考 |
| --- | --- |
| 模块边界或跨模块行为 | [Architecture](docs/ARCHITECTURE.md)；相关 [ADR](docs/adr/)；未落地部分看 spec |
| Go / Vue 实现 | [Code Patterns](docs/CODE_PATTERNS.md) |
| 选择或运行检查 | [Testing](docs/TESTING.md#verification-by-task) |
| 工具链、构建或打包失败 | [Build Requirements](docs/BUILD_REQUIREMENTS.md) |
| 热更新开发 | `task dev` |
| 设置 schema、默认值或生成代码 | [Settings](docs/SETTINGS.md) |
| 标题翻译与语言检测 | pitfall 11、12 |
| 出站 HTTP、代理或全文抓取 | pitfall 8–10、21；相关 ADR |
| 格式化或行尾噪音 | pitfall 17 |
| 重新提出已否决的功能 | 相关 [ADR](docs/adr/) 或 [out-of-scope](docs/out-of-scope/) |

pitfall 的索引与正文在 [docs/pitfalls.md](docs/pitfalls.md)。

## 完成与授权

- 完成授权范围与 [Testing](docs/TESTING.md#verification-by-task) 中适用的检查；更新受影响的文档、fixture 与生成文件，不夹带无关清理。
- 明确的当前请求或先前授权覆盖其指定的动作与范围。只为缺失的决定、范围扩大或未获批准的破坏性后果提问；已回答的授权问题不重复问。
- 检查最终 diff，只移除本任务产生的无关改动，保留用户的编辑。只在相关改动、失败或证据未决时重跑已通过的检查。
- 报告结果、实际跑过的检查与剩余限制。需要人工验收时，先完成独立的工作，并给出具体产物与复现步骤。
  被阻塞的检查不能说成通过，也不因一项检查被阻塞就停下全部工作。
- 提交、合并与发布在属于授权任务时进行。评审或验证未提交的改动不要求干净的工作区。
- 检查命令见 [Testing](docs/TESTING.md#running-tests)，聚合入口见 [Scripts](scripts/README.md)。只格式化目标文件并保留行尾（pitfall 17）。
  `make clean` 是显式删除产物的工具，不是完成步骤。

## 相关文档

- [Architecture](docs/ARCHITECTURE.md)、[Code Patterns](docs/CODE_PATTERNS.md)、[Testing](docs/TESTING.md)、[Settings](docs/SETTINGS.md)、
  [Build Requirements](docs/BUILD_REQUIREMENTS.md)、[Pitfalls](docs/pitfalls.md)
- [Decision Records](docs/adr/)：为什么是现在这样；[Out of Scope](docs/out-of-scope/)：被否决的想法与理由
- 工作图（Issue、Epic、人工门）：`forge tracker`，不是文件；`forge tracker human` 列出等你处理的事项

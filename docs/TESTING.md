# Testing Guide

This document covers testing strategies and patterns for LiteRSS.

## Backend Testing (Go)

测试用表驱动：`tests := []struct{...}` 加 `t.Run(tt.name, ...)`，每个用例写明输入与期望，失败信息带实际值。

### 数据库与迁移

- 库测试用 `t.TempDir()` 下的真实文件，不用 `:memory:`：连接池里每个内存连接是各自独立的空库。
- 新增迁移步骤（已追加进 `migrations`）时：用 `openRaw` 打开临时库并 `migrate(ctx, db, migrations[:len(migrations)-1])` 建出上一版本，
  写入数据，再 `migrate(ctx, db, migrations)`，断言版本号前进、旧数据保留、新结构可用；对同一文件重开再调用一次，断言不重跑。
  不要用 `Open` 建「上一版本」：它已经迁到最新。框架本身的 1→2 样例在 `internal/database/migrate_test.go`，那里的 v2 是测试里的假步骤，
  追加到 `migrations` 的副本上（`append(migrations[:len(migrations):len(migrations)], step)`），不改包级切片。
- 失败的步骤要连同版本号一起回滚，`TestMigrateFailureLeavesVersionUnchanged` 守住这一点。

### 假 FreshRSS

同步相关的测试只对假服务跑，不连真实 FreshRSS。`internal/freshrss/freshrsstest.Server` 是一个 `http.Handler`，
复现 spec D6 列出的服务端行为（两种 ID 格式、`n` 无上限、续页丢首条、`it`/`xt`、`edit-tag` 恒回 `OK`、`mark-all-as-read` 按 `ts` 截断、
会话过期回 401；见 pitfall 28–30），以及不要会话的图标缓存：订阅的 `iconUrl` 一律指向 `f.php?h=<源 ID>`，
`Feed.Icon` 为空时 `f.php` 回占位图 `freshrsstest.Placeholder`（也在 `PlaceholderPath`）；其余接口一律 404。

- 测试里：`fake := freshrsstest.New("user", "secret")`，`AddFeeds` / `AddItems` 播种（条目 `ID` 就是抓取时间的微秒数），
  `srv := httptest.NewServer(fake)`，客户端用 `freshrss.NewClient(srv.URL, "user", "secret")`。
- 模拟别处发生的事：`SetRead`（别的设备读掉）、`RemoveFeed`（取消订阅，条目随之删除）、`ExpireSessions`（令牌过期）。
- 断言服务端收到了什么：`Item`、`Items`、`EditTags`、`MarkAlls`、`Logins`。
- `RejectItem` 是故障注入，不是 FreshRSS 的行为：点名该条目的 `edit-tag` 回 400，供推送器的二分定位测试用。

开发实例要连的假服务用 `tools/fake-freshrss`，它用 `freshrsstest.Generate` 生成订阅与约 100 天内的文章（中英文标题、部分已读、同 URL 重发、缺发布时间；奇数号源有图标，偶数号源只有占位图）：

```bash
go run ./tools/fake-freshrss            # 127.0.0.1:1240，账号 dev / dev；-feeds -items -seed -addr -user -pass 可调
curl --noproxy "*" -d "Email=dev&Passwd=dev" http://127.0.0.1:1240/api/greader.php/accounts/ClientLogin
```

开发实例里的 FreshRSS 地址填 `http://127.0.0.1:1240`。只监听回环地址，`-addr` 给非回环 IP 时拒绝启动。
`POST /_fake/add?feed=1&n=3`（抓到新条目）、`/_fake/read?i=<id>`（别处读掉，`&read=0` 为改回未读）、`/_fake/unsubscribe?feed=1`、
`/_fake/expire` 在两次同步之间扮演别的设备与服务端。数据只在内存里，重启即重新生成。

### API 路由

- `internal/routes/routes_test.go` 的 `TestRouteTable` 遍历 `routes.Table`：方法只能是 GET / POST / PUT / DELETE、标了 `Mutates` 的路由恰好是非 GET 的那些、
  路径在 `/api/` 下、不重复；每个 handler 包一层计数探针后，对每个 `Mutates` 路由（路径参数填 `1`）发 GET，断言回 405 且没有任何 handler 运行。
- handler 测试用 `newTestAPI`：临时目录里的真实库加 `routes.Handler`，用 SQL 直接播种文章，经 `httptest` 发请求。
  已读动作走真实的 `syncer.Service`（远端恒失败，推送只会退避），结果从卡片的 `read` 读回，所以断言的是用户看到的显示状态。
  查询本身的边界（完整快照与 `newest`、显示状态、同 URL 折叠、摘录）在 `internal/library` 的测试里覆盖。
- 内容动作的 handler 测试主要用空配置（没有百度与模型）走通响应形状与 404/400（全文翻译另接一个假模型，见下）；行为在 `internal/enrich` 的测试里对 `httptest` 替身覆盖：
  假百度按行查表回译文并记录每次的 `q`，假模型按 OpenAI 格式回固定 Markdown 并记录用户提示词，假网页 `/ok` 回可提取的文章、其余路径回 403。
  断言看存进库的值（译文、「已判定中文」、摘要）、送给百度与模型的内容，以及失败时的中文文案。
  全文翻译（`enrich/translate_test.go`）的假模型把用户提示词当 JSON 块数组读，逐块回「译:」+ 原文（套一层代码围栏），`answer` 钩子可以改某次回应（少回一块、回 500），`batches` 记下每批送了什么；`content_test.go` 另有一个接了假模型的 `TestTranslateArticle` 走通成功、未配置与失败三种回应。

### 假发布服务

应用内更新（spec D20）的测试只对假发布服务跑。`internal/update/updatetest.Server` 是一个 `http.Handler`，在同一个基址上回应
`/repos/<repo>/releases/latest` 与 `/<repo>/releases/download/<tag>/<name>`：`Publish(updatetest.Release{Tag, Files})` 发布一个版本，
`SHA256SUMS` 按 `Files` 自动生成；`NoSums` 去掉清单，`Corrupt` 让某个资产的内容与清单不符，`Rate` 限速（字节 / 秒）以便看到下载进度。
测试里把 `Checker.API` 与 `Checker.Downloads` 都设成 `httptest` 的地址，`Updater.Run`、`Quit`、`Locate` 换成记录调用的替身。
macOS 的替换脚本在有 `sh` 的机器上真跑（`TestMacSwapScript`，Windows 上用 Git 的 `sh`），系统命令换成 `echo`。

开发实例要走一遍更新时用 `tools/fake-release`：

```bash
go run ./tools/fake-release             # 127.0.0.1:1241，发布 v99.0.0，三个平台各一个 8 MB 的随机「安装包」，1 MB/s；-version -size -rate -corrupt -no-sums 可调
curl --noproxy "*" -X POST "http://127.0.0.1:1241/_fake/release?corrupt=1&rate=0"   # 不重启切换：校验不符 / 缺清单 / 速度
```

启动开发实例前设 `LITERSS_UPDATE_API=http://127.0.0.1:1241`（只对开发构建生效），并用 `POST /api/settings/update` 把 `proxy_mode` 设为 `direct`
（本机的系统代理会接走回环请求）。设置面板「检查更新」→「更新到 99.0.0」会依次显示下载进度与「开发构建不安装」，`-corrupt` 时显示校验不符与「去发布页」；
开发构建从不启动安装包，本会执行的命令写在 `data\logs\debug.log`。不要在取证页里点「去发布页」：它会在系统浏览器里打开真实 GitHub。
`installer.nsi` 可以用本机 NSIS 在 scratch 目录编译检查（`makensis`），但不要运行生成的安装包：它会写当前用户的开始菜单、桌面快捷方式与 `HKCU` 卸载项。

## 前端单测

- vitest + jsdom，`@vue/test-utils` 挂组件。后端用 `src/test/fakeBackend.ts` 的 `FakeBackend`：按 ARCHITECTURE「API 路由」的形状在内存里回应前端用到的路由，
  `install()` 换掉 `fetch`；`add` 播种文章，`tree` 设订阅树，`syncStates` 排好长轮询依次返回的状态（排空后挂起），`callsTo(method, path)` 断言前端发了什么。
  它只模拟前端看得见的规则（快照顺序、显示状态、批次与撤销令牌、译文），不模拟同步；等待异步链用 `settle()`，计时相关的用 `vi.useFakeTimers()`。
- store 测试（`src/stores/*.test.ts`）守住：快照在视图内稳定（点开与批量已读只变灰，同步后不替换）、新条目只进横幅且点击才载入、
  范围与 `ts` 的取法、撤销调用后端令牌与 410 的处理、标题译文只请求一次；`App.test.ts` 对整页核对侧栏树与计数（侧栏没有底栏）、列表行形态、右键菜单、横幅与经顶栏应用菜单打开设置。
  `TitleBar.test.ts` 核对窗口按钮、拖动与双击（应用菜单按钮不算拖动区），以及应用菜单的两项、同步状态文案与同步中「立即同步」置灰。
- `FakeBackend` 的 `content` 播种 RSS 正文，`fullTexts`、`summaries` 按 ID 排好抓全文与摘要的回应（缺省分别是 `no_link` 与「还没有配置大模型」；也可以放 Promise 以观察进行中的状态），`translator` 按收到的块回全文翻译（可以返回 Promise 以观察「翻译中…」；缺省为「还没有配置大模型」）。
  `stores/detail.test.ts` 守住打开不抓全文、缓存全文直接显示、按钮抓取成功替换与失败保留正文、摘要随全文重做、太短时不能摘要、换文章丢弃旧响应，以及换文章或翻译途中抓到全文时丢弃慢到的译文；`ArticleDetail.test.ts` 核对抓全文按钮与提示、浮动条、链接外开、图片查看器，以及「翻译」的三次切换、中文文章没有按钮、翻译中与失败、抓到全文后回到原文。块的提取与对照插入在 `utils/bilingual.test.ts`（代码块、表格、公式不翻，译文按纯文本写入）。
  浮动条只有图标，测试按按钮的类名（`summary`、`translate`、`bionic`、`fetch`、`ext`、`read`）找按钮，断言 `aria-label`、`aria-pressed`、禁用与图标（`iconOf` 按第一个图形的属性对照 `icons.ts`）。
  Bionic Reading 的开关、正文重建与只加粗一次也在 `ArticleDetail.test.ts`：正文含代码时要等动态载入的增强完成才加粗，用 `vi.waitFor` 等、再多等一会儿确认没有第二次加粗；
  词长查表（与 text-vide fixation 1 档一致）、词的切分与跳过的元素在 `utils/bionic.test.ts`，读写开关与保存失败在 `stores/prefs.test.ts`。
- `FakeBackend` 的 `settings` 是已存设置（凭据按明文放，GET 时回空串并列进 `saved_secrets`，清单外的键与空串凭据写入回 400，清除走 `secrets/clear`），`tests` 排好两个测试连接的回应（缺省成功），`update` 是检查更新的回应，`updateSteps` 排好应用内更新依次报告的进度（`start` 回第一项，之后每次 `status` 前进一项并停在最后一项）。
  `SettingsModal.test.ts` 守住导航顺序与一次只显示一组、切换分组不丢草稿 / 测试结果 / 检查更新结果、保存一次提交所有分组的改动并在导航上标出改过与不合法的分组（同步间隔不合法时跳回 FreshRSS 组）、保存只提交清单内改过的键（已存凭据不回写）、密钥的掩码 / 修改 / 取消 / 清除与撤销 / 未保存各状态、测试连接的传参与成功 / 失败显示、拒收时不关闭，以及「更新到 X」只点一次、进度轮询、失败原因与发布页、打开时显示已有的更新进度。
- 清洗器载荷矩阵在 `utils/sanitize.test.ts`：html-sanitize.md 第 1–4 节的载荷都不能留下事件属性、脚本类 URL 或嵌入元素，指向自身源与回环的地址被删，
  `mailto:`、`data-sanitized-class`、外部图片与 MathML 保留。改清洗规则时先加载荷，再确认去掉对应规则时测试会失败。

## Verification by task

Select checks by the affected behavior. These are the ordinary development defaults; explicit task requirements still apply. Expand for shared interfaces, persistence, concurrency or build integration. A documentation-only edit does not require compiling the app.

| Change | Completion evidence |
| --- | --- |
| Documentation or agent instructions | Check links, commands and agreement with configuration; no product tests unless an executable contract changed |
| Development scripts or check entry points | Exercise success/failure paths and command ordering, parse platform wrappers, check source files remain unchanged; test on the platforms the script supports when available and report any unavailable platform |
| Local Go behavior | Relevant package tests with timeout, relevant vet/build checks; add regression coverage for changed behavior |
| Shared Go APIs, persistence, synchronization or broad backend changes | Backend build, vet and full internal tests; migrations cover pre-upgrade data and repeat initialization, concurrency changes cover the affected synchronization contract |
| Frontend behavior | Relevant unit tests and frontend build; run the full unit suite for shared components/stores or broad changes |
| Settings schema | Run the schema generator; verify both frontend and backend |
| Desktop startup, embedded assets, build configuration or a requested desktop artifact | `wails3 build`; inspect the resulting artifact. Build frontend before Go when using direct commands. `go test ./internal/identity` checks the installer and bundle metadata under `build/` against the production identity and version |
| Release preparation | Full source checks plus dependency/version validation via `make release-check`; target-platform packaging and human desktop acceptance remain separate |

Tests should check behavior and failure cases, not mirror implementation. Do not create tests for simple prose or formatting edits. Repeat passing checks only after relevant edits, failures or unresolved evidence.

If a check is blocked, finish independent work and report the exact command and limitation. Agents gather API and UI evidence only from an isolated development build (AGENTS.md「开发实例与取证」); native-window look, tray, autostart and real-account behavior are accepted by the user on the built app, and automated checks do not stand in for that. Do not require a clean worktree or a commit to validate a patch. Actual publishing requires the intended committed revision, not an arbitrary checkpoint commit to silence a check.

## Running Tests

Commands below run from the repository root unless noted. For an automated run use one-shot commands; `npm test` is interactive watch mode.

```bash
# Full source validation: read-only lint/format, unit tests, frontend then Go build
python scripts/verify.py check
# Release validation: above once, plus non-writing module diff, audit and versions
python scripts/verify.py release

# Backend: full suite or narrow package selection
go test -timeout=5m ./internal/...
go test -timeout=5m ./internal/fulltext -run TestSpecificFunction
go test -tags production ./internal/identity   # production identity selected by the release build tag
go vet ./...
go build ./...   # requires frontend/dist; build frontend first if absent or changed

# In frontend/
npm run test:unit
npm run test:unit -- path/to/example.test.ts
npm run build
npm run lint:check

# Read-only Go formatting, ignoring CRLF/LF differences (from repo root)
python scripts/verify.py format internal/path/to/changed.go
# No file arguments checks all tracked Go files
python scripts/verify.py format

# Verification-tool regression tests
python -m unittest discover -s scripts -p "test_verify.py"
```

`npm run lint` and `npm run format` are explicit rewriting utilities. Prefer targeted `npx eslint <files> --fix` / `npx prettier <files> --write` for intended files. Go formatters emit LF: preserve each file's original ending when applying output. Keep pre-existing user edits and inspect the final content diff; do not restore files solely because they appear in `git status`.

`npm run test:unit` also runs `frontend/devProxy.test.js`, the Vite dev-proxy `Origin` rewrite (pitfall 24).

For backend coverage when it helps investigate a gap:

```bash
go test -timeout=5m -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out -o coverage.html
```

Frontend coverage is not currently a configured package script; add a matching coverage provider only when coverage tooling is part of the task. See [Scripts](../scripts/README.md) for wrappers and prerequisites.

## 浏览器取证

界面与 CSP 的取证都在隔离的开发实例上做（AGENTS.md「开发实例与取证」），用 `scripts/browser-forensics.mjs`（Node 22+）经 Chrome DevTools 协议读取：
每个请求的状态与 CSP 头、控制台输出、页面源以外的资源，可选截图；加 `--csp-probe` 时注入内联脚本、内联样式与 `data:` 的 `<object>`，三者都应被拒。

1. `cd frontend && npm run build`，再在仓库根 `go build -o <scratch>\LiteRSS.exe .`（不带 `production` 标签）。
2. 启动实例；要检查 WebView2 窗口本身时先设 `LITERSS_WEBVIEW2_DEBUG_PORT=9333`（只对开发构建生效）。PowerShell：
   `$env:LITERSS_WEBVIEW2_DEBUG_PORT="9333"; $p = Start-Process <scratch>\LiteRSS.exe -WorkingDirectory <scratch> -PassThru`，
   然后 `Remove-Item Env:LITERSS_WEBVIEW2_DEBUG_PORT`。
3. 浏览器通道用 Playwright 的 `chrome-headless-shell.exe`（`%LOCALAPPDATA%\ms-playwright\chromium_headless_shell-*\chrome-headless-shell-win64\`），
   不用 `chrome.exe` 或 `msedge.exe`：本机 AdGuard 会改写它们的回环流量（pitfall 25）。
   `chrome-headless-shell.exe --remote-debugging-port=9336 --user-data-dir=<scratch>\shell-profile --window-size=1400,900 about:blank`
4. 取证：
   - 浏览器通道：`node scripts/browser-forensics.mjs http://127.0.0.1:9336 http://127.0.0.1:1235/ <scratch>\channel.png --csp-probe`
   - WebView2 窗口：`node scripts/browser-forensics.mjs http://127.0.0.1:9333 http://wails.localhost/ <scratch>\webview2.png --csp-probe`
   通过的样子：每个请求 200 且带 spec D16 的 CSP；加载期控制台为空；`external` 为空；探针三项都报违规。截图用 Read 查看，对照 spec D15。
   需要数据时先起 `tools/fake-freshrss`（见「假 FreshRSS」），再用 `POST /api/settings/update` 写入 `freshrss_server_url=http://127.0.0.1:1240`、`dev` / `dev`，
   写入后自动同步一轮。点击、右键等交互用 CDP 的 `Input.dispatchMouseEvent` 在同一个页签里驱动后再 `Page.captureScreenshot`；取证脚本本身只做加载与截图。
   `tools/fake-freshrss` 生成的条目链接都是 `*.example.com`，抓全文只会得到连不上。要取证详情的抓取成功 / 失败时，在仓库里临时写一个 main 包（取证后删掉）：
   用 `freshrsstest.New` + `AddFeeds`/`AddItems` 放几条短正文的条目，链接指向同一程序在另一个回环端口上起的页面（一页正常文章、其余回 403），
   并把开发实例的 `proxy_mode` 设为 `direct`。正文图片用 `data:image/svg+xml` 生成：清洗器会删掉指向回环主机的图片。
5. 回环防护可直接用 curl 验证：`curl --noproxy "*" -X POST -H "Origin: http://localhost:1235" http://127.0.0.1:1235/api/version` 应为 403，
   不带 `Origin` 或带 `http://127.0.0.1:1235` 时过防护。
6. 结束时只 `Stop-Process` 自己启动的实例与 headless shell 的 PID。

需要热更新时 `cd frontend && npm run dev`，从 `http://127.0.0.1:5173/` 打开，`/api` 代理到 1235 的开发实例；取证默认仍用上面「构建后由开发实例托管」的方式。

## Continue Reading

- [Architecture Overview](ARCHITECTURE.md)
- [Code Patterns](CODE_PATTERNS.md)

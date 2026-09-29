# 架构

> 重做进行中：本文只描述已经落地的代码。目标形态以 [spec](specs/literss-rb0/spec.md) 为准；
> 旧实现用 `legacy-final` 标签对照（见 AGENTS.md「重做进行中」）。

## 现状

旧的同步、数据库访问、HTTP handler 与路由、AI profile、AI 翻译、内容缓存和整个前端已经删除。
主干此时是一个能编译的壳加一组留用模块，界面有侧栏、列表、详情与设置面板；业务 API 目前有同步状态、立即同步、列表读路径（快照、卡片、正文、未读计数、订阅树）、已读动作（单篇、批量、全部已读、撤销）、内容动作（抓全文、标题翻译、摘要）与杂项（设置读写、测试连接、在浏览器打开、检查更新与应用内更新），新模块按 spec D1 的顺序逐块长回来。

- 后端：Go + Wails v3，`main.go` 开一个窗口、单实例、托管 `frontend/dist`，打开本地库、启动同步调度，并在回环地址上启动 desktopapi。
- 前端：Vue 3 + TypeScript + Vite + Vitest + Pinia，三栏与设置面板都已接上 API（见下「前端」）。
- 通信：前端只通过相对路径 `/api/...` 访问后端，不依赖 Wails 绑定，所以同一份构建能在 Wails 窗口和浏览器取证通道里运行。

## 壳（`main.go`、`shell.go`、`internal/shell`）

- 身份：名字、单实例 UniqueID、数据目录、desktopapi 端口、开机自启值名、更新检查仓库与能否启动更新安装包（`InstallsUpdates`）都取自 `internal/identity.Current()`。
  带 `-tags production` 的正式构建用正式身份（`io.github.mistakey.literss`、用户配置目录下的 `LiteRSS`、端口 1236）；
  其他构建（`go build`、`go test`、`wails3 dev`）整体换成开发身份（独立 UniqueID、exe 旁的 `data\`、端口 1235），
  所以任何未加标签的二进制都碰不到已安装的实例（ADR 0012）。正式端口避开旧版 MrRSS 的 1234。
- 数据目录：开发身份总是 exe 旁的 `data\`；正式身份在 exe 旁有 `portable.txt` 时也用它。WebView2 用户数据固定放在
  数据目录下的 `webview2\`，不落到默认的 `%APPDATA%\<exe 文件名>`。
- 日志写入数据目录下的 `logs\debug.log`；启动时把上一次的日志移到 `debug.log.1`，只保留一份。
- 本地库是数据目录下的 `literss.db`，打不开时退出。同步服务的 FreshRSS 账号在每个周期开始时从设置读取，三项有一项为空时周期以「FreshRSS 未配置」失败；
  同一配置共用一个 `freshrss.Clients` 客户端。退出时先停调度并等在跑的周期结束，再 `syncer.Service.Close`（唤醒长轮询），最后关 desktopapi 与库（pitfall 31）。
- `internal/webui.Site` 把 `/api/` 前缀交给 API mux（`routes.Handler`，挂 `routes.Table` 的全部路由），其余交给 `frontend/dist` 的静态 handler；
  Wails 资源通道经 `webui.WailsMiddleware` 挂它，`/wails` 前缀留给 Wails 运行时。
- CSP：静态 handler（`webui.Static`）是唯一下发 `Content-Security-Policy` 响应头的地方，两条通道因此总是同一份策略（spec D16）；
  WebView2 采用这个响应头（pitfall 23）。API 响应不带 CSP。
- 无边框窗口（spec D4）：Windows 主窗口 `Frameless`，保留系统阴影与贴靠；macOS 用 `MacTitleBarHidden`，红绿灯压在前端顶栏左上。
  前端顶栏兼作标题栏：拖动与 Windows 的边缘缩放由前端直接给 WebView2 / WKWebView 宿主发 `wails:drag`、`wails:resize:<边>`，
  Wails 窗口据此进入系统的移动 / 缩放循环；最小化、最大化（还原）、关闭经 `/api/window/*` 调 `desktopShell` 的同名方法。
  关闭走 `WebviewWindow.Close`，触发与点 × 相同的 `WindowClosing` 钩子。Windows 11 悬停最大化按钮弹出的贴靠布局没有接（要换 WebView2 合成宿主）。
- 单实例：第二个实例启动时把已有窗口调到前台后退出（托盘里藏着的也显示出来，最大化的保持最大化）。
- 托盘（`shell.go`，菜单文案固定中文）：左键显示窗口，右键菜单「显示窗口 / 立即同步 / 退出」。托盘总在，与 `close_to_tray` 无关；
  「立即同步」与从睡眠恢复（`events.Common.SystemDidWake`）调 `Scheduler.Trigger`，窗口获得焦点调 `Scheduler.TriggerOnFocus`。
- 关闭到托盘：关窗口时每次读 `close_to_tray`，开着就隐藏窗口，否则与托盘「退出」一样退出整个应用（macOS 最后一个窗口关掉后进程不会自己退出）。
- 窗口位置（`internal/shell/window.go`）：关窗口、藏到托盘与托盘「退出」时各存一次 `window_*`（内部键，经 `settings.Store`）。
  最小化时什么都不存（位置是占位坐标，pitfall 33）；最大化时只存 `window_maximized`，保留上次的正常位置与大小；小于 400×300 的不存。
  启动时位置是占位坐标、从没存过（0,0）或尺寸过小就居中，最大化的以最大化打开。不检查保存的位置是否还在某块屏幕上。
- 开机自启（`internal/shell/autostart*.go`）：Windows 写 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 下以 `AutostartValueName` 为名的值
  （带引号的 exe 路径），macOS 写 `~/Library/LaunchAgents/<值名>.plist`。启动时按 `startup_on_boot` 对齐一次（开着就改写成当前 exe 路径）；
  设置面板改这个键时经 `settings.Panel.Autostart` 先落到系统，系统拒绝则设置不变、API 回 500，写库失败则把系统改回去。开发身份的值名是 `LiteRSS-dev`，旧版的 `MrRSS` 不碰。
- 自动检查更新（`update.Watch`）：启动 1 分钟后检查，之后每 24 小时一次，每次先读 `update_check_enabled`；有新版本时弹一个问答对话框
  （Windows 上是系统的「是 / 否」）。能在应用内更新时问「现在更新吗」，选「是」开始应用内更新（见下「应用内更新」），失败时再弹对话框给原因并问是否去发布页；
  不能时（便携版等）问是否去发布页，选「是」经 `internal/browser` 打开。同一版本在一次运行里只问一次，重启后会再问。
- 应用内更新（`update.Updater`，spec D20，ADR 0019）：设置面板与托盘对话框共用一个状态机，同一时间只跑一个：
  `idle` → `downloading`（带字节进度）→ `verifying` → `installing` → 壳 `quit`（不看 `close_to_tray`）；失败进 `failed`，带中文原因与发布页。
  开始时重新取最新发布，按 `runtime.GOOS/GOARCH` 选 `LiteRSS-<ver>-windows-<arch>-installer.exe` 或 `-darwin-universal.dmg`（其次 `-darwin-<arch>.dmg`），
  资产的下载地址必须恰为 `https://github.com/<UpdateRepo>/releases/download/<tag>/<name>`；缺安装包或缺 `SHA256SUMS` 就不下载，
  下载到系统临时目录下的 `<身份名>-update`（启动时清空），边下边算 SHA-256，与 `SHA256SUMS` 所列不符即删文件。下载用共用出网客户端的副本（同一 transport 与代理，去掉 90 秒总超时，改由 30 分钟上下文限时）。
  安装去向由 `update.Locate` 判定：便携版（exe 旁有 `portable.txt`）、Windows exe 旁没有安装包写的 `Uninstall.exe`、macOS 不在 `.app` 里或 `.app` 所在目录不可写时只能去发布页（检查结果 `in_app=false`）；
  Windows 以 `<安装包> /S /D=<exe 所在目录>` 启动（参数不加引号，NSIS 要求 `/D=` 在最后），目录不可写时经 ShellExecute `runas` 提权；
  macOS 挂载 DMG 到下载目录的 `mnt`，`ditto` 复制成 `.app` 旁的 `.<名>-update.app`，再起一个脱离会话的 `sh` 脚本（`update.MacSwap.Script`）：
  等 LiteRSS 的 PID 退出 → 旧的改名为 `.<名>-old.app` → 新的改名就位（失败则把旧的改回）→ 删旧的、卸载 DMG、`open` 结果。
  开发构建（`identity.InstallsUpdates` 为假）照常下载与校验，停在 `not_installed` 并把本会执行的命令写进日志；
  它还认环境变量 `LITERSS_UPDATE_API`，把发布 API 与下载都指向假发布服务（`tools/fake-release`），正式构建忽略该变量。
- 在系统浏览器打开链接经 `internal/browser`：只放行带主机的绝对 `http(s)` URL，其余拒绝。
- desktopapi：正式构建只挂 API mux；开发构建（`identity.BrowserChannel`）挂整个 `webui.Site`，这就是浏览器取证通道，
  入口 `http://127.0.0.1:1235/`（ADR 0012）。两种情况外面都包一层 `middleware.Recovery`。
- 回环防护（`desktopapi.protectLocalAPI`）：Host 必须是回环；`Origin` 为空或严格等于监听器自身的源（`Server.Origin()`，如 `http://127.0.0.1:1235`）
  才放行，`localhost` 别名也拒绝；`Sec-Fetch-Site` 为 `same-site` 或 `cross-site` 一律拒绝。Chrome 对同源 POST 与带 `crossorigin` 的
  同源资源 GET 都带 `Origin`，所以不能「带 `Origin` 即拒」。
- 开发构建设了环境变量 `LITERSS_WEBVIEW2_DEBUG_PORT` 时，WebView2 在该回环端口开远程调试，供取证脚本检查真实窗口；正式构建忽略它。

## 本地库（`internal/database`）

- `database.Open(ctx, path)` 打开（不存在则新建）SQLite 库并迁移到最新版本；`main.go` 打开数据目录下的 `literss.db`。
  每个连接依次设 `auto_vacuum=incremental`（只在建表前生效，供保留期清理后 `incremental_vacuum` 回收空间）、WAL、
  `synchronous=NORMAL`、`busy_timeout=5000`、`foreign_keys=1`。
- 版本化迁移：`PRAGMA user_version` 是库的 schema 版本，新库从 1 起步。`migrations[i]` 把版本从 i 升到 i+1，每步与版本号在同一个事务里提交，
  失败时 schema 与版本号都不动；库版本高于本构建认识的版本时拒绝打开。
- 列级权威由表表达（spec D6、D9）：
  - FreshRSS 所有、只由同步拉取写入（例外：推送成功后 `server_read` 按推送的值写入）：`feeds`（主键 stream ID `feed/…`）、`tags`（主键 `user/-/label/…`）、`feed_tags`，以及
    `articles` 的全部列——条目 ID（十进制，主键）、stream ID、URL、标题、图片、`published_at`、`server_read` 镜像。
  - `article_contents` 存拉取带来的 RSS 正文；`fulltext_cache`、`title_translations`、`summaries`（摘要 Markdown 与它的说明 `note`）、`article_translations`（全文译文，schema 版本 3）是本地数据，同步从不写。
    正文与全文分表，互不覆盖。
  - 用户意图：`pending_read`（每条目一行）与 `pending_mark_all`；显示状态 = 有意图取意图，否则取 `server_read`。
  - `meta` 是键值表，存上次成功同步时间这类非设置的记录；`settings` 是设置的键值表，只经 `internal/settings` 读写。
- 以 `item_id` 为键的正文、全文、标题译文、全文译文、摘要、`pending_read` 都 `ON DELETE CASCADE` 到 `articles`，删文章即连带删除。
  因此更新这些行只能 `ON CONFLICT DO UPDATE`，`INSERT OR REPLACE` 会先删行再级联清掉子表（pitfall 26）。
  `articles.stream_id` 不设外键。
- 时间：条目 ID 就是 FreshRSS 的抓取时间（微秒，`database.FetchedAt`），不另存抓取时间列。`published_at` 是 Unix 秒，只用于显示与排序，
  入库前经 `database.NormalizePublishedAt`：缺失、≤ 0、解析失败或晚于抓取时间 1 天以上时取抓取时间，其余原样保留。
- `title_translations` 有行即「已判定」，没有行是唯一的未判定状态，空译文被 `CHECK` 拒收（pitfall 12）。

## 同步（`internal/syncer`）

- `syncer.New(db, remote)` 建服务，`remote` 在每个周期与每次推送开始时取一次 FreshRSS 客户端（`syncer.Remote` 接口，`*freshrss.Client` 实现它），
  所以改了配置下次生效。`RunCycle` 与推送共用一把互斥：周期在跑时，后到的调用等它结束再跑自己的一轮。`Close` 停掉推送定时器、唤醒所有 `WaitState`，
  并等在跑的推送或周期结束。唯一的周期调用方是调度器。
- 意图写入（`intents.go`）只写意图表，从不改 `server_read`：
  - `SetRead`：单篇标已读 / 未读，意图覆盖同 URL 组的全部条目。
  - `MarkItemsRead`：「此篇及以上 / 以下」按前端快照里的条目 ID，只对显示为未读的条目（含其同 URL 组）写意图。
  - `MarkStreamRead`：源、标签或全部订阅写一行 `pending_mark_all`，`ts` 是条目 ID（抓取时间，pitfall 30）；传 0 时取本地该范围最新条目。
    写入时删掉它覆盖的未读意图（推送总是先发 mark-all，留着会被盖掉；这些条目在撤销集合里），已读意图与它一致、保留；
    被覆盖条目在别的流里的同 URL 副本另写已读意图，并进撤销集合。
  - 每次写入在同一事务里从 `meta` 的 `intent_seq` 计数器取一个 `seq`；改写已有意图时 `attempts` 清零。
  - 显示状态的 SQL 是 `syncer.DisplayRead`（对别名 `a` 的文章）：有 `pending_read` 取其值，否则被 `pending_mark_all` 覆盖即已读，否则取镜像。
- 撤销：两种批量都返回 `Batch{Token, Count}`，令牌与「这次由未读变为已读的条目 ID」只存内存 60 秒（`UndoTTL`），用一次即失效。
  `Undo` 对还没发出的 mark-all 直接删行，只给此后仍显示已读的条目写未读意图；已发出或正在发的 mark-all（推送器记在内存的在途集合里）
  与按 ID 的批量，对全部条目写未读意图。
- 推送器（`push.go`）：意图写入后约 1 秒防抖推送（新写入重置计时）；失败时按 5 秒起翻倍、最多 5 分钟退避重试。一次推送先按 `seq` 发 mark-all，
  再把 `pending_read` 按值分组、每请求 250 个 `i` 发 `edit-tag`。成功后按 `(条目, seq)` 删意图（推送在途时改写的意图因 `seq` 不同而保留），并在同一事务里把 `server_read`
  写成推送的值（mark-all 则把范围内 `ts` 以下的条目记为已读），否则意图一删显示就退回旧镜像；下次拉取照常纠正。
  服务端归咎于条目的 4xx（`APIError.RejectsItem`）把批次二分到单条，单条被拒计入 `attempts` 并记 `last_error`，满 5 次丢弃并记日志，
  文章随后跟随镜像；其他失败中止本次推送，意图原样保留、不计次数。`edit-tag` 的 200 不证明改动生效（pitfall 29），纠正靠下次拉取的镜像。
- 调度（`schedule.go`，spec D6）：`Scheduler.Run` 启动即跑一轮，此后每轮结束后等 `freshrss_auto_sync_interval` 分钟（每次重新读设置，小于 1 取默认 30）或一次触发。
  `Trigger` 给立即同步、托盘与从睡眠恢复用，`TriggerOnFocus` 给窗口获得焦点用，距上一轮开始不足 `FocusGap`（1 分钟）时跳过。
  周期只在 `Run` 的 goroutine 上串行跑，跑的时候来的多次触发并成跑完后的一轮；任何一轮都重新计间隔。触发来源见上「壳」。
- 同步状态（`state.go`，spec D14）：`State` 含 `rev`、运行中、上一轮新增条目数、待推送数（`pending_read` 与 `pending_mark_all` 行数之和）、
  上次成功时间、错误。任何字段变化 `rev` 加一并唤醒等待者；`rev` 从 1 起，所以首次请求 `since=0` 立即返回。
  变化点：周期开始（运行中）、周期结束（新增数，失败的周期也写入它补进的条目数；待推送数；成功时清错误并记时间，失败只记错误）、
  意图写入后与定时推送后（只有待推送数变了才加 `rev`；计数与发布在同一把锁里，慢的计数不会盖掉新的）。
  `WaitState(ctx, since)` 在 `since` 与当前 `rev` 不同时立即返回（包括客户端带着上次运行的 `rev`），否则等变化、`ctx` 结束或 `Close`。
- 一个周期依次为：推送意图 → 拉取 → 清理。推送失败不拦拉取（拉取不碰意图），但周期以失败返回、不记 `last_sync_at`，并按退避安排重试。拉取与清理：
  1. `subscription/list` 与 `tag/list`：`feeds`、`tags` 以 upsert 只写服务端所有的列，`feed_tags` 按订阅重写；随后删掉服务端已没有的源与标签、
     这些源的文章（连带正文、全文、译文、摘要、`pending_read`）以及指向已消失流的 `pending_mark_all`。
     返回 0 个订阅而本地有源或有文章时视为服务端异常，整步不写并记日志。
  2. 未读 ID 集：`reading-list` 排除 `read`，`n` 一次取足（10 万）；超量才续页，接受 pitfall 28 的边角（被丢的一条本周期记为已读，下周期纠正）。
  3. 高水位扫描：从最新往旧读 `reading-list` 的 ID（同样一次取足），到「本地最大条目 ID − 10 分钟」或「现在 − 90 天」为止。
  4. 未读集与扫描结果里本地没有的条目，按旧到新每批 100 条经 `stream/items/contents` 补齐，不论已读与否；写 `articles` 与 `article_contents`，
     列表缩略图取正文里第一个绝对 `http(s)` 的 `<img src>`。从旧到新是为了周期中断时高水位不越过没补上的条目，否则其中已读的下次扫不到。
  5. `server_read` 镜像：在未读集里为 0，其余本地文章为 1。拉取从不写服务端，也不碰意图表。
  6. 清理：抓取时间早于 90 天（`syncer.Retention`）、`server_read = 1` 且没有 `pending_read` 行的文章连带删除，有删除时排空 `incremental_vacuum`。
- 周期成功后在 `meta` 的 `last_sync_at` 记 Unix 秒。`RunCycle` 返回新增、因取消订阅删除、因保留期清理的文章数。

## API 路由（`internal/routes`，spec D13）

- 路由表：`routes.Table(Deps)` 是 `/api` 全部路由的唯一清单（方法、路径、是否改状态 `Mutates`、handler），`routes.Handler` 把它挂进一个 `ServeMux`，`main.go` 只用它；
  不在表里的路由不存在。desktopapi 与 Wails 资源通道挂的是同一个 mux，所以两条通道路径相同（spec D14）。
- 约定：
  - 会改状态的路由只用 POST / PUT / DELETE，从不 GET；方法不符由 `ServeMux` 回 405，handler 不运行。`routes_test.go` 的 `TestRouteTable` 遍历全表守住这一点。
  - 查询参数与路径参数不合法回 400（纯文本英文原因）；找不到文章回 404；其余错误记日志、回 500 不带细节。界面文案由前端给。
  - JSON 响应一律带 `Cache-Control: no-store`。条目 ID 以 JSON 数字传递（微秒级抓取时间，远在 2^53 以内）。
- 同步：`GET /api/sync/state?since=<rev>`（长轮询，最多挂 `LongPollTimeout` 25 秒，超时返回当时的状态；`since` 缺省为 0）；
  `POST /api/sync/run`（调 `Scheduler.Trigger`，返回 202）。`GET /api/version` 返回版本号。
- 列表读路径，查询在 `internal/library`（只读，显示状态与流范围复用 `syncer.DisplayRead`、`syncer.InStream`，所以视图与它的全部已读覆盖同一批文章）：
  - `GET /api/articles?view=unread|all&stream=<id>`：视图的快照 `{ids, newest}`，按 `(published_at DESC, item_id DESC)` 一次返回全部 ID，不分页（spec D8）；
    `view` 缺省 `unread`（显示为未读），`stream` 缺省全部订阅，也可以是 `feed/<n>` 或标签。成员由前端持有，之后已读的只变灰，同步来的新条目只经重新取快照进入列表；
    「此篇及以上 / 以下」从这份完整列表取 ID，所以覆盖界面还没加载的部分。`newest` 是成员里最大的条目 ID（最新抓取时间），即该视图全部已读的 `ts`（spec D7），空视图为 0。
  - `GET /api/articles/cards?ids=<id>,...`：按给定顺序返回卡片（每次至多 `library.MaxCards` 200 个），已不在库里的 ID 跳过（保留期清理）。
    卡片含源标题（源已不在 `feeds` 时为空）、URL、标题、标题译文（空为未判定，等于原标题为「已判定中文」）、缩略图、`published_at`、显示状态，
    以及后端从 RSS 正文抽的纯文本摘录 `excerpt`（`library.Excerpt`：去标签、解实体、跳过脚本样式等不可见元素、合并空白、最多 200 字符），前端用插值渲染。
  - `GET /api/articles/{id}/content`：`{content, fulltext}`，RSS 正文与已缓存的全文原样（不可信 HTML，由前端清洗，spec D16）；没有的为空串。
  - `GET /api/unread-counts`：`{total, feeds, tags}`，按显示状态计未读，同 URL 组在每个范围内计一次（URL 为空的条目各计一次）；没有未读的源与标签不出现。
    三者在一条语句里算出，彼此一致。
  - `GET /api/subscriptions`：`{categories: [{id, label, feeds}], feeds}`，标签按名、源按标题排序（不区分 ASCII 大小写）；
    属于多个标签的源在每个标签下各出现一次，`feeds` 是不属于任何标签的源。
- 已读动作（`routes/read.go`）经 `routes.Intents`（`*syncer.Service`）只写意图，推送器随后发给 FreshRSS（spec D6、D7）。请求体是 JSON，未知字段与类型不符回 400；已不在库里的 ID 静默忽略：
  - `POST /api/articles/{id}/read` `{read}`：单篇标已读 / 未读（连同同 URL 组），204，不给撤销令牌。
  - `POST /api/articles/read` `{ids}`：「此篇及以上 / 以下」，`ids` 是前端从快照取的范围；只对显示为未读的写意图。
  - `POST /api/streams/read` `{stream, ts}`：源、标签（含其下的源）或全部订阅全部已读到 `ts`（当前视图传快照的 `newest`；省略或 0 取该范围本地最新条目），写 `pending_mark_all`。
  - 两个批量动作返回 `{token, count}`，`count` 是这次由未读变为已读的文章数（含同 URL 的副本）。`POST /api/undo` `{token}` 撤销该批，204；
    令牌只在内存保存 `syncer.UndoTTL` 60 秒，用过、过期或重启后回 410。
- 内容动作（`routes/content.go`，经 `internal/enrich`，见下节）都用 POST：它们写本地缓存表，并替用户出网。失败原因以中文文案随 200 返回，界面直接显示；只有找不到文章（404）、参数不合法（400）与程序错误（500）走状态码：
  - `POST /api/articles/{id}/fulltext`：`{outcome, content, message}`，`outcome` 为 `success` 或一种失败（含 `no_link`：文章没有可抓的 `http(s)` 链接），`message` 是失败的中文原因。
  - `POST /api/articles/translate-titles` `{ids}`（至多 `library.MaxCards` 个）：`{titles: [{id, translated_title}], message}`，只含已判定的标题；留在未判定的不出现，`message` 说明原因（百度未配置或出错）。
  - `POST /api/articles/{id}/summary`：`{html, note}`，`html` 是渲染好的摘要（仍由前端清洗），为空表示没有生成、`note` 给原因；有摘要时 `note` 说明它不是基于全文（「摘要基于 RSS 正文。」）。
  - `POST /api/articles/{id}/translation` `{blocks}`：全文翻译（spec D21），`blocks` 是阅读区所显示正文里要翻的块文字（1 到 3000 块，不能有空块）；回 `{blocks, message}`，`blocks` 是逐块对应的纯文本中文，为空表示没有译文、`message` 给原因（模型未配置、调用失败、块数对不上）。
- 设置（`routes/settings.go` 只解析与回应，规则在 `settings.Panel`，spec D10）：
  - `GET /api/settings`：`{settings, saved_secrets}`。`settings` 含面板编辑的全部键（不含 `internal`），按 schema 类型给 JSON 字符串、布尔或整数，形状同前端生成的 `SettingsData`；
    凭据（`encrypted`）一律为空串，`saved_secrets` 列出已存了值的凭据键——存下的凭据不回到前端。
  - `POST /api/settings/update` `{key: value, ...}`：只写给出的键，值须是该键类型的 JSON 值。`internal` 键、schema 以外的键、类型不符、空串的凭据、同步间隔小于 1、
    装不上的代理（手动模式缺主机或端口）都整批拒收回 400，什么也不写。成功时回与 GET 相同的形状；写了 `proxy_*` 就立即 `httputil.ConfigureProxyFromSettings`，
    写了 `freshrss_*`（账号或同步间隔）就触发一次同步。写入不用 `PUT /api/settings`：同一路径已有 GET，路由表测试要求改状态路由的 GET 回 405。
  - `POST /api/settings/secrets/clear` `{key}`：删除一个已存凭据（非凭据键回 400），回与 GET 相同的形状，副作用同写该键（`proxy_*` 重装代理、`freshrss_*` 触发同步）。
  - `POST /api/settings/freshrss/test` `{freshrss_server_url?, freshrss_username?, freshrss_api_password?}` 与 `POST /api/settings/llm/test`
    `{llm_endpoint?, llm_model?, llm_api_key?}`：`{ok, message}`，`message` 是中文结论。省略的字段取已存的值（面板拿不到已存的凭据，不改就不传），
    其他字段回 400。FreshRSS 用一个新客户端登录一次（局域网，不走代理，pitfall 9）；模型经共用出网客户端发一个极短请求（`summary.Model.Check`，失败原因按 `ai.ErrUnauthorized`、`ai.ErrModelNotFound` 区分）。各限时 20 秒。拒收一律匹配 `settings.ErrInvalid`，handler 据此回 400。
- 杂项（`routes/app.go`）：`POST /api/browser/open` `{url}` 经 `browser.Opener` 交给系统浏览器，204；不是绝对 `http(s)` 地址回 400（前端离开窗口的唯一出口，spec D4）。
  `GET /api/update/check`：`internal/update` 向 `https://api.github.com/repos/<identity.UpdateRepo>/releases/latest` 取最新正式发布（未认证，经共用出网客户端），
  `{current_version, latest_version, update_available, release_url, in_app, message}`；`release_url` 只会是该仓库的页面，失败时其余字段为空、`message` 给中文原因；
  `in_app` 表示有新版本且能在应用内安装。它不看 `update_check_enabled`：那个开关只管壳的自动检查（见上「壳」）。
  `POST /api/update/start` 开始应用内更新（已在进行就不另起），回 202 与当前进度；`GET /api/update/status` 回
  `{state, version, received, total, message, release_url}`，`state` 为 `idle` / `downloading` / `verifying` / `installing` / `not_installed` / `failed`（见上「应用内更新」）。
- 窗口按钮（`routes/window.go`，spec D4）：`POST /api/window/minimise`、`/api/window/maximise`（最大化，已最大化时还原）、`/api/window/close`
  （与点 × 相同，按 `close_to_tray` 藏到托盘或退出），都回 204；由壳实现的 `routes.Window` 执行。浏览器取证通道里前端不显示这些按钮。

## 内容动作（`internal/enrich`，spec D11、D21）

- `enrich.Service` 每次调用都从设置读百度与模型配置，改了即生效。出网客户端由 `main.go` 注入：网页用 `httputil.CreateWebScrapingClient`（30 秒），百度与模型共用一个 `httputil.CreateHTTPClient`（90 秒）（pitfall 8）。四张结果表都用「文章仍在才写」的 upsert，保留期清理与之并发时不会写出孤行。
- 抓全文：`fulltext_cache` 有行直接返回；否则抓文章链接，经 `fulltext.Extract` 判出结论，只缓存成功（pitfall 21），失败的中文文案来自 `fulltext.Outcome.Message`。只有阅读区的「抓取全文」按钮调它（spec D11）。成功时在同一个事务里写缓存并删掉该文章的摘要与全文译文（它们基于 RSS 正文），下次按全文重做。同一文章并发的抓取合并为一次，抓取不随某个请求取消，由客户端超时兜底。
- 标题翻译：同一时间只跑一次，已有行的原样返回；标题（空白合并为一行）经 `translation.LanguageDetector.ShouldTranslate` 判为中文的存原标题（「已判定中文」，pitfall 11、12）；其余按行拼成至多 `translation.BaiduMaxQueryBytes` 的请求交给百度，请求之间隔 1.1 秒（标准版每秒一次）。百度原样返回的存原标题；百度未配置或出错时这些条目不写行，保持未判定。
- 摘要：`summaries` 存模型返回的 Markdown，每次返回时经 `summary.RenderHTML` 渲染（gomarkdown 的 `SkipHTML` 丢弃原始 HTML，`Safelink` 只留安全链接）。新摘要只用阅读区显示的正文、从不抓取：`fulltext_cache` 有行用全文，否则用 RSS 正文并在 `note` 注明；可见字符不足 300 时不调模型、只给原因（RSS 正文太短时提示可以先抓全文）。基于 RSS 正文的摘要写库时若全文已在这期间缓存则不写，免得旧摘要盖过全文。输入经 `htmltext.Text` 抽成纯文本、至多 2 万字符；提示词写死、输出固定中文（`internal/summary`）。模型未配置（端点或模型为空）或调用失败也以 `note` 说明。摘要与它的 `note` 一起入库（`summaries.note`，schema 版本 2），再次打开照样说明；模型调用随请求取消，已生成的摘要即使请求已结束也写入。
- 全文翻译（`enrich/translate.go`）：块由前端从阅读区显示的正文里取（`utils/bilingual.ts`），后端不解析 HTML。`article_translations` 每篇一行，`source_hash` 是送来的块数组（JSON）的 SHA-256，只有同样的块才命中缓存，所以正文变了会重翻并覆盖。未命中时按至多 3000 字符、40 块一批（单块超长时独占一批），同时至多 3 批交给模型（`summary.Model.Translate`：写死的提示词，要模型回与输入一一对应的 JSON 字符串数组，容忍代码围栏；`max_tokens` 8192）。任一批失败或块数对不上即取消其余批、整篇失败、不写库，`message` 给中文原因；全部成功才以 upsert 写入。

## 设置（`internal/settings`、`internal/config`、`internal/crypto`）

- 设置清单是 `internal/config/settings_schema.json`（spec D10），`tools/settings-generator` 由它生成默认值、每键元数据（`config.Lookup`）
  与前端类型；用法见 [Settings](SETTINGS.md)。
- `internal/settings.Store` 读写 `settings` 表（`main.go` 的同步账号与间隔读它）：写入时 schema 以外的键整批拒收、值按类型校验；读取时忽略 schema 以外的存值并记日志。
  `settings.Check` 单独判一个键值能否写入。
- `settings.Panel` 是设置面板经 API 做的事：按 schema 类型收发值、凭据不回显、拒收内部键与装不上的代理（`ErrInvalid`），写后装代理或触发同步，写 `startup_on_boot` 前先经 `Autostart` 钩子落到系统，以及两个测试连接。
- `settings.MinimizedWindowPos`：任一坐标 ≤ -10000 即 Windows 最小化占位位置，不保存也不采用（pitfall 33）；壳的窗口位置用它。
- 凭据（schema 里 `encrypted` 的键）由 `Store` 经 `internal/crypto` 加解密，调用方只见明文。Windows 用 DPAPI（当前用户范围，
  spec D3），macOS 用主机派生密钥的 AES-GCM（`machinekey.go`）。

## 前端（`frontend/`）

- `index.html` 不加载任何外部资源；`src/main.ts` 装上 Pinia 并挂载 `App.vue`：顶栏 `TitleBar`，其下按布局原型变体 B 的三栏（`AppSidebar`、`ArticleList`、`ArticleDetail`），加左下角的 `UndoSnackbar`。
- `TitleBar` 兼作无边框窗口的标题栏（spec D4）：`utils/frame.ts` 的 `detectHost` 看页面里有没有 `chrome.webview`（Windows）或
  `webkit.messageHandlers.external`（macOS）。顶栏左侧是图标 +「LiteRSS」合成的应用菜单按钮（通用 `ContextMenu`：「立即同步」右侧附同步状态、同步中置灰，调 `POST /api/sync/run`；「设置…」打开设置面板），
  名称右边是同步状态文案（spec D15）；按钮与菜单都不算拖动区，菜单挂在顶栏外，遮罩上的按下不会冒泡成拖动。
  没有宿主（浏览器取证通道）时它就是一条普通顶栏。有宿主时，在顶栏按下后移动才发 `wails:drag`
  （按下就交给系统会吞掉双击）；双击在 Windows 调 `POST /api/window/maximise`，在 macOS 发 `wails:drag:doubleclick`（Wails 只在 macOS 处理它，跟随系统偏好）。
  Windows 上还画最小化 / 最大化（还原）/ 关闭三个按钮，并由 `installEdgeResize` 在窗口边缘 5px（角为 L 形 15px）换光标、按下后移动发 `wails:resize:<边>`；
  窗口铺满工作区（`isMaximised`）时按钮显示「还原」且不缩放。
- `src/api.ts` 是 `/api` 的唯一客户端（类型与路由见上「API 路由」），组件与 store 不直接 `fetch`。
- store（`src/stores/`）：
  - `reader`：视图（未读 / 全部）与范围（流 ID：全部订阅、标签或源）、快照、卡片、选中、订阅树与未读数（spec D8）。进入视图时取一次快照 `{ids, newest}`，成员此后固定，
    卡片按快照顺序每页 100 个载入（滚到底部附近再取）；点开即标已读、单篇与批量已读都只改显示状态，行变灰不离开列表。同一 URL 的已载入卡片随之一起变（后端按 URL 组写意图）。
    同步周期结束时（`afterSync`）重新取未读数、订阅树与已载入卡片的状态，并对当前视图重取快照、只把不在快照里的 ID 记为横幅的「N 篇新文章」；点横幅、切换视图或范围才换快照。
    「此篇及以上 / 以下」从完整快照切范围；当前视图的「全部标为已读」以快照 `newest` 为 `ts`，侧栏右键非当前视图时不带 `ts`（spec D7）。
    慢到的旧视图响应按进入次数丢弃；本地改过显示状态的卡片不被改动之前发出的刷新覆盖。未判定的标题在卡片载入后请求一次译文，失败的不重试（pitfall 12）。
    组件触发的动作失败时在 snackbar 给中文提示（单篇已读失败还原显示），翻页失败不前移、滚动时重试。
  - `sync`：长轮询 `GET /api/sync/state?since=<rev>`，失败 5 秒后重试；状态从运行中变为停下或上次成功时间变化时调用 `reader.afterSync`。
    `syncLabel` 把状态变成顶栏与应用菜单里的同步状态文案（未配置账号按错误串结尾判断，错误串带 `sync:` 这类前缀）。
  - `detail`：跟着 `reader.selectedId` 取选中文章的 RSS 正文与已缓存的全文，有全文就显示全文，从不自动抓取。`fetchFullText`（「抓取全文」按钮）才 `POST /api/articles/{id}/fulltext`：
    成功则全文替换显示，已有摘要（或摘要说明）时清掉并重新请求摘要；失败保留 RSS 正文并显示后端给的中文原因。显示的正文不足 300 个可见字符（`utils/article.ts`，与后端摘要门槛一致）或正在抓取时摘要不可用；
    RSS 正文太短且还没有全文时 `suggestFullText` 提示先抓全文。「摘要」按钮才 `POST /api/articles/{id}/summary`；没生成时显示 `note` 的原因、按钮可再点。
    「翻译」（`toggleTranslation`，只对标题未判定为中文的文章，spec D21）点击时才用 `utils/bilingual.ts` 的 `textBlocks` 从清洗后的显示正文取块（没有可翻的文字就不请求、给出原因）（段落、列表项、小标题、引用、图注；`pre`、表格、公式里的不取），`POST /api/articles/{id}/translation`，整篇回来才切到对照；再点回原文、再点直接用已有译文。失败停在原文并显示原因，可再点重试。抓到全文后译文作废、回到原文。
    换文章后慢到的旧响应按序号丢弃，正文在翻译途中换了（抓到全文）也丢弃。
  - `snackbar`：批量已读的撤销提示与操作失败提示，8 秒后消失、悬停停表；撤销调 `POST /api/undo`，410 时直接消失，其他失败保留「撤销」可重试，成功后回调刷新卡片与计数。
- 侧栏：分段切换（未读带总未读数）、「全部订阅 → 分类（可折叠）→ 源」与未读数、不属于标签的源排在最后。未读视图只列有未读的源和还有源可列的分类（`reader.sidebarTree`），用户点选的那一项读到零未读仍保留，选别的项或切换视图后才隐藏；全部视图列出全部。源用按 ID 取色的字母徽标，不加载源图标。
  右键任一节点「全部标为已读」。侧栏没有底栏：立即同步与设置都在顶栏应用菜单里。
- 列表：宽松行（源与时间、标题最多两行、只对已判定为中文的文章显示一行摘录、有 `image_url` 才显示缩略图，英文译文打「译」标记并以原标题作悬停提示），
  右键「标为已读 / 未读、此篇及以上 / 以下标为已读、在浏览器打开」。右键菜单是通用的 `ContextMenu`（Esc、点外面、滚动、失焦都关闭）。
- 详情：无工具栏；标题块（源、时间、译文标题，有译文时下方是原标题）、抓全文提示（进行中 / RSS 正文太短时建议先抓全文 / 失败原因 +「在浏览器打开」）、摘要框、正文；
  「摘要 / 翻译（标题不是中文时；对照时为「原文」，请求中为「翻译中…」）/ 抓取全文（还没显示全文时）/ 在浏览器打开 / 标为未读（已读时）」在底部浮动条。对照时 `ArticleBody` 用 `interleave` 在同一份清洗后的 HTML 的同样的块里插入 `span.literss-tr`（`textContent` 写入，块尾，有嵌套块时在第一个嵌套块前），再经 `sanitizeArticleHtml`；译文淡色加左侧细线。正文排版写死（spec D15）。正文与摘要里的 http(s) 链接经 `POST /api/browser/open` 交给系统浏览器，`mailto:` 交给系统。
- 不可信 HTML（spec D16）：正文（`ArticleBody`）与摘要（`ArticleSummary`）是仅有的两个 `v-html`，都只经 `utils/sanitize.ts` 的 `sanitizeArticleHtml`（对照视图在清洗结果里插入纯文本译文后再清洗一次：多一轮序列化与解析，不让变异型 XSS 钻空子）。
  它先在 `DOMParser` 的惰性文档里把 `iframe`、`embed`、`object`、`video`、`audio` 换成「在浏览器打开嵌入内容」链接（只收 http(s)，没有就删），把 `data-src`/`data-original` 换进 `src`；
  再交 DOMPurify（HTML + MathML，禁 style/form 类标签与 style/id/name 属性），属性钩子让 URL 只收绝对 http(s)（链接另收 `mailto:`、图片另收 `data:image/*`；`srcset` 按浏览器的切法逐个候选检查，描述符只收 `100w`、`2x` 这种形状），
  并删掉指向应用自身源与回环主机（`localhost`、`*.localhost`、127/8、0/8、`::1`）的地址。列表缩略图的 `image_url` 经同一条规则（`safeImageUrl`）。
  清洗之后才做增强（`utils/enhance.ts`：KaTeX 公式、highlight.js 代码高亮，类名同时读 FreshRSS 的 `data-sanitized-class`），正文含 `$`、`\`、`<pre>` 或 `math` 时才动态载入这个模块。
- 图片查看器（`ImageViewer`，spec D15）：点正文里的图打开，收正文里全部图片；滚轮以指针为中心缩放，双击在适应窗口与原始大小之间切换，图片超出窗口时可拖动平移，
  点背景 / ✕ / Esc 关闭，多图左右箭头与方向键切换，载入失败时给提示；几何在 `utils/viewer.ts`。正文与摘要的链接点击规则在 `utils/links.ts`。
- 设置面板（`SettingsModal` + `stores/settings.ts`，spec D10）：固定大小的模态框（780×520，窗口小时收缩），左侧导航按 `SETTING_GROUPS` 依次为 FreshRSS、大模型、标题翻译、网络代理、应用、关于，
  右侧一次只显示一组（`v-show`，切换不丢草稿、测试结果与检查更新状态），底部常驻取消 / 保存。导航项用圆点标出有未保存改动（`dirtyGroups`）或有不合法值（`invalidGroups`，目前只有同步间隔）的分组，
  保存因不合法值被拒时跳到那一组并聚焦输入框。
  打开时 `GET /api/settings` 作草稿，「保存」一次把所有分组里清单（生成的 `settingsDefaults` 的键）内与载入值不同的键交给 `POST /api/settings/update`，成功后关闭；取消、Esc、点背景丢弃草稿。
  凭据不回显（`SecretField`，状态在 `settings.secretState`）：已存的显示不可编辑的掩码与「修改」「清除」，「修改」给空输入框并可取消，「清除」在保存前可撤销、保存时调 `POST /api/settings/secrets/clear`；没存过的是普通输入框，填了才提交；两个测试连接只传表单里的非凭据值与填过或清除的凭据，结果显示后端给的中文。
  手动代理才显示类型、地址与认证；「关于」显示 `GET /api/version`，「检查更新」调 `GET /api/update/check`：有新版本且 `in_app` 时显示「更新到 X」，
  点一次调 `POST /api/update/start`，之后每 500 毫秒轮询 `GET /api/update/status` 显示下载进度条与各步文字，失败时显示原因与「去发布页」；
  不能应用内更新时只给「去发布页」（`POST /api/browser/open`）。打开面板时若已有更新在进行或已结束（例如托盘开始的），直接显示其进度。
- 样式：`src/style.css` 放原型的全局 CSS 变量与基础样式，主题只用 `prefers-color-scheme` 跟随系统；组件样式写 scoped CSS。
- 图标：`src/components/Icon.vue` 渲染 `icons.ts` 里的内联 SVG 形状，不引图标库、不经 `v-html`。
- `npm run dev` 的 Vite 服务器只监听 `127.0.0.1:5173`，把 `/api` 代理到开发实例 1235，并只改写 5173 的 `Origin`（`devProxy.js`，pitfall 24）。

## 留用模块

这些模块保持原位，到用它的步骤再裁剪（spec D1）。

| 包 | 作用 |
| --- | --- |
| `internal/desktopapi` | 回环监听的本地 HTTP 服务；只接受回环地址，`protectLocalAPI` 做来源检查。不设 `WriteTimeout`；`Shutdown` 不取消在途请求，长轮询靠同步服务先关闭来唤醒（pitfall 31） |
| `internal/freshrss` | FreshRSS Google Reader API 客户端：订阅与标签、`stream/items/ids`、`stream/items/contents`、`edit-tag`、`mark-all-as-read`，条目 ID 为 `int64`（`ParseItemID` 认十进制与长格式）。首次调用时登录，401 时重新登录重试一次；`Clients.For` 让同一配置共用一个已登录客户端。局域网服务器用自己的 HTTP 客户端（pitfall 9）。`freshrsstest` 是测试缝假服务，`tools/fake-freshrss` 把它提供给开发实例（[Testing](TESTING.md#假-freshrss)）；`internal/syncer` 的拉取调用订阅、标签与两个 `stream/items` 接口，推送器调用 `edit-tag` 与 `mark-all-as-read` |
| `internal/fulltext` | 全文抓取：`fetch.go` 负责出网，`extract.go` 把一次响应判为 `success` / `no_content` / `blocked` / `parse_failed` / `unreachable`（pitfall 21），`message.go` 给各结论的中文原因；调用方是 `internal/enrich` |
| `internal/translation` | 百度翻译客户端（`Baidu.TranslateLines`，按行批量译成中文）与语言检测；短文本先按书写系统判语言（pitfall 11） |
| `internal/summary`、`internal/ai` | 摘要与全文翻译的模型调用：写死的中文提示词、摘要的 Markdown 渲染、译文 JSON 数组的解析；`ai` 是模型调用，OpenAI 兼容与 DeepSeek 两种请求格式 |
| `internal/htmltext` | 把不可信 HTML 抽成可见纯文本（列表摘录与摘要输入共用） |
| `internal/utils/httputil` | 出站 HTTP 客户端与三态代理（ADR 0004，pitfall 8、10） |
| `internal/update` | 检查更新与应用内更新：GitHub 最新正式发布与当前版本比较；`Updater` 下载、校验并按 `Locate` / `PlanInstall` 安装（spec D20）；`Watch` 是壳的定时自动检查；`updatetest` 是假发布服务 |
| `internal/utils/fileutil` | 按当前身份解析数据目录与日志路径，日志轮转 |
| `internal/utils/urlutil` | URL 规范化与比较 |
| `internal/errors` | 带错误码的应用错误类型（目前无调用方） |
| `internal/middleware` | HTTP 中间件（panic 转 500） |
| `internal/version` | 版本号 |

全文抓取把「这次响应该得出什么结论」放在不出网的代码里判断，因此能离线覆盖；五种结论都以 HTTP 200 返回，
只有程序级错误才走错误通道。清洗后的 readability 结果至少要有 50 个非空白 Unicode 字符，或保留了图片、音频、视频节点，才算成功。

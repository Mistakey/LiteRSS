# Pitfalls

付过代价的知识，按任务只读相关条目。编号沿用旧 AGENTS.md 的引用：退役的号不复用，新条目从 23 起编。
实测数字与源码行号是当时的证据，改动依赖它们时先核实。

| # | 主题 |
| --- | --- |
| [8](#pitfall-8) | 出站请求统一走 `httputil.CreateHTTPClient` |
| [9](#pitfall-9) | 系统代理的绕过列表只部分生效，局域网 FreshRSS 用自己的客户端 |
| [10](#pitfall-10) | `go-ieproxy` 缓存系统代理，需要定时刷新 |
| [11](#pitfall-11) | 短文本先按书写系统判语言 |
| [12](#pitfall-12) | 标题译文有行即「已判定」 |
| [17](#pitfall-17) | 行尾混用，格式化工具会翻转行尾 |
| [21](#pitfall-21) | readability 空正文不等于解析失败 |
| [23](#pitfall-23) | WebView2 采用 CSP 响应头，不需要 `<meta>` |
| [24](#pitfall-24) | Vite 开发代理只改写 5173 的 `Origin` |
| [25](#pitfall-25) | AdGuard 会改写 chrome.exe 与 msedge.exe 的回环流量 |
| [26](#pitfall-26) | `INSERT OR REPLACE` 会级联删掉文章的本地数据 |
| [28](#pitfall-28) | FreshRSS 续页 `c` 是包含上界，且无条件丢掉第一条 |
| [29](#pitfall-29) | FreshRSS 的 `edit-tag` 对任何 `i` 都回 `OK` |
| [30](#pitfall-30) | `mark-all-as-read` 的 `ts` 与条目 ID 比较，是抓取时间 |
| [31](#pitfall-31) | 长轮询不能有更短的 `WriteTimeout`，`Shutdown` 不会唤醒它 |
| [33](#pitfall-33) | 最小化窗口的位置是占位值 -32000，按缩放会变成 -21333 等 |
| [34](#pitfall-34) | 本机 npm 认为同步的 lockfile，CI 的 `npm ci` 可能拒收 |

## Pitfall 8

**Every outbound request to the internet must go through `httputil.CreateHTTPClient(timeout)`** (LAN calls are the documented exception, see the next item): it is the single place that applies the app-wide `proxy_mode` (`system` / `direct` / `manual`, see [ADR 0004](adr/0004-代理三态默认跟随系统.md)). A bare `&http.Client{}` silently opts out of the setting — `net/http` only honours `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`, and a proxy set via Windows Internet Settings (what Clash Verge, v2rayN etc. write in system-proxy mode) is invisible to it. The WebView2 frontend *does* read it, since it is Chromium, so a hand-rolled client makes frontend and backend disagree about whether a request is proxied. `httputil.ConfigureProxyFromSettings` installs the resolver from the stored settings at startup (`main.go`) and again whenever `POST /api/settings/update` writes a `proxy_*` key, which first refuses settings that cannot be installed; clients read it per request, so they never need rebuilding. 反过来也成立：**不要为了某一个调用方去改它的 transport 设置**。摘要模型与百度翻译共用 `main.go` 里用这个构造函数建的同一个客户端（`enrich.Clients.API`；模型的测试连接与更新检查也用它，更新下载用它的浅拷贝，只去掉总超时、共用 transport），动 `ForceAttemptHTTP2`、TLS 或超时之类的字段会连带改变 AI、翻译与更新检查的出站行为。需要不同 transport 行为的调用方（例如要伪装成浏览器的网页抓取）应当有自己的构造函数，而不是改这一个。

## Pitfall 9

**The system proxy's bypass list is only partly honoured**: go-ieproxy hands Windows' `ProxyOverride` to `golang.org/x/net/http/httpproxy` after swapping `;` for `,`, but that package understands exact hosts, domain suffixes and CIDR — not the `192.168.*` / `<local>` wildcard syntax Windows writes. Measured: `http://192.168.1.15:11181` resolves to the proxy, `localhost` (a literal entry) does not. In practice the request still arrives, because Clash routes private ranges direct by its own rule (measured: HTTP 400 from the LAN FreshRSS server both ways, 31 ms direct vs 18 ms via the proxy) — but it arrives *because the proxy tool is configured well*, not because LiteRSS asked for a direct connection. So `internal/freshrss/client.go` keeps its own client: a LAN server has no reason to be routed through an internet proxy, and the core sync path should not depend on someone's rule set. Judge any new LAN-facing call the same way.

## Pitfall 10

**`go-ieproxy` caches the system proxy configuration for the life of the process**: `ieproxy.GetConf()` is guarded by a `sync.Once`, and `GetProxyFunc()` captures the result in its closure. A user who toggles Clash's system-proxy switch while LiteRSS is running would keep hitting the old configuration forever. `httputil.SystemProxy()` therefore calls `ieproxy.ReloadConf()` on a 30-second TTL instead of holding one resolver; do not "optimise" that back into a single call.

## Pitfall 11

**Short text is classified by Unicode script before the statistical detector runs**: `whatlanggo` (behind `LanguageDetector.DetectLanguage`) is a trigram model and is unreliable below roughly 50 characters — exactly the length of every feed title. Measured on real titles: `OpenAI 发布新模型` comes back Indonesian at 0.612, `Rust 1.75 发布` at 0.027, `GPT-4 与 Claude 3 的对比评测` at 0.068; the last two fall under the 0.5 confidence floor, return `""`, and the old `ShouldTranslate` then translated them "for safety". `ShouldTranslate` now asks `scriptLanguage` first (`internal/translation/script_language.go`): whichever of kana, Hangul or Han holds at least 20% of the letter-class characters names the language (kana and Hangul are weighed before Han, since Japanese prose is dense with kanji); anything the script cannot settle — every Latin-script language, so English vs Spanish vs French — still falls through to the trigram model, which is what script-first means and all it claims. Every script is weighed by share, never by presence: a Chinese headline borrowing "の" as an ornament must stay Chinese. The threshold is low because product names are long and Chinese words are short (`Node.js 22 发布` is only 0.25); the accepted cost is that a mostly-English sentence behind a short Chinese lead-in reads as English. Do not "simplify" the script pass away, and do not add a request field asking callers to declare that their text is a title — the rule lives in `ShouldTranslate` precisely so every caller gets it. Simplified and Traditional are one reading language (`sameReadingLanguage`, used by both `ShouldTranslate` and `ShouldTranslateFullText` so a body and a title cannot disagree): a `zh` target skips a Traditional title rather than round-tripping it through a provider, which reverses the pre-2026-08-28 contract.

## Pitfall 12

**标题译文有行即「已判定」，不等于「已翻译」**：检测认定标题已是中文时，`title_translations` 存回的是原标题。没有行是唯一的「未判定」状态，清除译文就是删行；空值由表上的 `CHECK` 拒收，不能用来表示未判定。把「译文等于标题」当成「从未翻译」的代码会在每次渲染时重新请求——旧版反复重发标题就是这个循环。同步从不写这张表。反过来，**只有真的判定过才写行**：`enrich.Service.TranslateTitles` 在百度未配置或出错时不写这些条目（保持未判定，`message` 说明原因），若把失败写成原标题，英文标题会被永久当成「已判定中文」、再也不翻译；百度原样返回的译文才存原标题。前端的两个谓词：还要不要请求——`translated_title` 为空且本会话没请求过（`stores/reader.ts` 的 `translate`，失败的不重试，等下次启动）；显示什么——有译文用译文、否则原标题，译文等于原标题时才算中文并显示摘录（`utils/format.ts` 的 `displayTitle`、`isChinese`）。不要把「译文等于标题」当成还要翻译。

## Pitfall 17

**Line endings are mixed, and the formatters flip them**: most Go files in the working tree are CRLF (36 of 48 after the rebuild's cleanup), so `gofmt -l .` is *never* empty in this repo — that is the pre-existing state, not something a change introduced. It does not make the tool useless, it just means the list has to be filtered: a file is only a *real* violation if `gofmt` still differs after both sides are stripped of `\r` (`diff <(tr -d '\r' < f) <(gofmt f | tr -d '\r')`). As of 2026-08-28 that filtered list is empty; keep it that way, and when fixing one, write the result back with the file's *original* ending — `gofmt -w` emits LF and would turn a two-line fix into a whole-file diff. The frontend config files are mixed too (`index.html` and `eslint.config.js` are CRLF, `knip.json` is LF), so a script doing exact text replacement must detect each file's ending first — hard-coding either one makes half the matches fail. `npm run format`, `npm run lint` (which is `eslint --fix`) and `go mod tidy` rewrite endings wholesale: `core.autocrlf=true` with no `.gitattributes` means the content diff is empty and the commit is unaffected, but `git status` fills with dozens of phantom "modified" files that bury the real change (measured: 65 files touched by `lint`, only 25 of them real). Only restore formatting-only changes caused by this task after comparing with its initial status; preserve pre-existing edits. Current read-only commands are in [Testing](TESTING.md). The observations above describe the older rewriting lint command.

## Pitfall 21

**`readability.Article.Node == nil` 是「这页没有正文」，不是解析失败**：`codeberg.org/readeck/go-readability/v2@v2.1.2/article.go:19` 的注释写明 `Node` "may be nil if there were errors or if article content was blank"，`RenderHTML` / `RenderText` 随后才返回 `the Node field is nil`。这个错误串读起来像库出了问题，实际含义是 readability 判定这一页提不出文章正文——2026-08-29 的实测中它出现在 youtube 视频页、SPA 落地页和一个 PDF 上，全都是**本来就没有文章**的页面，不是抓取或解析出了错。把它当成 500 上报会让「这页没有正文」和「站点拒绝访问」「网络不通」挤进同一个错误通道，用户无从判断该不该为它开一条适配 Issue。要区分对待，见 [ADR 0007](adr/0007-不引入无头浏览器做全文抓取.md)。

`Node != nil` 也不自动等于成功：readability 曾把 20 字残渣当成完整正文。质量门槛必须在 readability 清洗后统计**非空白 Unicode 字符**，少于 50 个时归入 `no_content`；清洗结果仍含图片、音频或视频则例外保留，避免误杀短图文。不要改成 UTF-8 字节数（会让中英文门槛不同），也不要对序列化 HTML 计数（标签长度与用户可见内容无关）。这个结论仍走既有 200 + outcome 契约，因此不得写入成功全文缓存；前端（`stores/detail.ts`）对非 `success` 的 outcome 保留 RSS 正文，显示 `message` 与「在浏览器打开」，全文抓不到时摘要按 RSS 正文长度决定能不能做。

## Pitfall 23

**WebView2 采用 `webui.Static` 下发的 `Content-Security-Policy` 响应头**，所以 CSP 只在这一处写，不在构建期往 `index.html` 注入 `<meta>`（spec D16 的后备方案不需要）。
2026-09-27 实测（Wails `v3.0.0-beta.26`，WebView2 运行时 153.0.4234）：开发实例窗口里 `http://wails.localhost/` 的文档与资源响应都带完整的 CSP 头；
经 WebView2 远程调试注入的内联 `<script>`、内联 `<style>` 和 `data:` 的 `<object>` 都被拦下，控制台报 `script-src-elem`、`style-src-elem`、`object-src` 违规；
正常加载与 Wails 自己注入的运行时都不产生违规。取证步骤见 [Testing](TESTING.md#浏览器取证)。
经 CDP `Runtime.evaluate` 执行的代码不受页面 CSP 约束（其中的 `eval` 能跑），探针要看内联元素是否生效，不能拿 `eval` 当证据。
每次升级 Wails 都要重跑这项（ADR 0011）：Wails 在 Windows 上自己构造 WebView2 响应，换版本可能丢响应头。

## Pitfall 24

**Vite 开发代理只把 `Origin: http://127.0.0.1:5173` 改写为开发实例的源，其他 `Origin` 原样转发**（`frontend/devProxy.js`）。
开发实例的回环防护只放行与自身源严格相等的 `Origin`（ADR 0012），Chrome 在同源 POST 上会带 `Origin`，不改写时 Vite 页面的写操作全是 403；
反过来，整体删掉 `Origin` 会让任何经代理的异源请求都被当成 curl 放行。Vite 只监听 `127.0.0.1:5173`（`strictPort`），页面必须从这个地址打开，
从 `localhost:5173` 打开时 `Origin` 不匹配，写操作被拒。`task dev` 不经这个代理：`wails3 dev` 用 `--port` 覆盖端口，API 走 Wails 资源通道。
Vite 开发服务器直接下发页面，不经 `webui.Static`，所以热更新模式下没有 CSP 头；CSP 相关的取证只用构建后由开发实例托管的页面。

## Pitfall 25

**本机的 AdGuard 按进程名过滤流量，连回环也不放过**：用 `chrome.exe` 或 `msedge.exe`（含 `--headless`）打开 `http://127.0.0.1:1235/` 时，
AdGuard 把响应的 CSP 头改写成自己的版本（加入 `local.adguard.org` 与 `'unsafe-inline'`，`frame-src` 改为 `'self' data:`），
还注入脚本并向 `local.adguard.org` 和 `rating.adguard.com` 发请求。这会让 CSP、外部请求与控制台三项取证全部失真。
浏览器取证改用 Playwright 自带的 `chrome-headless-shell.exe`（不在 AdGuard 的过滤名单里），结果与 WebView2 一致；
WebView2（`msedgewebview2.exe`）未被过滤。不要为了取证去改用户的 AdGuard 设置。

## Pitfall 26

**`INSERT OR REPLACE` 会级联删掉文章的本地数据**：`REPLACE` 冲突策略是先删旧行再插新行，而本地库每个连接都开着 `foreign_keys`，删行会触发 `ON DELETE CASCADE`。实测（2026-09-27，modernc.org/sqlite v1.56.0）：对已有摘要的文章 `INSERT OR REPLACE INTO articles` 后，`summaries` 里那一行没了；正文、全文、标题译文与 `pending_read` 同理，丢意图还会把用户刚做的已读悄悄吞掉。拉取写 `articles`（以及 `feeds`、`tags`）一律用 `INSERT ... ON CONFLICT(主键) DO UPDATE SET ...`，只更新服务端所有的列。

## Pitfall 28

**FreshRSS 的续页 `c` 是上一页最后一个条目 ID，服务端把它当包含上界，多取一条再无条件丢掉第一条**（`greader.php` 的 `streamContentsItemsIds`、
`streamContents`）。排序键是条目 ID（抓取顺序），不是发布时间。作为边界的那一条若在两页请求之间不再满足过滤条件（例如取未读集时它在别的设备上被读掉），
被丢掉的就成了下一条真正的结果，这一条永远取不到。`n` 服务端不设上限，所以取 ID 集时一次要足够大的 `n`，只在超量时才续页并接受这个边角（spec D6）。
假服务照原样复现，`TestContinuationIsInclusiveAndDropsFirst` 守住；不要在假服务里「修好」它，否则同步的测试会对一个不存在的服务端通过。

## Pitfall 29

**`edit-tag` 对任何 `i` 都回 `OK`**：非纯数字（或以 0 开头）的 `i` 被当作长格式 ID 的十六进制尾段解析，读不出来就是 0；不存在的条目静默跳过，
结尾无条件 `exit('OK')`。所以 200 不能证明服务端改了状态，用 URL 或旧格式充当 ID 时服务端回 `OK` 却什么也没改（旧版 pitfall 14 的教训）。
条目 ID 永远是身份，推送成功后删意图并把推送的值先写进 `server_read` 镜像；真改失败时，下次拉取的镜像会纠正显示。

## Pitfall 30

**`mark-all-as-read` 的 `ts` 直接当条目 ID 上界比较**（`id <= ts`，服务端不做单位换算），条目 ID 是抓取时间的微秒数，所以 `ts` 是**抓取时间**，
不是发布时间，也不是规划文档里写过的纳秒。`ts` 为 0 或缺省时服务端取「现在」，会把用户没看过的新条目一并标掉，`freshrss.Client.MarkAllAsRead` 因此拒收非正值。
列表按 `(published_at DESC, item_id DESC)` 排序，与 `ts` 的截断顺序不同，「此篇及以上 / 以下」不能换算成 `ts`，只能按条目 ID 逐条推送（spec D7）。

## Pitfall 31

**`GET /api/sync/state` 挂起约 25 秒，承载它的 `http.Server` 不能设更短的 `WriteTimeout`**：写超时从读完请求头起算，到点时连接被掐断，
前端只看到网络错误，而不是超时后的正常快照。desktopapi 因此不设 `WriteTimeout`，以后加超时也必须长于 `routes.LongPollTimeout`。
另一半是退出：`http.Server.Shutdown` 只关监听器并等活动请求自己结束，**不取消请求 context**，挂着的长轮询会把退出拖满 25 秒。
所以长轮询同时等 `syncer.Service` 的关闭信号，`main.go` 必须先 `syncService.Close()` 再关 desktopapi（`TestClosingTheServiceLetsShutdownFinishAtOnce`）；
这也覆盖 Wails 资源通道上的长轮询。不要改成在 `Shutdown` 里取消所有请求的 context：那会连带取消退出瞬间正在写的已读意图。

## Pitfall 33

**Windows 把最小化窗口放在占位位置 (-32000, -32000)，读成逻辑像素时还会按显示缩放变小**：150% 缩放下是 -21333，200% 下是 -16000。
把这个占位位置当窗口位置存进 `window_x` / `window_y`（旧版就这样存下了 -21333），下次启动窗口会落在屏幕外。
所以只比较 -32000 不够。`settings.MinimizedWindowPos` 把任一坐标 ≤ -10000 视为占位值：真实的多显示器排布到不了这么远（两块 4K 屏左侧也只有 -3840），
而 -32000 要到 320% 缩放才越过这条线。壳保存与恢复窗口位置时不保存、不采用它（spec D4）。

## Pitfall 34

**`npm ci` 用的是 CI 上 Node 24 自带的 npm，它比本机的 npm 11.6.2 新，对 lockfile 更严**：可选的 wasm 绑定（`@napi-rs/wasm-runtime`）的 peer 依赖
`@emnapi/core`、`@emnapi/runtime` 在旧 npm 生成的 lockfile 里没有顶层条目，本机 `npm ci` 照过，CI 报 `EUSAGE` 与 `Missing: @emnapi/core@… from lock file`，
之后的 `vite` 找不到。改依赖后用 `npx -y npm@11.20.0 install --package-lock-only`（或更新的 npm 11）重生成 lockfile 再提交。2026-09-28 新仓库首次 CI 实测。

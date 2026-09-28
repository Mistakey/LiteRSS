# 正文 HTML 清洗与 CSP（mrrss-280.15）

> Epic mrrss-280「重做并精简 MrRSS」的调研节点产物。只给推荐，不含目标代码。
> 输入：`frontend.md`（mrrss-280.7，本问题的来源）、`dev-isolation.md`（mrrss-280.9）、`desktop-shell.md`（mrrss-280.1）、
> ADR 0003（图片直连）。代码基于 `main` @ `69eebd48`。实测日期 2026-09-27，环境为 Windows 11、Chrome 153.0.8010.52 headless、
> go-readability `v2.1.2`、gomarkdown `v0.0.0-20250810172220`、DOMPurify `3.4.16`、KaTeX（`frontend/node_modules` 现装版本）。
> 实测只用一次性探针，没有启动 MrRSS，也没有读用户的数据库或 FreshRSS 服务器。

## 结论

1. **在前端的渲染入口处清洗，用 DOMPurify，并且只清洗这一处。** 新前端只有两个 HTML 入口：正文（`ArticleBody`）和
   摘要（`ArticleSummary`）。两者都先经过同一个 `sanitizeArticleHtml()`，再交给 `v-html`。后端照原样存、照原样返回，
   入库和出库都不清洗。
2. **清洗配置采用白名单，并补两条 DOMPurify 默认不管的规则。**
   - 使用 `USE_PROFILES: { html: true, mathMl: true }`，不启用 SVG。禁掉 `style`、`form`、`input`、`button`、`textarea`、
     `select`、`option`、`dialog` 这些标签，以及 `style`、`id`、`name` 这些属性。`iframe`、`embed`、`object` 本来就不在
     默认白名单里，不额外放开（嵌入视频的处置见第 5 条）。
   - URL 属性（`href`、`src`、`poster`、`srcset`、`cite`）只接受绝对的 `http:`/`https:` 地址，`a[href]` 另外接受 `mailto:`，
     图片额外接受 `data:image/*`。**凡是解析后指向应用自身源（`location.origin`）或回环主机的 URL，一律删除。** 这一条 CSP
     挡不住，只能靠清洗（见依据 4）。
   - 顺序是先做字符串层的变换（`convertLazyImages`），再清洗，最后做 DOM 层的增强（KaTeX、highlight.js）。增强生成的标记
     是可信的，所以放在清洗之后。
3. **CSP 作为第二层，用响应头下发，由 Go 端给前端静态文件统一加上。** WebView 走 Wails 资源处理器
   （`main.go` 的 `CombinedHandler.fileServer`），浏览器取证通道走 desktopapi 挂载的静态文件（`dev-isolation.md` 第 4 条）。
   两条通道共用一个「前端静态文件 handler」，CSP 头在这个 handler 里设置一次。Vite 开发服务器不经过它，热更新不受影响。
   推荐策略：

   ```text
   default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' http: https: data:;
   media-src http: https:; font-src 'self' data:; connect-src 'self'; frame-src 'none';
   object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'
   ```

   正文图片直连（ADR 0003）需要 `img-src http: https:`；KaTeX 字体有一个被 Vite 内联成 `data:font/woff`，所以
   `font-src` 要带 `data:`（依据 5）。策略里不含 `'unsafe-inline'` 和 `'unsafe-eval'`，因此 `index.html` 里现有的内联主题
   脚本必须删除（`frontend.md` 已经定了要删）。
4. **后端要配合的两件事：** 所有会改状态的接口只接受非 GET 方法；摘要的 Markdown 渲染关闭原始 HTML 透传，并删除正则版
   `SanitizeHTML` 和已经没有调用方的 `CleanHTML`。前一件是清洗之外的纵深防御，后一件让摘要只输出 Markdown 能产生的标记。
   摘要到了前端照样再清洗一遍。
5. **嵌入内容（`iframe`、`embed`、`object`）不渲染，改成一条「在浏览器打开嵌入内容」链接。** 链接地址取原 `src`，
   仍然只接受 `http:`/`https:`，点击走 `POST /api/browser/open`。这与动线一致：视频这类内容去浏览器看，旧的音视频播放器
   也已删除。`<video>` 和 `<audio>` 标签保留（FreshRSS 会给附件生成它们），由 CSP 的 `media-src` 放行。
   是否保留这两个标签，由规格定；删掉它们也不影响本文其余结论。

## 依据

### 1. 两个来源实际会留下什么

**FreshRSS（`/reader/api/0/stream/contents` 的 `summary.content`）：入库时已经按白名单清洗过，但不能当作唯一防线。**

一手来源是 FreshRSS 1.30.0（2026-09-09 发布）的 `app/Models/SimplePieCustom.php` 和 `app/Models/Entry.php`：

- 从 1.28.0 起，FreshRSS 使用 HTML 白名单（CHANGELOG：「Implement HTML whitelist for SimplePie sanitizer #7924」）；
  1.30.0 又修了一处「sanitizer whitelist stripping order」（#9066）。`allowed_html_elements_with_attributes` 不含 `script`、
  `style`、`svg`、`form`、`object`、`embed`。属性只有逐元素列出的那些，外加 `aria-*`、`data-*` 和 `dir`/`lang`/`title` 等
  少数全局属性，所以 `on*` 和 `style` 都进不来。`disallow_uri_schemes(['javascript'])`。
- **`iframe` 是放行的**，并被强制加上 `sandbox="allow-scripts allow-same-origin"`。
- `rename_attributes(['id', 'class'])` 会把属性改名为 `data-sanitized-class` 和 `data-sanitized-id`（SimplePie
  `Sanitize.php:944`）。`strip_attributes` 删掉了 `data-original`。
- 清洗发生在**抓取入库时**。1.28 之前抓到的条目仍是旧黑名单清洗的结果，服务器升级不会回头重洗。
- API 输出时还会拼接内容：MrRSS 读的是 compat 模式下的 `summary.content`，它等于 `Entry::content(true)`，也就是在已清洗的
  正文后面再拼上附件 HTML（`Entry.php:232-290`）。其中 `src="{$elink}"`、`title="' . $etitle . '"` 是直接字符串拼接的，
  没有经过白名单。`$elink` 做过 `is_remote_uri` 检查，`$etitle` 是否转义取决于入库时的处理，本节点没有继续追。
  另外，输出还会按 `API_MAX_COMPAT_CONTENT_LENGTH = 500000` 字节做 `mb_strcut`，截断处可能留下半个标签。

所以，FreshRSS 这一路在新版服务器上基本是干净的。但它依赖用户服务器的版本和历史数据，输出阶段还有一段未清洗的拼接，
客户端不能假定它安全。

**go-readability 全文（`internal/fulltext/extract.go` → `RenderHTML`）：几乎不做安全清洗。**

读源码（`go-readability/v2@v2.1.2/parser.go`）加实测。实测方法是用 `readability.FromReader` 和 `RenderHTML` 处理一页
构造的恶意文章，调用方式与 `Extract` 相同。

| 输入 | 输出 | 源码位置 |
| --- | --- | --- |
| `<script>`、`<noscript>`、`<style>`、`<link>`、`<meta>`、`<base>`、`<form>` | 删除 | `removeScripts`（1668）、`removeNodes(style)`（396）、conditional clean |
| `style` 等表现属性，以及 `class`（`page` 除外） | 删除 | `cleanStyles`（1855）、`ClassesToPreserve: ["page"]`（116） |
| `onclick`、`onmouseover`、`onerror`、`ontoggle`、`onloadstart`，以及 SVG `<animate onbegin>` | **保留** | 没有任何处理 `on*` 的代码 |
| `href="javascript:…"`（全小写） | 转成纯文本 | `fixRelativeURIs`（229-246），用的是区分大小写的 `strings.HasPrefix` |
| `JavaScript:…`、`" javascript:…"`、`java&#x09;script:…`、`data:text/html,…` | **保留** | 同上 |
| 任一属性值匹配视频正则的 `<iframe>`、`<embed>`、`<object>` | **保留** | `isVideoEmbed`（1960-1980）检查的是**任意属性**，不只是 `src` |
| `<iframe srcdoc="<script>…" title="//www.youtube.com/embed/y">` | **保留** | 同上：`title` 命中正则 |
| `<iframe src="https://evil.example/?ref=//www.youtube.com/embed/x">` | **保留** | 同上 |
| `<math><mi xlink:href="javascript:…">` | 保留 | — |

### 2. 未清洗时实际能做什么（实测）

用一个一次性的同源服务器模拟 `/api`。页面把上表中保留下来的片段通过 `innerHTML` 插入（这就是 `v-html` 做的事），
每个载荷都试图向 `/api/hit` 发 POST，然后在 Chrome 153 headless 里跑四种组合：

| 载荷 | 不清洗、无 CSP | 不清洗、有 CSP 头 | DOMPurify、无 CSP | DOMPurify、有 CSP 头 |
| --- | --- | --- | --- | --- |
| `<img onerror>` | 执行 | 拦截（`script-src-attr`） | 属性已删 | 属性已删 |
| `<details ontoggle>` | 执行 | 拦截 | 属性已删 | 属性已删 |
| `<svg><animate onbegin>` | 执行 | 拦截 | 删成空 `<svg>` | 删成空 `<svg>` |
| `<video onloadstart>` | 执行 | 拦截 | 属性已删 | 属性已删 |
| `<iframe srcdoc="<script>parent.fetch(…)">` | **执行，并能调用父页面的 `fetch`** | 拦截（`srcdoc` 继承父页面 CSP） | 整个元素已删 | 整个元素已删 |
| `<a href="JavaScript:…">` 被点击 | 执行 | 拦截（`script-src-elem`） | `href` 已删 | `href` 已删 |
| `<a href=" javascript:…">` 被点击 | 执行 | 拦截 | `href` 已删 | `href` 已删 |

结论：readability 输出的全文可以直接在应用源下执行脚本。清洗和 CSP 各自都能挡住这些脚本执行向量。
DOMPurify 默认保留了 `code` 上的 `class="language-go"`，也保留了正常的外部图片。

**这个问题不只出现在浏览器取证通道里。** 在 WebView 里，`/api/*` 经 Wails 资源处理器直接进 `apiMux`，完全不经过
desktopapi 的 `protectLocalAPI`（`main.go:88-108`）。页面还可以用 `import('/wails/runtime.js')` 调用 Wails 运行时
（Wails `pkg/application/mcp_tools_enabled.go:233` 就是这样用的），即使新前端自己不 import 它。所以，正文里只要有一段
脚本能执行，就能以用户身份调用全部 `/api` 和 Wails 运行时。

### 3. 摘要这条路：正则清洗可以绕过（实测）

`summary_handlers.go` 用 `textutil.ConvertMarkdownToHTML` 生成摘要 HTML，流程是 gomarkdown 渲染（`html.CommonFlags`，
没有 `SkipHTML`，原始 HTML 会透传）再做正则 `SanitizeHTML`。摘要的输入是不受信的文章正文，提示词注入就能让模型输出
任意 HTML。把 `textutil.go` 里这两个函数原样拷进探针后运行：

| 模型输出 | `ConvertMarkdownToHTML` 的结果 |
| --- | --- |
| `<img src=x onerror=fetch('/api/articles/mark-all-read')>` | 原样保留：事件属性的正则要求值带引号 |
| `<svg/onload=alert(1)>` | 原样保留 |
| `<a href="java&#115;cript:alert(1)">` | 原样保留：正则匹配的是字面量 `javascript:` |

所以，摘要 HTML 必须走同一个前端清洗器，后端的正则版应删除，而不是继续修补。

### 4. 同源 GET 是清洗和 CSP 都要单独处理的缺口（实测）

在「DOMPurify 默认配置 + CSP 头」这个最严格的组合下，再加两个载荷：`<img src="/api/hit?…">` 和写成绝对地址的
`<img src="http://127.0.0.1:18731/api/hit?…">`。**两者都向服务器发出了 GET。** 发请求不需要脚本，而 `img-src` 为了支持
ADR 0003 的图片直连必须放行 `http:`/`https:`；按 CSP 的规则，scheme-source `http:` 也匹配应用自身源，所以 CSP 挡不住。

现有后端里恰好有靠 GET 就能改状态的接口，因为 handler 不检查请求方法：

- `GET /api/articles/mark-all-read`：不带参数时全局标为已读，并推送到 FreshRSS（`article_bulk.go` `HandleMarkAllAsRead`）。
- `GET /api/articles/read?id=…&read=…`、`GET /api/articles/favorite?id=…`（`article_sync.go`）。
- `GET /api/browser/open?url=…`：在系统浏览器里打开任意地址（`browser_handlers.go` 显式支持 GET）。
- `POST /api/feeds/update` 的 handler 也没有检查请求方法（`feed_handlers.go`），但它解析 JSON body，`<img>` 触发不了。

也就是说，今天只要一篇文章里有一行
`<img src="http://wails.localhost/api/articles/mark-all-read">`，打开它就会把全部文章标为已读并推送到服务器，用不着任何
脚本。重做后接口会重写（sync-model），规格要写明两件事：一是会改状态的接口一律只接受非 GET 方法；二是清洗器删除指向
自身源和回环主机的 URL。两层都要做，因为漏掉任何一个 GET 接口，都会重新变成可被利用的点。

### 5. CSP 与 KaTeX、highlight.js、图片直连、图片查看器共存（实测与源码）

- **KaTeX**：在同一个 CSP 头下调用 `katex.render('\frac{a}{b}+\sqrt{x^2}', …)`，结果与无 CSP 时完全一致：24 个带 `style`
  的节点、高度 34.7px、字体 `KaTeX_Main`，也没有出现 `style-src` 或 `font-src` 违规。原因是 `katex.render` 通过 CSSOM
  （`element.style[…]`）写样式，`style-src` 不管 CSSOM；只有 `renderToString` 输出的 `style="…"` 标记才会被拦，而现有
  代码没有使用它（`useArticleRendering.ts`）。KaTeX 字体由 Vite 打包到 `/assets`，其中 `KaTeX_Size3` 的 woff 被内联为
  `data:font/woff`（`frontend/dist/assets/index-*.css`），所以 `font-src 'self' data:`。
- **highlight.js**：输出只有带 `class` 的 `<span>`（`lib/core.js` 中不出现 `style=`），写入方式是对 `textContent`
  高亮后的结果做 `innerHTML`，不受 CSP 影响。
- **图片直连（ADR 0003）**：`img-src 'self' http: https: data:`，与现状相同。探针里的外部图片正常发出了请求。
  `'self'` 留给应用自己的图片；新前端图标改成内联 SVG 以后，如果不再需要，可以去掉。
- **图片查看器**：新 `ImageViewer` 只用 `<img :src>`、指针事件和 `transform`（`frontend.md` 已经删掉复制和下载，旧代码
  `ImageViewer.vue:249-275` 里的 canvas 和 `blob:` 路径随之消失），都在上面的策略之内。`transform` 用 Vue 的 `:style`
  绑定写入，走的是 CSSOM。
- **Vue**：`:style`、`v-show` 走 CSSOM；构建产物的 CSS 是外部文件；`script-src 'self'` 放行 Vite 的模块脚本。
  `index.html` 里的内联主题脚本、Google Fonts 和 unpkg 的外部资源都要按 `frontend.md` 删除，否则会被这条 CSP 拦下。

### 6. 为什么在前端清洗

| 方案 | 取舍 |
| --- | --- |
| **前端 DOMPurify（推荐）** | 用浏览器自己的解析器清洗，清洗结果与最终渲染之间没有解析器差异（后端清洗的主要风险就是 mXSS）。它就在唯一的 sink 前面，正文、摘要和以后新增的 HTML 入口都经过同一个函数。规则里的「删除自身源 URL」需要知道 `location.origin`，只有前端知道。库体积 28.9 KB（gzip 后 11.4 KB），许可证为 MPL-2.0 或 Apache-2.0，维护活跃（npm 最近一次发布是 2026-09-23）。在 vitest 的 jsdom 里可以直接写单测。 |
| 后端出库时清洗（如 bluemonday） | Go 的 `x/net/html` 与 Chromium 解析结果不同，恰好会在 SVG/MathML 命名空间和畸形标签上出分歧；摘要要再接一次；不知道前端的源；而且 go.mod 要新增依赖。 |
| 后端入库时清洗 | 同上，而且清洗策略一改，历史数据就要全部重洗；旧库迁移（`data-migration.md`）还要额外处理这件事。原样存储时，策略修改会立即作用于全部历史数据。 |
| 浏览器内置 `Element.setHTML()`（Sanitizer API） | Chrome 153 已经支持，但默认配置连 `<img src>` 都删了（实测输出只剩 `<a>` 和 `<p>`）；jsdom 不支持，单测跑不了；要用 ref 手动写入，不能配合 `v-html`。等 jsdom 支持后可以重新评估。 |

「只在前端清洗」意味着 `/api/articles/content` 返回的仍是原始 HTML。现在消费它的只有前端，以及把它当文本读的
mrrss-assistant skill。规格要把「HTML 进 DOM 之前必须经过 `sanitizeArticleHtml`」写成前端约束，并在 eslint 保留
`vue/no-v-html` 规则，只在这两个组件里豁免。

## 对现有渲染代码的影响（规格与实现 Issue 参考）

- `useArticleRendering` 用 `.math`、`.MathJax` 类和 `script[type*="math"]` 找公式，用 `class="language-*"` 识别代码语言。
  但 FreshRSS 已经把 `class` 改名为 `data-sanitized-class`，readability 也把 `class` 删了（实测输出 `<code>package main</code>`），
  清洗器还会删掉 `<script>`。所以这几条路径对两种来源都基本失效，实际起作用的只有 `$…$` 定界符、`data-math` 和自动识别语言。
  两个可选做法：让增强逻辑同时读 `data-sanitized-class`；在 `fulltext` 里给 readability 打开 `KeepClasses`
  （`ClassesToPreserve` 只能列具体类名，不能匹配 `language-*`），再由清洗器决定保留哪些 class。
  这属于渲染质量问题，不影响安全结论，由实现 Issue 决定。
- `convertLazyImages` 依赖 `data-src`/`data-original`。DOMPurify 默认保留 `data-*`，所以放在清洗之前或之后都可以。
  推荐放在之前，让清洗器检查最终的 `src`。FreshRSS 已经删了 `data-original`，这条路径只对全文生效。
- 列表的「一行中文摘录」应由后端抽成纯文本，前端用插值渲染，不走 `v-html`。

## 被否决的方案

- **只加 CSP、不清洗**：挡不住第 4 节的同源 GET，也挡不住样式层面的干扰（覆盖整页的定位元素、伪造的按钮和表单）。
- **只清洗、不加 CSP**：清洗器一旦配置失误或有漏洞，就没有第二层防护。CSP 的成本只是一个响应头，而且实测证明它与
  KaTeX、highlight.js 和图片直连都能共存。
- **修补后端正则 `SanitizeHTML`**：正则清洗 HTML 做不对，第 3 节的三条绕过都来自同一个根因。
- **CSP 用 `<meta>` 写在 `index.html`**：会作用于 Vite 开发服务器，而它的 HMR 用 JS 插入 `<style>`，会被 `style-src 'self'`
  拦下；`frame-ancestors` 在 meta 里也无效。只有当响应头在 WebView2 里被证实不生效时，才用构建期插入 meta 的方式兜底
  （Vite `transformIndexHtml` 只在 build 时执行）。
- **为嵌入视频放开 `iframe`**：FreshRSS 给 iframe 加的 `allow-scripts allow-same-origin` 沙箱只能保护跨源的 `src`；
  readability 的视频判断又能被任意属性骗过（第 1 节），`srcdoc` 可以借此进来。按动线，视频去浏览器看，不值得为它开这个口子。

## 规格需要接住的事项

- 前端约束：HTML 只能经 `sanitizeArticleHtml()`（DOMPurify，配置见结论 2）后进入 `v-html`；只有正文和摘要两个入口。
  首批单测覆盖本文第 1、2、4 节的载荷，外加 `data-sanitized-class` 和 `mailto:` 这类应该保留的情况。
- 新增前端依赖 `dompurify`（与 `frontend.md` 的删除清单一起写入依赖变更）。
- Go 端：前端静态文件 handler 在两条通道上都设置 CSP 头（策略见结论 3），并用 Go 测试断言两个挂载点都带上了这个头。
- 后端 API 约定：会改状态的接口只接受 POST/PUT/DELETE，GET 一律返回 405；写进 API 约定和 Swagger（Common Issues 18）。
- 摘要：gomarkdown 加上 `html.SkipHTML`（或者直接把 Markdown 交给前端渲染），删除 `textutil.SanitizeHTML`、`CleanHTML`，
  以及 `ConvertMarkdownToHTML` 里的正则步骤。
- 验收：在隔离的开发实例（`dev-isolation.md`）里确认 WebView2 采用了 Wails 资源处理器返回的 CSP 头（例如控制台里能看到
  违规报告，或者 `eval` 抛出 `EvalError`），并在浏览器取证通道里跑一遍第 2 节的载荷矩阵。原生窗口的观感仍由用户验收。

## 复现步骤

探针都是一次性的，不在仓库里。按下面的描述可以重建：

1. **readability**：建一个临时 Go 模块，依赖 `codeberg.org/readeck/go-readability/v2 v2.1.2`，对一页包含第 1 节表中各元素的
   HTML 调用 `readability.FromReader(f, url)` 和 `RenderHTML`，然后看输出。
2. **摘要正则**：把 `internal/utils/textutil/textutil.go` 里的 `RenderMarkdown` 和 `SanitizeHTML` 原样拷进临时模块，
   对第 3 节的三条输入调用 `SanitizeHTML(RenderMarkdown(in))`。
3. **浏览器矩阵**：用一个 Go 静态服务器（`127.0.0.1` 上的临时端口）提供 `probe.html`、`purify.min.js`，以及
   `frontend/node_modules/katex/dist` 里的 `katex.min.js`、`katex.min.css` 和 `fonts/`。页面带上 `?csp=header` 时，
   服务器才设置 CSP 头。`/api/hit` 记录收到的请求，`/log` 接收 `securitypolicyviolation` 事件和 KaTeX 的测量结果。用
   `chrome --headless=new --user-data-dir=<临时目录> --virtual-time-budget=4000 --dump-dom <url>` 分别跑四种组合，
   然后对照日志。
4. **Sanitizer API**：在 headless Chrome 里对一个 div 调用 `setHTML()`，传入 `<img src=x onerror=…>` 等片段，再读 `innerHTML`。

## 新发现（建议入图，本节点不建）

- **现有版本就能通过 GET 改状态（与重做无关，今天就能被利用）。** `GET /api/articles/mark-all-read`、`/api/articles/read`、
  `/api/articles/favorite`、`/api/browser/open` 不检查请求方法。在当前 WebView 里，一篇文章只要带一个指向
  `http://wails.localhost/api/articles/mark-all-read` 的 `<img>`，打开它就会全部标为已读并推送到 FreshRSS。Epic 的
  Rulings 说旧代码不再投入，所以是忍到重做完成，还是先热修，需要用户裁决。
- **现有版本的全文和摘要都能执行脚本。** readability 保留 `on*` 属性和视频正则放行的 `iframe srcdoc`；摘要的正则清洗可以被
  无引号的事件属性绕过。在 WebView 里，这样的脚本可以调用全部 `/api` 和 Wails 运行时。处置方式同上，需要用户裁决。
- **FreshRSS compat 输出里的附件 HTML 是清洗之后才拼接的**（`Entry.php` 里 `$etitle` 直接拼进 `title="…"`）。是否能注入
  属性，要看入库时是否转义；这属于上游 FreshRSS 的潜在问题，本节点没有继续验证。客户端清洗之后，它对 MrRSS 不再有影响。
- **`Referrer-Policy` 没有设置**：图片直连时会带上 `http://wails.localhost/` 作为来源。ADR 0003 的成功率就是在这个条件下
  测出来的，所以本节点没有改动它。要改成 `no-referrer`，得先重新测一次防盗链的影响。

---
status: accepted
---

# 不信任的 HTML 在前端用 DOMPurify 清洗，外加 CSP 响应头；嵌入媒体改为外链

> Source: literss-rb0 (D16)；规划地图 mrrss-280

文章正文（FreshRSS 下发或 readability 抓取）和 AI 摘要都是不受信的 HTML。2026-09 在 Chrome headless 实测：未清洗时 `<img onerror>`、`<details ontoggle>`、`<svg><animate onbegin>`、`<iframe srcdoc>`、`javascript:` 链接都能在应用源下执行脚本，进而以用户身份调用全部 `/api`（WebView 里 `/api` 经 Wails 资源处理器直达，不经回环防护）乃至 Wails 运行时。决定两层防护：正文与摘要只经同一个 `sanitizeArticleHtml()`（DOMPurify，HTML + MathML 白名单，禁 style/form 类标签与 `style`/`id`/`name` 属性；URL 只收绝对 `http(s):`、`mailto:`、`data:image`，并删除解析后指向应用自身源或回环主机的地址）再进 `v-html`；eslint 保留 `vue/no-v-html`，只在这两个组件豁免。后端原样存取，删除正则式 `SanitizeHTML`/`CleanHTML`，摘要的 Markdown 渲染不透传原始 HTML。CSP 由 Go 端的前端静态文件 handler 在 Wails 资源与 desktopapi 两条通道统一以响应头下发（`script-src`/`style-src 'self'`，`img-src 'self' http: https: data:`，`font-src 'self' data:`，`frame-src`/`object-src 'none'`）。`iframe`、`embed`、`object`、`video`、`audio` 一律替换为「在浏览器打开嵌入内容」链接（只接受 `http(s):`）。

## 被否决的方案

**只靠后端清洗。** 旧的正则 `SanitizeHTML` 实测被三种写法绕过：不带引号的事件属性、`<svg/onload=…>`、实体编码的 `java&#115;cript:`，根因是正则清洗 HTML 做不对。换成 Go 的 HTML 解析器（如 bluemonday）也与 Chromium 解析有分歧（mXSS 风险），且后端不知道前端的源，无法删除指向自身源的 URL。

**只靠 CSP。** 挡不住指向应用自身的同源 GET（如 `<img src="/api/...">`），也挡不住覆盖整页的定位元素、伪造按钮等样式层干扰；清洗器与 CSP 实测都能单独拦住已知的脚本向量，两层并用是为了防单点失误，CSP 的成本只是一个响应头。

**保留 iframe 白名单以显示嵌入视频。** FreshRSS 给 iframe 的 `allow-scripts allow-same-origin` 沙箱只保护跨源 `src`；readability 的视频判断能被任意属性骗过，`srcdoc` 可借此进来。按阅读动线视频本就去浏览器看，不值得开这个口子。

## 后果

- 任何新增的 HTML 入口都必须经过 `sanitizeArticleHtml()`；列表摘录由后端抽成纯文本、前端插值渲染，不走 `v-html`。
- CSP 不含 `'unsafe-inline'`/`'unsafe-eval'`：不能写内联脚本；KaTeX 被 Vite 内联的 `data:` 字体依赖 `font-src data:`。若 WebView2 不采用响应头，退到构建期注入 `<meta>`，不直接写进 `index.html`（会拦掉 Vite 开发服务器的 HMR 样式）。
- 清洗与 CSP 放在前端和响应头，改策略立即作用于全部历史数据，无需重洗库。

---
status: accepted
---

# 前端重写：去掉 i18n、Tailwind 与 Wails 绑定，主题只跟随系统

> Source: literss-rb0 (D15)；规划地图 mrrss-280

重做为 LiteRSS 时，旧前端要删的约 38% 与要重写的约 26% 合计近三分之二，且集中在阅读动线主干；同步模型、单一 AI 配置与路由删减会同时改掉后端接口。决定按选定的三栏布局从空骨架重写前端，旧代码里能用的模块按清单逐个搬入，其余整体删除，不保留新旧界面并存的过渡期。技术栈保留 Vue 3 + TypeScript + Vite + Vitest + Pinia；删除 Tailwind（改用 CSS 变量 + 组件内 scoped CSS）、vue-i18n（界面文案直接写中文）、`@phosphor-icons/vue`（换成少量内联 SVG 的 `Icon` 组件）、`@wailsio/runtime` 与 Cypress。主题只跟随系统 `prefers-color-scheme`，不设主题开关。前端只用相对 `/api` 路径，不 import 任何 Wails 包，也不请求任何外部资源，因此同一份代码能在 Wails 窗口和开发用的浏览器取证通道里走同一条路径。

## 被否决的方案

**原地精简旧前端。** 每一步都会经过「旧 store + 新 API」的中间态，而这些中间态本来就跑不起来；旧 store 的 pin 机制、多套轮询与 `mergePinnedArticles` 是给旧同步模型打的补丁，新模型下整段删除，精简等于重写 store。改名后作为新项目发布，也没有必须保持可用的旧界面。

**保留 i18n 以备多语言。** 只有一个中文用户，`language` 设置已删除；所有界面文件本就要重写，保留键表只让每处文案多一次间接查找，且约四成 `t()` 调用在要删除的文件里。将来真要加语言，再引入的成本与今天保留相当。

## 后果

- 后端原本读 `language` 的两处（摘要输出语言、托盘菜单文案）改为固定中文，`language` 设置项随之删除。
- `index.html` 不能再有内联主题脚本：CSP 不含 `'unsafe-inline'`（见 [ADR 0014](0014-不信任的-HTML-在前端用-DOMPurify-清洗外加-CSP.md)），深色模式只靠 CSS 媒体查询。
- 前端若需要桌面能力（打开浏览器、窗口操作），一律走 `/api`，由 Go 端代办，不在前端调 Wails 运行时。

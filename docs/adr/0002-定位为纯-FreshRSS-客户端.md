---
status: accepted
---

# MrRSS 定位为纯 FreshRSS 客户端，不自己抓取订阅源

> Source: mrrss-275349 (D1)；spec 原文 `git show fe3c371b:docs/tracker/closed/2026-08/精简自用版/spec.md`

本仓库的 101 个订阅源 `is_freshrss_source` 全为 1，订阅与分类都在 NAS 上的 FreshRSS 里维护，而代码一直把 FreshRSS 源显式排除在本地抓取路径之外（`internal/feed/fetcher.go:217`、`internal/feed/task_manager.go:179`、`internal/handlers/core/scheduler.go:150`）——那套完整的 RSS 抓取实现从未执行过。2026-08-27 决定把定位坐实：删除 `internal/feed` 整包及其外围（订阅源发现、OPML/JSON 导入导出、脚本源、xpath 源、email/IMAP 源），MrRSS 只负责读、标题翻译、抓全文、AI 摘要。

## 被否决的方案

**保留最小的原生 RSS 抓取作为退路**（比如 NAS 挂了，或想临时加个源不上 NAS）。`internal/feed/source/rss.go` 只是解析器，真正的抓取依赖 `task_manager` + `fetcher` + `scheduler` 整套调度，保留它等于 `internal/feed` 大部分仍在，代码里遍布的 `if feed.IsFreshRSSSource` 双语义分支也还在。这条退路买的是心理保险，付的是长期的双语义维护税——而实际加源次数是 0。

## 后果

- **本地无法添加订阅源，这是预期状态，不是缺失功能。** 加源一律在 FreshRSS 端进行。看到「不能添加订阅」不要当成 bug 去补。
- NAS 上的 FreshRSS 不可用时，MrRSS 只能读已同步的存量文章。
- 「本地源 / FreshRSS 源」的二元语义消失，`IsFreshRSSSource` 判断随之全部删除。
- 可逆，但代价不对称：`internal/feed` 一旦删除，恢复需要重建抓取调度、解析、重试、进度上报整条链路。

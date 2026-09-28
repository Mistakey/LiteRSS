---
status: accepted
---

# 作为独立新项目 LiteRSS 发布，取代 ADR 0001

> Source: literss-rb0 (D17)；规划地图 mrrss-280。取代 [ADR 0001](0001-断开上游跟随.md)。

MrRSS 被重做为只服务一条阅读动线的 FreshRSS 桌面客户端后，除 FreshRSS HTTP 层、全文抓取、百度翻译、代理、加密等少数留用模块外全部重写，与上游已不是同一个产品。决定以独立新项目 **LiteRSS** 发布：代码搬用了 MrRSS 的部分模块，许可证保持 GPL-3.0，LICENSE 原样保留，README 写一行「派生自 MrRSS」，除此之外对外不出现 MrRSS。不再跟随上游，也不再按 ADR 0001 的方式 cherry-pick：需要上游某个修复时，对照上游提交手工移植，因为双方共有的文件已所剩无几。重做期间在本仓库带历史开发；发布时以 orphan 提交作为新仓库起点，本地目录同时改名。新仓库必须公开：更新检查用的是未认证的 GitHub releases API。

新仓库具体怎么建、旧 fork `Mistakey/MrRSS` 如何处置，待定，由用户在发布前确认。

## 被否决的方案

**沿用 fork，只在 GitHub 改名。** 仓库仍挂在上游的 fork 网络下，PR 默认指向上游，不像新项目；而且改名后 GitHub 会重定向，已安装的旧版 v1.3.28 会把新项目的发布当成自己的更新。

**新仓库推入完整历史。** 旧历史里约 1700 个提交是上游作者对 MrRSS 的开发，对新项目没有意义；带着它，blame、日志和旧文档会持续把原版的实现和写法带进新项目的上下文（用户裁决，mrrss-280.16）。开发期对照旧实现所需的历史留在本地仓库，新写的文档不引用旧提交号，所以 orphan 起点不会留下死链。

## 后果

- ADR 0001 记录的两条事实已过时：GitHub 上的 fork 父仓库现在是 `DevXDojo/MrRSS`，不是 `WCY-dt/MrRSS`；本地 `upstream/main` 是 `main` 的祖先，只是从未 fetch，并不代表 0001 所说的「落后多个 release」。
- 手工移植时以上游当前仓库为准，不要尝试 `git merge upstream/<branch>` 或依赖合并基。

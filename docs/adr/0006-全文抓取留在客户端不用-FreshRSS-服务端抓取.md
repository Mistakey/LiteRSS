---
status: accepted
---

# 全文抓取留在 MrRSS 客户端，不用 FreshRSS 服务端的抓取能力

> Source: mrrss-c58513 (D1)；spec 原文 `git show fe3c371b:docs/tracker/全文抓取增强/spec.md`

FreshRSS 服务端自带全文抓取——订阅源设置里的「Article CSS selector on original website」，填入 CSS 选择器后它在服务端抓取正文并随同步下发，MrRSS 一行代码不改就能受益。[ADR 0002](0002-定位为纯-FreshRSS-客户端.md) 既然已经把订阅、分类、已读状态全部交给 FreshRSS，把抓取也交出去是顺理成章的下一步。2026-08-29 决定不这么做：抓取留在客户端。

## 被否决的方案

**在 FreshRSS 端逐源配置 CSS 选择器，MrRSS 什么都不做。** 决定性理由是代理：MrRSS 跑在用户的 Windows 机器上，出站请求经 `httputil` 的三态代理走系统代理（Clash，见 [ADR 0004](0004-代理三态默认跟随系统.md)）；NAS 上的 FreshRSS 是裸连。2026-08-29 实测中抓到的 `nytimes.com`、`engadget.com` 全靠这条代理链路，换到 NAS 侧只会更差——而抓不到的大头恰好都在墙外。

次要理由是覆盖面：FreshRSS 只能按订阅源配置，而失败最集中的 buzzing 系四个聚合源（1557 篇，占全库 22%）指向 **676 个不同域名**，其中 561 个仅出现一次。逐源配置对这条长尾无能为力。此外 FreshRSS 端逐篇抓取会拖慢同步。

## 后果

- **「抓全文」是 MrRSS 自己的职责，不要试图把它推给 FreshRSS。** 这与 ADR 0002 不矛盾：那条 ADR 列出的四项保留职责里本来就有「抓全文」，交出去的是订阅管理与状态，不是阅读增强。
- 抓取规则与抓取能力都得在本仓库维护，长期成本由本项目承担。
- 用户仍可随时在 FreshRSS 端为个别订阅源配置选择器，两者不冲突——服务端抓到的全文会随同步进入 `article_contents`，走的是正常的同步内容路径。
- 可逆：真要迁到服务端，删掉客户端抓取即可，代价对称。

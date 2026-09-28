# 不做给外部 agent 的操作接口

**决定（2026-09，用户裁定，项目 LiteRSS 重做）：删除 mrrss-assistant Skill、它的发布打包、Swagger 文档与生成注释；本地 API 只为 LiteRSS 自己的前端设计。**

## 为什么否决

用户动线里没有「让 agent 替我操作阅读器」这一步。重做后的 API 约二十个路由，完全按新前端的需要划定，
Skill 现有的收藏、稍后读、AI profile、筛选等操作已全部没有对应接口，保留就得另外设计一套对外开放的接口。

维护代价：Skill 的接口说明从 Swagger 生成，Swagger 又来自 handler 注释，三份抄本长期互相漂移（一次审计查出 46 处错误）。
只有一个消费者时，路由表的表驱动测试足以守住接口约定，前端调用集中在一个 API 模块里，不需要第二份抄本。
开发取证用 `curl` 和浏览器取证通道，也不依赖这个 Skill。

## 因此

- 发布不再附带 skills 包；接口的唯一真相是路由表测试。
- 若将来确实需要让 agent 操作 LiteRSS，应做成 CLI 或 MCP server，按当时的路由重新设计，而不是恢复一份描述 HTTP 接口的 Skill。

追溯：Epic literss-rb0（forge tracker show literss-rb0）

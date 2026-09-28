---
status: accepted
---

# MrRSS 只做桌面应用，server 与 docker 部署形态删除

> Source: mrrss-275349 (D12)；spec 原文 `git show fe3c371b:docs/tracker/closed/2026-08/精简自用版/spec.md`

MrRSS 上游同时提供两种形态：Wails 桌面应用，和一个 `//go:build server` 的 headless 变体
（`main-core.go` 240 行 + `Dockerfile.server` + `docker-compose.yml` + `docker-release.yml`
+ Taskfile 的三个 server 目标 + 两个 handler 的 server 变体）。

而本仓库的部署形态是固定的：FreshRSS 已经在 NAS 上常驻，订阅、分类与已读状态都在它那里
（见 [ADR 0002](0002-定位为纯-FreshRSS-客户端.md)）。再跑一份 headless 的 MrRSS 没有位置——
它既不是订阅源的来源，也没有第二个消费端。2026-08-27 决定删掉整个 server 形态。

## 被否决的方案

**保留 server 模式当作「以后可能想在 NAS 上跑」的退路。** 代价不是那 240 行，而是
`internal/routes` 长期维持两套配置（`DefaultConfig` / `ServerConfig`）、两个 handler 的
双变体、以及每个新 handler 都要回答「server 下怎么办」。买的是一个没有需求支撑的可能性。

## 后果

- **只有一个入口 `main.go`，只有一套路由配置。** `internal/routes` 的 `Config`、
  `DefaultConfig`、`ServerConfig`、`RegisterAPIRoutesWithConfig` 一并消失，
  `RegisterAPIRoutes` 与 `WrapWithMiddleware` 各只剩一套行为。
- **`internal/middleware` 缩到三个导出符号。** `cors.go` 与 `logger.go` 只由 `ServerConfig`
  置为 true，`ServerConfig` 一删它们立刻是死代码，随之删除；`middleware` 现在只剩
  `Middleware`、`Apply`、`Recovery`。新增中间件前先确认它有真实的开启路径。
- **`internal/desktopapi` 不受影响。** 那 150 行只有桌面回环监听（127.0.0.1:1234）与跨域
  防护，本来就没有 server 分支。skill agent 连的正是这个回环 API，属于保留面。
- **swagger 的位置变了，内容没变。** 原 `docs/SERVER_MODE/swagger.json` 记的是全部 72 条
  路由，其中只有 5 条与 server 沾边且都有桌面实现；它同时是
  `skills/mrrss-assistant/references/api.md` 的唯一生成源。目录名属于 server 模式，内容不属于，
  因此迁到 `docs/api/swagger.json`，总说明块从 `main-core.go` 搬进 `main.go`，
  `make swagger` 改为 `swag init -g main.go -o docs/api`。
- **`go.mod` 掉了 swaggo 及其 20 个传递依赖**（swag 只在生成 swagger 时用，改为按需安装）。
- **「docker 模式」指部署形态，不指工具链。** `setup:docker` 与 `build/*/Taskfile.yml` 的
  `build:docker` 是 Wails 交叉编译用的容器，不在本决定范围内，保留未动。
- 可逆，但代价不对称：恢复 headless 形态需要重建入口、双套路由配置与两个 handler 变体，
  而不只是把文件捡回来。

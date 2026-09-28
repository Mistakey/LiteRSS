# LiteRSS repo config

Wails v3 桌面应用：Go 后端 + Vue 前端，`frontend/dist` 由 Go 侧 embed，改动要经一次构建才进应用。

## Verification

- `Go 改动`: `go build ./... && go vet ./... && go test -timeout=5m ./internal/...`
- `前端改动`: `cd frontend && npm run test:unit && npm run build`
- `settings_schema.json 改动`: `go run tools/settings-generator/main.go`，再按上两条各跑一遍
- `human acceptance`: 原生窗口观感、托盘、自启、真实账号行为、真实网络抓取结果与跨会话阅读动线由用户在自建构建上判断；API 与界面行为由 agent 在隔离开发实例上取证（AGENTS.md「开发实例与取证」），界面经浏览器取证通道 `http://127.0.0.1:1235/`；需要文章与订阅数据的界面取证等假 FreshRSS 落地后接上

## Doc layout

Per forge:living-docs conventions, deviations only:

- 架构文档：`docs/ARCHITECTURE.md`

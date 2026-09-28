# 同步状态推送通道（mrrss-280.11）

> Epic mrrss-280「重做并精简 MrRSS」的规划节点产物，解决 feature-inventory.md 冲突 C1。只做推荐，不含目标代码。
> 依据：`main` @ `b5e85793` 的 `internal/desktopapi/server.go`、`main.go`，以及 go.mod 钉住的 Wails `v3.0.0-beta.8` 源码。

## 结论

**用 HTTP 长轮询：`GET /api/sync/state?since=<rev>`。不用 SSE，也不用 Wails 事件。**

- 后端的同步服务维护一份 `sync:state` 快照（运行中、新条目数、待推送数、上次成功时间、错误），外加单调递增的 `rev`。
  每次状态变化时 `rev++`，并唤醒等待者。
- 请求里 `since` 小于当前 `rev`（包括首次请求 `since=0`）时，立即返回快照；否则最多挂起约 25 秒，期间状态变了就返回新快照，
  超时则返回原快照。前端循环请求，出错时退避后重连。
- sync-model.md「同步时机」里的 Wails 事件 `sync:state` 改成这个端点，载荷不变。旧的 `/api/freshrss/status`、`pollProgress`
  和 5 秒轮询一起删掉。托盘、启动、定时等任何来源触发的同步，前端仍然都能收到（这是 sync-model 要这条通道的原因）。
- 两份结论因此可以同时成立：前端仍是纯 `/api` 客户端（desktop-shell.md），推送也由后端主动发起（sync-model.md）。

## 依据

### SSE 在 Windows 的 WebView 里用不了

WebView 里的 `/api/*` 请求走 Wails 资源服务器（`main.go` 的 `AssetOptions.Handler/Middleware`）。beta.8 在 Windows 上的响应
写入器 `internal/assetserver/webview/responsewriter_windows.go` **故意不实现 `http.Flusher`**：`Write` 只写进内存缓冲，
`Finish` 时才一次性交给 WebView2。源码注释原话："This is also why a streaming response cannot work on Windows today"。
所以 SSE 或任何流式响应，在用户的主平台上都要等请求结束才能到达页面。

绕路也行不通：让 WebView 页面（源为 `http://wails.localhost`）直连回环监听器 `127.0.0.1:1234` 上的 SSE，属于跨源请求，
会带上 `Origin`，被 `protectLocalAPI` 拒绝；放开它就等于允许 WebView 以外的网页跨源访问。而且回环端口被占用时
`desktopapi.Start` 只记日志不退出，推送通道不能依赖它。

长轮询的每个响应都是完整结束的，缓冲写入器正好能处理。资源服务器的 `dispatchWorkers` 固定为 0，每个请求单独一个 goroutine
（`assetserver_webview.go` 的注释专门提到长时间挂起的请求），一个挂起的长轮询不会卡住其他请求。

### Wails 事件加浏览器降级是两套实现

前端要为 WebView 和浏览器通道各写一套订阅逻辑，而 agent 在浏览器通道里取证时走的是降级那一套，测不到用户实际运行的路径。
这正好抵消了浏览器通道的意义，所以不选。

### 长轮询的代价可以接受

单用户、单窗口，状态变化一分钟最多几次。空闲时每 25 秒一个请求，成本可以忽略。延迟与推送相同，因为状态一变，挂起的请求就立即返回。

## 对 desktopapi 回环安全的影响

长轮询本身不需要放宽防护：它是 GET，WebView 路径根本不经过 `protectLocalAPI`；浏览器通道中同源的 `fetch` GET 按 Fetch 规范
不带 `Origin`（同源且方法为 GET/HEAD 时不序列化 Origin），`Sec-Fetch-Site` 为 `same-origin`，能通过现有检查。

**但浏览器通道整体和现有防护冲突，与推送通道选哪种无关**：`protectLocalAPI` 只要请求带了 `Origin` 就拒绝，而浏览器对同源的
POST/PUT/DELETE 也会带 `Origin`。前端从回环监听器加载后，所有写操作（标已读、改设置）都会返回 403。浏览器通道落地时，防护要改成：

- `Origin` 为空，或者**严格等于监听器自身的源**（`http://127.0.0.1:<端口>`，由 `Server.Address()` 得出，不做通配、不认 `localhost` 别名）时放行；
- `Sec-Fetch-Site` 为 `cross-site` 或 `same-site` 时拒绝；
- 保留 Host 必须是回环地址的检查（防 DNS 重绑定）。

这样仍然挡住其他网页的跨源请求（它们的 `Origin` 不同），也不需要引入 token。desktop-shell.md 提到的 token 只在「本地服务 +
系统浏览器作主壳」时才需要，那个方案已被否决。

另外两点实现时要注意：

- `desktopapi` 的 `http.Server` 没有设置 `WriteTimeout`，挂起 25 秒不会被截断，以后也不要加小于长轮询时长的写超时。
- `Shutdown` 会等活动连接结束。长轮询的等待要同时监听请求 context 和服务关闭信号（例如 `RegisterOnShutdown` 时关闭唤醒通道），
  否则退出时最多要多等 25 秒。

## 规格需要接住的事项

- sync-model.md「同步时机」一节把 Wails 事件改成本端点；「重写范围」里的 `Events` 接口改为 `rev` 加等待。
- 端点新增或删除时，按 pitfall 18 同时核对路由、Swagger 和 mrrss-assistant Skill（`/api/freshrss/status` 会被删除）。
- `protectLocalAPI` 的同源放行归到浏览器取证通道的 Issue，并配上测试：同源 POST 放行，异源 POST、`cross-site` 和非回环 Host 仍然拒绝。

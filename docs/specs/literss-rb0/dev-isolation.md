# 开发实例隔离与浏览器取证通道（mrrss-280.9）

> 原型节点 mrrss-280.9 的结论，供 mrrss-280「MrRSS 重做规格」引用。实验日期 2026-09-26。
> 并入了 mrrss-280.11（`sync-push-channel.md`）关于 `protectLocalAPI` 与浏览器通道冲突的输入。
> 实验代码只在分支 `prototype/mrrss-280.9-dev-isolation`（提交 `b9ab4185`，未推送、不合并）。
> 环境：Windows 11，Go 1.27.0，Wails `v3.0.0-beta.8`，Chrome 153.0.8010.52。
> 实验期间用户的 MrRSS（`C:\tools\MrRSS\MrRSS.exe`，PID 36492，启动于 22:46:00）一直在运行，
> 没有被结束、重启或通知，也没有读写它的 `%APPDATA%` 数据目录和 `debug.log`。

## 结论

1. **Common Issues 7 的「启动即退出」是单实例锁，不是 agent 环境的问题。** 只读探针确认用户实例持有
   Wails 的命名互斥量 `wails-app-com.mrrss.app-sim` 和消息窗口 `wails-app-com.mrrss.app-sic`。
   Wails v3 在 Windows 上用 `CreateMutex` 抢锁，抢不到就向已有实例发送 `WM_COPYDATA`，然后调用
   `os.Exit`（`pkg/application/single_instance_windows.go`、`application.go:206-216`）。所以任何
   使用 `com.mrrss.app` 的构建都会停在 `Starting Wails v3...`。
2. **只换 UniqueID 并隔离数据目录后，agent 会话能正常启动桌面实例。** 窗口标题为 `MrRSS`，有有效的
   `MainWindowHandle`，WebView2 环境创建成功，回环 API 返回 200。用户实例不受影响。
3. **开发实例需要隔离四样东西，缺一样都会碰到用户实例：**

   | 共享点 | 现状 | 隔离方式（实验已验证） |
   | --- | --- | --- |
   | 单实例锁 | `UniqueID` 硬编码为 `com.mrrss.app` | 开发构建使用不同的 ID（实验用 `com.mrrss.dev.agent`） |
   | 数据目录（`rss.db`、`logs/debug.log`） | `%APPDATA%\MrRSS`；启动时以截断模式打开 `debug.log` | 在 exe 旁放 `portable.txt`，数据写到 `<exe 目录>\data`。现有机制，不用改代码 |
   | WebView2 用户数据 | 未设置 `WebviewUserDataPath` 时使用 `%APPDATA%\<exe 文件名>`（`internal/webview2/pkg/edge/chromium.go:187-195`） | 把 `Windows.WebviewUserDataPath` 设到数据目录下（例如 `data\webview2`） |
   | 回环端口 | `desktopapi.DefaultAddress` 固定为 `127.0.0.1:1234`；绑定失败只记日志，程序继续运行 | 开发实例使用其他端口（实验用 `1235`） |

   实验时还把子进程的 `APPDATA`/`LOCALAPPDATA` 重定向到沙箱，作为额外保险。结束后沙箱里的 `Roaming`
   是空的，`Local` 下只有系统生成的 `Microsoft\Windows`。真实 `%APPDATA%` 下没有出现
   `MrRSS-dev.exe` 或 `MrRSS-server.exe` 目录。
4. **浏览器取证通道选「desktopapi 挂前端静态文件」，不选 Wails `-tags server`。** 前提是按第 5 条修改
   `protectLocalAPI`。
5. **现有 `protectLocalAPI` 连前端静态文件都会拦下，不只是写操作。** 实测结果比 mrrss-280.11 的判断更严重：
   Vite 构建的 `index.html` 用 `<script type="module" crossorigin>` 和 `<link rel="stylesheet" crossorigin>`
   加载资源，Chrome 对这类 CORS 模式的同源 GET 也会发送 `Origin`。因此前端被拦截时 CSS 返回 403，响应类型是
   `text/plain`，页面无法渲染。防护应改为：`Origin` 为空，或者**严格等于监听器自身的源**时放行；拒绝
   `Sec-Fetch-Site` 为 `cross-site` 或 `same-site` 的请求；保留回环 Host 检查。

## 实测：同源请求什么时候带 `Origin`

在一个一次性回显服务（`prototypes/dev-isolation/originecho`）上，用 Chrome 153 headless 打开同源页面，
记录服务端收到的请求头：

| 请求 | `Origin` | `Sec-Fetch-Site` | `Sec-Fetch-Mode` |
| --- | --- | --- | --- |
| `fetch` GET | 空 | `same-origin` | — |
| `fetch` POST / PUT / DELETE | `http://127.0.0.1:1237` | `same-origin` | — |
| `<link rel="stylesheet" crossorigin>` GET | `http://127.0.0.1:1237` | `same-origin` | `cors` |
| `<script type="module" crossorigin>` GET | `http://127.0.0.1:1237` | `same-origin` | `cors` |

从 `http://localhost:1237/` 打开时，结果相同，`Origin` 变为 `http://localhost:1237`。

结论：mrrss-280.11 所说「同源 `fetch` GET 不带 `Origin`」得到实测确认，长轮询 GET 能通过现有检查。
但「GET 就不带 `Origin`」**不能推广到所有 GET**：带 `crossorigin` 的资源加载和模块脚本也会带上它。

## 通道方案对比（两种都实际跑过）

| | desktopapi + 前端静态文件 | Wails `-tags server` |
| --- | --- | --- |
| 能否构建运行 | 可以 | 可以。同一个 `main.go` 加 `-tags server` 就能编译，`WAILS_SERVER_PORT=1236` 生效，前端能加载，事件 WebSocket 能连上 |
| 回环安全 | 沿用 `protectLocalAPI`（修改后见下文矩阵） | **没有任何 Origin 防护**：带 `Origin: https://evil.example` 和 `Sec-Fetch-Site: cross-site` 的 POST `/api/settings` 返回 200。监听地址由 `WAILS_SERVER_HOST` 环境变量决定，设成 `0.0.0.0` 就会把 API 无防护地暴露到局域网 |
| 与用户真实路径的差距 | 窗口外的同一个进程、同一套路由；桌面窗口可以同时开着 | 另一种构建：没有单实例、窗口和托盘，Wails runtime 走 WebSocket 而不是 WebView 桥 |
| 监听器数量 | 1 个 | 2 个（server 监听器 + 仍会启动的 desktopapi） |
| 与 ADR 0005 的关系 | 不新增构建变体，只是开发开关 | 重新引入 `//go:build server` 这一构建形态，正是 ADR 0005 删掉的东西 |

**修改后的防护矩阵**（实验中在 1235 端口实测，请求为 POST `/api/settings`）：

| 请求 | 结果 |
| --- | --- |
| 前端页面自身：同源 `Origin` + `same-origin` | 200 |
| 另一个回环端口的页面：`Origin: http://127.0.0.1:3000` + `same-site` | 403 |
| 外部网页：`Origin: https://evil.example` + `cross-site` | 403 |
| 不带 `Origin`（skill、curl） | 200 |

修改后，Chrome headless 打开 `http://127.0.0.1:1235/` 能完整渲染三栏界面，CSS 和 JS 都放行，控制台没有 403。

实验里的防护比较的是 `"http://" + r.Host`，也就是请求自带的 Host。正式实现应按 mrrss-280.11 的建议，
和 `Server.Address()` 推出的源**严格相等**，不接受 `localhost` 别名。这样从 `http://localhost:<端口>/`
打开的页面会被拒绝（上面的实测说明它发送的 `Origin` 是 `http://localhost:<端口>`），所以取证入口统一写成
`http://127.0.0.1:<端口>/`。

## 与 ADR 0005 的边界（建议写入规格）

浏览器取证通道是**开发与验收用的开关，不是部署形态**：

- 同一个桌面二进制，同一个 `main.go`，不引入 `//go:build server`，不新增路由配置。
- 默认关闭。只有开发开关打开时，才在 desktopapi 回环监听器上挂前端静态文件。开关是环境变量还是构建标签，
  由实现 Issue 决定；安装版默认不开启。
- 只监听回环地址（`validateLoopbackAddress` 已强制），使用独立的数据目录，与用户实例隔离。
- ADR 0005 否决的是「headless MrRSS 作为另一种运行形态」（NAS 或 docker）。这个通道没有无窗口运行模式，
  也不对外提供服务，因此不在 ADR 0005 的范围内。

## 复现步骤

在分支 `prototype/mrrss-280.9-dev-isolation` 上：

1. 准备 `frontend/dist`（在 `frontend/` 里 `npm run build`，或复制现有构建产物）。
2. 只读确认锁的持有者：`go run ./prototypes/dev-isolation/probe com.mrrss.app com.mrrss.dev.agent`。
   这个探针只 `OpenMutex` 和 `FindWindowEx`，不创建锁，也不发消息。
3. 启动隔离实例：`pwsh prototypes/dev-isolation/run.ps1`。输出 PID 后，检查
   `Get-Process -Id <pid>` 是否有 `MainWindowTitle=MrRSS`；`curl --noproxy "*" http://127.0.0.1:1235/api/version`
   应返回 200；`prototypes/dev-isolation/run/data/logs/debug.log` 里应看到 `Running in PORTABLE mode`，
   数据库路径在 `run\data` 下。
4. 浏览器通道：`chrome --headless=new --user-data-dir=<临时目录> --screenshot=... http://127.0.0.1:1235/`。
5. 防护矩阵：用 curl 带上不同的 `Origin` 和 `Sec-Fetch-Site` 请求 POST `/api/settings`。
   debug.log 里的 `PROTO desktopapi allow/403` 行记录每次判定。
6. server 变体：`pwsh prototypes/dev-isolation/run.ps1 -Server`，然后访问 `http://127.0.0.1:1236/`。
7. 用 `Stop-Process -Id <pid>` 结束自己启动的进程。

注意：本机设置了 `HTTP_PROXY=http://127.0.0.1:7897`。不加 `--noproxy "*"` 时，curl 访问回环地址也会
经过代理，响应头会多出 `Proxy-Connection`。

## 对 AGENTS.md Common Issues 7 的更正建议（交 forge:living-docs，本节点不改）

建议改为：

> 7. Launching a build that uses the installed app's single-instance ID (`com.mrrss.app`) while the
> user's MrRSS is running only signals that instance and exits after `Starting Wails v3...`; with no
> instance running it opens the user's real data and truncates their `debug.log`. Never launch such
> a build from an agent session. A dev instance needs its own UniqueID, a portable data dir
> (`portable.txt`), its own WebView2 user-data path and a non-1234 loopback port; with those it starts
> normally in the agent session, and the browser forensics channel drives its UI. Human visual and
> real-desktop acceptance remains separate from automated checks.

具体措辞等开发隔离开关落地后再定。在那之前，「开发构建不得使用 `com.mrrss.app`」这一条可以先写进去。

## 应入图的后续

- 开发实例隔离开关：UniqueID、`WebviewUserDataPath`、desktopapi 地址，外加开发构建默认启用 portable 数据目录。
- 浏览器取证通道：desktopapi 在开发开关下挂前端静态文件；`protectLocalAPI` 改为同源严格相等，并补测试
  （同源 POST 和带 `crossorigin` 的同源资源 GET 放行；异源、`same-site`、`cross-site`、非回环 Host 拒绝）。
- `main.go` 截断 `debug.log`：即使做了隔离，截断也会丢掉上一次运行的日志。可以考虑改成轮转。
- 前端 `index.html` 对 `fonts.gstatic.com` 做了 `preconnect`，与「不代理、隐私优先」的取向是否一致，需要确认。

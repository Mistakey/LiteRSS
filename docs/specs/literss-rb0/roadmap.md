# 重做路线与迁移顺序（mrrss-280.8）

> Epic mrrss-280「重做并精简 MrRSS」的汇总节点产物，作为 forge:spec 的直接输入。只给结论，不含目标代码。
> 输入：本目录全部节点文档（`desktop-shell.md`、`sync-model.md`、`feature-inventory.md`、`layout-prototype/`、`frontend.md`、
> `dev-isolation.md`、`data-migration.md`、`sync-push-channel.md`、`read-retention.md`、`html-sanitize.md`），用户裁决
> mrrss-280.4、.6、.14，以及 mrrss-280.8 上的 8 条汇总待办评论。本文不修改那些文档；两者冲突时，以本文「五、跨文档调和」为准。
> 仓库事实基于 `main` @ `ffd4f844`：`origin` = `Mistakey/MrRSS`（GitHub 上是 `DevXDojo/MrRSS` 的 fork，公开），
> `upstream` = `WCY-dt/MrRSS`，`upstream/main` 已是 `main` 的祖先；LICENSE 为 GPL-3.0；Wails 钉在 `v3.0.0-beta.8`；
> 用户日常使用的是已安装的 v1.3.28。

## 结论

1. **新骨架、逐块迁移，不原地删改。** 第一步一次性删掉所有「删除 / 重写」类代码，换上新的壳、身份常量、开发隔离和前端空骨架；
   留用的模块（FreshRSS HTTP 层、全文抓取、百度翻译与语言检测、代理、加密、desktopapi、设置生成器、摘要的模型调用）留在原位，
   到用它的那一步再裁剪。不设新旧代码并存的过渡期。
2. **在本仓库内重建，保留完整历史；改名时推到一个新的独立 GitHub 仓库（不是 fork），旧仓库归档。** 名字、仓库形态、旧仓库去留和
   tracker 键由用户拍板（新建节点 mrrss-280.16）。重做前几步用占位身份常量，不等这个答案。
3. **迁移顺序 S0–S9**（见「四」）：准备 → 清场与壳骨架 → 数据层与设置 → 同步核心与假 FreshRSS → 旧库导入 → API → 前端 → 壳集成
   → 改名、文档与发布 → 切换。每步都有 agent 能独立取得的验证点；原生窗口观感和真实账号行为由用户在 S7、S9 验收。
4. **重做期间旧版照常日常使用**，条件是：开发实例和 agent 的验证**只连假 FreshRSS、只用隔离数据目录**，不碰真实账号和真实库。
   新旧两版不得同时连同一个账号运行（旧版的差异推送会回退已读），因此用真实账号试新版就等于切换，必须先从托盘彻底退出旧版。
5. 「Not yet specified」里的改名与发布已能写成精确问题，转为 human 节点 mrrss-280.16；Wails 版本钉住不需要用户决定，直接写成规格
   条目（见「五」R19）。重算后 Epic 不再有未定的迷雾。

## 一、路线：原地删改，还是新骨架逐块迁移

| | 原地删改 | 新骨架、逐块迁移（推荐） |
| --- | --- | --- |
| 后端 | 同步核心、数据库、路由都要换主键和权威模型（sync-model「结论」），中间态跑不起来 | 旧同步、旧库访问、旧 handler 在 S1 整体删除；新模块按依赖顺序长出来 |
| 数据库 | 原地改表同时踩 pitfall 19、20（data-migration「结论」） | 新库从 `user_version = 1` 起步，旧库只读导入一次 |
| 前端 | 近三分之二要删或重写，且集中在动线主干（frontend「为什么不原地精简」） | 从空骨架起，复用清单里的模块逐个搬入 |
| 每步可验证性 | 每步都要让旧界面和新后端同时可用，验证点是「没坏」，而不是「新东西对了」 | 每步交付一块新能力，配自己的测试和取证 |
| 代价 | 大量写了就删的适配代码 | S1 之后到 S6 之前，主干构建出的应用没有可用界面（旧版照常用已安装的二进制，不受影响） |

旧代码的参考方式：S0 打标签 `legacy-final`，需要对照时用 `git worktree add` 检出只读副本，或 `git show legacy-final:<路径>`。
不在主干里保留 `legacy/` 目录：`go build ./...` 会同时构建两套代码，旧代码也会被 agent 当作现行实现读到。

## 二、仓库：新仓库，还是本仓库内重建

「新仓库」有两种做法，所以实际比较三种：

| | A · 本仓库内重建，改名时推到新的独立仓库（推荐） | B · 新仓库从零历史起步 | C · 沿用 Mistakey/MrRSS，只在 GitHub 改名 |
| --- | --- | --- | --- |
| 历史 | 全部保留。本目录各文档、ADR（如 ADR 0005 引用 `fe3c371b`）、pitfall 引用的提交号继续有效 | 全部断开，文档里的提交号成为死链 | 全部保留 |
| ADR 0001 的 upstream cherry-pick | 合并基仍在，技术上仍能 cherry-pick。实际价值很小：重做后与上游共有的只剩 FreshRSS HTTP 层、全文抓取、代理等少数文件，其余全部重写 | 没有合并基，只能手工移植补丁 | 同 A |
| 是否像新项目 | 是：独立仓库，不在 fork 网络里 | 是 | 否：仍挂在 `DevXDojo/MrRSS` 的 fork 网络下，PR 默认指向上游 |
| forge tracker | `.forge/project`（`mrrss`）随历史带过去，Epic 与全部节点原样延续，不需要 `init` | 要么手工带上 `.forge/project`，要么 `init` 新键，Epic 留在旧项目，两边分裂 | 不变 |
| 旧版更新检查 | 旧版 v1.3.28 指向 `Mistakey/MrRSS` 的 releases；旧仓库归档后发布页仍在，照常工作 | 同 A | 仓库改名后 GitHub 重定向，旧版会把新项目的发布当成自己的更新 |
| 本地工作目录 | 不变。改名时把 `origin` 改指新仓库，旧的改名为 `legacy-origin` 或删除；`upstream` 可保留 | 要新建工作目录，worktree、原型分支都要迁移 | 不变 |

推荐 A。几个细节：

- **推送时机**：重写提交**不推到** `Mistakey/MrRSS`，让旧仓库的 `main` 停在 `legacy-final`，归档后就是干净的旧版存档。新仓库一旦由
  mrrss-280.16 定下名字就建立并作为 `origin`，此后的提交都推到那里；在那之前提交只在本地（主工作树与本地分支），由用户决定是否另做备份。
- **ADR 0001** 在 S8 被一条新 ADR 取代：新项目与上游不再有跟随关系；需要上游某个修复时，对照上游提交手工移植，能直接 cherry-pick 的
  文件越来越少。ADR 0001 状态改为 superseded，不删除。
- **许可证不可选**：代码派生自 GPL-3.0 的 MrRSS，且保留了部分模块，新项目仍是 GPL-3.0，README 写明来源。
- **仓库必须公开**：更新检查（U10 保留）用未认证的 GitHub releases API。
- `CNAME`（`mrrss.ch3nyang.top`）是上游的站点域名，S8 删除。

## 三、重做期间旧版的日常使用

- **S0–S8：旧版照常用。** 用户继续用已安装的 v1.3.28（不从本仓库重新构建）；主干在 S1 之后不再产出可用的旧版，但不需要：
  Epic Rulings 已定旧代码不再投入，旧 bug 与安全问题都忍到切换。
- **开发与 agent 验证不碰真实账号和真实数据。** S1 落地开发隔离后，开发实例使用独立 UniqueID、portable 数据目录、独立 WebView2
  用户数据和 1235 端口（dev-isolation「结论」3），并且**只连 S3 提供的假 FreshRSS**。原因有两个：agent 不应改写用户账号的已读状态；
  旧版正在同一账号上运行，开发实例标的已读可能被旧版差异推送回退，验证结果会被干扰。
- **新旧版不得同时连同一个账号运行。** 旧版每个周期先拉已读集、再重新取一次已读集做差异推送；两次之间别处标的已读会被推回未读
  （sync-model「新发现」1）。新版只推用户明确做过的意图，不会伤害旧版，但会被旧版伤害。
  - 交替使用（先彻底退出一个，再打开另一个）风险很低：旧版启动后先把服务端已读状态拉到本地，只有拉已读集失败时才会回退。
  - 所以「用真实账号试用新版」就是 S9 切换：先从托盘彻底退出旧版（旧版默认关闭到托盘，关窗口不等于退出）。
- **新版对旧版的防护**：不只在导入时探测旧版互斥量 `wails-app-com.mrrss.app-sim`（data-migration「旧版仍在运行」），而是**每个同步
  周期开始时都探测一次**（只 `OpenMutex`，成本可忽略）。探测到时，同步状态带上 `legacy_running`，侧栏底部的同步状态显示
  「旧版 MrRSS 正在运行，会把已读改回未读，请退出」。不暂停新版的同步：暂停保护不了任何东西，回退是旧版做的。发布说明写同一句话。
- **回退路径**：新版从不修改旧目录和旧库，所以切换后若新版有阻断问题，退出新版、打开旧版即可；旧版启动后会把服务端状态拉回本地。

## 四、迁移顺序与可验证点

每一步可以由 forge:slice 切成多个实现 Issue；验证命令以 `.forge/config.md` 为准（Go：`go build ./... && go vet ./... && go test
-timeout=5m ./internal/...`；前端：`npm run test:unit && npm run build`）。「agent 取证」都在隔离开发实例 + 假 FreshRSS 上做。

### S0 · 准备（不写代码）

- 在重做开始前的最后一个提交上打标签 `legacy-final`，推到旧仓库。
- mrrss-280.16（名字与仓库）可以并行等待，不阻塞 S1–S6。
- 验证点：标签存在；`git worktree add <旁路目录> legacy-final` 能检出旧代码供对照。

### S1 · 清场、身份常量、壳骨架、开发隔离、浏览器通道、CSP

- 删除 feature-inventory / frontend.md 判为「删除」或「重写」的全部代码：`frontend/src`、Cypress、Tailwind、vue-i18n、
  `@phosphor-icons/vue`、`@wailsio/runtime`、`frontend/bindings/`；旧同步（`bidirectional_sync.go`、同步队列、`*WithSync`、
  `performImmediate*`、`client.go` 里无调用方的 `SyncService`）、旧库访问、AI profile、AI 翻译、媒体缓存、window handler、旧路由。
  留用模块保持原位，删掉它们对已删代码的引用。
- 新增身份常量包（名字、UniqueID、数据目录名、desktopapi 端口、开机自启注册表值名、更新检查仓库），开发构建整体覆盖为开发值；
  名字先用占位，S8 只改这一处。
- 新 `main.go`：Wails 升到当时最新 beta 并钉住（R19）；单实例、窗口、系统浏览器打开链接。托盘、关闭到托盘、开机自启、窗口位置留到 S7。
- 开发隔离开关：UniqueID、portable 数据目录、`WebviewUserDataPath`、1235 端口（dev-isolation「应入图的后续」）。
- 浏览器取证通道：开发开关下 desktopapi 挂前端静态文件；`protectLocalAPI` 改为「`Origin` 为空或严格等于 `Server.Address()` 推出的源，
  拒绝 `same-site`/`cross-site`，保留回环 Host 检查」。
- 两条通道共用一个前端静态文件 handler，在其中下发 CSP 头（html-sanitize 结论 3）。
- `debug.log` 由截断改为轮转（保留上一次运行的日志）。
- 前端空骨架：无外部资源的 `index.html`（删 Google Fonts、`preconnect`、unpkg 图标脚本、内联主题脚本）、`main.ts`、原型的 CSS 变量、
  `Icon` 组件、三栏空壳、主题只跟随系统。
- **验证点**：
  - Go 单测：防护矩阵（同源 POST 与带 `crossorigin` 的同源资源 GET 放行；异源、`same-site`、`cross-site`、非回环 Host、
    `localhost` 别名拒绝）；两个挂载点都带 CSP 头；开发构建的身份常量与正式构建不同。
  - agent 在用户实例运行时启动隔离开发实例：`127.0.0.1:1235/api/version` 返回 200，用户实例 1234 仍返回 200 且 PID 不变，
    真实 `%APPDATA%` 下没有新目录。
  - Chrome headless 打开 `http://127.0.0.1:1235/`：三栏空壳渲染，控制台没有 403 和 CSP 违规，网络面板没有外部请求。
  - WebView2 是否采用 CSP 响应头：在隔离实例里确认（方法由实现 Issue 定，例如开发开关下前端启动自检 `eval` 是否抛 `EvalError`
    并写入 debug.log）。不生效时退到构建期注入 `<meta>`（html-sanitize「被否决的方案」末条）。

### S2 · 数据层与设置

- 新库结构从 `PRAGMA user_version = 1` 起步，带版本化迁移框架（不再是「每次启动重跑、忽略错误」的做法）。
  表：订阅与标签（服务端所有）、文章（主键为条目 ID，含 stream ID、URL、标题、图片、`published_at`、镜像 `server_read`）、
  RSS 正文、全文缓存、标题译文、摘要、`pending_read`、`pending_mark_all`、`meta`。列级权威按 sync-model「权威关系」，
  正文与全文分表，不再需要 pitfall 20 那种列级约定。
- 设置 schema 收缩为 R11 的清单，重跑设置生成器；设置层拒收未知键（写入返回 400，加载时忽略并记日志）。
- 凭据加密：Windows 用 DPAPI（当前用户范围），其他平台沿用现有方案；旧格式 `MrRSS-v1:` 的解密函数只留给 S4 导入器（R13）。
- **验证点**：Go 单测覆盖建库与 `user_version`、迁移框架从 1 升 2 的样例、未知键拒收、加密字段往返；生成器产物与 schema 一致，
  前端生成的类型能编译。

### S3 · 同步核心与假 FreshRSS

- 假 FreshRSS：一个 Go 测试用的 greader 服务（`httptest`），外加一个可运行的开发工具，用生成的订阅和文章供开发实例连接。必须复现
  已查明的服务端行为：长格式与十进制 ID、`n` 不设上限、续页 `c` 为包含上界且丢首条、`it`/`xt` 过滤、edit-tag 对任何 `i` 都回 `OK`、
  `mark-all-as-read` 按 `ts` 生效、令牌过期回 401。
- 客户端新增 `stream/items/ids`、`stream/items/contents`、`mark-all-as-read`；同一配置复用已登录的客户端。
- 同步服务：意图写入（单篇、按 ID 批量、按流全部已读、撤销）、推送器（防抖约 1 秒、按值分组、每请求约 250 个 `i`、比较 `seq` 后删除、
  退避与二分定位）、同步周期（R1 的新步骤）、调度（R3）、同步状态快照加 `rev` 与长轮询等待（R4）、旧版探测（「三」）。
- **验证点**（全部对假 FreshRSS）：续页边界条目被别处读掉时的行为与 read-retention「新发现」1 一致；拉取不覆盖待推送意图；
  推送在途时写入的新意图不被误删；单条被拒时二分定位且计入 `attempts`；取消订阅的源的文章被删除，`subscription/list` 返回 0 个订阅时跳过；
  清理规则（抓取时间早于 90 天、显示为已读、无意图）；高水位扫描补上在别处读掉的新条目；长轮询在状态变化时立即返回、25 秒超时返回、
  `Shutdown` 时被唤醒。agent 取证：开发实例连假 FreshRSS，`curl` 长轮询端点看到周期开始与结束两次 `rev` 变化。

### S4 · 旧库导入

- 按 data-migration.md 实现，带 R5、R12、R13 的修订：不设 `read_at`；`published_at` 按旧字符串格式解析，失败时用抓取时间；
  窗口坐标丢弃最小化哨兵值；AI 配置按旧解析链取一条写入 `llm_*` 键；未同步队列转 `pending_read`；探测到旧版运行时推迟导入；
  支持用环境变量指定旧库路径。
- **验证点**：Go 单测用一个**合成的旧库**（按 `legacy-final` 的旧建表语句在测试里生成，不取自用户真实库），覆盖：17 篇无 ID 文章被丢弃、
  同 URL 多条目各自保留、「已判定中文」的译文原样搬运、未同步队列按最后一条转意图且丢星标、白名单外的设置键被丢弃、
  旧格式凭据解密失败时只丢这一项、导入完成先于第一个同步周期、旧版在运行时推迟且不写 `legacy_import`。
  对用户真实库的导入只在 S9 由用户完成；agent 不读真实库。

### S5 · API

- 路由按 R10 的清单重写。会改状态的接口只收 POST/PUT/DELETE，GET 一律 405。列表摘录由后端抽成纯文本。
- 同步更新 Swagger（`docs/api/swagger.json`）与 `skills/mrrss-assistant`（pitfall 18）。
- **验证点**：表驱动测试遍历路由表，断言每个改状态的路由对 GET 返回 405；handler 单测覆盖快照、按 ID 取卡片、按 ID 标已读、撤销；
  重新生成的 Swagger 与路由一致；Skill 里的路径与路由一致。

### S6 · 前端

- 按 frontend.md「重写路线建议」第 2–6 步：store → 侧栏 → 列表 → 详情 → 设置页。正文与摘要只经 `sanitizeArticleHtml()`
  （DOMPurify）进入 `v-html`，eslint 保留 `vue/no-v-html` 并只在这两个组件豁免。
- **验证点**：vitest 覆盖 store 的「快照在视图内稳定」「新到条目只进横幅，点击才载入」（沿用旧 `app.test.ts` 的测试意图）、撤销、
  清洗器的载荷矩阵（html-sanitize 第 1、2、4 节，外加应保留的 `mailto:`、`data-sanitized-class`）；每个子步骤由 agent 在浏览器通道里
  对「隔离实例 + 假 FreshRSS」截图，对照原型变体 B 与 mrrss-280.6 的修改。

### S7 · 壳集成

- 托盘（中文菜单：显示窗口、立即同步、退出）、关闭到托盘、开机自启（注册表值名取自身份常量）、窗口位置（退出时保存一次，最小化哨兵
  坐标不保存也不采用）、窗口获得焦点与从睡眠恢复时触发同步、更新检查指向新仓库的发布页（需要 mrrss-280.16 的答案）。
- **验证点**：Go 单测覆盖窗口状态的哨兵过滤、开机自启注册表键名、更新检查的仓库地址；隔离实例能启动并响应 API。
  托盘、关闭到托盘、开机自启和原生窗口观感由用户在自建构建上验收（Epic Rulings）。

### S8 · 改名、文档与发布

- 身份常量改为新名字；Go module 名、安装包（NSIS 产品名与安装目录）、`build/config.yml`、发布工作流改到新仓库；删除 `CNAME`；
  README 与 LICENSE 说明来源；按 mrrss-280.12 的方案完成文档收尾，新增 ADR，ADR 0001 标为 superseded；推送到新仓库并发布第一个版本。
- **验证点**：新仓库 CI 通过；发布产物能在装有旧版的机器上并行安装（安装目录、数据目录、UniqueID、开机自启键都与旧版不同）；
  新安装的应用在旧版运行时启动，能看到「旧版正在运行」提示且不导入。

### S9 · 切换（用户）

- 用户：从托盘彻底退出旧版 → 安装新版 → 首次启动自动导入并查看导入报告 → 对照 FreshRSS 网页端核对未读列表 → 在新版标几篇已读并在网页端确认
  → 核对托盘、自启、更新提示 → 用几天后卸载旧版 → 在 GitHub 归档旧仓库。
- 这是一个 human 验收节点，由 forge:slice 在交付尾部建立；验收清单即上面这些步骤加 S1、S7 里标明由用户验收的项。

依赖关系：S1 → S2 → S3 → S4；S5 依赖 S3（S4 可与 S5 并行）；S6 依赖 S5；S7 依赖 S1，更新检查部分依赖 mrrss-280.16；
S8 依赖 S1–S7 与 mrrss-280.16；S9 依赖 S8。

## 五、跨文档调和（forge:spec 的直接输入）

每条写明来源和结论。原文档不改；规格按这里写。

**R1 · 同步周期（修订 sync-model 第 3–5 步，吸收 read-retention 与 C2）**
每个周期串行执行，由 `runExclusiveSync` 保证互斥：

1. 探测旧版互斥量，结果写进同步状态。
2. 推送：清空 `pending_read` 与 `pending_mark_all`。
3. `subscription/list`、`tag/list`，只覆盖服务端所有的列；**成功且返回订阅数大于 0（或本地原本就没有订阅）时**，删除 stream ID 不在列表中的
   本地文章及其正文、全文、译文、摘要、意图（mrrss-280.14 选 A）。
4. 未读 ID 集：`stream/items/ids?s=reading-list&xt=read`，`n` 取一个足够大的值一次取完，只有超量时才用 `c` 续页并接受边界漏一条的情况。
5. 高水位扫描：`stream/items/ids?s=reading-list&r=d`，从最新往旧读，读到「本地最大条目 ID − 10 分钟」或「现在 − 90 天」为止。
6. 补内容：未读集与高水位扫描中本地没有的 ID，用 `stream/items/contents` 每批约 100 条补齐，不论是否已读。
7. 更新镜像：在未读集里的 `server_read = 0`，其余本地条目 `server_read = 1`。
8. 清理：抓取时间（条目 ID ÷ 10^6 秒）早于「现在 − 90 天」、显示为已读、没有待推送意图的文章连带删除，然后 `incremental_vacuum`。

不再拉星标集，也没有 `server_starred` 列和星标意图（R2）。「已读条目不下载」作废。

**R2 · C2：收藏删除后的同步**（feature-inventory C2，mrrss-280.4）
不拉星标集、不下载星标正文、不推星标意图；服务端的星标保持原样。意图表只剩一个字段，简化为 `pending_read(item_id 主键, value, seq,
attempts, last_error)`，语义与 sync-model 的 `pending_state` 相同；data-migration 里的 `field = read` 即此表。

**R3 · C3：同步时机**（feature-inventory C3，mrrss-280.4）
删除 `freshrss_sync_on_startup`。后端调度固定为：启动时、每 `freshrss_auto_sync_interval` 分钟、从睡眠恢复、窗口获得焦点（距上次不足 1 分钟跳过）、
托盘「立即同步」、点击侧栏底部同步状态。意图推送在每次操作后防抖约 1 秒触发，失败退避重试，不等下一个周期。

**R4 · 长轮询替代 Wails 事件**（sync-push-channel，C1）
`GET /api/sync/state?since=<rev>`，挂起约 25 秒；载荷为运行中、新条目数、待推送数、上次成功时间、错误，加上 `legacy_running`。
sync-model「重写范围」里的 `Events` 接口改为「当前快照 + 按 `rev` 等待」。删除 `/api/freshrss/status`、`/api/freshrss/sync-feed`，
前端三套轮询随旧前端删除。desktopapi 不加短于长轮询的 `WriteTimeout`；等待同时监听请求 context 与关闭信号，`Shutdown` 时唤醒。

**R5 · 保留期与时间基准**（read-retention，mrrss-280.14）
保留期 90 天，按抓取时间，写死。data-migration「导入时 `read_at` 取导入时间」作废，不设 `read_at`。`published_at` 只用于显示与排序：
缺失、为 0、解析失败或晚于抓取时间 1 天以上时用抓取时间；早于抓取时间的原样保留。已取消订阅的源按 R1 第 3 步删除。
data-migration「不只靠重新拉取」的理由要更新：新模型会重新下载 90 天内的全部条目，但导入仍然必要，因为译文、摘要和凭据拉不回来；
导入的 10938 篇中抓取时间早于 90 天的已读文章会在第一个周期末尾被清理（包括其中的摘要），这是 mrrss-280.14 接受的结果。

**R6 · 「此篇及以上 / 以下标为已读」的推送语义**（mrrss-280.5 发现）
**按条目 ID 逐条，不换算成 `ts` 区间。** `mark-all-as-read` 的 `ts` 按条目 ID（抓取顺序）截断，而列表按 `(published_at DESC, item_id DESC)`
排序，「以上」在抓取顺序里不是连续区间，换算必然多标或漏标。做法：前端持有视图的完整有序 ID 快照（sync-model「本地列表分页」），
直接把范围内的 ID 列表发给后端（「以下」方向可能上万个，约 200 KB 的请求体，可以接受）；后端只对显示为未读的条目写 `pending_read`，
按同 URL 分组展开到组内全部 ID（sync-model「身份」），返回撤销令牌。推送器按每请求约 250 个 `i` 分批。快照过大退化为 keyset 时，
前端按游标补齐范围内的 ID 后再发送。

**R7 · 撤销 snackbar 的后端语义**（frontend.md 新发现，mrrss-280.6）
**后端按批次撤销。** 所有批量已读（源/分类/全部订阅的「全部标为已读」、「此篇及以上/以下」）都返回撤销令牌；后端在内存里保存令牌对应的
「这次由未读变为已读的条目 ID」，保存约 60 秒（比 snackbar 的 8 秒宽裕），应用重启即失效。撤销时：
- 对这些 ID 写 `pending_read = 0`（新的 `seq`），推送器照常推送；
- 如果批次是 `pending_mark_all` 且尚未推送（`seq` 未变），直接删除这一行；已推送的则只能按上面逐条改回未读。
  服务端在 `ts` 之前抓到、本地还没同步到的条目，撤销后仍是已读，不专门处理。
「全部标为已读」的 `ts`：列表当前视图就是这个范围时，取快照中最新条目的抓取时间；从侧栏右键对非当前视图操作时，取本地该范围内最新条目的抓取时间。
单篇的点开即已读和「标为未读」不走撤销，直接写意图。

**R8 · 侧栏计数与列表快照**（mrrss-280.5 发现）
侧栏的未读数是实时值（按显示状态计，同 URL 折叠后计一次），每次操作和每次同步后随 `rev` 刷新；列表成员在快照内固定，已读只变灰。
同步带来新条目时列表顶部显示「N 篇新文章」，侧栏计数立即增加。两者口径不同是预期的，不另做提示。

**R9 · AI 配置键名**（mrrss-280.10 发现）
新键用 `llm_endpoint`、`llm_model`、`llm_api_key`（加密），**不用** feature-inventory 暂定的 `ai_endpoint`/`ai_model`/`ai_api_key`：
它们与旧库 `settings` 里的遗留键同名，而那组值（`gpt-4o-mini`、空密钥）从未生效。导入器只从 `ai_profiles` 按旧解析链取值，从不读旧的
`ai_*` 键。`ai_summary_prompt`、`ai_usage_*` 等全部不继承。

**R10 · 路由清单**（feature-inventory 第五节按 mrrss-280.4、.6 修订）
保留或新增：列表快照、按 ID 取卡片、正文、抓全文、标题翻译、摘要、单篇标已读/未读、按 ID 批量标已读（取代 `mark-relative`，两个方向）、
按流全部已读（含子分类，取代 `mark-all-read`）、撤销、未读计数、订阅树、设置读写、测试 FreshRSS 连接与测试模型连接、`POST /api/sync/run`、
`GET /api/sync/state`、`POST /api/browser/open`、`/api/version`、检查更新。删除其余全部，包括 `/api/window/*`（窗口位置在壳的 Go 侧）、
下载与安装更新、翻译与摘要清空、缓存管理。具体路径由规格定。

**R11 · 设置清单**（feature-inventory 第一节按 mrrss-280.4、.6 修订）
用户可见：`freshrss_server_url`、`freshrss_username`、`freshrss_api_password`、`freshrss_auto_sync_interval`、`baidu_app_id`、
`baidu_secret_key`、`llm_endpoint`、`llm_model`、`llm_api_key`、`proxy_mode`、`proxy_type`、`proxy_host`、`proxy_port`、
`proxy_username`、`proxy_password`、`update_check_enabled`、`close_to_tray`、`startup_on_boot`。内部：`window_x`、`window_y`、
`window_width`、`window_height`、`window_maximized`。
`freshrss_last_sync_time` 删除：增量靠本地最大条目 ID，上次成功时间存 `meta`（data-migration 已定不导入）。
feature-inventory 里被裁决推翻的建议：托盘、关闭到托盘、开机自启保留（mrrss-280.4）；图片查看器保留；ActivityBar 删除（mrrss-280.6 推翻
mrrss-280.4）；「标记以下为已读」保留；`ConfirmDialog` 删除（批量已读不确认，改用撤销）；排版、用量、提示词相关设置全部删除。

**R12 · 设置面板形态**（mrrss-280.5 发现：没有节点负责）
一个模态框、一页滚动，分组依次为：FreshRSS（地址、用户名、密码、同步间隔、测试连接）、摘要模型（端点、模型、密钥、测试连接）、
标题翻译（百度 App ID、密钥）、网络代理、应用（关闭到托盘、开机自启、检查更新）、关于（版本）。设置太少，不做左侧标签页；
样式用原型的 CSS 变量。

**R13 · 旧库导入的补充**（mrrss-280.10 评论）
- 窗口坐标：导入时丢弃 -32000、-21333 这类哨兵值；新壳保存时也不写入最小化状态下的坐标（S7）。
- 设置层拒收未知键（S2）。
- 凭据：旧格式用旧方法解密，按新方式（Windows DPAPI）重新加密；解密失败只丢这一项并在导入报告里提示重填。
- 旧版探测：导入时推迟（data-migration），之后每个周期都探测（「三」）。

**R14 · 安全约定**（html-sanitize，mrrss-280.15 评论）
写进 API 约定：改状态的接口只收 POST/PUT/DELETE，GET 返回 405，并同步 Swagger（Common Issues 18）。CSP 头按 html-sanitize 结论 3，
S1 验收 WebView2 是否采用。摘要 Markdown 加 `SkipHTML`，删除 `SanitizeHTML`、`CleanHTML`。`Referrer-Policy` 本次不改：改成
`no-referrer` 前要重测防盗链（ADR 0003），不属于本 Epic。

**R15 · 嵌入与媒体**（html-sanitize 结论 5 留给规格）
`iframe`、`embed`、`object`、`video`、`audio` 一律替换为「在浏览器打开嵌入内容」链接（只接受 `http:`/`https:`，走 `POST /api/browser/open`）。
理由：U11 已删播放器，真实库 0 篇音视频，默认删；CSP 随之去掉 `media-src`。

**R16 · 正文渲染质量**（mrrss-280.15 评论）
公式与代码语言识别要兼容 FreshRSS 的 `data-sanitized-class`，或给 readability 打开 `KeepClasses`，由详情渲染的实现 Issue 选；
不影响安全结论。`fetchOutcome` 改为直接返回中文原因文案。

**R17 · 浏览器通道与 Vite 开发代理**（mrrss-280.7、.9 评论）
取证默认用「构建后由开发实例托管」。需要热更新时，Vite 代理只把 `Origin: http://127.0.0.1:5173` 改写为目标监听器的源，其他 `Origin`
原样转发（仍被拒绝），不整体去掉 `Origin`。取证入口统一写 `http://127.0.0.1:<端口>/`，不用 `localhost`。

**R18 · 壳的范围**（desktop-shell 被 mrrss-280.4 推翻的部分）
壳做：窗口、单实例、系统浏览器打开链接、窗口位置、托盘（菜单文案固定中文）、关闭到托盘、开机自启、焦点与唤醒触发同步。
desktop-shell 里「删托盘/自启」「剪贴板改 `navigator.clipboard`」两条作废：复制按钮已全部删除，前端没有剪贴板调用。
摘要输出语言与托盘文案固定中文，随 `language` 删除（frontend.md）。

**R19 · Wails 版本钉住**（Epic「Not yet specified」，desktop-shell「规格需要接住的事项」）
S1 升到当时最新的 v3 beta（不低于 `v3.0.0-beta.26`）并钉住；此后只为一个具体的修复或阻断问题升级，PR 写明原因。每次升级重跑：
隔离实例能启动、单实例仍然生效、`WebviewUserDataPath` 仍可用、CSP 头仍被采用、长轮询不被缓冲写入器截断、托盘 API 可用。
探测旧版用的互斥量名 `wails-app-com.mrrss.app-sim` 由已安装的旧二进制决定，与新版钉哪个 Wails 版本无关。
退回 Wails v2 仍是后备，只在 v3 出现阻断性缺陷时考虑。

**R20 · 其他小项**
- `debug.log` 轮转（S1）。
- `index.html` 的 Google Fonts `preconnect` 删除（frontend.md 已答 dev-isolation 的疑问）。
- FreshRSS 默认 `keep_unreads = false`，积压 3 个月以上或每源超过 200 篇的未读会被服务端删除：写进 README 的「用户须知」和首个发布说明。
- `docs/specs/literss-rb0/layout-prototype/` 与本目录一起在 forge:finish 时处理。

## 六、重算地图

- Epic「Not yet specified」只有一条：改名与作为新项目发布（新名字、新仓库、发布页、Wails 版本钉住与升级节奏）。
  - 新名字、仓库形态、旧仓库去留、tracker 键：需要用户决定，已写成精确问题，新建 human 节点 **mrrss-280.16「定新名字与发布仓库」**
    （问题原文在节点正文）。它不阻塞任何规划节点：规格用占位身份常量写，S7 的更新检查和 S8 改名等它。
  - 发布页：由 mrrss-280.16 的仓库决定（`https://github.com/<owner>/<新仓库>/releases`），不单独成题。
  - Wails 版本钉住与升级节奏：不需要用户决定，已写成 R19。
- 该条迷雾移除后 Epic 没有剩余迷雾。规划完成还差：mrrss-280.12（文档体系）与 mrrss-280.16（用户拍板）。

## 七、给 mrrss-280.12（文档体系重做方案）的输入

约束来自 mrrss-280.12 的正文：重做期间 agent 读到的文档不能与所处阶段的代码矛盾。下表列出每一步让哪些现有文档内容失效，
供它决定「在哪一步改、改成什么」。

| 步骤 | 失效的文档内容 | 新增要记录的事实 |
| --- | --- | --- |
| S1 | ARCHITECTURE 的前端、handler、路由、同步部分；CODE_PATTERNS 的 i18n、Tailwind、前端模式；TESTING 的 Cypress 与前端部分；BUILD_REQUIREMENTS 的 Windows CGO/mingw；AGENTS.md Common Issues 3（`task dev` 是否仍成立）、7（按 dev-isolation 更正）、22；pitfall 15、22 | 开发隔离与浏览器取证通道的用法、身份常量、CSP、`legacy-final` 标签的对照方法 |
| S2 | SETTINGS.md；AI_CONFIGURATION.md 与 .zh.md；pitfall 6、16、19、20；Common Issues 4、5、6、19、20 | 新库的版本化迁移、设置拒收未知键、DPAPI |
| S3 | pitfall 13、14；ARCHITECTURE 的同步部分 | 新同步模型（条目 ID 身份、意图表、无差异推送、周期步骤、长轮询）、假 FreshRSS 的用法 |
| S4 | — | 旧库导入的规则与一次性 |
| S5 | `docs/api/swagger.json`、`skills/mrrss-assistant`（路由全部变化） | 改状态接口只收非 GET |
| S6 | CODE_PATTERNS 与 TESTING 的前端部分（S1 起为空骨架，此时重写） | HTML 只经 `sanitizeArticleHtml` 进入 `v-html` |
| S7 | ARCHITECTURE 的壳部分 | 托盘、自启、窗口位置归壳 |
| S8 | README、README_zh、CHANGELOG、SKILLS.md 与 .zh、CNAME、ADR 0001（superseded）、AGENTS.md 与 CLAUDE.md 的项目名 | 新项目身份、来源说明、`keep_unreads` 用户须知 |

仍然有效、可以原样保留的：pitfall 8、9、10（代理）、11、12（标题语言检测与「已判定」语义，改为新译文表）、17（行尾）、18（路由三份抄本）、
21（readability 空正文）；ADR 0002、0003、0004、0005、0006、0007；`docs/out-of-scope/`。

建议补录的新 ADR（每条对应一个节点结论）：新项目改名与独立仓库（取代 ADR 0001）；以条目 ID 为身份、意图表、取消差异推送；
同步状态用 HTTP 长轮询；前端重写并去掉 i18n 与 Tailwind；HTML 在前端清洗加 CSP；浏览器取证通道是开发开关不是部署形态（与 ADR 0005 的边界）；
本地保留 90 天按抓取时间；旧库一次性只读导入；托盘与自启保留（推翻 desktop-shell 的删除建议）；Wails v3 钉版与升级规则。

S1 删除旧代码后到各文档重写之前，文档与代码必然有一段不一致。mrrss-280.12 需要决定是随步骤逐份改写，还是 S1 时先给失效文档加
「已过时，见 roadmap」的标记。

## 八、新发现（未入图）

1. **`upstream/main` 已是 `main` 的祖先**：本地的 upstream 引用停在 fork 时的状态，`git log main..upstream/main` 为空，与 ADR 0001 的
   「落后多个 release」描述不一致，只是说明没有 fetch 过。无需处理，ADR 0001 被取代时一并说明。
2. **GitHub 上的 fork 父仓库已是 `DevXDojo/MrRSS`**，不是 ADR 0001 写的 `WCY-dt/MrRSS`（上游已迁移或改名）。新 ADR 写来源时以实际为准。
3. **旧仓库根目录的 `CNAME`（`mrrss.ch3nyang.top`）是上游站点的域名**，对本仓库没有意义；S8 删除。

# 文档体系重做方案（mrrss-280.12）

> Epic mrrss-280「重做并精简 MrRSS」的规划节点产物，作为 forge:spec 的输入。只给方案，本文不改任何现有文档。
> 输入：Epic Rulings（文档并入重做、ADR 保留并补录）、`roadmap.md`（尤其「四」的 S0–S9 与「七」的失效清单）、
> `dev-isolation.md`「对 AGENTS.md Common Issues 7 的更正建议」、forge:living-docs 的文档约定，以及 `main` @ `2949786e`
> 上的现有文档。本文与 roadmap「七」冲突时，以本文为准（修订见「四」末尾）。

## 结论

1. **迁移规则：文档与代码在同一个提交里变化，不加「已过时」标记。** 哪一步删除了代码，就在同一步删除描述它的文档段落；哪一步让新形态
   落地，就在那一步的 Issue 回写里写上。重做期间尚未落地的部分，唯一权威是 spec（canonical window 内），不预先写进
   `ARCHITECTURE.md`。S1 起 `AGENTS.md` 带一节「重做进行中」，指向 spec 和 `legacy-final`，到 forge:finish 删除 spec 时一并删掉。
2. **结构**：`AGENTS.md` 仍是唯一的共享指令文件，`CLAUDE.md` 保持现在的一行 `@AGENTS.md` 薄适配器。pitfall 从
   `docs/AGENT_PITFALLS.md` 改名到约定位置 `docs/pitfalls.md`。重写：ARCHITECTURE、TESTING、CODE_PATTERNS（大幅缩短）、
   SETTINGS（缩短）、BUILD_REQUIREMENTS（更正）、README、CHANGELOG。删除：AI_CONFIGURATION 两份、Cypress README、`CNAME`；
   按推荐方案还删除 swagger、`skills/mrrss-assistant` 与 SKILLS 两份（需用户确认，Q1）。原样保留：ADR 0002–0007、
   `docs/out-of-scope/`、`scripts/README.md`（随脚本变化再改）。新增：`CONTEXT.md`（由 forge:spec 建）、ADR 0008 起的一组新 ADR、
   几份新的 out-of-scope 记录。
3. **时间点**：绝大多数失效发生在 S1，不是 roadmap「七」列的 S2、S3：S1 一次性删除了旧同步、旧库访问、AI profile 和整个前端，
   pitfall 6、13、14、15、16、19、20、22 都在 S1 失效。新事实在让它成立的那一步写入（详见「三」）。
4. **ADR**：新增 9 条（0008–0016），其中 0016 取代 ADR 0001；在 forge:spec 收尾归档时写入（0016 等 mrrss-280.16 的答案），
   不留到 S8。托盘与自启保留、GET→405、DPAPI 不写 ADR（不满足三条标准）。删掉的功能写进 `docs/out-of-scope/`，不写 ADR。
5. **Common Issues 7**：S1 落地开发隔离开关的同一个提交里，改写为「开发实例规则」，同时改 `.forge/config.md` 的 human acceptance 一行和
   TESTING.md 对它的引用。在那之前不改（Epic Rulings：随重做处理）。
6. **mrrss-assistant 与 swagger**：推荐在 S1 随旧路由一起删除，以「路由表测试」作为 API 的唯一真相，pitfall 18 随之退役；
   这等于删掉「让外部 agent 操作用户数据」这项功能，需用户确认（Q1）。

## 一、原则

- **同一提交**：删除代码的提交同时删除描述它的文档；新增模块的 Issue 在回写中更新 ARCHITECTURE 与相关 pitfall
  （forge:living-docs「架构文档在形态改变时写」）。forge:slice 切 Issue 时，把本文「三」里对应步骤的文档项写进 Issue 的验收条件。
- **删除优于标记**：「已过时，见 roadmap」的横幅挡不住 agent 读下面的内容，反而让同一份文件同时陈述两种形态。
  失效段落直接删；需要对照旧实现时用 `git show legacy-final:<路径>`。
- **ARCHITECTURE 只写已经存在的东西**：S1 后它只描述空骨架和留用模块，逐步长回来。目标形态在 spec 里，ARCHITECTURE 顶部一行说明这一点。
- **重做进行中**：S1 起 `AGENTS.md` 有一节（五六行）：Epic 与 spec 路径；「目标形态以 spec 为准，本仓库文档只描述已落地的代码」；
  旧实现的对照方法（`legacy-final` 标签 + `git worktree add`）；README 与 CHANGELOG 在 S8 之前描述的是旧版。forge:finish 删除 spec 时删除这一节。
- **编号不复用**：pitfall 编号保留、退役的号不再使用，新条目从 23 起编；ADR 按顺序续号。已关闭 Issue 与提交信息里的引用不会指错。
- **语言**：重写的文档用中文（与 ADR、spec、用户一致）；代码标识符、命令、路径原样。

## 二、目标结构

| 文件 | 去留 | 新形态 |
| --- | --- | --- |
| `AGENTS.md` | 重写 | 见下「AGENTS.md 的形态」 |
| `CLAUDE.md` | 保留 | 维持 `@AGENTS.md` 加一句适配说明；S8 只在有 Claude 专属适配内容时才加，现在没有 |
| `CONTEXT.md` | 新增 | 由 forge:spec 建：条目 ID、同 URL 组、意图（`pending_read` / `pending_mark_all`）、服务端所有列、显示状态、列表快照、高水位、保留期、撤销令牌、旧版、开发实例、浏览器取证通道等术语。其他文档只用不定义 |
| `docs/ARCHITECTURE.md` | 重写 | 只写「是什么」：组件、路径、边界、数据流一页可读；理由链到 ADR。目标 200 行以内（现 505 行） |
| `docs/pitfalls.md` | 改名 + 重写 | 由 `docs/AGENT_PITFALLS.md` 改名（S1）。只收付过代价的知识；去留见「五」 |
| `docs/TESTING.md` | 重写 | 验证选择表、运行命令、假 FreshRSS 的用法、隔离开发实例与浏览器取证步骤、合成旧库的测试夹具；删 Cypress |
| `docs/CODE_PATTERNS.md` | 重写并缩短 | 只留本项目特有的约定：出站 HTTP 客户端、参数化 SQL、意图写入（不直接改 `server_read`）、改状态接口不收 GET、HTML 只经 `sanitizeArticleHtml` 进 `v-html`、CSS 变量与 `Icon` 组件。通用 Vue/Go 教程式内容删除。目标 300 行以内（现 1090 行） |
| `docs/SETTINGS.md` | 重写并缩短 | 生成器的用法、拒收未知键、加密字段、`llm_*` 键名；设置清单以 schema 为准，不再抄一份 |
| `docs/BUILD_REQUIREMENTS.md` | 更正 | Windows 不需要 CGO/mingw；Wails 钉版与升级重跑项链到 ADR；Linux/macOS 段落在平台范围未收窄前保留 |
| `docs/AI_CONFIGURATION.md`、`.zh.md` | 删除 | AI profile 删除；单一模型配置在设置面板和 SETTINGS.md 里足够 |
| `docs/api/swagger.json` | 删除（推荐，Q1） | 路由表测试取代；否则 S5 重新生成 |
| `skills/mrrss-assistant/`、`docs/SKILLS.md`、`.zh.md` | 删除（推荐，Q1） | 连同 `release.yml` 的 `package-skills` 任务、`Makefile` 的 `swagger*` 目标 |
| `frontend/cypress/README.md` | 删除 | 随 Cypress 删除 |
| `docs/adr/0001` | 保留，改 superseded | 状态改为 `superseded by ADR-0016`，正文不动 |
| `docs/adr/0002`–`0007` | 保留 | 原样有效。0003、0006 里提到的旧代码路径在 S1 后可能不存在：只在该 ADR 被重新引用时核对，不预先改 |
| `docs/adr/0008`–`0016` | 新增 | 见「四」 |
| `docs/out-of-scope/` | 保留并新增 | 见「四」末 |
| `README.md`、`README_zh.md` | 重写，合并 | S8 起只留中文 `README.md`：新项目身份、来源与 GPL-3.0、`keep_unreads` 用户须知、与旧版并存和切换步骤 |
| `CHANGELOG.md` | 重开 | S8 以新项目第一个版本起头；旧版历史留在 `legacy-final` |
| `CNAME` | 删除 | S8（roadmap「八」3） |
| `.forge/config.md` | 修改 | S1 改 human acceptance 一行；Doc layout 的偏离声明随改名更新（pitfall 回到约定路径后无需声明） |
| `scripts/README.md`、`build/README.md` | 保留 | 随 `verify.py`、构建配置的实际变化修改（S1 删 Cypress 与 Tailwind 后核对 lint 步骤） |
| `docs/roadmap.md` | 不新建 | Epic 与 tracker 已承担方向；Epic 结束后若有新方向再建 |
| `docs/specs/literss-rb0/` | 保留到 finish | canonical window 内是未落地部分的权威；forge:finish 时连同 `layout-prototype/` 删除 |

### AGENTS.md 的形态

五节，不超过约 80 行：

1. **项目边界**：新定位（桌面上的 FreshRSS 未读阅读器，动线一句话）；FreshRSS 是订阅与已读的权威，本地只推用户明确的意图；
   隐私（无统计、无外部字体与脚本、CSP）、凭据加密、参数化 SQL、不信任的 HTML 只经清洗器。ADR 0002 的「不做本地订阅」保持。
2. **重做进行中**（S1 加，finish 删）：见「一」。
3. **开发实例与取证**：Common Issues 7 的更正文本（见「六」）；浏览器取证通道入口统一写 `http://127.0.0.1:<端口>/`；
   本机 `HTTP_PROXY` 下 `curl` 要加 `--noproxy "*"`。
4. **按任务阅读**：保留现在的表格形式，指向 ARCHITECTURE、CODE_PATTERNS、TESTING、SETTINGS、BUILD_REQUIREMENTS、ADR、out-of-scope 与 pitfalls。
   原 Common Issues 1–5 这类「指路」条目并入这张表，不再单列。
5. **完成与授权**：现有内容基本保留（它与代码无关）。

Common Issues 编号列表取消：它现在只是 pitfall 的索引加几条指路。pitfall 索引放在 `docs/pitfalls.md` 开头，AGENTS.md 按任务表链过去。
`.forge/config.md` 与 TESTING.md 里「Common Issues 第 7 条」的引用改指「开发实例与取证」一节。

## 三、按步骤的更新计划

每一行的改动由该步骤的实现 Issue 在回写中完成；标 **spec** 的由 forge:spec 收尾归档完成。

| 步骤 | 删除 / 改写 | 新增 |
| --- | --- | --- |
| spec 收尾 | — | `CONTEXT.md`；ADR 0008–0015；out-of-scope 新增记录（见「四」） |
| S0 | 无 | 无（`legacy-final` 的用法在 S1 写进 AGENTS.md） |
| S1 | **AGENTS.md** 重写为上节形态（项目名仍用旧名，身份常量是占位）；**Common Issues 7** 更正；`docs/AGENT_PITFALLS.md` → `docs/pitfalls.md`，删除 pitfall 6、13、14、15、16、19、20、22；AI_CONFIGURATION 两份、Cypress README 删除；**ARCHITECTURE** 删到只剩留用模块与新骨架；**CODE_PATTERNS** 删前端、样式、i18n、Tailwind、旧 handler 与旧库模式；**TESTING** 删 Cypress 与前端测试模式，改 human acceptance 说明；**BUILD_REQUIREMENTS** 更正 CGO/mingw，写入钉定的 Wails 版本；`.forge/config.md` 的 human acceptance 一行；按推荐方案删除 swagger、`skills/mrrss-assistant`、SKILLS 两份、`package-skills` 任务、`swagger*` 目标，pitfall 18 退役 | ARCHITECTURE：身份常量包、开发隔离开关、浏览器取证通道、`protectLocalAPI` 的同源规则、CSP 下发点、`debug.log` 轮转；TESTING：隔离开发实例的启动与取证步骤；pitfall：WebView2 是否采用 CSP 头的实测结论（S1 验收结果）、Vite 代理只改写 5173 的 `Origin` |
| S2 | **SETTINGS** 重写（schema 收缩、拒收未知键）；pitfall 12 改写为新译文表的「已判定」语义（前端谓词部分留到 S6） | ARCHITECTURE：新库结构、`user_version` 迁移框架、列级权威、正文与全文分表；TESTING：迁移测试写法；CODE_PATTERNS：迁移只能追加版本；pitfall：`llm_*` 键名避开旧 `ai_*` 遗留键；凭据在 Windows 用 DPAPI（写进 ARCHITECTURE，不是 pitfall） |
| S3 | pitfall 8 里指向已删文件的路径（`internal/ai/client.go`、`handlers/update/`）按新位置改 | ARCHITECTURE：同步服务（意图、推送器、周期步骤、调度、长轮询、旧版探测）；TESTING：假 FreshRSS 的测试用法与开发工具；pitfall：FreshRSS 续页 `c` 包含上界且丢首条、`edit-tag` 对任何 `i` 都回 `OK`（原 pitfall 14 的知识）、`mark-all-as-read` 按抓取时间的 `ts` 截断、长轮询不得有短于它的 `WriteTimeout`、旧版互斥量名由已安装旧二进制决定 |
| S4 | — | ARCHITECTURE：旧库导入（一次性、只读、导入报告、推迟条件）；TESTING：合成旧库夹具；pitfall：最小化哨兵坐标（-32000、-21333） |
| S5 | pitfall 18 若保留（Q1 选 B），按新路由改写 | ARCHITECTURE：路由表与 API 约定（改状态只收非 GET、撤销令牌 60 秒）；CODE_PATTERNS：handler 模式；TESTING：路由表测试 |
| S6 | pitfall 12 的前端谓词部分、pitfall 21 的前端回落描述按新前端改写 | ARCHITECTURE：前端结构（store、三栏组件、快照与横幅）；CODE_PATTERNS：前端约定；TESTING：vitest 范围、清洗器载荷矩阵、浏览器通道截图取证 |
| S7 | — | ARCHITECTURE：壳（托盘、关闭到托盘、自启、窗口位置、焦点与唤醒触发同步、更新检查） |
| S8 | README 两份合并重写、CHANGELOG 重开、`CNAME` 删除；AGENTS.md 与 CLAUDE.md 的项目名；ADR 0001 改 superseded；BUILD_REQUIREMENTS 的安装包与发布说明 | ADR 0016（取代 0001）；README 的用户须知（`keep_unreads`、与旧版不可同时连同一账号） |
| S9 | 无 | 无（用户验收） |
| finish | 删除 AGENTS.md「重做进行中」一节；删除 `docs/specs/literss-rb0/` | forge:finish 的 living-doc 扫尾：核对每份文档与代码一致 |

**对 roadmap「七」的修订**：
- pitfall 6、16、19、20 与 Common Issues 6、19、20 在 **S1** 失效（S1 删了 AI profile 和旧库访问），不是 S2。
- pitfall 13、14 在 **S1** 失效（S1 删了旧同步），不是 S3；其中仍然成立的服务端行为在 S3 以新 pitfall 写回。
- Common Issues 3（`task dev`）：S1 删前端后需要重新核对热更新入口，由 S1 的 Issue 决定保留还是改写。
- `docs/api/swagger.json` 与 `skills/mrrss-assistant` 在 S1 就与代码矛盾（旧路由被删），按推荐方案在 S1 删除，而不是等到 S5。
- 新 ADR 在 spec 收尾时写（forge:living-docs「ADR 在设计收尾归档时写」），不是 S8；只有取代 ADR 0001 的那条等 mrrss-280.16。

## 四、ADR 清单

编号按写入顺序，下表是建议。所有条目都过了三条标准（难以撤回、没有上下文会让人意外、有真实的取舍）；`Source` 写 `mrrss-280`。

| 编号 | 标题（建议） | 来源 | 被否决的方案（要写进 ADR） |
| --- | --- | --- | --- |
| 0008 | 以 FreshRSS 条目 ID 为身份，本地只推用户意图，取消差异推送 | sync-model、roadmap R1、R2、R6 | 以 URL 为主键；拉取后与本地做差异推送（旧版回退已读的根源）；「以上/以下」换算成 `ts` 区间 |
| 0009 | 本地库是 90 天内 reading-list 的完整副本，按抓取时间清理 | read-retention、R5 | 只下未读；按已读时间（`read_at`）保留；可配置保留期 |
| 0010 | 同步状态走 HTTP 长轮询，前端不依赖 Wails 运行时 | sync-push-channel、R4 | Wails 事件；SSE；前端定时轮询 |
| 0011 | 桌面壳继续用 Wails v3，钉版且只为具体问题升级 | desktop-shell、R19 | Tauri / 纯浏览器 + 本地服务；跟最新 beta；退回 Wails v2（保留为后备） |
| 0012 | 开发实例隔离与浏览器取证通道是开发开关，不是部署形态 | dev-isolation、R17；与 ADR 0005 的边界 | 取证靠 server 模式（ADR 0005 已删）；放宽 `protectLocalAPI` 到 same-site；只换数据目录不换 UniqueID |
| 0013 | 前端重写：去掉 i18n、Tailwind 与 Wails 绑定，主题只跟随系统 | frontend、R18 | 原地精简旧前端；保留 i18n 以备多语言 |
| 0014 | 不信任的 HTML 在前端用 DOMPurify 清洗，外加 CSP；嵌入媒体改为外链 | html-sanitize、R14、R15 | 后端清洗；只靠 CSP；保留 iframe 白名单 |
| 0015 | 标题翻译只用百度，大模型只做摘要；旧库一次性只读导入 | Epic Rulings D1–D3、data-migration、R9、R13 | 标题走大模型；多 AI profile；原地升级旧库 |
| 0016 | 作为独立新项目发布，不再是 MrRSS 的 fork（取代 0001） | roadmap「二」、mrrss-280.16 | 沿用 fork 只改名；零历史新仓库 |

说明：
- 0015 把两件事合在一条是为了少一条 ADR；如果 forge:spec 觉得读者会分开查，拆成两条（「标题翻译只用百度」「旧库一次性只读导入」）。
- 0016 写明 roadmap「八」的两条事实：GitHub fork 父仓库已是 `DevXDojo/MrRSS`，本地 `upstream/main` 未 fetch 过，不代表 0001 的「落后」描述。
- **不写 ADR 的**：托盘与自启保留（桌面应用有托盘不令人意外，desktop-shell 的删除建议随 spec 删除而消失）；改状态接口不收 GET（安全约定，
  写进 CODE_PATTERNS 并由路由表测试守住）；Windows DPAPI（写进 ARCHITECTURE）；「新骨架逐块迁移」（过程决定，重做完成后没有读者会问）。
- **写进 `docs/out-of-scope/` 的**（删掉的功能，防止被重新提出；每份写决定和理由，一两段即可）：收藏与稍后读；全文翻译与 AI 标题翻译；
  界面多语言；本地筛选器与隐藏文章；多 AI profile 与用量统计；应用内播放器与嵌入媒体；按 Q1 结论，外部 agent 操作接口（mrrss-assistant）。
  时机同 ADR：spec 收尾。

## 五、pitfall 去留

| pitfall | 去留 | 时机 |
| --- | --- | --- |
| 6（AI 配置只在 `ai_profiles`） | 删除 | S1 |
| 8（出站走 `CreateHTTPClient`） | 保留，S3 修正文件路径 | S3 |
| 9、10（局域网绕过、系统代理缓存） | 原样保留 | — |
| 11（短文本按书写系统判语言） | 原样保留 | — |
| 12（译文非空即「已判定」） | 改写：后端部分 S2，前端谓词部分 S6 | S2、S6 |
| 13、14（同 URL 多条目、待推送优先） | 删除；仍成立的服务端行为（`edit-tag` 对任何 `i` 回 `OK`、永久失败要有上限）在 S3 以新条目写回 | S1 删，S3 写 |
| 15（打开的文章要钉住） | 删除；新前端用快照，天然不存在这个问题 | S1 |
| 16（订阅同步只写服务端列） | 删除；列级权威改由新库的表结构表达，写进 ARCHITECTURE | S1 |
| 17（行尾混用） | 保留；S1 删掉 `frontend/src` 后改写前端部分的实测数字 | S1 |
| 18（路由三份抄本） | Q1 选 A 时退役（S1）；选 B 时 S5 改写 | S1 或 S5 |
| 19、20（DROP COLUMN、正文列隔离） | 删除；新库版本化迁移、正文全文分表 | S1 |
| 21（readability 空正文） | 保留，S6 改前端回落描述 | S6 |
| 22（Cypress 设置夹具） | 删除 | S1 |
| 23 起 | 新增，见「三」各步骤 | S1–S4 |

## 六、Common Issues 7 的更正

S1 落地开发隔离开关的同一个提交里，写进 AGENTS.md「开发实例与取证」一节（按 dev-isolation 的建议，措辞对齐落地后的开关名）：

> 已安装版本的单实例 ID（`com.mrrss.app`，以及 S8 之后新名字的正式 ID）在 agent 会话里一律不得用于启动：用户实例在运行时，新进程只会通知它
> 然后在 `Starting Wails v3...` 后退出；用户实例没运行时，它会打开用户的真实数据并截断其日志。agent 只启动开发构建：独立 UniqueID、
> portable 数据目录、独立 WebView2 用户数据目录、非 1234 端口（默认 1235），只连假 FreshRSS。这样的实例在 agent 会话里能正常启动，
> UI 通过浏览器取证通道 `http://127.0.0.1:1235/` 驱动。原生窗口观感、托盘、自启与真实账号行为仍由用户在自建构建上验收，自动化检查不能代替。

同时改：`.forge/config.md` 的 `human acceptance` 一行（改为「原生窗口观感、托盘、自启、真实账号行为由用户验收；UI 行为由 agent 在隔离实例 +
假 FreshRSS 的浏览器通道里取证」）；TESTING.md 引用 Common Issues 7 的句子。S1 之前原文不动：旧代码没有隔离开关，原文对旧代码仍然正确，
而 dev-isolation 结论已经在 spec 里。

## 七、mrrss-assistant 与 Swagger

现状：`skills/mrrss-assistant` 让外部 agent 通过本地 API 操作用户数据，随 release 打包为 `MrRSS-<版本>-skills.zip`；它的
`references/api.md` 从 `docs/api/swagger.json` 生成，而 swagger 又来自 handler 上的 `@Router` 注释。pitfall 18 记录的就是这三份抄本互相漂移
（2026-08-28 审计出 46 处错误）。

重做后的情况：
- R10 的路由清单完全按新前端的需要划定，没有为外部 agent 设计的接口；Skill 现有的收藏、稍后读、AI profile、筛选等操作全部没有对应路由。
- 用户动线里没有「让 agent 操作阅读器」这一步（Epic Rulings：留下的功能要能对上动线中的一步）。
- 开发取证用 `curl` 和浏览器通道，不需要这个 Skill；`protectLocalAPI` 对不带 `Origin` 的请求放行，删掉 Skill 不影响取证。
- 新 API 约二十个路由，只有一个消费者（自己的前端）。路由表测试（S5 已计划）能守住「改状态只收非 GET」，前端调用集中在一个 API 模块里，
  不需要第二份抄本。

推荐：S1 随旧路由一起删除 Skill、SKILLS 两份、swagger、`swag` 注释（随旧 handler 删除）、`Makefile` 的 `swagger*` 目标和
`release.yml` 的 `package-skills` 任务；pitfall 18 退役；外部 agent 接口写进 out-of-scope。以后要恢复，从 `legacy-final` 取回再按新路由重写。
这是删除一项已发布的功能，所以作为 Q1 交用户决定。

## 八、需要用户决定

### Q1 · mrrss-assistant 与 Swagger 的去留

`skills/mrrss-assistant` 让 Codex 之类的外部 agent 通过本地 API 查看和操作你的 MrRSS 数据，随每次发布打包；swagger 是它的接口文档来源。
重做后的 API 只为新前端设计，Skill 的大部分操作已无对应接口。

- **A · 删除 Skill 与 Swagger** — S1 随旧路由一起删，发布不再附带 skills 包，pitfall 18 退役，维护成本归零；代价是失去「让 agent 操作阅读器」的能力，
  以后要用需按新路由重写。
- **B · 保留，S5 按新路由重写** — Skill 与 swagger 在 S1 删除旧内容、S5 重新生成，保留三份抄本的核对义务（pitfall 18）；
  需要额外定义哪些接口对外部 agent 开放。

➡️ **Recommend A**：用户动线里没有这一步，新路由也不是为外部 agent 设计的；你若平时确实在用 Codex 操作 MrRSS，就选 B。

## 九、新发现（未入图）

1. **roadmap「七」把多条失效放晚了一步**：S1 删了 AI profile、旧同步和旧库访问，pitfall 6、13、14、16、19、20 与 swagger、Skill 都在 S1 就与代码矛盾。
   已在「三」末尾修订，forge:slice 以本文为准。
2. **`.forge/config.md` 的 human acceptance 一行在 S1 后不再成立**（它说桌面应用无法在 agent 会话中启动），而它每个会话都会注入；
   roadmap 的 S1 清单没有列它。已放进「三」S1 行。
3. **平台范围没有明确结论**：R13 写「其他平台沿用现有方案」，但没有节点决定新版是否仍发布 Linux/macOS；BUILD_REQUIREMENTS 的 Linux/macOS
   段落和 CI 的平台矩阵依赖这个答案。建议 forge:spec 写明（若只做 Windows，属于「我们不做」，应入 ADR 或 out-of-scope）。
4. **ADR 写入时机与 roadmap 不一致**：roadmap S8 写「新增 ADR」，而 forge:living-docs 要求设计收尾归档时写；本文按后者，只有取代 0001 的那条在 S8。

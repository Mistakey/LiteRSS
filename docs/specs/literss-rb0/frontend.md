# 前端：重写还是精简（mrrss-280.7）

> Epic mrrss-280「重做并精简 MrRSS」的规划节点产物。只给推荐，不含目标代码。
> 输入：壳选型（`desktop-shell.md`，mrrss-280.1）、同步模型（`sync-model.md`，mrrss-280.2）、功能与设置清单
> （`feature-inventory.md`，mrrss-280.3）及用户裁决 mrrss-280.4、布局原型（`layout-prototype/`）及评审结论
> mrrss-280.6、推送通道（`sync-push-channel.md`，mrrss-280.11）、开发隔离与浏览器通道（`dev-isolation.md`，mrrss-280.9）。
> 代码统计基于 `main` @ `bbf61aa1` 的 `frontend/src`，不含测试文件；行数为 `wc -l`。

## 结论

1. **按选定布局重写前端，不在原地精简。** 新前端从空骨架起步，界面结构、样式和数据层照原型变体 B（按 mrrss-280.6 修改）
   重新搭建；旧代码中可复用的模块按本文「复用清单」逐个搬入，其余随旧前端整体删除。不保留新旧界面并存的过渡期。
2. **技术栈基本不换：Vue 3 + TypeScript + Vite + Vitest，Pinia 保留。** 去掉 Tailwind、vue-i18n、`@wailsio/runtime`、
   Cypress 和 `@phosphor-icons/vue`；样式改用原型的 CSS 变量加组件内 scoped CSS。
3. **i18n 基建整体拆掉。** 界面固定中文，文案直接写在模板里，不保留键表或 `t()` 包装。后端两处读 `language` 设置的地方
   （摘要输出语言、托盘菜单文案）改为固定中文，随 `language` 设置项一起删除。
4. **新前端是纯 `/api` 客户端**：不 import 任何 Wails 包，不向外网请求任何资源，在 WebView 和浏览器取证通道里走同一套代码路径。

## 依据：现有组件与选定布局的差距

把 `frontend/src` 的 137 个非测试文件（27524 行）逐个对照 mrrss-280.4 的裁决和 mrrss-280.6 的布局，分为四类：

| 类别 | 含义 | 文件 | 行数 | 占比 |
| --- | --- | --- | --- | --- |
| 删除 | 对应功能已被裁决删除 | 59 | 10529 | 38.3% |
| 重写 | 功能保留，但结构与新布局或新数据模型不兼容，改不如重写 | 15 | 7246 | 26.3% |
| 改写沿用 | 逻辑可留，但要去掉 i18n、Tailwind 或被删功能的分支 | 49 | 8574 | 31.2% |
| 原样复用 | 纯函数或生成代码，无界面、无 i18n 依赖 | 14 | 1175 | 4.3% |

### 动线核心全部落在「重写」

「重写」一类正是动线①–⑤经过的每一层：

| 文件 | 行数 | 为什么不能改 |
| --- | --- | --- |
| `stores/app.ts` | 732 | 分页 `page++` 加 pin 机制加 `pollProgress`/5 秒轮询，`Filter` 含收藏、稍后读；同步模型改为「有序 ID 快照 + 按 ID 取卡片」，主键换成 FreshRSS 条目 ID，推送改为长轮询 `GET /api/sync/state`（sync-model「重写范围」已把前端 store 的列表与同步部分列为重写） |
| `ArticleList.vue` | 1026 | 卡片模式、卡片弹窗详情、筛选器、清空稍后读、悬停已读、底部「全部已读」占了大半；新列表要的是快照、「N 篇新文章」横幅和右键「以上 / 以下已读」 |
| `ArticleItem.vue` | 508 | 新行样式是变体 C 的宽松行：两行标题 + 一行中文摘录 + 右侧缩略图 |
| `ArticleContent.vue` + `.css` | 1233 + 1376 | 与全文翻译深度交织（不区分大小写的 `translat` 在 `.vue` 中出现 230 次，`.css` 中 122 次），还挂着浮动目录和音视频播放器；D2/U4 改为点开即抓全文 |
| `ArticleDetail.vue`、`ArticleToolbar.vue`、`useArticleDetail.ts` | 163 + 173 + 667 | 新详情无工具栏、无上下篇、无关闭按钮，操作在底部浮动条；`useArticleDetail` 里上下篇、收藏、稍后读、复制/下载图片都已删除 |
| `Sidebar.vue` | 355 | 新侧栏顶部是「未读 / 全部」分段切换，底部是同步状态与设置；ActivityBar 取消 |
| `App.vue`、`main.ts`、`style.css` | 350 + 65 + 235 | 三栏骨架、主题（改为只跟随系统）、界面字体检测、i18n 初始化都要换 |
| `Toast.vue`、`useNotifications.ts` | 116 + 141 | 新的撤销 snackbar：左下角、约 8 秒、悬停不消失、带「撤销」动作，所有批量已读共用 |
| `useResizablePanels.ts` | 106 | 原型三栏没有拖动调整，是否保留由实现 Issue 按布局定 |

原型本身只有 `app.js` 562 行加 `style.css` 263 行，就实现了三栏、列表快照、右键菜单、撤销、详情浮动条和图片查看器的全部交互
（数据是假的）。新界面的规模与它相当，而旧代码中动线核心就有约 6500 行，其中大部分服务于已删功能。

### 「改写沿用」也几乎每个文件都要动

- **i18n**：旧前端 70 个文件引用 i18n，`t('…')` 调用约 634 处。「改写沿用」的 49 个文件里有 26 个用 i18n（315 处调用），
  「重写」类另有 65 处，「删除」类 251 处；`locales/en.ts` 664 行、`zh.ts` 650 行。
- **Tailwind**：「改写沿用」类有 25 个文件的模板用 Tailwind 工具类，`select.css` 有 39 处 `@apply`。
- 组合起来：「改写沿用」的文件没有一个能原样搬过去；它们省下的是**逻辑和结构**，不是代码行。

### 为什么不原地精简

- 要删的 38% 和要重写的 26% 相加近三分之二，而且集中在动线主干；原地精简每一步都会经过「旧 store + 新 API」的中间态。
  同步模型、单一 AI 配置和删除路由会同时改掉后端接口，这些中间态本来就跑不起来，也没有用户会用。
- 旧 store 的 pin 机制、三套轮询和 `mergePinnedArticles` 是为旧同步模型打的补丁（pitfall 15）；在新模型下它们要整段删除，
  精简等于重写 store。
- Epic 的 Rulings 要求「从用户动线出发，不从现有代码打补丁」「旧代码不再投入」，而且重做后改名、作为新项目发布，
  没有必须保持可用的旧界面。

## 技术栈

| 项 | 推荐 | 理由 |
| --- | --- | --- |
| Vue 3 + TypeScript + Vite | 保留 | 「改写沿用」和「原样复用」合计约 9700 行是 Vue SFC 和 TS，换框架就全部作废；换来的只是写法差异，动线和规模都不需要。 |
| Pinia | 保留 | 新 store 只剩列表快照、当前选中、视图（未读 / 全部）与范围、同步状态几块，Pinia 成本很低；现有 `app.test.ts` 的测试写法（`createPinia` + mock fetch）可以直接沿用到新 store。 |
| Tailwind | 删除 | 原型已被评审接受，它的样式就是约 260 行带 CSS 变量的普通 CSS，并用 `prefers-color-scheme` 处理深色模式。把原型样式直接搬成全局变量 + scoped CSS，比翻译成工具类更不容易走样。设置页没有原型，按同一套变量重做样式。 |
| vue-i18n | 删除 | 见下文。 |
| `@phosphor-icons/vue` | 删除 | 原型用的是约 15 个内联 SVG 路径，评审看到的就是它们；做成一个 `Icon` 组件即可。 |
| katex、highlight.js | 保留 | 正文里的公式与代码渲染（`useArticleRendering.ts`），行为不变。 |
| `@wailsio/runtime` | 删除 | 唯一调用是剪贴板（`utils/clipboard.ts`），复制按钮已全部删除，这个依赖随之消失，不需要改成 `navigator.clipboard`。 |
| Cypress | 删除 | 见「测试」。 |

## 复用清单

### 原样复用（14 个文件，1175 行）

`composables/article/fetchOutcome.ts`、`titleTranslationRequest.ts`、`useArticleRendering.ts`，`utils/lazyImages.ts`、`browser.ts`，
`composables/core/useSettings.generated.ts`、`types/settings.generated.ts`（重新生成即可），`useSettingsValidation.ts`，
`composables/ui/useModalClose.ts`、`useContextMenu.ts`，`types/context-menu.ts`、`select.ts`，`config/defaults.ts`。
其中 `fetchOutcome.ts` 返回的是 i18n 键，要改成直接返回中文原因文案，改动只有几行。

### 改写沿用（49 个文件，8574 行）

- **设置页**：`SettingsModal`、`FreshRSSSettings`、`ProxySettings`/`NetworkTab`、`AboutTab`、`GeneralTab`/`ApplicationSettings`
  （只剩托盘、关闭到托盘、开机自启，mrrss-280.4 推翻了删除建议）、`UpdateSettings`、`UpdateAvailableDialog` + `useAppUpdates`
  （去掉下载与安装，只跳发布页）；`AIProfileModal` 收敛为单一配置表单（端点 / 密钥 / 模型 / 测试连接），`TranslationSettings`
  只剩百度凭据。表单基础件 `settings/base/*` 中保留的 10 个、`useSettings`、`useSettingsAutoSave` 一并沿用。
- **侧栏树**：`FeedList`、`SidebarCategory`、`SidebarFeed`、`useSidebar`（U8 保留树，右键「全部标为已读」含子分类，去掉重命名、
  同步此源、打开网站）。
- **详情零件**：`ArticleTitle`（译文 + 原文）、`ArticleBody`、`ArticleSummary`（去掉复制），`useArticleSummary`（U1 按钮触发；
  D2 直接用已抓或正在抓的全文）、`useArticleTranslation`（只处理标题）、`useArticleActions`（右键菜单裁到保留项）。
- **通用件**：`BaseModal`、`BaseSelect` + `useSelect`、`ContextMenu`、`ModalFooter`、`ImageViewer`（按 mrrss-280.6 改为以指针为
  中心缩放、双击切换适应/原始、拖动平移、多图左右切换，去掉复制与下载）。
- **类型与工具**：`types/models.ts`（主键改为 FreshRSS 条目 ID）、`global.d.ts`、`settings.ts`，`utils/date.ts`（去掉 `t` 参数）、
  `imageCache.ts`（列表缩略图）。
- `ArticleContent.css` 虽归入「重写」，其中 `.prose` 的代码块、表格和 highlight.js 深色配色可以挑出来作新正文样式的底子；
  字号、行高、行宽和字体按 mrrss-280.6 写死为原型值（17px、1.8、700px、标题 26px，系统无衬线）。

### stores

`stores/app.ts` 不复用，按 sync-model 重写。可以沿用的是它的**测试意图**：`app.test.ts` 里「正在阅读的文章不因后台刷新消失」
「同步后刷新不丢页」两条，在新模型下对应「快照在视图内稳定」「新到条目只进横幅、点击才载入」，应作为新 store 的首批测试。

### 测试

| 现有测试 | 行数 | 去向 |
| --- | --- | --- |
| `fetchOutcome`、`titleTranslationRequest`、`useArticleRendering`、`lazyImages` 的单测 | 247 | 随被测模块原样搬过去 |
| `stores/app.test.ts` | 122 | 换成新 store 的测试，保留测试意图（见上） |
| `App.test.ts`、`ArticleContent.test.ts`、`TranslationSettings.test.ts` | 326 | 删除：测的是界面字体、全文重载、翻译提供方切换，均已删除或重写 |
| `floatingToc`、`translationMode`、`useContentTranslation`、`bilibili`、`youtube` 的单测 | 817 | 删除，被测功能已删除 |
| `test/mocks/wails.ts` | 22 | 删除，前端不再依赖 Wails |
| Cypress：11 个 e2e 规格（约 2050 行）、fixtures、plugins、support | — | 整体删除。它们测的是自动更新、主题与语言切换、设置持久化等已删功能，还依赖手工维护的设置 fixture（pitfall 22）；界面级验证改由浏览器取证通道承担 |

vitest 单测总计约 1510 行，可直接复用的约 250 行（16%）。

### i18n

整体拆掉 `src/i18n/`（1340 行）和 `vue-i18n` 依赖，文案直接写中文。理由：

- 只有一个中文用户，`language` 设置已删除（feature-inventory「设置项」），没有第二种语言要切换。
- 反正所有界面文件都要重写或改写，保留键表只会让每处文案多一次间接查找，而且约四成 `t()` 调用（251 处）本就在要删除的文件里。
- 保留 i18n 的唯一收益是将来加语言。按 Epic 的「默认删」原则，这种假设性需求不留基建；真需要时再引入，成本与今天保留相当。

后端随之固定中文：`internal/handlers/summary/summary_handlers.go` 读 `language` 决定摘要输出语言，`main.go` 托盘菜单按
`language` 选中英文文案；两处都改为中文常量，与 U3（摘要提示词写死）一并处理。

## 纯 `/api` 客户端与浏览器取证通道的要求

新前端要同时在 WebView（源 `http://wails.localhost`）和浏览器取证通道（`http://127.0.0.1:<开发端口>/`）里跑同一份构建。
为此规格应写入以下约束：

- **不依赖壳**：不 import `@wailsio/runtime` 或 `frontend/bindings`；外部链接一律走 `POST /api/browser/open`（`utils/browser.ts`
  已经如此）。同步状态用长轮询 `GET /api/sync/state?since=<rev>`，不用 Wails 事件或 SSE（sync-push-channel.md）。
- **只用相对路径 `/api/...`**，不写死主机和端口，这样两个源下都成立。
- **不加载任何外部资源**。现在的 `index.html` 会向 `fonts.googleapis.com`/`fonts.gstatic.com` 取 Inter 字体，并从
  `unpkg.com` 加载 `@phosphor-icons/web` 脚本。这与「隐私优先、无分析」相悖，在离线时拖慢首屏，还让浏览器通道的取证结果受外网影响。
  新前端字体用系统字体（mrrss-280.6 已定），图标内联，两者都删除。这回答了 dev-isolation.md 末尾关于 `preconnect` 的疑问。
- **主题只跟随系统**：用 `prefers-color-scheme`，删除 `index.html` 里读 `localStorage.themePreference` 的内联脚本和 `.dark-mode` 类。
- **写操作都是同源 `fetch`**：依赖 dev-isolation.md 第 5 条对 `protectLocalAPI` 的修改（同源严格相等放行）。前端自身不需要
  任何 token 或特殊头。
- **开发时的热更新**：`vite.config.js` 目前没有 `/api` 代理。若要在浏览器里用 Vite 开发服务器（`http://127.0.0.1:5173`）
  配合开发实例，代理会把 5173 的 `Origin` 原样转发，被严格同源的防护拒绝。可选做法：代理里去掉 `Origin` 头，或只用
  「构建后由开发实例托管」的方式取证。由浏览器通道的实现 Issue 决定。

## 重写路线建议

规格与切片时参考，不在本节点定死：

1. 新骨架：`index.html`（无外部资源）、`main.ts`、全局 CSS 变量（取自原型 `style.css`）、`Icon`、三栏布局壳；删除旧的
   `frontend/src`、Cypress、Tailwind 与 i18n 依赖。
2. 数据层：新 store（视图、范围、ID 快照、按 ID 取卡片、选中、长轮询同步状态），先写测试。依赖后端的快照与长轮询端点。
3. 侧栏：分段切换、分类/源树（沿用 `FeedList` 等）、底部同步状态与设置入口、右键「全部标为已读」+ 撤销 snackbar。
4. 列表：宽松行、新文章横幅、右键「此篇及以上 / 以下标为已读」、标题译文懒加载（沿用 `titleTranslationRequest`）。
5. 详情：标题块、正文渲染（沿用 `useArticleRendering`、`lazyImages`）、点开即抓全文与失败提示（沿用 `fetchOutcome`）、
   摘要、底部浮动条、图片查看器。
6. 设置页：FreshRSS、AI 单一配置、百度凭据、代理、托盘与自启、更新提示、关于。

每一步都能在浏览器取证通道里由 agent 截图核对；原生窗口观感仍按 Epic Rulings 由用户在自建构建上验收。

## 规格需要接住的事项

- 前端整体重写与技术栈取舍（本文结论 1、2），以及删除的依赖清单：`tailwindcss`、`@tailwindcss/postcss`、`postcss`、
  `autoprefixer`、`vue-i18n`、`@phosphor-icons/vue`、`@wailsio/runtime`、`cypress`、`@cypress/vue`、`@cypress/vite-dev-server`。
- `language` 设置删除后，后端摘要语言与托盘菜单文案固定为中文。
- 新前端「不加载外部资源」写进约束；`frontend/bindings/`（Wails 生成的绑定目录，前端未引用）一并删除。
- 设置生成器（`tools/settings-generator`）继续产出 `settings.generated.ts` 与 `useSettings.generated.ts`，schema 收缩后重新生成。
- `docs/TESTING.md`、`docs/CODE_PATTERNS.md` 里关于 Cypress、i18n、Tailwind 的段落随文档重写一并更新（Epic Rulings「文档并入重做」）。

## 新发现（建议入图，本节点不建）

- **文章 HTML 未经清洗就进 `v-html`。** `GET /api/articles/content` 直接返回 RSS 正文或 readability 抽出的全文
  （`internal/handlers/article/article_content.go`、`internal/fulltext/extract.go`），`textutil.SanitizeHTML` 只用于
  Markdown 转换；前端 `ArticleBody.vue` 用 `v-html` 渲染，也没有 CSP。正文来自不受信的网页，新前端在浏览器通道里与 `/api`
  同源、写操作被放行，一段带 `on*` 属性的 HTML 就能调用写接口。需要核实 FreshRSS 与 go-readability 实际留下了什么，并决定
  在后端清洗还是前端清洗，外加 CSP。
- **撤销 snackbar 的后端语义**：「全部标为已读」直接执行、8 秒内可撤销。撤销是前端回写一组「未读」意图，还是后端提供按批次
  撤销，需要和 sync-model 的 `pending_state`/`pending_mark_all` 对齐。
- **浏览器通道下的 Vite 开发代理与严格同源防护冲突**（见上文「开发时的热更新」），归入浏览器取证通道的 Issue。

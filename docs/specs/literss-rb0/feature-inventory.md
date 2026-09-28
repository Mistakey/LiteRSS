# 功能与设置清单草案（mrrss-280.3）

供 mrrss-280.4 拍板。依据是 Epic mrrss-280 的 `## Rulings`，按以下几条判断：

- **动线**：① 未读列表 → ② 扫中文标题 → ③ 点开（中文读正文 / 英文看 AI 摘要）→ ④ 值得细读或剪藏的去浏览器 → ⑤ 以上已读。
- **D1**：英文标题翻译保留，列表只显示中文译文，原文放在详情页。
- **D2**：点「摘要」时若正文被截断，先自动抓全文，全文替换正文显示；抓取失败时，正文够长就基于 RSS 摘要生成并注明原因，太短就不摘要，显示原因和「在浏览器打开」。
- **D3**：只有一套大模型配置（用于摘要），标题翻译只用百度。
- **习惯**：全程用鼠标；收藏和稍后读几乎不用；细读、全文翻译、剪藏都在浏览器里完成。
- 按钮和设置**默认删除**，留下的每一项都要对应动线中的某一步。

建议取值说明：

- **保留**：新版照留。
- **删除**：新版不提供；如果表格写了「固定为……」，就是去掉开关、把行为写死。
- **合并**：并入另一项或交给兄弟节点决定（会注明去向）。
- **不确定**：见文末《不确定项》，每项附有倾向。

## 汇总

| 范围 | 条目 | 保留 | 删除 | 合并 | 不确定 |
| --- | --- | --- | --- | --- | --- |
| 设置项（settings_schema.json） | 58 | 18 | 25 | 5 | 10 |
| 按钮与菜单项（详情工具栏、摘要区、右键菜单、侧栏） | 28 | 7 | 18 | 1 | 2 |
| 前端组件（frontend/src/components，78 个文件） | 78 | 31 | 29 | 7 | 11 |
| composables（30 个文件） | 30 | 15 | 9 | 4 | 2 |
| 后端（internal/handlers，按路由与文件） | 45 | 15 | 21 | 4 | 5 |

最关键的几条：

1. **翻译只剩百度凭据**：translation_enabled、mode、only_mode、target_language、provider 五个开关全部删除，行为固定为「列表显示中文译文，详情页显示原文」（D1）；全文翻译 `translate-text` 和 `useContentTranslation` 一并删除（全文翻译在浏览器里做）。
2. **AI 只剩一套配置**：AI profile 列表、选择器、自定义请求头、`/api/ai/profiles*` 和翻译提示词全部删除，改为单一的「端点 / 密钥 / 模型」配置外加一个「测试连接」（D3）。
3. **收藏、稍后读、隐藏、筛选器、快捷键整体删除**，侧栏的「全部 / 收藏 / 稍后读」视图也删除，只保留未读视图（依据：用户习惯、全程鼠标）。
4. **全文抓取不再设开关**：full_text_fetch_enabled 删除，抓全文只作为 D2 摘要流程里自动执行的一步。中文截断正文要不要也自动抓全文，列为不确定项 U4。
5. **阅读增强件整体删除**：浮动目录、页内查找、图片查看器、复制链接、复制摘要、重新加载、「在应用内以渲染模式查看」都删除（细读在浏览器里做）。
6. **壳与同步跟随兄弟节点**：托盘、开机自启、关闭到托盘删除（壳选型 mrrss-280.1）；窗口位置由壳在 Go 侧记忆，前端不再参与；同步调度全部放到后端，前端轮询删除，同步状态改由后端推送（同步审计 mrrss-280.2）。

和兄弟节点结论相关的行，理由里注明了来源：「壳选型」指 docs/specs/literss-rb0/desktop-shell.md（mrrss-280.1），「同步审计」指 docs/specs/literss-rb0/sync-model.md（mrrss-280.2）。

---

## 一、设置项（58）

### 通用与桌面壳

| 键 | 默认 | 建议 | 理由 |
| --- | --- | --- | --- |
| `update_check_enabled` | true | 不确定 | 打包与发布在 Epic 里仍是 Not yet specified；壳选型建议最多保留「有新版本时提示并跳到发布页」（壳选型 mrrss-280.1）（U10）。 |
| `language` | en-US | 删除 | 只有一个中文用户，界面固定为中文；i18n 基建是否一起拆掉由重做路线决定。 |
| `theme` | auto | 删除 | 固定为跟随系统，动线里没有需要手动切换主题的一步。 |
| `startup_on_boot` | false | 删除 | 用户要读的时候才会打开应用（壳选型 mrrss-280.1）。 |
| `close_to_tray` | true | 删除 | 托盘整体删除，关窗口就退出进程；动线不依赖后台常驻（壳选型 mrrss-280.1）。 |
| `shortcuts` | "" | 删除 | 用户全程用鼠标。 |
| `shortcuts_enabled` | true | 删除 | 同上。 |

### 列表与阅读行为

| 键 | 默认 | 建议 | 理由 |
| --- | --- | --- | --- |
| `default_view_mode` | rendered | 删除 | 第③步一律在应用内打开，去浏览器是第④步的单独动作，不需要设成默认行为。各源的 `article_view_mode` 一并删除。 |
| `show_hidden_articles` | false | 删除 | 「隐藏」功能已删除，不想读的文章按第⑤步标为已读即可。 |
| `hover_mark_as_read` | false | 不确定 | 可以替代或补充「以上已读」，是一种清列表的方式（U5）。 |
| `show_article_preview_images` | true | 合并→界面布局节点 | 第②步是扫标题；列表带不带图由布局决定，不再做成开关。 |
| `show_floating_toc` | false | 删除 | 需要细读的文章去浏览器（第④步）。 |
| `full_text_fetch_enabled` | true | 删除 | D2 规定抓全文是摘要流程里自动执行的一步，不设开关。 |
| `auto_show_all_content` | false | 不确定 | 中文截断正文要不要在点开时自动抓全文（U4）。各源的 `auto_expand_content` 随之决定。 |
| `layout_mode` | normal | 合并→界面布局节点 | 只保留一种布局，由布局节点决定；卡片和紧凑模式删除。 |
| `feed_drawer_pinned` | true | 合并→界面布局节点 | 侧栏形态由布局决定（U8）。 |
| `feed_drawer_expanded` | true | 合并→界面布局节点 | 同上。 |

### 排版

| 键 | 默认 | 建议 | 理由 |
| --- | --- | --- | --- |
| `content_font_family` | system | 不确定 | Epic 已把正文排版列为 Not yet specified（U9）。 |
| `content_font_size` | 16 | 不确定 | 同上；倾向只留这一项。 |
| `content_line_height` | 1.6 | 不确定 | 同上。 |
| `ui_font_family` | system | 删除 | 界面字体固定为系统字体，与阅读动线无关。 |
| `ui_font_size` | 16 | 删除 | 同上。 |

### 翻译（D1、D3）

| 键 | 默认 | 建议 | 理由 |
| --- | --- | --- | --- |
| `translation_enabled` | false | 删除 | 固定开启。第②步要求列表显示中文标题（D1）。 |
| `translation_mode` | title_only | 删除 | 固定为只翻译标题；全文翻译在浏览器里做（D1、用户习惯）。 |
| `translation_only_mode` | false | 删除 | 固定为列表只显示译文、详情页显示原文（D1）。 |
| `target_language` | zh | 删除 | 固定为中文。 |
| `translation_provider` | baidu | 删除 | 标题翻译只用百度（D3）。 |
| `baidu_app_id` | "" | 保留 | 百度翻译的必要凭据（D3）。 |
| `baidu_secret_key` | "" | 保留 | 同上，继续加密存储。 |

### AI 与摘要（D2、D3）

| 键 | 默认 | 建议 | 理由 |
| --- | --- | --- | --- |
| `ai_translation_prompt` | 内置 | 删除 | AI 翻译分支已删除（D3）。 |
| `ai_summary_prompt` | 内置 | 不确定 | 可以写死内置提示词，也可以留给用户编辑（U3）。 |
| `ai_usage_tokens` | 0 | 不确定 | 用量统计（U2）。 |
| `ai_usage_limit` | 20000 | 不确定 | 用量上限；如果 U1 选自动摘要，这是控制成本的护栏（U2）。 |
| `ai_translation_profile_id` | "" | 删除 | 只保留一套大模型配置（D3）。 |
| `ai_summary_profile_id` | "" | 合并→单一大模型配置 | D3 要求只有一套配置；改为「端点 / 密钥 / 模型」三项新设置（密钥加密），取代 `ai_profiles` 表。 |
| `summary_enabled` | true | 删除 | 第③步英文文章就是靠摘要来读，这个开关没有意义。 |
| `summary_length` | medium | 删除 | 固定一种长度，写进提示词。 |
| `summary_trigger_mode` | manual | 不确定 | 英文文章点开时自动摘要，还是点「摘要」按钮再生成（U1）。 |

### 网络

| 键 | 默认 | 建议 | 理由 |
| --- | --- | --- | --- |
| `proxy_mode` | system | 保留 | 同步和抓全文都要走代理；三态、默认跟随系统（ADR 0004）。 |
| `proxy_type` | http | 保留 | 同上。 |
| `proxy_host` | 127.0.0.1 | 保留 | 同上。 |
| `proxy_port` | 7890 | 保留 | 同上。 |
| `proxy_username` | "" | 保留 | 同上，继续加密存储。 |
| `proxy_password` | "" | 保留 | 同上，继续加密存储。 |
| `retry_timeout_seconds` | 60 | 删除 | 固定为一个超时值，动线里没有需要用户调它的一步。 |

### FreshRSS

| 键 | 默认 | 建议 | 理由 |
| --- | --- | --- | --- |
| `freshrss_enabled` | false | 删除 | 应用定位为纯 FreshRSS 客户端，恒为开启（ADR 0002）。 |
| `freshrss_server_url` | "" | 保留 | 第①步的未读列表来自 FreshRSS。 |
| `freshrss_username` | "" | 保留 | 同上。 |
| `freshrss_api_password` | "" | 保留 | 同上，继续加密存储。 |
| `freshrss_auto_sync_interval` | 30 | 保留 | 后端调度「每 N 分钟同步一次」的 N，让未读列表保持新鲜；前端不再按它轮询（同步审计 mrrss-280.2）。 |
| `freshrss_sync_on_startup` | true | 删除 | 由后端固定在启动时同步，另外在唤醒或窗口重新获得焦点时补一次（同步审计 mrrss-280.2）。审计原文把这个键写成调度条件，这里建议改为固定行为，不再提供开关。 |
| `freshrss_last_sync_time` | "" | 保留 | 内部状态，用于增量同步。 |

### 存储与内部状态

| 键 | 默认 | 建议 | 理由 |
| --- | --- | --- | --- |
| `max_cache_size_mb` | 500 | 删除 | 固定一个上限，自动清理；动线里没有需要管理缓存的一步。 |
| `window_x` | 0 | 保留 | 内部状态，壳的四件事之一「记住窗口位置」（壳选型 mrrss-280.1）。 |
| `window_y` | 0 | 保留 | 同上。 |
| `window_width` | 1024 | 保留 | 同上。 |
| `window_height` | 768 | 保留 | 同上。 |
| `window_maximized` | false | 保留 | 同上。 |

---

## 二、按钮与菜单项

### 详情页工具栏（ArticleToolbar）

| 按钮 | 建议 | 理由 |
| --- | --- | --- |
| 关闭 / 返回 | 保留 | 看完一篇回到第①步。 |
| 显示 / 隐藏译文 | 删除 | D1 固定为列表显示译文、详情页显示原文，不需要切换。 |
| 标为已读 / 未读 | 保留 | 第⑤步；误读后可以改回未读，这是唯一的撤销手段。 |
| 收藏 | 删除 | 用户几乎不用收藏。 |
| 稍后读 | 删除 | 同上。 |
| 在浏览器打开 | 保留 | 第④步。 |
| 复制链接 | 删除 | 剪藏在浏览器里做（第④步）。 |
| 重新加载正文 | 删除 | 重试由 D2 的失败提示承担，其余情况直接「在浏览器打开」。 |
| 摘要 | 保留（形态见 U1） | 第③步英文文章的入口（D2）。 |
| 复制摘要（ArticleSummary 内） | 删除 | 剪藏在浏览器里做；删掉之后，前端不再需要任何剪贴板调用，壳选型提到的 `navigator.clipboard` 替换就只剩兜底意义（壳选型 mrrss-280.1）。 |

### 文章右键菜单（useArticleActions）

| 菜单项 | 建议 | 理由 |
| --- | --- | --- |
| 标为已读 / 未读 | 保留 | 第⑤步。 |
| 标记以上为已读 | 保留 | 第⑤步「以上已读」：扫完一段后清掉，鼠标就能完成。 |
| 标记以下为已读 | 删除 | 动线是从上往下扫，只需要「以上」。 |
| 收藏、稍后读 | 删除 | 用户习惯（计 2 项）。 |
| 在应用内以渲染模式查看 | 删除 | 外部打开模式已删除，这一项没有存在前提。 |
| 隐藏 / 取消隐藏 | 删除 | 不想读的直接标为已读。 |
| 复制标题、复制链接 | 删除 | 剪藏在浏览器里做（计 2 项）。 |
| 在浏览器打开 | 保留 | 第④步，列表里就能直接去浏览器。 |

### 侧栏视图与侧栏右键菜单

| 项 | 建议 | 理由 |
| --- | --- | --- |
| 活动栏：全部文章 | 不确定 | 已读文章要不要有回看入口（U7）。 |
| 活动栏：未读 | 合并→界面布局节点 | 未读是唯一的主视图（第①步），不再需要单独的切换按钮。 |
| 活动栏：收藏、稍后读 | 删除 | 用户习惯（计 2 项）。 |
| 源 / 分类：全部标为已读 | 不确定 | 和「以上已读」有重叠（U6）。 |
| 源：同步此源 | 删除 | 全局同步已经够用，少一个按钮。 |
| 源：打开网站 | 删除 | 动线里没有这一步。 |
| 分类：重命名 | 删除 | 分类由 FreshRSS 管理（ADR 0002）；本地改名会被下次同步覆盖。 |

以上按单项计共 28 项：保留 7、删除 18、合并 1、不确定 2。「摘要」按钮的形态另见 U1，按「保留」计。

---

## 三、前端组件（frontend/src/components）

### article/

| 组件 | 建议 | 理由 |
| --- | --- | --- |
| ArticleList | 保留 | 第①步。 |
| ArticleItem | 保留 | 第②步，列表行只显示中文标题（D1）。 |
| ArticleCardItem | 删除 | 卡片布局已删除。 |
| ArticleDetail | 保留 | 第③步。 |
| ArticleDetailModal | 合并→ArticleDetail | 它是卡片布局专用的弹窗详情；只保留一种详情形态。 |
| ArticleContent | 保留 | 第③步中文正文 / 英文摘要的分支，以及 D2 的失败原因提示。 |
| ArticleToolbar | 保留 | 精简到第二节表格里保留的按钮。 |
| parts/ArticleBody | 保留 | 第③步显示中文正文。 |
| parts/ArticleSummary | 保留 | 第③步显示英文摘要（D2）。 |
| parts/ArticleTitle | 保留 | D1：详情页同时显示原文标题和译文。 |
| parts/AudioPlayer | 不确定 | 取决于是否订阅了播客（U11）。 |
| parts/VideoPlayer | 不确定 | 同上（U11）。 |
| parts/FloatingToc | 删除 | 细读去浏览器（第④步）。 |

### common/ 与 modals/common/

| 组件 | 建议 | 理由 |
| --- | --- | --- |
| BaseModal | 保留 | 设置页的弹窗壳。 |
| BaseSelect | 保留 | 设置表单要用。 |
| BaseMultiSelect | 删除 | 只有筛选器在用。 |
| ContextMenu | 保留 | 鼠标用户的「以上已读」「在浏览器打开」都在右键菜单里。 |
| FindInPage | 删除 | 细读去浏览器。 |
| ImageViewer | 删除 | 细读去浏览器。 |
| ModalFooter | 保留 | 弹窗通用件。 |
| Toast | 保留 | 同步失败、翻译失败等提示要用。 |
| ConfirmDialog | 保留 | 批量标记已读前的确认。 |
| InputDialog | 删除 | 只有分类重命名在用。 |
| MultiSelectDialog | 删除 | 全局多选弹窗，没有保留下来的使用场景。 |

### modals/filter/

| 组件 | 建议 | 理由 |
| --- | --- | --- |
| ArticleFilterModal | 删除 | 动线只有未读列表，不需要条件筛选。 |
| ConditionItem | 删除 | 同上。 |

### modals/settings/ 与 modals/update/

| 组件 | 建议 | 理由 |
| --- | --- | --- |
| SettingsModal | 保留 | 设置入口。 |
| about/AboutTab | 保留 | 显示版本，自建构建验收时要用。 |
| ai/AITab | 保留 | 承载单一大模型配置（D3）。 |
| ai/AIProfileModal | 合并→AITab | 表单字段收敛为「端点 / 密钥 / 模型 / 测试」（D3）。 |
| ai/AIProfileList | 删除 | 不再有 profile 列表（D3）。 |
| ai/AIProfileSelector | 删除 | 同上。 |
| ai/AIUsageSettings | 不确定 | 用量显示与上限（U2）。 |
| content/ContentTab | 合并→AITab | 只剩百度凭据和（可能的）摘要提示词，并成一个「翻译与摘要」页。 |
| content/TranslationSettings | 合并→AITab | 只剩百度凭据（D1、D3）。 |
| content/SummarySettings | 合并→AITab | 只剩 U1 和 U3 两个待定项。 |
| feeds/FeedsTab | 删除 | 订阅由 FreshRSS 管理；各源的本地偏好一并删除。 |
| feeds/FeedManagementSettings | 删除 | 同上（各源的 hide_from_timeline、view mode、auto expand）。 |
| feeds/DataManagementSettings | 删除 | 缓存改为自动清理。 |
| freshrss/FreshRSSSettings | 保留 | 第①步的数据来源。 |
| general/GeneralTab | 合并→壳相关项 | 只剩 U10 的更新提示；如果它也删掉，这一页就不要了。 |
| general/ApplicationSettings | 删除 | 语言、主题、界面字体、开机自启、关闭到托盘都已删除（壳选型 mrrss-280.1）。 |
| general/DataManagementSettings | 删除 | 缓存改为自动清理。 |
| general/UpdateSettings | 不确定 | 随壳选型（U10）。 |
| network/NetworkTab | 保留 | 代理设置（ADR 0004）。 |
| network/ProxySettings | 保留 | 去掉其中的超时项。 |
| reading/ReadingDisplayTab | 合并→排版项 | 只剩 U9 和 U5；两项都删掉的话，这一页就不要了。 |
| reading/ArticleDisplaySettings | 删除 | 其中的视图模式、布局、预览图都已删除或转给布局节点。 |
| reading/ContentSettings | 删除 | 全文抓取开关已删除，U4 如果保留也不做成开关。 |
| reading/InteractionSettings | 删除 | 隐藏文章已删除；悬停已读见 U5。 |
| reading/TypographySettings | 不确定 | 正文排版（U9）。 |
| shortcuts/ShortcutsTab | 删除 | 用户全程用鼠标。 |
| shortcuts/ShortcutItem | 删除 | 同上。 |
| update/UpdateAvailableDialog | 不确定 | 随壳选型（U10）。 |

### settings/（设置表单基础件）

| 组件 | 建议 | 理由 |
| --- | --- | --- |
| base/SettingGroup、SettingItem、SubSettingItem、TipBox、StatusBox、NestedSettingsContainer | 保留（6 项） | 剩下的少数设置页仍要用。 |
| base/StatusBoxGroup | 删除 | 只有 AI profile 的批量测试和用量页在用；单一配置的测试结果用 StatusBox 显示即可。 |
| base/SettingControl：Toggle、Input、Number、Select | 保留（4 项） | 同上。 |
| base/SettingControl：TextArea | 删除 | 只有提示词在用；如果 U3 选可编辑，就改回保留。 |
| base/SettingControl：Button | 删除 | 只有快捷键和数据管理在用；「测试连接」改用普通按钮。 |
| composite/SettingWithSelect、SettingWithToggle | 删除（2 项） | 剩下的设置太少，不值得保留组合件。 |
| advanced/KeyValueList、KeyValueInput | 删除（2 项） | 只有 AI profile 的自定义请求头在用（D3）。 |
| FontFamilySelect | 不确定 | 随 U9。 |
| index.ts | 保留 | 基础件的导出。 |

### sidebar/

| 组件 | 建议 | 理由 |
| --- | --- | --- |
| ActivityBar | 删除 | 收藏、稍后读视图已删除；「全部文章」见 U7。 |
| Sidebar、FeedList、SidebarCategory、SidebarFeed | 不确定（4 项） | 要不要按源或分类筛选未读（U8）。 |

---

## 四、composables（frontend/src/composables）

| composable | 建议 | 理由 |
| --- | --- | --- |
| ai/useAIProfiles | 合并→单一配置 | D3。 |
| ai/useAIProvider | 删除 | 这是按模型名识别厂商图标的逻辑，只有 profile 列表在用。 |
| article/fetchOutcome | 保留 | D2 的失败分类和原因文案。 |
| article/floatingToc | 删除 | 浮动目录已删除。 |
| article/titleTranslationRequest | 保留 | D1 标题翻译。 |
| article/translationMode | 删除 | 翻译模式已固定（D1）。 |
| article/useArticleActions | 保留 | 精简到第二节表格里保留的右键菜单项。 |
| article/useArticleDetail | 保留 | 第③步。 |
| article/useArticleFilter | 删除 | 筛选器已删除。 |
| article/useArticleRendering | 保留 | 第③步正文渲染。 |
| article/useArticleSummary | 保留 | 第③步和 D2。 |
| article/useArticleTranslation | 保留 | D1，只处理标题。 |
| article/useContentTranslation | 删除 | 全文翻译在浏览器里做。 |
| core/useAppUpdates | 不确定 | 随壳选型（U10）。 |
| core/useSettings.generated | 保留 | 由生成器产出。 |
| core/useSettings | 保留 | 设置读写。 |
| core/useSettingsAutoSave | 保留 | 设置自动保存。 |
| core/useSettingsValidation | 保留 | FreshRSS、代理等表单的校验。 |
| core/useSidebar | 不确定 | 随 U8。 |
| core/useWindowState | 合并→壳 | 窗口位置由壳在 Go 侧退出时保存一次，前端不再参与（壳选型 mrrss-280.1）。 |
| filter/useConditionOptions | 删除 | 筛选器已删除。 |
| filter/useFilterConditions | 删除 | 同上。 |
| filter/useFilterFields | 删除 | 同上。 |
| ui/useContextMenu | 保留 | 右键菜单。 |
| ui/useKeyboardShortcuts | 删除 | 用户全程用鼠标；Esc 关闭详情改由详情页自己处理。 |
| ui/useModalClose | 保留 | 弹窗关闭。 |
| ui/useNotifications | 保留 | 提示。 |
| ui/useResizablePanels | 合并→界面布局节点 | 面板能否拖动调整由布局决定。 |
| ui/useSelect | 保留 | 下拉框。 |
| ui/useShowPreviewImages | 合并→界面布局节点 | 与 show_article_preview_images 一起处理。 |

共 30 项：保留 16、删除 9、合并 3、不确定 2。

---

## 五、后端接口（internal/handlers，按 routes）

| 组 / 路由 | 建议 | 理由 |
| --- | --- | --- |
| **article**：`/api/articles` | 保留 | 第①步列表。 |
| `/api/articles/read` | 保留 | 第⑤步，改动即时推送到 FreshRSS。 |
| `/api/articles/mark-relative` | 保留 | 「以上已读」要用；「以下」方向可以去掉。 |
| `/api/articles/unread-counts` | 保留 | 未读数。 |
| `/api/articles/content` | 保留 | 第③步正文。 |
| `/api/articles/fetch-full` | 保留 | D2 抓全文。 |
| `/api/articles/mark-all-read` | 不确定 | 随 U6。 |
| `/api/articles/favorite` | 删除 | 收藏已删除。 |
| `/api/articles/toggle-hide` | 删除 | 隐藏已删除。 |
| `/api/articles/toggle-read-later` | 删除 | 稍后读已删除。 |
| `/api/articles/clear-read-later` | 删除 | 同上。 |
| `/api/articles/filter` | 删除 | 筛选器已删除（包括 article_filters.go）。 |
| `/api/articles/filter-counts` | 删除 | 同上。 |
| `/api/articles/reload-content` | 删除 | 「重新加载」按钮已删除。 |
| `/api/articles/cleanup` | 删除 | 改为后台自动清理，不再对外暴露接口。 |
| `/api/articles/cleanup-content` | 删除 | 同上。 |
| `/api/articles/content-cache-info` | 删除 | 同上。 |
| **translation**：`/api/articles/translate` | 保留 | D1 标题翻译。 |
| `/api/articles/translate-text` | 删除 | 全文翻译在浏览器里做。 |
| `/api/articles/clear-translations` | 删除 | 设置页不再提供清空入口；目标语言固定，也不需要重新翻译。 |
| `/api/ai-usage` | 不确定 | 随 U2。 |
| `/api/ai-usage/reset` | 不确定 | 随 U2。 |
| **summary**：`/api/articles/summarize` | 保留 | 第③步和 D2。 |
| `/api/articles/clear-summaries` | 删除 | 同 clear-translations 的理由。 |
| **ai**：`/api/ai/profiles`、`/api/ai/profiles/`（列表、增删改、设默认、单个测试） | 删除（2 条路由） | D3。 |
| `/api/ai/profiles/test-all` | 删除 | 同上。 |
| `/api/ai/profiles/test-config` | 合并→单一配置的「测试连接」 | D3 之后仍然需要一种方式确认配置可用。 |
| **settings**：`/api/settings` | 保留 | 设置读写。 |
| settings/translation_mode_compat.go | 删除 | 翻译模式已固定，旧值兼容层没有意义。 |
| **feed**：`/api/feeds` | 保留 | 显示源名称；如果 U8 保留侧栏，侧栏也要用它。 |
| `/api/feeds/update` | 删除 | 只用来写各源的本地偏好和分类重命名，这两样都已删除。 |
| **freshrss**：`/api/freshrss/sync` | 保留 | 第①步的数据来源。 |
| `/api/freshrss/status` | 合并→后端同步事件 | 同步状态改由后端在同步周期开始和结束时推送，替代前端的 5 秒轮询（同步审计 mrrss-280.2）；推送通道见文末「兄弟节点之间的冲突」。 |
| `/api/freshrss/sync-feed` | 删除 | 「同步此源」已删除。 |
| **browser**：`/api/browser/open` | 保留 | 第④步。 |
| **window**：`/api/window/state` | 合并→壳 | 窗口位置记忆放到壳的 Go 侧，不再经过 HTTP（壳选型 mrrss-280.1）。 |
| `/api/window/save` | 合并→壳 | 同上，改为退出时保存一次（壳选型 mrrss-280.1）。 |
| **update**：`/api/check-updates` | 不确定 | 随 U10。 |
| `/api/download-update` | 删除 | Epic 已把自动更新列为未定；即使保留检查更新，下载和安装也只需要跳到发布页。 |
| `/api/install-update` | 删除 | 同上。 |
| `/api/version` | 保留 | AboutTab 显示版本，自建构建验收时要用。 |
| update/repo_config.go | 不确定 | 随 U10。 |
| **core、response** | 保留（2 项） | 公共基础件。 |

按单项计共 45 项：保留 17、删除 21、合并 2、不确定 5。

---

## 不确定项（请拍板）

| # | 问题 | 选项 | 倾向与理由 |
| --- | --- | --- | --- |
| U1 | 英文文章的摘要怎么触发（`summary_trigger_mode`） | A 点开时自动生成；B 点「摘要」按钮再生成 | 倾向 A，只对非中文文章生效：动线第③步写的是「英文看 AI 摘要」，没有额外点击。代价是每点开一篇就要消耗 token，要不要设上限见 U2。D2 的原文写的是「摘要」按钮，所以选 A 时，这个按钮就只剩「重新生成」的用途。 |
| U2 | 用量统计和上限（`ai_usage_tokens`、`ai_usage_limit`、AIUsageSettings、`/api/ai-usage*`） | A 保留上限，超出后停止自动摘要；B 全部删除 | U1 选 A 就倾向 A，因为这是防止误开大量文章导致费用失控的护栏；U1 选 B 就倾向 B。 |
| U3 | 摘要提示词能否编辑（`ai_summary_prompt`） | A 写死内置提示词；B 留一个文本框 | 倾向 A：设置默认删除，长度和语言都写进内置提示词。 |
| U4 | 中文文章正文被截断时，点开要不要自动抓全文（`auto_show_all_content`、各源的 `auto_expand_content`） | A 截断即自动抓，不设开关；B 不抓，由用户去浏览器看；C 保留按源开关 | 倾向 A：第③步「中文读正文」时碰上截断正文就读不下去；判断截断的规则和 D2 共用，失败时同样显示原因和「在浏览器打开」。 |
| U5 | 悬停标记已读（`hover_mark_as_read`） | A 删除；B 保留为开关 | 倾向 A：已经有「以上已读」，悬停已读容易误触，而且让「以上已读」变得多余。 |
| U6 | 全部标为已读（源、分类的右键菜单和 `/api/articles/mark-all-read`） | A 保留一个入口；B 删除，只用「以上已读」 | 倾向 A，只放在列表末尾或分类右键菜单：扫到列表底部时，「以上已读」就等于全部已读，但未读很多时需要一键清空。同步审计已经为它设计了 `mark-all-as-read` 和 `pending_mark_all`（同步审计 mrrss-280.2），保留的成本很低。 |
| U7 | 已读文章要不要有回看入口（「全部文章」视图） | A 删除；B 保留一个「最近已读」入口 | 倾向 A：误标已读可以用详情页的「标为未读」改回；真要回看就去 FreshRSS 网页端。 |
| U8 | 要不要订阅源侧栏（Sidebar、FeedList、SidebarCategory、SidebarFeed、useSidebar、feed_drawer_*） | A 不要侧栏，只有一个合并的未读列表；B 保留分类或源的树，用来筛选未读 | 需要用户说明平时会不会只看某个分类；和界面布局节点一起决定。 |
| U9 | 正文排版（`content_font_*`、`content_line_height`、TypographySettings、FontFamilySelect） | A 全部写死；B 只留字号；C 保留全部 | 倾向 B。Epic 已把它列为 Not yet specified，这里只决定设置项去留。 |
| U10 | 更新检查（`update_check_enabled`、UpdateSettings、UpdateAvailableDialog、useAppUpdates、`/api/check-updates`、repo_config） | A 保留「有新版本时提示并跳到发布页」；B 全部删除 | 壳选型已把自动下载和安装排除，最多保留 A（壳选型 mrrss-280.1）；最终取决于 Epic 的「打包、发布与自动更新」待定项。开机自启和托盘已改为删除，不再属于这个问题。 |
| U11 | 播客和视频播放器（AudioPlayer、VideoPlayer） | A 删除；B 保留 | 取决于是否订阅了带音频或视频附件的源：没有就删除；有的话，也可以改为「在浏览器打开」。 |

「以上已读」有两种理解：可以是动线的结局（前面几步做完后文章变为已读），也可以是一个具体动作（标记以上为已读）。本草案两种都照顾到了：详情页有「标为已读 / 未读」，右键菜单有「标记以上为已读」。请在拍板时确认这个理解。

## 新增设置项（因合并产生）

| 键（暂定） | 来源 | 说明 |
| --- | --- | --- |
| `ai_endpoint`、`ai_api_key`（加密）、`ai_model` | D3，取代 `ai_profiles` 表和两个 profile_id | 只有这一套配置，用于摘要。旧库怎么迁移（例如沿用默认 profile）归入 Epic 的「SQLite 迁移」待定项。 |

## 兄弟节点之间的冲突（请一并拍板或回到对应节点）

| # | 冲突 | 涉及 | 建议 |
| --- | --- | --- | --- |
| C1 | 同步审计用 Wails 事件 `sync:state` 推送同步状态；壳选型却要求前端完全不依赖 Wails（唯一的 Wails 调用是剪贴板，改掉后要能在浏览器通道里运行，供 agent 取证）。 | 同步审计、壳选型、`/api/freshrss/status` | 推送改走 HTTP 通道（例如 SSE），不用 Wails 事件；这样两个结论都能成立。 |
| C2 | 同步审计仍然每个周期拉取星标集并下载星标条目的正文；本清单删除了收藏。 | 同步审计、收藏 | 如果拍板删除收藏，同步就不再拉星标集、星标意图也不必排队推送。服务端的星标状态保持原样，不受影响。 |
| C3 | 同步审计把 `freshrss_sync_on_startup` 写成调度条件；本清单建议删除这个开关，固定为启动即同步。 | 同步审计、`freshrss_sync_on_startup` | 以拍板结果为准，并回头改同步审计里的这一处。 |


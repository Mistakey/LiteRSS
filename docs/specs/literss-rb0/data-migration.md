# 旧库数据迁移方案（mrrss-280.10）

> Epic mrrss-280「重做并精简 MrRSS」的规划节点产物。只给方案，不含目标代码。
> 输入：同步模型（`sync-model.md`，mrrss-280.2）、功能与设置清单（`feature-inventory.md`，mrrss-280.3）
> 以及 mrrss-280.4 的用户裁决。代码行号基于 `main` @ `3ec13865`。
> 实测数据来自用户真实库 `%APPDATA%\MrRSS\rss.db`（2026-09-27 00:26，只读打开后复制快照查询，
> 查询完毕快照已删除）。凭据类字段只统计长度，没有读取内容。

## 结论

**新建数据库，首次启动时从旧库单向导入一次，只搬选定的数据。** 不在原库上原地迁移，也不只靠重新从
FreshRSS 拉取。

- **不原地迁移**：新模型把主键从本地自增 ID + URL 换成 FreshRSS 条目 ID，已读/星标拆成镜像列加
  `pending_state` 意图表，译文、摘要、全文移入本地表（`sync-model.md`「权威关系」）。旧表结构几乎每张都要重建，
  原地改表会同时踩到 pitfall 19（`DROP COLUMN` 不回收空间、`runMigrations` 每次启动都会重跑）和
  pitfall 20（正文整行替换会抹掉新增列）。何况改名后数据目录本身就会变（见「首次启动发现旧库」），新文件是自然的
  落点。旧库保持原样不动，就是现成的回退路径：新版有问题时，用户照旧运行老版即可。
- **不只靠重新拉取**：新同步模型只下载「未读集 + 星标集」的正文，已读条目不下载（`sync-model.md`「拉取与分页」第
  4 步），C2 删除收藏后星标集也不再拉取。而用户的旧库 **10938 篇全部已读**。只重新拉取的话，「全部」视图
  （U7 保留）会从空开始，已存的标题译文和摘要全部丢失，三份凭据也要重新输入。
- **导入量很小，成本不在「重新生成」而在「丢失历史」**：真正值钱的只有 509 条英文标题译文、70 篇摘要和 48 MB
  RSS 正文；全文只有 1 篇（见「关键数据」）。这些数据搬过来几乎没有成本，所以一并导入。

## 关键数据（真实库实测）

| 项 | 数值 | 对迁移的含义 |
| --- | --- | --- |
| `articles` 总数 | 10938 | 发布时间最早为 `2001-01-01`（坏日期），最晚为 `2026-09-26`；2026-09 有 3658 篇，2026-08 有 2133 篇 |
| 已读 / 未读 | 10938 / 0 | 全是已读历史，新同步模型不会重新下载 |
| 有 `freshrss_item_id` | 10921（99.84%） | 格式为长格式 `tag:google.com,2005:reader/item/<16 位十六进制>`，可转成 64 位整数主键；没有重复 ID |
| 没有条目 ID | 17 | 无法用条目 ID 定位，不导入 |
| 同 URL 多行 | 13 组共 30 行 | 以条目 ID 为身份后各自保留，展示层折叠（`sync-model.md`「身份」） |
| 收藏 / 隐藏 / 稍后读 | 0 / 0 / 0 | 功能已删除，列也不导入，不会丢失任何东西 |
| 音频 / 视频 URL | 0 / 0 | 与 U11 删除播放器一致 |
| `author`、`original_summary` | 全空 | 不导入 |
| `translated_title` 非空 | 4050 | 其中 3422 条等于原标题（pitfall 12 的「已判定为中文」），628 条是真译文，其中 509 条的原标题不含汉字 |
| 不含汉字的标题 | 4699 | 平均 38 个字符；其中已判定的只有 518 条，其余是懒翻译，从未显示过 |
| AI 摘要（`articles.summary`） | 70 | 平均 151 字，中文；全部是用户手动点「摘要」生成的，最近一篇是 2026-09-26 |
| `article_contents.content` | 10484 行，共 48 MB | 平均 4828 字符；454 篇文章没有正文行 |
| `article_contents.full_content` | 1 行 | 全文抓取几乎没用过 |
| `translation_cache` | 百度 9661 行（原文共 140 万字符）、AI 2779 行（67 万字符） | 包含全文翻译 `translate-text` 的正文段落（单条最长 5207 字符）；AI 行都在 2026-02 至 03 之间，属于已删除的 AI 翻译分支 |
| `freshrss_sync_queue` | 9 行，全部已同步（`synced_at` 非空），`retry_count` 最大为 0 | 快照时没有待推送的本地动作 |
| `feeds` | 101 个，全部有 `freshrss_stream_id`；6 个分类 | `hide_from_timeline` 为 0 个，`article_view_mode` / `auto_expand_content` 各 2 个非空 |
| `ai_profiles` | 1 行（`deepseek`，`deepseek-flash`），`is_default = 0` | `ai_summary_profile_id` 为空，实际走「无默认 → 取第一个」的回落（`ai_profiles.go:194-196`） |
| `settings` 中旧的 `ai_endpoint` / `ai_model` / `ai_api_key` | `https://api.openai.com/...` / `gpt-4o-mini` / 空 | 早已不用的遗留键，**不是**实际生效的配置 |
| `settings` 键数 | 112 | 大量是已删除功能的残留键（Notion、Zotero、DeepL、自定义翻译、RSSHub、媒体缓存、网速测试等） |
| `window_x` / `window_y` | -21333 / -21333 | 是 Windows 最小化时的哨兵坐标，被当作窗口位置保存了下来 |
| `PRAGMA journal_mode` / `user_version` | `wal` / 0 | 旧库没有版本号，导入器只能按表结构识别 |

## 导入内容与映射

新库的表结构由实现 Issue 决定，下面只规定「搬什么、怎么对应」。

| 旧数据 | 处理 | 说明 |
| --- | --- | --- |
| `feeds` | **不导入** | 首次同步用 `subscription/list` 重建（服务端所有）。各源的本地偏好列已全部删除（清单 mrrss-280.4） |
| `articles`（有条目 ID 的行） | 导入 | 主键 = 条目 ID（取长格式最后一段十六进制，转成十进制整数）；所属订阅通过 `feeds.freshrss_stream_id` 换成 stream ID；带上 `title`、`url`、`image_url`、`published_at`；`is_read` 写入镜像 `server_read`，第一次同步会无条件覆盖它 |
| `articles`（没有条目 ID 的 17 行） | 丢弃 | 全部已读，无法定位，也无法推送 |
| `articles.translated_title` | 导入本地译文表 | 原样搬运，**包括等于原标题的值**：pitfall 12 规定非空就代表「已判定」，搬成空值会让这 3422 条重新请求百度 |
| `articles.summary` | 导入本地摘要表 | 70 条 |
| `article_contents.content` | 导入（RSS 正文） | 新模型不再下载已读条目的正文，这是「全部」视图里点开旧文章时唯一的正文来源 |
| `article_contents.full_content` | 导入（全文缓存），或者直接丢弃 | 只有 1 行，怎么处理都行；导入时与 RSS 正文分列写入，保持 pitfall 20 的列级隔离 |
| `is_favorite` / `is_hidden` / `is_read_later` / `audio_url` / `video_url` / `original_summary` / `author` / `unique_id` | 丢弃 | 功能已删除，或者列为空 |
| `translation_cache` | **丢弃** | 标题的判定结果已经存在 `translated_title` 里；其余是全文翻译的段落和已删除的 AI 翻译 |
| `freshrss_sync_queue` 中未同步的行 | 转换为 `pending_state` | 见下一节 |
| `ai_profiles` | 按旧的解析链取**一条**，转成新的单一配置 | 解析链：`ai_summary_profile_id` 指向的 profile → `is_default = 1` 的 profile → id 最小的 profile（与 `ProfileProvider.GetProfileForFeature` 一致）。取 `endpoint`、`model`、`api_key`（解密后用新库的方式重新加密）；`custom_headers` 丢弃（D3）。**不读** `settings` 里遗留的 `ai_endpoint` / `ai_model` / `ai_api_key` |
| `settings` | 按白名单导入 | 见下表 |
| `media_cache/`、`logs/`、`scripts/`、`monitor_device_id.txt` | 不导入 | |

`settings` 白名单（清单与裁决之后仍然保留的项）：

| 键 | 处理 |
| --- | --- |
| `freshrss_server_url`、`freshrss_username`、`freshrss_auto_sync_interval` | 原样导入 |
| `freshrss_api_password`、`baidu_secret_key`、`proxy_password` | 用旧格式解密，再按新库的方式加密 |
| `baidu_app_id`、`proxy_mode`、`proxy_type`、`proxy_host`、`proxy_port`、`proxy_username` | 原样导入 |
| `update_check_enabled`、`close_to_tray`、`startup_on_boot` | 原样导入（托盘和开机自启已被裁决保留） |
| `window_width`、`window_height`、`window_maximized` | 导入；`window_x` / `window_y` 等于 -32000 或 -21333 这类最小化哨兵值时丢弃，让壳居中显示 |
| `freshrss_last_sync_time` | **不导入**。新库的第一次同步必须是完整周期，不能是增量 |
| 其余所有键 | 丢弃，包括 `ai_usage_*`（U2 删除）和 `ai_summary_prompt`（U3 写死） |

**凭据能否解密**：`crypto.Encrypt` 的密钥由 PBKDF2 从 `hostname-GOOS-GOARCH-machine-id` 派生（`internal/crypto/encryption.go:40-71`），
与应用名、数据目录都无关，所以改名不影响解密。前提是新项目的导入器保留一个能读 `MrRSS-v1:` 格式的解密函数。
在 Windows 上 machine-id 为空，密钥实际上只取决于主机名：用户改过主机名，或者把库拷到另一台机器上，解密都会失败。
这种情况下只丢弃这一项凭据，导入继续，并在导入结果里提示「请重新填写 FreshRSS 密码 / 百度密钥 / AI 密钥」。

## 迁移期间未推送的本地动作（pitfall 14）

旧队列是只追加的动作日志，新模型是「每个（条目，字段）只保留最新期望值」。转换规则：

1. 取 `synced_at IS NULL` 且 `retry_count < MaxSyncPushRetries` 的行（与旧代码的 `pendingSyncCondition` 相同）。
   已被旧代码放弃的行不再复活。
2. 通过 `article_id` 找到条目 ID；找不到的（文章没有条目 ID）丢弃并记日志：旧代码本来就推不出去（`errNoItemLink`）。
3. `mark_read` / `mark_unread` 映射为 `field = read` 的意图；`star` / `unstar` 丢弃（C2：收藏删除后不再推送星标意图，
   服务端的星标保持原样）。
4. 同一（条目，字段）有多行时，按 `created_at`、再按 `id` 取**最后**一行，`seq` 按这个顺序分配。这样顺带修掉了
   旧队列「按固定动作顺序推送」的问题（`sync-model.md`「新发现」第 2 条）。
5. 意图和文章在同一个事务里写入，而且整个导入在**第一次同步周期开始之前**完成。第一次周期「先推送、再拉取」，
   拉取只覆盖镜像、不碰意图表，所以这些动作不会被覆盖。

真实库在快照时没有待推送的行，但导入器仍然必须实现这一段：用户可能刚在旧版里离线标了一批已读，就立刻打开新版。

## 首次启动发现旧库

### 路径

改名以后，`fileutil.GetDataDir` 里硬编码的 `"MrRSS"`（`internal/utils/fileutil/fileutil.go:86`）会换成新名字，
Wails 的 `UniqueID` `com.mrrss.app`（`main.go:244`）也会换掉。新应用按下面的顺序查找旧库，只查这些固定位置，
不提供「选择旧库文件」的界面（设置默认删）：

| 平台 | 旧库位置 |
| --- | --- |
| Windows | `%APPDATA%\MrRSS\rss.db` |
| macOS | `~/Library/Application Support/MrRSS/rss.db` |
| Linux | `$XDG_DATA_HOME/MrRSS/rss.db`，未设置时用 `~/.local/share/MrRSS/rss.db` |
| 便携版 | 新应用自己是便携版（exe 旁有 `portable.txt`）时，额外查 `<exe 目录>\data\rss.db`，这只覆盖「新 exe 放进旧便携目录」的情况 |

少见的情况（旧库在别的目录）用一个环境变量或命令行参数指定路径即可，不做界面。

### 触发条件与状态记录

- 新库里有一张 `meta` 表（或同等设施）记录 `legacy_import`：`done` / `skipped` / `failed`，加上来源路径、
  时间和各类计数。**只要有这条记录，就再也不查找旧库**。
- 新库刚建好、这条记录不存在、而且找到了旧库时，自动导入，不先弹窗询问。旧库是只读的，导入本身没有破坏性；
  用户想从头开始，删掉新数据目录就行。导入完成后用一条提示报告结果（导入了多少篇文章、译文、摘要，哪些凭据需要重新填写）。
- 没有找到旧库时记 `skipped`，按全新安装处理。

### 识别与读取

- 按表结构识别，因为旧库的 `user_version` 为 0：必须有 `articles`、`settings`、`article_contents`、`feeds`，
  并且 `articles` 有 `freshrss_item_id` 列。缺少任何一样就记 `failed` 并提示，不猜测。
- 用只读 URI（`file:...?mode=ro`）打开，先 `VACUUM INTO` 到新数据目录下的临时文件，再从这份拷贝导入，导完删除。
  这样读的是一致的快照，旧库和旧版进程都不受影响。
- 注意：旧库处于 WAL 模式，只读连接仍会在旧目录里生成空的 `rss.db-wal` / `rss.db-shm`（本节点实测：只读查询后这两个文件
  出现了，主库的修改时间没有变）。这不算写入数据，但文档和提示里不要承诺「完全不碰旧目录」。

### 旧版仍在运行

改名后 `UniqueID` 不同，新旧两版可以同时运行，同时同步同一个 FreshRSS 账号。旧版的差异推送会把别处的已读改回未读
（`sync-model.md`「新发现」第 1 条），所以两者并存会互相干扰。

- 导入前只读探测旧版的单实例互斥量 `wails-app-com.mrrss.app-sim`（与 `dev-isolation.md` 的探针方法相同：
  只 `OpenMutex`，不创建）。探测到旧版在运行时，推迟导入，提示「请先退出旧版 MrRSS」，等下次启动再试。
  推迟期间不写 `legacy_import`，同步也不开始，免得新库先做了增量拉取、导入再覆盖它。
- 导入完成后提示用户卸载旧版。新应用**不删除**旧目录、不改旧库、不注销旧版的开机自启注册表项
  `HKCU\...\Run\MrRSS`（`internal/utils/startup.go:46-52`）。这些都交给用户或旧版的卸载程序处理；真实库当前
  `startup_on_boot = false`，这一项没有残留。

### 顺序

1. 建新库（新结构）。
2. 查找 → 识别 → 探测旧版 → 快照 → 在一个事务里导入（文章、正文、译文、摘要、意图、设置、AI 配置）→ 写 `legacy_import`。
3. 启动同步调度：第一次周期先推送导入的意图，然后完整拉取订阅、标签和状态 ID 集，镜像被无条件纠正，
   新的未读条目按正常流程补内容。

在 10938 篇、48 MB 正文的规模下，快照加导入预计只需要几秒，不需要进度界面。

## 重新生成的成本（如果不导入）

| 数据 | 数量 | 重新生成的成本 | 判断 |
| --- | --- | --- | --- |
| 英文标题译文（百度） | 真译文 509 条，约 1.9 万字符；如果「全部」视图里全部 4699 个英文标题都要显示，约 18 万字符 | 百度通用翻译按字符计费，免费额度按版本从每月 5 万字符到 100 万字符以上不等（以用户的百度控制台为准，本节点没有核实用户的版本）。标准版的 QPS 为 1，重翻 4699 条大约需要 80 分钟 | 标题是懒翻译，只有显示时才请求，所以不导入也不会一次性花掉额度；但 3422 条「已判定为中文」的记录会跟着重新请求，完全是浪费。导入的成本只是一列文本 |
| AI 摘要 | 70 篇 | 每篇输入约 1.2k–3k token（正文平均 4828 字符加提示词），输出约 200 token，合计约 10–20 万 token；按 `deepseek-flash` 这一档的价格是几角钱 | 钱不是问题，问题是摘要是手动点出来的（U1 选了 B），不导入的话，回看时要重新点、重新等 |
| 全文抓取 | 1 篇 | 可以忽略 | 导不导入都行 |
| RSS 正文 | 10484 篇，48 MB | 新同步模型不会重新下载已读条目；要取回只能另写一个「按 ID 批量取内容」的回填，而 FreshRSS 可能已经按自己的保留策略清掉了旧条目 | 不导入就等于丢失，「全部」视图里的旧文章点开是空白 |
| 三份凭据 | FreshRSS、百度、AI | 用户重新输入一次 | 能解密就导入 |

结论：重新生成在钱上很便宜，但会丢数据，还要重复操作；导入这些数据的成本几乎为零。

## 留给规格或实现的问题

- **保留期**：`sync-model.md` 把「已读、超过保留期的本地条目可以删除」的具体数值留给了本节点。本节点不定数值，只定规则：
  导入的文章与同步来的文章走**同一条**清理路径，导入器不自己截断。旧库没有已读时间，只有 `published_at`，其中还有
  `2001-01-01` 这样的坏日期；如果保留期按「已读时间」计算，导入时要把 `read_at` 设为导入时间，否则第一次清理就会删掉大半历史。
- **`published_at` 的格式**：旧库存的是 Go `time.Time` 的字符串形式（例如 `2026-09-26 21:48:03 +0800 CST`），
  导入器要按这个格式解析；解析失败时退回到条目 ID 推出的时间，或者丢弃这一行。
- **订阅已取消的文章**：旧库里的 stream ID 在第一次同步后可能已经不存在。这些文章是保留（显示为「未知订阅」）还是在清理时删掉，
  由规格决定；导入器照常导入。

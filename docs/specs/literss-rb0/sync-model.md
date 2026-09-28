# 同步层审计与新同步模型（mrrss-280.2）

> Epic mrrss-280「重做并精简 MrRSS」的规划节点产物。只审计与设计，不含目标代码。
> 行号基于 `main` @ `ec75c036`。FreshRSS 行为以其 `p/api/greader.php`（edge 分支）为一手来源。

## 结论

**同步核心重写，不在现有设计上重构。** 已知问题分两类：

- **同一个设计缺陷的症状**：pitfall 13、pitfall 14、commit c81564c6、失败时重复入队，以及本次审计新发现的几处（见「新发现」）。缺陷有三层，彼此叠加：
  1. **身份错位**：本地文章以 URL（外加 `unique_id`）为身份，FreshRSS 以条目 ID 为身份，`freshrss_item_id` 只是一个可空的附加列。
  2. **双权威**：本地库既是服务端状态的镜像，又在推送阶段被当作权威，用「全量差异推送」把本地状态写回服务端。
  3. **动作日志式队列**：待推送的变更是一张只追加的动作日志，不是「每篇文章每个字段的期望值」。它唯一的作用是挡住拉取对本地状态的覆盖，所以不得不再加有界重试、逐条二分、快照时机等一层层补偿。
  pitfall 13/14 记录的那些守卫，全是在补这三层缺陷。换成新模型后，这些守卫的前提都不存在了。
- **独立的实现 bug**（新模型会顺带消除它们的成因，但它们本身不是同步设计的问题）：mrrss-zot（OFFSET 分页）、全部已读不含子分类、状态轮询没在启动时开启、pitfall 15（刷新时把正在读的文章移出列表）、pitfall 16（feeds 行混有两种权威）。

`internal/freshrss/client.go` 里的 HTTP 层（登录、`editTag`、`APIError.RejectsItem`、`streamContentsTarget`、局域网直连 transport）和 `runExclusiveSync` 可以保留。`bidirectional_sync.go`、`freshrss_sync_db.go` 的队列、`article_db_sync.go` 里的 `*WithSync` 系列、handler 里的 `performImmediate*` 全部重写。`client.go:605-807` 的旧 `SyncService` 已经没有调用方（`NewSyncService` 只剩定义），直接删掉。

## 逐项证据

### 1. mrrss-zot：未读列表 OFFSET 分页漏文章 —— 证实，独立 bug

- 后端把页码换算成偏移：`offset := (page - 1) * limit`（`internal/handlers/article/article_basic.go:60`）。
- SQL 是 `ORDER BY a.published_at DESC LIMIT ? OFFSET ?`（`internal/database/article_db.go:260`），没有 `id` 作次级排序，同一时间戳的文章顺序也不稳定。
- 前端每次追加页时 `page++`（`frontend/src/stores/app.ts:232`），`data.length < limit` 就把 `hasMore` 置为 false（`app.ts:211-213`）。
- 标已读会缩小 `is_read = 0` 的结果集（`article_db.go:219`），下一页的 OFFSET 于是跳过同样数量的未读文章。「全部已读」只作用于已加载的文章（`ArticleList.vue:504-506`、`718-720`），被跳过的那些永远标不到。
- 这是列表查询层的 bug，与 FreshRSS 同步无关：没有同步时照样会发生。

### 2. 状态轮询没在启动时开启 —— 证实，独立 bug

- `startFreshRSSStatusPolling`（`app.ts:504`）唯一的调用点是设置页里「FreshRSS 从关闭切到开启」的分支（`FreshRSSSettings.vue:165-167`）。`App.vue` 启动流程（`App.vue:102-184`）只调用了 `startAutoRefresh` 和 `refreshFeeds`。
- 前端自己发起的同步有 `pollProgress`（`app.ts:472-498`）负责收尾刷新，所以平时看不出来。受影响的是**不经前端发起**的同步：托盘菜单「刷新」直接调 `TriggerSync`（`main.go:487-490`），同步完成后界面不刷新。
- 这是前端接线漏了一处。更深的问题是用轮询去发现后端事件本身（500 ms 和 5 s 两套轮询并存），新模型改由后端推送事件。

### 3. 全部已读不含子分类 —— 证实，独立 bug（遗留概念）

- 列表按分类取 feed 用 `category = ? OR category LIKE ?`，参数是 `category + "/%"`（`article_db.go:176-177`）。
- 全部已读用 `category = ?`：`article_batch_db.go:34-36`、`article_db_sync.go:356`；按时间相对标记也一样：`article_batch_db.go:80`、`article_db_sync.go:428`。前端本地更新用严格相等 `(feed?.category || '') !== category`（`app.ts:322`）。
- 根源是 MrRSS 单机时代「以 / 分隔的嵌套分类」概念。FreshRSS 的标签是扁平的（`createFeedsFromSubscriptions` 直接取 `cat.Label`，`bidirectional_sync.go:458-466`），只有标签名本身带 `/` 时才会触发这种不一致。
- 另外，远端的全部已读是逐篇发 edit-tag（`collectSyncRequests`，`article_db_sync.go:491-510`），没有用 FreshRSS 的 `mark-all-as-read`。

### 4. 失败时重复入队 —— 证实，设计缺陷的症状

- `SyncArticleStatus` 在 edit-tag 失败时自己入队一次（`bidirectional_sync.go:151-161`），返回错误后调用方又入队一次：`article_sync.go:99-104`，批量路径 `performImmediateBulkSync` 同样如此（`article_bulk.go:379-383`）。登录失败（`bidirectional_sync.go:124-126`）只入队一次，edit-tag 失败入队两次。
- `EnqueueSyncChange` 是纯 INSERT（`freshrss_sync_db.go:131-136`），没有按（文章，字段）去重的约束。所以根子在队列语义（只追加的日志），不在哪一次多调用了一下。

### 5. Pitfall 13（重复条目跟随最新 ID）—— 设计缺陷的症状

- 本地以 URL 查找文章：`GetArticleByURL` 用的是 `WHERE url = ? LIMIT 1`，没有排序（`article_db_sync.go:137-138`）。库里的唯一键是 `unique_id`（`schema.go:43`），`url` 不唯一，同一个 URL 有多行时取到哪一行是任意的。
- FreshRSS 以 GUID 生成条目。重发的文章会产生多个同 URL 的条目，各自有读状态。为此加了三道守卫：`dedupeArticlesByURL`（`bidirectional_sync.go:799-818`）、`laterItemID` 跳过旧条目（`657-659`）、`applyServerStatus` 只认已关联的条目（`344-346`）；推送端还要 `remoteState` 按 ID 或 URL 双索引（`1004-1027`）。
- 如果以条目 ID 为身份，这四处都不需要。

### 6. Pitfall 14 与 commit c81564c6（待推送优先、有界重试、edit-tag 必须用条目 ID）—— 设计缺陷的症状

- c81564c6 修的是「用 URL 发 edit-tag，服务端回 OK 却什么都没改」。一手来源证实了这一点：greader.php 对非纯数字的 `i` 执行 `hex2dec(basename($e_id))`，非法值会变成 0；而 `editTag()` 结尾是无条件的 `exit('OK')`。成因是身份错位：文章可以没有条目 ID，于是有了 URL 回退。
- 「拉取不能覆盖待推送的变更」这条规则，是因为拉取直接写本地唯一的一份状态（`bidirectional_sync.go:677-697`、`358-363`）。所以需要队列快照（`404-415`）、快照要尽量晚读（注释 `401-403`、`244-246`）、需要有界重试（`freshrss_sync_db.go:20-24`，`recordPushRejection` 在 `1203-1224`）、批量被拒后逐条重发（`1169-1184`），还需要 `errNoItemLink`（`1050-1053`、`1119-1125`）。
- 如果把「服务端镜像」和「本地意图」分成两张表，由读取时合成，拉取就可以无条件写镜像，上面这些时序补偿都用不上。

### 7. Pitfall 15（打开过的文章固定在列表里）—— 独立问题（列表模型）

- 成因是 `fetchArticles` 每次刷新都整页替换（`app.ts:189-219`），而服务端对列表做了过滤。于是用 `pinnedArticleIds` 和 `mergePinnedArticles` 补回来（`app.ts:90`、`162-172`），`fetchTicket`/`refreshBase` 防止晚到的响应覆盖新列表（`app.ts:177-227`，来自 c81564c6）。
- 这与同步的权威模型无关，是列表成员随状态变化这一设计导致的，和 mrrss-zot 同源。

### 8. Pitfall 16（同步只写服务端的列）—— 独立问题（feeds 行混有两种权威）

- 同步路径已经收窄为 `UpdateFeedWithOptions`，只写 `Title`/`Category`（`bidirectional_sync.go:499-506`），当前实现没有问题。
- 重做时，本地 feed 设置（`hide_from_timeline`、`article_view_mode`、`auto_expand_content`）大多会随按钮与设置精简而删掉。留下的放到独立的 `feed_prefs` 表，feeds 表就完全归服务端所有，这条 pitfall 也就不存在了。

## 新发现（审计中发现，未入图）

1. **差异推送会把别处的已读改回未读。** `pushToServer` 重新取远端已读集（`bidirectional_sync.go:873`），只要「本地未读、远端已读」就推 unread（`940-943`）。两种情况会触发：
   - 拉取第 4 步读远端已读集失败（`273-275` 只记日志然后继续），而推送阶段读取成功；
   - 用户在拉取第 4 步和推送读取之间，在别的设备上读了文章。
   两种情况下，都会把服务端的已读改回未读。
2. **队列按固定动作顺序推送，不按用户操作顺序。** `pushActions` 固定为 read→unread→star→unstar（`1038-1043`，`1143` 按这个顺序遍历）。同一篇先 unread 再 read，服务端最终会是 unread；只是同一次 Sync 后面的差异推送碰巧又改了回来。
3. **待推送快照与推送的上限不一致。** 拉取只读 1000 条（`405`），推送只取 500 条（`835`）。离线批量标记超过 1000 条时，拉取会覆盖掉快照之外那部分。
4. **每次同步都全量重下全部内容。** 每个订阅用 `n=10000` 分页取完（`193-226`）；已读流在拉取和推送各取一次（`273`、`873`）；星标流也取两次（`240`、`882`）。已读流返回的是带正文的全部已读条目，随使用时间增长而线性膨胀。
5. **即时推送每篇文章登录一次。** `SyncArticleStatus` 每次调用都先 `Login`（`124`）。批量路径逐篇串行调用（`article_bulk.go` 的 `performImmediateBulkSync`）。前端「全部已读（可见）」会并行发出 N 个 `/api/articles/read`（`ArticleList.vue:504-506`、`718-720`），每个请求各起一个 goroutine、各自登录，而且不检查 `res.ok`。
6. **死代码。** `client.go:605-807` 的 `SyncService` / `getOrCreateFreshRSSFeed` / `buildCategoryPath` 没有调用方。

## 新同步模型

### 身份

- 文章主键是 **FreshRSS 条目 ID**，存为 64 位十进制整数，由 greader 的长格式十六进制 ID 转换而来。`stream/items/contents` 两种格式都接受，`stream/items/ids` 返回十进制。
- URL 只是一个属性，没有「未关联条目的文章」这种状态，也就没有 URL 回退。
- 同 URL 的多个条目（重发）各自保留，在**展示层**折叠：未读列表对同一 URL 只显示最新的一条。标已读时对这组条目的所有 ID 一起发 edit-tag，这样旧副本不会在别处冒出来成为未读。

### 权威关系

| 数据 | 权威 | 本地存放 |
| --- | --- | --- |
| 订阅、标签、条目内容 | FreshRSS | 镜像，同步时整体覆盖 |
| 已读、星标 | FreshRSS | 镜像列 `server_read` / `server_starred` |
| 用户刚做、尚未确认的已读/星标 | 本地意图 | `pending_state` 表 |
| 译文、摘要、全文、阅读偏好 | 本地 | 本地表，同步不写 |

- 显示的状态 = 该字段有意图就用意图的值，没有就用镜像。在查询中合成（`COALESCE`/`LEFT JOIN`）或者写成视图。
- 拉取**无条件**覆盖镜像，不看队列。意图不会被拉取覆盖，因为两者本来就不在同一张表。
- **取消全量差异推送。** 本地永远不会凭「本地与远端不同」就改服务端，只推送用户明确做过的意图。

### 待推送意图（队列语义）

- 表 `pending_state(item_id, field, value, seq, attempts, last_error)`，主键是 `(item_id, field)`。写入用 UPSERT：同一字段只保留**最新**的期望值，`seq` 单调递增。重复入队和顺序颠倒都不可能再出现。
- 另有 `pending_mark_all(stream_id, ts_ns, seq, attempts, ...)`，表示一个流级别的全部已读。
- **写路径**：用户操作在一个事务里 UPSERT 意图，然后通知推送器。推送器防抖约 1 秒，按 `(field, value)` 分组发送批量 edit-tag（每请求约 250 个 `i`），全部已读用 `mark-all-as-read`。
- **确认**：服务端返回 2xx 后，执行 `DELETE ... WHERE item_id=? AND field=? AND seq=?`（比较后删除）。请求在途期间写入的更新意图的 `seq` 不同，因此不会被误删。
- **失败**：传输错误、401/403/408/429、5xx 不计入次数，指数退避后重试；401 先重新登录。能归咎到条目的 4xx 计入 `attempts`，批量失败时逐条二分找出责任条目（保留现有 `RejectsItem` 的判定）。达到上限就丢弃这条意图并记日志，文章随后跟随镜像。
- edit-tag 无条件回 `OK`，没法确认服务端是否真的改了。只要身份永远是条目 ID，这一点就不构成风险；万一改失败，下一次状态拉取会让镜像纠正显示。

### 拉取与分页

每个同步周期串行执行，由 `runExclusiveSync` 保证同一时刻只有一个周期：

1. **先推送**：清空意图，让本周期拉到的状态已经包含自己的改动。
2. **订阅与标签**：`subscription/list`、`tag/list`，只覆盖服务端拥有的列。
3. **状态 ID 集**（只取 ID，便宜）：
   - 未读集：`stream/items/ids?s=user/-/state/com.google/reading-list&xt=user/-/state/com.google/read`
   - 星标集：`stream/items/ids?s=user/-/state/com.google/starred`
   - 用 `c` 续页。FreshRSS 的 continuation 就是上一页最后一个条目 ID，本身就是 keyset 分页。
   - 镜像更新：在未读集里的记 `server_read=0`，其余本地条目记 `server_read=1`；星标同理。
4. **补内容**：对未读集和星标集中本地没有的 ID，调 `stream/items/contents`（POST 多个 `i`，每批约 100 条）取正文。已读条目不下载。
5. **清理**：已读、未星标、且超过保留期的本地条目可以删除（保留期与迁移另议）。

这样每次同步的成本只与「未读数 + 星标数」成正比，不随历史增长；也不会再出现「本地有、服务端已清理」的幽灵未读，因为不在未读集里的都按已读处理。

### 全部已读

- 发送 `mark-all-as-read`，参数 `s` 为 `feed/<id>`、`user/-/label/<名>` 或 `reading-list`，`ts` 取当前列表快照中**最新条目的抓取时间**（纳秒）。服务端在用户看列表之后才收到的新条目不会被误标。
- 本地对同一范围、抓取时间不晚于 `ts` 的条目写 `pending_mark_all`，显示时并入意图合成。
- 分类用标签名做精确匹配，删除 `LIKE 'cat/%'`。

### 本地列表分页

- 进入一个视图（或用户主动刷新）时，后端返回该视图的**有序 ID 快照**，排序键为 `(published_at DESC, item_id DESC)`；前端再按 ID 分批取卡片数据。
- 列表成员在快照内固定，标已读只改卡片状态，不改成员。这一点同时解决 mrrss-zot 和 pitfall 15：不再需要 `pin`/`merge`/`fetchTicket` 这套机制。
- 同步完成后如有新条目，在列表顶部提示「N 篇新文章」，点击后重新取快照；不自动替换正在阅读的列表。
- 如果快照过大（例如数万条），退化为 keyset 游标 `(published_at, item_id) < cursor`，语义不变。

### 同步时机（替代轮询）

- **调度在后端**：启动时同步（`freshrss_sync_on_startup`），此后每 N 分钟一次；系统从睡眠恢复、窗口重新获得焦点时补一次，距离上次同步不足 1 分钟的跳过。前端的 `startAutoRefresh`、`pollProgress` 和 5 秒状态轮询全部删除。
- **意图推送**：用户每次操作后防抖约 1 秒触发；失败按退避重试，不必等到下一次全量周期。
- **通知**：后端在周期开始和结束时发 Wails 事件（`sync:state`，内容包括是否在运行、新条目数、待推送数、上次成功时间和错误）。托盘、启动、定时等任何来源触发的同步，前端都会收到。
- **客户端复用**：同一份配置复用一个已登录的 `Client`，401 时重新登录；不再每个操作都登录一次。

## 重写范围与保留项

- **保留**：`client.go` 的登录、令牌、`editTag`、`APIError.RejectsItem`、`streamContentsTarget`、局域网 transport（pitfall 8/9）；`runExclusiveSync`。
- **新增**：`stream/items/ids`、`stream/items/contents`、`mark-all-as-read` 三个客户端调用。
- **重写**：同步服务（新增一个模块，对外接口大致为 `RecordIntent`、`MarkStreamRead`、`RunCycle`、`Events`）；意图表；列表快照查询；前端 store 的列表与同步部分。
- **删除**：旧 `SyncService`、`freshrss_sync_queue`、`*WithSync` 系列、`performImmediate*`、差异推送、`pendingChanges`/`remoteState`/`dedupeArticlesByURL`/`laterItemID` 等补偿逻辑，以及前端的三套轮询和 pin 机制。pitfall 13、14、15 随实现一起改写或删除，16 视 `feed_prefs` 的落地情况改写。
- **不在本节点范围**：旧库、已存译文、摘要、全文怎么迁移到以条目 ID 为主键的新表（Epic 的「Not yet specified」），以及保留期的具体数值。

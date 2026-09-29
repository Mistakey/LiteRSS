# 代码约定

只收本项目特有的约定。重做进行中：前端、handler、数据库与同步的约定随各步骤的新代码写回，目标形态见
[spec](specs/literss-rb0/spec.md)。

## 组织

- 文件长度（约 300–400 行）是检查职责的信号，不是拆分门槛；只在边界更清楚或更好测时再拆。
- 按职责分包，测试与源码放在一起；文件数量本身不要求新建目录。
- 检查项按 [Testing](TESTING.md#verification-by-task) 选择；完整桌面构建只用于桌面集成或交付二进制。

## 设置

增删改设置项按 [Settings](SETTINGS.md) 的 schema 与生成器流程做，前端键名与后端一致（snake_case）。

## Go

- 需要取消或截止时间的操作第一个参数传 `context.Context`；纯函数不加用不到的 context。
- 错误用 `fmt.Errorf("...: %w", err)` 带上操作语境，返回零值与错误，不 panic。
- SQL 一律参数绑定，绝不把输入拼进 SQL；自己创建的 `stmt`、`rows` 要 `Close`，遍历后检查 `rows.Err()`。
- 改表结构只能在 `internal/database/migrate.go` 的 `migrations` 末尾追加一步；已发布的步骤不改、不删、不重排，
  已跑过它的库不会再跑。每步写成接收 `*sql.Tx` 的函数，版本号由框架在同一事务里设置，步骤里不碰 `user_version`。
- 每个连接都开着 `foreign_keys`，而 `PRAGMA foreign_keys` 在事务里无效：迁移步骤里 `DROP TABLE articles`（或 `feeds`、`tags`）
  会按 `ON DELETE CASCADE` 删光子表。需要重建被引用的表时，先给框架加上「关外键执行、结束后 `PRAGMA foreign_key_check`」的步骤形态。
- 写 `articles`、`feeds`、`tags` 这类被级联引用的表用 `INSERT ... ON CONFLICT(...) DO UPDATE`，不用 `INSERT OR REPLACE` / `REPLACE`（pitfall 26）。
- 出站 HTTP 一律走 `httputil.CreateHTTPClient`（pitfall 8）；局域网 FreshRSS 例外（pitfall 9）。

### 输入校验

不可信的 URL 只收带主机的绝对 `http(s)`（`internal/browser`、`enrich.fetchable`、`routes` 的在浏览器打开）；拼接本地路径时用 `filepath.Rel` 判断是否越出基准目录，前缀比较会放过 `/base2` 这类兄弟目录。

## API handler

- 路由只在 `internal/routes` 的 `Table` 里登记，一条一行：方法、路径（`ServeMux` 模式，路径参数写 `{id}`）、handler 构造函数。
  会改状态的路由标 `Mutates: true` 并用 POST / PUT / DELETE；只读路由用 GET。
- handler 是返回 `http.Handler` 的构造函数，依赖从参数传入（`Deps` 里的接口或服务），不读包级状态。handler 只解析参数、调服务、写响应；
  查询与业务规则放在服务包（读路径在 `internal/library`，意图写入在 `internal/syncer`）。
- JSON 用 `writeJSON`（带 `no-store`）。列表返回空数组而不是 `null`。服务包用哨兵错误区分 400 / 404，其余错误在 handler 记日志、回 500。
- 改状态的路由收 JSON 请求体，用 `decodeBody`（限大小、拒未知字段，失败回 400）；必填字段用指针或检查零值，缺了回 400。无返回内容的动作回 204。

## 前端

- 前端是纯 `/api` 客户端：只用相对路径 `/api/...`，不 import Wails 包，不加载任何外部字体、脚本或样式（CSP 也会拦下）。
  唯一的例外是无边框顶栏给窗口宿主发的 `wails:drag` / `wails:resize:*` 等消息，只在 `utils/frame.ts` 与 `TitleBar` 里（spec D4）。
- 样式用 `src/style.css` 的 CSS 变量加组件 scoped CSS，深色只靠 `prefers-color-scheme`；不写内联 `<style>` 或 `style` 属性字符串（CSP `style-src 'self'`）。
- 图标用 `Icon` 组件，新图标加进 `src/components/icons.ts` 的形状表，不用 `v-html` 塞 SVG。
- 不可信 HTML 只经 `sanitizeArticleHtml` 进 `v-html`，而且只在 `ArticleBody`、`ArticleSummary` 两个组件里（对照视图的 `interleave` 往清洗结果里加纯文本译文后，结果再经一次 `sanitizeArticleHtml`）；eslint 在其余地方把 `vue/no-v-html` 当错误。
  渲染后再改正文（增强、Bionic Reading）只用 DOM API 在已有节点上改，不拼 HTML 字符串再解析。
  单独进 `src`/`href` 的后端地址（如列表缩略图）也要过同一条 URL 规则（`safeImageUrl`），外部链接走 `POST /api/browser/open`，不在窗口里跳转。
- 动态位置用 `:style` 对象绑定（经 CSSOM 写入，CSP 不拦）；颜色这类有限取值用样式类（如 `FeedBadge` 的 `c0`–`c7`）。
- 后端调用只经 `src/api.ts`；跨组件的状态放 Pinia setup store，组件只调 store 的动作。视图里的快照与卡片按 `reader` 的规则变化：
  操作只改显示状态，列表成员只在进入视图或点新文章横幅时换。自己的已读操作之后由 store 主动刷新计数，不等同步状态的 `rev`（意图写入不一定改 `rev`）。
- 可能被较新请求取代的异步结果（视图快照、卡片）在写入前比对发起时的序号，丢弃过期响应。
- 文案直接写中文；后端的英文错误串不直接显示，由前端映射成中文。

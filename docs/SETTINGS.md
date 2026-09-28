# 设置

设置清单只有一份：`internal/config/settings_schema.json`（spec D10）。本文不抄清单，只讲它怎么变成代码、怎么存取。

## Schema

每个键一项，文件里按设置面板的分组排列（freshrss → llm → translation → network → app → internal），缩进 2 空格；改动时保持这个顺序，
生成物才按键名排序。

```json
"llm_api_key": {
  "type": "string",
  "default": "",
  "category": "llm",
  "encrypted": true
}
```

| 字段 | 含义 |
| --- | --- |
| `type` | `string`、`int` 或 `bool`；存库时都是字符串，写入时按类型校验（`bool` 只收 `true` / `false`，`int` 须能被 `strconv.Atoi` 解析） |
| `default` | 默认值，类型须与 `type` 一致 |
| `category` | 面板分组；`internal` 表示只由后端写（窗口位置与大小），不进前端生成物 |
| `encrypted` | 凭据为 `true`，只能用在 `string` 上 |

模型配置的键是 `llm_*`，不是 `ai_*`（pitfall 27）。上次成功同步时间这类记录存库的 `meta` 表，不是设置。

## 生成器

```bash
go run tools/settings-generator/main.go
```

先校验 schema（类型、默认值、加密字段），再写出：

| 文件 | 内容 |
| --- | --- |
| `internal/config/defaults.json` | 默认值，由 `config.go` 嵌入 |
| `internal/config/config.go` | `Defaults` 结构体、`Get()`、`GetString(key)` |
| `internal/config/settings_keys.go` | 每个键的类型、分组、是否加密：`config.Lookup(key)`、`config.SettingsKeys()` |
| `frontend/src/types/settings.generated.ts` | 前端可编辑键的 `SettingsData` 接口与 `settingsDefaults`（不含 `internal`） |

生成物都不要手改。Go 文件经 `gofmt`，TS 文件按仓库的 prettier 风格写出，所以对未改动的 schema 重跑不产生 diff——出现 diff 就说明
schema 与生成物不同步。改了 schema 之后按 `.forge/config.md` 跑 Go 检查与前端检查。

## 存取（`internal/settings`）

设置存本地库的 `settings` 表（`key`、`value`）。`settings.New(db)` 返回 `Store`：

- `Load(ctx)` 返回 schema 里每个键：有存值用存值，否则用默认值。表里 schema 以外的键（例如旧版遗留键）被忽略并记一行日志，只记键名。
- `Get(ctx, key)` 取一个键；schema 以外的键返回 `*UnknownKeysError`。
- `Update(ctx, values)` 在一个事务里写入。只要有一个键不在 schema 里就整批拒收（`*UnknownKeysError`），值不符合类型也整批拒收
  （`*InvalidValueError`，不带值）；HTTP 层把这两种错误回 400。`internal` 键同样能写，前端能不能写由 HTTP 层决定。

## HTTP 接口

前端经 `GET /api/settings` 与 `POST /api/settings/update` 读写，规则在 `settings.Panel`（形状见 [Architecture](ARCHITECTURE.md) 的 API 路由一节）。与 `Store` 不同的地方：

- 值按 schema 类型以 JSON 字符串、布尔或整数收发，对应生成的 `SettingsData`；`internal` 键既不返回也不接受写入（400）。
- 凭据不回显：读出时为空串，另有 `saved_secrets` 列出已存值的键。面板只提交用户改过的键，没动的凭据不传；传空串表示清除。
- 写入 `proxy_*` 前先确认代理装得上，写后立即生效；写入 `freshrss_*` 后触发一次同步。

## 加密字段

`encrypted: true` 的键由 `Store` 写入前用 `crypto.Encrypt` 加密、读出时解密，调用方拿到的总是明文，空值存为空。

- Windows：DPAPI，当前用户范围，密文为 `dpapi:` + base64 的 DPAPI blob；别的用户或别的机器打不开。
- macOS：主机名等派生 PBKDF2 密钥，AES-256-GCM，前缀 `MrRSS-v1:`（沿用最初写入的前缀，已存的值才读得出）。
- 存值解密失败（库被拷到另一台机器或另一个用户下）时按空值返回并记日志，由用户重填。

# LiteRSS

桌面上的 FreshRSS 未读阅读器（Windows 与 macOS）。

派生自 [MrRSS](https://github.com/WCY-dt/MrRSS)，以 GPL-3.0 发布，见 [LICENSE](LICENSE)。

## 它做什么

只服务一条阅读动线：打开未读列表 → 扫一遍中文标题 → 点开感兴趣的文章（中文读正文，英文看 AI 摘要）
→ 值得细读或剪藏的在浏览器里打开 → 读过的都标为已读。

- 订阅、分类与已读状态都以 FreshRSS 为准。LiteRSS 只把你明确做过的操作（标已读 / 未读、全部标为已读）推给服务端，
  从不把在别处读掉的文章改回未读。加订阅、改分类在 FreshRSS 网页端做。
- 英文标题用百度翻译成中文显示在列表里，原文在详情页。
- 正文被截断的文章，点开后自动抓全文；点「摘要」用你配置的 OpenAI 兼容模型生成中文摘要。
- 本地保留抓取后 90 天内的文章，「全部」视图可以回看。
- 托盘、关闭到托盘、开机自启、窗口位置记忆；有新版本时在应用内一键更新。
- 正文与摘要里的脚本不执行，不加载任何外部字体或脚本，没有统计。凭据加密保存。

## 使用须知

- **FreshRSS 默认会删除旧的未读文章。** FreshRSS 的归档默认 `keep_unreads = false`：抓取超过 3 个月、或每个订阅源超过 200 篇的文章，
  即使未读也会被服务端清掉，LiteRSS 随后也看不到它们。要保留积压的未读，在 FreshRSS 的归档设置里改为保留未读，
  或调大保留期与数量。
- **新旧版不要同时连同一个账号运行。** 旧版 MrRSS 同步时会把别处的已读改回未读。LiteRSS 检测到旧版在运行时，
  侧栏底部会提示退出旧版。

## 从旧版 MrRSS 切换

LiteRSS 与旧版可以同时安装：安装目录、数据目录、单实例锁、开机自启项与本地端口都不同，互不覆盖。

1. 从托盘彻底退出旧版 MrRSS（旧版默认关闭到托盘，关窗口不等于退出）。
2. 安装并启动 LiteRSS。首次启动会自动导入旧版的文章、标题译文、摘要、凭据和设置；旧目录和旧数据库只读，不会被修改。
   如果这时旧版还在运行，导入会等到它退出后再进行。
3. 对照 FreshRSS 网页端核对未读列表；在 LiteRSS 里标几篇已读，确认网页端也变成已读。
4. 用几天没有问题后再卸载旧版。在此之前想退回：退出 LiteRSS、打开旧版即可，旧版启动后会从服务端拉回最新状态。

某个凭据在导入时解不开（例如换了电脑）会被跳过，同步或翻译失败时在设置里重新填写即可。

## 安装

从 [Releases](https://github.com/Mistakey/LiteRSS/releases/latest) 下载：

- Windows 安装版：`LiteRSS-{版本}-windows-amd64-installer.exe` / `LiteRSS-{版本}-windows-arm64-installer.exe`（按当前用户安装，默认 `%LOCALAPPDATA%\Programs\LiteRSS`，不需要管理员权限）
- Windows 便携版：`LiteRSS-{版本}-windows-{架构}-portable.zip`（解压即用，数据在程序旁的 `data` 文件夹）
- macOS：`LiteRSS-{版本}-darwin-universal.dmg`

有新版本时，Windows 安装版与 macOS 在设置「关于」或托盘提示里点一次「更新」即自动下载、校验（`SHA256SUMS`）、安装并重启；便携版请到发布页下载。

首次启动后在设置里填 FreshRSS 地址、用户名和 API 密码（在 FreshRSS 个人资料的 API 管理里设置，不是登录密码）。
标题翻译需要百度翻译的 App ID 与密钥，摘要需要一个 OpenAI 兼容的模型端点；不填也能正常阅读。

数据位置：

- Windows：`%APPDATA%\LiteRSS\`
- macOS：`~/Library/Application Support/LiteRSS/`
- 便携版：程序旁的 `data\`

## 从源码构建

需要 Go（版本见 `go.mod`）、Node.js 24、[Task](https://taskfile.dev/) 与同版本的 Wails v3 CLI，详见 [构建依赖](docs/BUILD_REQUIREMENTS.md)。

```bash
cd frontend && npm install && cd ..
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
task build      # 正式构建，产物在 build/bin/
task package    # 安装包
```

直接 `go build` 得到的是开发构建：它使用独立的身份与 exe 旁的数据目录，不会碰已安装的 LiteRSS。
开发与测试说明见 [AGENTS.md](AGENTS.md) 与 [测试指南](docs/TESTING.md)。

## 发版

推送 `vX.Y.Z`（预发布用 `vX.Y.Z-beta.N`）标签会触发 GitHub Actions 在 Windows 与 macOS 上构建并上传安装包，
发布说明取自 [CHANGELOG.md](CHANGELOG.md) 同版本一节。也可以在 Actions → `Release` → `Run workflow` 手动触发。

## 许可证

GPL-3.0，见 [LICENSE](LICENSE)。

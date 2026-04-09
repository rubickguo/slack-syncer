# slack同步器

`slack同步器` 是一个基于 [`slackdump`](https://github.com/rusq/slackdump)
二次开发的桌面工具，用来把 Slack 频道内容或指定线程同步到本地，方便归档、
检索、整理和二次处理。

这个仓库保留了上游 `slackdump` 的 GPL-3.0 许可证，并在此基础上增加了
Wails + React 图形界面，以及更适合日常使用的同步入口。

## 这个项目能做什么

- 通过图形界面登录 Slack
- 拉取当前工作区频道列表并按频道同步
- 直接粘贴 Slack 帖子或线程链接，按 URL 抓取上下文
- 将结果导出为 `HTML`、`Markdown`、`PDF`、`JSON` 或 `SQLite`
- 将线程按时间和标题整理成独立文件，便于沉淀到知识库或本地归档

## 项目结构

- `SlackSyncGUI/`
  桌面端 GUI，基于 Wails、React、TypeScript
- `cmd/`, `auth/`, `stream/`, `source/`, `internal/` 等目录
  上游 `slackdump` 的核心能力与依赖代码
- `ATTRIBUTION.md`
  说明上游来源和本仓库新增部分

## 当前定位

这是一个“带 GUI 的 Slack 本地同步工具”，不是对上游项目的简单改名。
仓库里包含上游代码，是为了让桌面端直接复用 `slackdump` 的登录、会话、
消息抓取和导出能力。

如果你只关心桌面端入口，优先看 `SlackSyncGUI/`。

## 本地开发

### 1. CLI 代码

仓库根目录是 Go 模块，保留了上游 `slackdump` 的源码结构。

```powershell
go test ./...
```

### 2. GUI

GUI 位于 `SlackSyncGUI/`：

```powershell
cd SlackSyncGUI
go test ./...
cd frontend
npm install
npm run build
```

如果你已经安装了 Wails，也可以在 `SlackSyncGUI/` 下执行对应的开发或打包命令。

## 使用说明

1. 输入工作区信息。
   支持输入 `myteam`、`myteam.slack.com` 或完整 `https://myteam.slack.com`。
2. 选择登录方式。
   可以手动填写 `d` Cookie，也可以通过浏览器登录辅助获取。
3. 进入控制台后，选择同步模式。
   可以按频道批量同步，也可以直接粘贴线程 URL。
4. 选择导出目录和格式。
5. 开始同步。

## 开源发布建议

如果你要在 GitHub 创建公开仓库，建议：

- 仓库展示名使用 `slack同步器`
- 仓库 slug 使用 ASCII，例如 `slack-syncer`
- 仓库描述可以写成：
  `A desktop Slack sync tool built on top of slackdump.`

## 许可证

本仓库沿用 GPL-3.0。详情见 [LICENSE](LICENSE) 和 [ATTRIBUTION.md](ATTRIBUTION.md)。

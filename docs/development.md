# 开发指南

## 本地运行

在 Windows x64 上安装 Go 1.27.3 或更新版本。在仓库根目录运行以下命令，开发数据会写入 `.local/dev`。

```powershell
go run . -data-dir .local/dev
```

`go tool mygo dev` 支持自动重启，并使用独立的开发数据目录。MyGo 与 Lobe MyGo 版本由 [go.mod](../go.mod) 固定。界面使用原生控件，布局和交互约定见 [UI 指南](../.agents/skills/lxmcl-ui/SKILL.md)。

## 配置与数据

默认数据目录为 `%LOCALAPPDATA%\org.caucraft.lxmcl`。

| 位置 | 内容 |
| --- | --- |
| `launcher.json` | 实例、启动选择、服务器收藏和全局设置 |
| `tasks.json` | 最近 24 条任务记录 |
| `cache/` | 共享游戏文件、Java 运行时和加载器安装日志 |
| `backups/instance-<id>/` | 实例存档的 ZIP 备份 |
| 实例游戏目录 | 存档、模组、资源包、光影包和 `launcher-logs/` |

CurseForge 查询和依赖下载需要 [API Key](https://console.curseforge.com/)。复制 [环境变量模板](../.env.example) 为 `.env`，填写 `LXMCL_CURSEFORGE_API_KEY` 后重启。进程环境变量优先，其次依序读取数据目录、工作目录和程序目录中的 `.env`。在页面上输入的密钥仅用于本次运行。请将密钥留在本机或 CI Secrets 中。

后端默认为简体中文。设置 `$env:LXMCL_LOCALE = 'en'` 可检查英文消息。新增文案时同步维护[语言文件](../internal/i18n/locales)的键和格式占位符。桌面界面使用中文。

## 服主整合包订阅

将导出的 `.mrpack` 和 `pack.json` 放到同一 HTTPS 站点，把清单地址发给玩家。更新时先上传新版整合包，再替换清单。以下 PowerShell 命令使用实际文件的大小和 SHA-256 生成清单。

```powershell
$pack = Get-Item .\instance-1-2.0.mrpack
@{ file = $pack.Name; size = $pack.Length; sha256 = (Get-FileHash $pack.FullName -Algorithm SHA256).Hash } |
    ConvertTo-Json | Set-Content -Encoding utf8NoBOM pack.json
```

## 验证

```powershell
go test ./...
go vet ./...
```

UI 用例在后台渲染。以下命令将资源与联机页面的截图保存到 `target/ui-check`。

```powershell
$env:LXMCL_UI_CAPTURE = 'target/ui-check'
go test -count=1 -run TestResourceAndServerEntrypoints .
Remove-Item Env:LXMCL_UI_CAPTURE
```

网络用例可注入 HTTP 传输，文件用例使用临时目录。系统窗控、输入法、在线账号授权和实际进入游戏需单独验收。模块职责与接口见[架构](architecture.md)。

## 打包与发布

```powershell
$env:NSISCONFDIR = (Resolve-Path packaging).Path
go tool mygo build -platform windows/amd64
```

[CI](../.github/workflows/ci.yml) 在 Windows 上测试、静态检查和打包，在 Linux 上测试后端。Windows 构建保留 14 天，包含便携版、安装包、许可归档、对应源码和 SHA-256 清单。源码包附带构建所需的 Go 依赖。

发布时更新 [mygo.json](../mygo.json) 的版本，推送对应的 `v主.次.修订` 标签。CI 成功后生成 Release 草稿。升级依赖时同步核对[第三方许可](../THIRD_PARTY_NOTICES.txt)。

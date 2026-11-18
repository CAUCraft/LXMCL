# 开发指南

## 运行与构建

推荐 Windows x64，Go 版本见 [`go.mod`](../go.mod)。以下命令使用独立的数据目录启动应用。

```powershell
go run . -data-dir .local/dev
```

也可运行 `go tool mygo dev` 进入开发模式。以下命令生成 Windows 便携版和安装程序。

```powershell
$env:NSISCONFDIR = (Resolve-Path packaging).Path
go tool mygo build -platform windows/amd64
```

产物位于 `target/mygo/windows-amd64/`，版本号见 [`mygo.json`](../mygo.json)。

## 配置与数据

默认数据目录为 `%LOCALAPPDATA%\org.caucraft.lxmcl`，也可从「设置 → 关于」打开。

| 路径 | 内容 |
| --- | --- |
| `launcher.json` | 设置、实例、账号选择和服务器收藏 |
| `tasks.json` | 最近 24 条任务 |
| `cache/` | 游戏、加载器、资源及 Java 缓存 |
| `cache/installer-logs/` | Forge / NeoForge 安装器日志 |
| `backups/instance-<id>/` | 存档备份 |
| 实例目录下的 `launcher-logs/` | 游戏进程日志 |

移除实例配置不会删除游戏目录。当前配置不存在时才导入旧配置；损坏文件不会被旧版数据覆盖。Windows 凭据使用当前用户的 DPAPI 加密，其他平台仅保留会话，不写明文凭据。

CurseForge 在线功能需要 [API Key](https://console.curseforge.com/)。复制 [`.env.example`](../.env.example) 为 `.env` 后填写 `LXMCL_CURSEFORGE_API_KEY`。环境变量优先；文件依次从数据目录、工作目录和程序目录查找，页面输入仅在当前会话覆盖。不要提交本地配置或凭据。

## 修改与验证

模块与并发约束见[架构](architecture.md)。桌面改动遵循[界面约定](../.agents/skills/lxmcl-ui/SKILL.md)。

按改动范围运行测试，优先复用现有用例。需要检查界面时，设置 `LXMCL_UI_CAPTURE` 后运行桌面测试。

```powershell
go test ./...
go vet ./...
$env:LXMCL_UI_CAPTURE = 'target/ui-capture'
go test . -count=1
Remove-Item Env:LXMCL_UI_CAPTURE
```

检查亮暗主题及 820 × 530 最小窗口。PR 的 Windows CI 将 MyGo 渲染截图保存为 `lxmcl-ui-screenshots`，保留 7 天。系统窗控、DPI、中文输入法、真实登录和进入游戏需在实机验证。

后端文案维护在 [`internal/i18n/locales`](../internal/i18n/locales)，中英文键和格式参数须一致。`LXMCL_LOCALE=en` 可切换后端消息，桌面标签目前为中文。保留 `errors.Is` 可识别的错误链，日志不得包含访问令牌。

## 提交与发布

PR 说明可见变化、兼容性影响和实测范围。

[CI](../.github/workflows/ci.yml) 在 Windows 运行全部测试、`go vet` 和打包，在 Linux 运行后端测试。Windows 构建产物保留 14 天，包含便携版、安装包、许可归档、含依赖的对应源码和 SHA-256 清单。

发布时更新 `mygo.json`，推送一致的 `v主.次.修订` 标签；CI 通过后生成 Release 草稿。发布前核对包内许可、校验清单及实机运行。依赖更新需同步检查[第三方许可](../THIRD_PARTY_NOTICES.txt)。

## 服主整合包订阅

将导出的 `.mrpack` 和 `pack.json` 放到同一 HTTPS 站点，把清单地址发给玩家。更新时先上传新版整合包，再替换清单。以下 PowerShell 命令使用实际文件的大小和 SHA-256 生成清单。

```powershell
$pack = Get-Item .\instance-1-2.0.mrpack
$manifest = @{ file = $pack.Name; size = $pack.Length; sha256 = (Get-FileHash $pack.FullName -Algorithm SHA256).Hash }
[IO.File]::WriteAllText((Join-Path $pack.Directory.FullName "pack.json"), ($manifest | ConvertTo-Json))
```

# 开发指南

## 运行与构建

推荐 Windows x64，Go 版本以 [`go.mod`](../go.mod) 为准。MyGo 与 Lobe MyGo 使用锁定依赖；没有 Node.js 或 WebView 构建步骤。

```powershell
go run . -data-dir .local/dev
go tool mygo dev
```

指定 `-data-dir` 可隔离测试数据；`mygo dev` 默认使用独立开发目录。应用运行不需要 Git LFS 视频。

```powershell
$env:NSISCONFDIR = (Resolve-Path packaging).Path
go tool mygo build -platform windows/amd64
```

产物位于 `target/mygo/windows-amd64/`。`NSISCONFDIR` 使用项目的安装器压缩配置。版本号统一在 [`mygo.json`](../mygo.json) 中维护。

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

职责划分和数据约束见[架构](architecture.md)。UI 使用 Lobe 组件，遵循[界面约定](../.agents/skills/lxmcl-ui/SKILL.md)，保留原 Logo、河谷首页和孪枭角色。文件、网络、子进程操作放在后台；界面结果必须对应提交时的实例和账号。

优先扩展现有用例，只为实际缺陷和关键行为补充回归。按改动范围运行测试；PR 的 CI 执行完整测试、静态检查和 Windows 打包。运行页的 `lxmcl-ui-screenshots` 提供 MyGo 测试窗口截图，保留 7 天。

```powershell
go test ./...
go vet ./...
$env:LXMCL_UI_CAPTURE = 'target/ui-capture'
go test . -count=1
Remove-Item Env:LXMCL_UI_CAPTURE
```

UI 测试采用 MyGo 软件渲染，覆盖导航、弹窗、亮暗主题和 820 × 530 最小窗口。截图放在 `target/`，不要提交截图、日志或测试数据。系统窗控、DPI、中文输入法、真实账号登录与进入游戏需在实机验证；创建 Java 进程不代表已进入游戏。

后端文案维护在 [`internal/i18n/locales`](../internal/i18n/locales)，中英文键和格式参数须一致。`LXMCL_LOCALE=en` 可切换后端消息，桌面标签目前为中文。保留 `errors.Is` 可识别的错误链，日志不得包含访问令牌。

## 提交与发布

一个提交完成一个功能或修复，附简短说明及验证结果。PR 描述说明可见行为、兼容性影响和测试范围；历史进度与验收记录放在 PR，不累积到项目文档。

[CI](../.github/workflows/ci.yml) 在 Windows 运行全部测试、`go vet` 和打包，在 Linux 运行后端测试。Windows 构建产物保留 14 天，包含便携版、安装包、许可归档、含依赖的对应源码和 SHA-256 清单。

发布时更新 `mygo.json`，推送一致的 `v主.次.修订` 标签；CI 通过后生成 Release 草稿。发布前核对包内许可、校验清单及实机运行。依赖更新需同步检查[第三方许可](../THIRD_PARTY_NOTICES.txt)。

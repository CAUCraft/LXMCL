# 开发指南

当前目标是 Windows x64。使用 Go 1.27.3 或以上；MyGo 和 Lobe MyGo 的版本由 `go.mod` 固定。原生运行不需要 CGO、Bun、Node.js、Rust 或 Git LFS 视频。

```powershell
go run .
go tool mygo dev
go test ./...
go vet ./...
go build -o target/lxmcl.exe .
# 生成 Windows 可执行文件及安装包
go tool mygo build -platform windows/amd64
```

`go tool mygo dev` 会在修改 Go 文件后重新构建并重启应用，默认使用数据目录下的 `dev` 子目录。如果本机设置了不存在的 `GOTMPDIR`，请清除该设置，或让它指向已有的临时目录。用 `go run . -data-dir .local/mygo-check` 可指定独立的验收目录。

## 数据

| 文件或目录 | 用途 |
| --- | --- |
| `%LOCALAPPDATA%\org.caucraft.lxmcl\launcher.json` | 新版设置、实例及启动选择 |
| 同目录的 `launcher-settings.json` 和 `instances.json` | 旧版数据来源；新版配置不存在时只读导入 |
| 同目录的 `cache` | 各实例共用的官方客户端、运行库和资源 |
| 同目录的 `tasks.json` | 最近 24 次下载、校验或启动记录 |
| 实例目录 | 存档、游戏设置和 `launcher-logs` |

移除实例仅删除配置记录。更改默认游戏目录只影响新实例。启动时若发现缓存缺损，会尝试从官方源修复一次；也可在实例详情主动下载并校验。任务页可以查看游戏日志末尾，或交给系统打开完整文件。

## 验证

- UI 测试用 MyGo 的软件渲染器检查实例导航与重命名弹窗，不启动操作系统窗口。可用 `LXMCL_UI_CAPTURE` 指定本机截图目录。
- 原生窗控、缩放、键盘和输入法需要在系统窗口中验证。
- Java 探测实际运行本机 Java。下载安装使用 Mojang 的版本清单和文件哈希。联机、Microsoft 登录和 Mod 加载不在当前实现范围。

## CI/CD

[GitHub Actions](https://github.com/CAUCraft/LXMCL/actions) 对提交到 `main`、`codex/**` 的改动以及发往 `main` 的 PR 执行检查。Windows x64 检查 Go 格式、依赖完整性、测试和 `go vet`，然后打包便携版与安装程序；Linux 并行运行启动器核心测试。两个作业都从 `go.mod` 读取 Go 版本，测试无需真实游戏账户或图形桌面。

Windows 作业的 `lxmcl-windows-amd64` 产物包含两个 EXE、第三方许可证压缩包和 `SHA256SUMS.txt`，保留 14 天。许可证也嵌入便携版，并随安装程序安装。调整依赖时，按 [第三方清单](../THIRD_PARTY_NOTICES.txt) 核对实际打包的版本与许可证原文。发布时在 `mygo.json` 更新版本，再推送相同版本的 `v主.次.修订` 标签。通过检查后，流水线创建 GitHub Release 草稿。

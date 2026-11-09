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

`go tool mygo dev` 会在修改 Go 文件后重新构建并重启应用。如果本机设置了不存在的 `GOTMPDIR`，请清除该设置，或让它指向已有的临时目录。用 `go run . -data-dir .local/mygo-check` 可隔离本地验收数据。

## 数据

| 文件或目录 | 用途 |
| --- | --- |
| `%LOCALAPPDATA%\org.caucraft.lxmcl\launcher.json` | 新版设置、实例及启动选择 |
| 同目录的 `launcher-settings.json` 和 `instances.json` | 旧版数据来源；新版配置不存在时只读导入 |
| 同目录的 `cache` | 各实例共用的官方客户端、运行库和资源 |
| 实例目录 | 存档、游戏设置和 `launcher-logs` |

移除实例仅删除配置记录。更改默认游戏目录只影响新实例。删除或移动共用缓存后，可以在实例详情重新下载并校验。

## 验证

- UI 测试用 MyGo 的软件渲染器检查实例导航与重命名弹窗，不启动操作系统窗口。可用 `LXMCL_UI_CAPTURE` 指定本机截图目录。
- 原生窗控、缩放、键盘和输入法需要在系统窗口中验证。
- Java 探测实际运行本机 Java。下载安装使用 Mojang 的版本清单和文件哈希。联机、Microsoft 登录和 Mod 加载不在当前实现范围。

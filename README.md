# LXMCL

CAUCRAFT 的 Minecraft Java 版原生启动器，基于 Go、MyGo 和 Lobe MyGo，支持 1.20.1 及之后的正式版本。

## 下载

Windows x64 开发版在 [GitHub Actions](https://github.com/CAUCraft/LXMCL/actions/workflows/ci.yml?query=branch%3Amain)。打开最近一次成功的 `main` 构建，在 **Artifacts** 下载 `lxmcl-windows-amd64`。解压后可直接运行 `LXMCL.exe`，也可使用其中的安装程序。发布版本见 [Releases](https://github.com/CAUCraft/LXMCL/releases)。

## 快速开始

1. 在首页点「切换账号」，选择 Microsoft、第三方皮肤站（Yggdrasil）或离线账号。
2. 打开「下载 → 游戏版本」，选择原版、Fabric、Forge、NeoForge 或 Quilt，点「安装实例」。
3. 在主页选择实例，点「启动游戏」或「下载并启动」。首次启动会校验并补齐游戏文件。

Windows x64 会优先使用兼容的本机 Java，必要时下载官方运行时。也可在设置中指定 Java。启动失败后，可在任务中心查看日志或重试。

## 常用功能

- 在**实例页**调整启动设置、更换游戏版本和加载器、复制实例和管理模组、资源包、光影。可将本地文件拖入当前分类、批量启停、自选排序，或按实例版本从 Modrinth 添加资源并查看前置。下载页也支持只保存文件。
- **整合包**可从 Modrinth 或 CurseForge 在线安装，也可导入本地 `.mrpack` 或 CurseForge `.zip`。CurseForge 在线查询及依赖下载需要 API Key。
- 在**游戏文件**页创建存档 ZIP，从最近三份备份或本地 ZIP 恢复世界。整份 `saves` 目录可按世界选择恢复。单世界 ZIP 若直接包含 `level.dat`，则以压缩包文件名恢复。同名世界另存为副本。
- 在**账号中心**预览皮肤和披风，Microsoft 账号可上传 PNG 皮肤。通过**联机**页收藏、编辑或临时直连服务器，查看公告、版本、人数和延迟。本次运行的联机实例可跟随主页或单独指定。
- 在**日志与崩溃报告**中搜索、筛选、复制和导出启动日志、游戏日志（含调试及压缩归档）和崩溃报告。

| 快捷键 | 操作 |
| --- | --- |
| Ctrl+K | 搜索实例 |
| Ctrl+J | 任务中心 |
| Ctrl+F | 在当前日志或崩溃报告中搜索 |
| F2 | 重命名当前浏览实例 |
| Esc | 关闭弹窗或返回 |
| F1 | 关于与版本信息 |

## 开发

安装 [Go](https://go.dev/dl/)（版本见 [go.mod](go.mod)），在仓库根目录运行 `go run . -data-dir .local/dev`。构建与测试见[开发指南](docs/development.md)，模块边界见[架构](docs/architecture.md)。

## 反馈与贡献

欢迎提交 PR，或在 [Issues](https://github.com/CAUCraft/LXMCL/issues) 中附上启动器版本、系统、复现步骤和相关日志。分享日志前请移除个人凭据。

## 许可

Copyright © 2026 CAUCRAFT and LXMCL contributors.

LXMCL 当前源码与文档按 [GPLv3（仅第 3 版）](LICENSE) 发布。此前以 [MIT](licenses/lxmcl-legacy-mit.txt) 发布的版本仍可按原许可使用。[第三方组件](THIRD_PARTY_NOTICES.txt)和[美术素材](ui/assets/README.md)各自沿用原许可。

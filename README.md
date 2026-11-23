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

- 在**实例页**调整启动设置、更换游戏版本和加载器、复制实例和管理模组、资源包、光影。模组列表可离线读取 JAR 名称与版本，支持按模组 ID 搜索。可将本地文件拖入当前分类、批量启停、自选排序，或按实例版本从 Modrinth 添加资源。在「下载 → 模组」切换到 CurseForge，填写 API Key 后可查看必需前置并安装所选文件。下载页支持保存到文件夹、加入队列，重启后可继续下载。
- **整合包**可从 Modrinth 或 CurseForge 在线安装，也可导入本地 `.mrpack` 或 CurseForge `.zip`。可在实例菜单导出 .mrpack，或从[服主订阅](docs/development.md#服主整合包订阅)安装并预览后续更新。更新保留同名资源的启停状态。模组更换文件名时按清单 ID 匹配。CurseForge 在线查询及依赖下载需要 API Key。
- 在实例菜单的**更换图标**中预览本地 PNG，应用到实例列表和概览，也可恢复加载器默认图标。
- 在实例菜单的**设置分组**中创建或选取分组，也可将实例移回未分组。实例列表支持按分组、搜索词和缓存状态组合筛选，重启后会记住分组选择。
- 实例列表可选默认顺序、按名称、最近游玩或累计时长排列，记住上次选择的顺序。
- 在**实例 → 概览**查看上次游玩和累计时长。启动器在本机记录游戏进程实际运行的时间，先关闭启动器时计到关闭为止。
- 在**实例 → 概览**添加多行备注，记录进度、坐标或下次游玩的计划。
- 在**实例 → 截图**按修改时间浏览游戏截图，预览原图比例和分辨率，或打开当前实例的截图文件夹。复制实例时可单独勾选「游戏截图」，将图库中的 PNG 连同原修改时间带入副本。
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

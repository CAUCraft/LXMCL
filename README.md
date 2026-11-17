# LXMCL

CAUCRAFT 的 Minecraft Java 版启动器，使用 Go、MyGo 和 Lobe MyGo 构建原生桌面界面。

## 下载

Windows x64：从 [GitHub Actions](https://github.com/CAUCraft/LXMCL/actions/workflows/ci.yml?query=branch%3Amain) 的成功构建下载开发版。正式版发布后见 [GitHub Releases](https://github.com/CAUCraft/LXMCL/releases)。

面向 Minecraft Java 版 1.20.1 及之后的正式版本。Windows x64 可按游戏元数据自动安装所需 Java，也可在设置中指定已有 Java。

- **实例**：从「下载 → 游戏版本」选择 Minecraft 版本和加载器，可直接安装原版、Fabric、Forge、NeoForge 或 Quilt。实例页可校验文件、调整启动设置；从实例菜单创建副本时，可选择一并复制模组、资源包、光影、配置和服务器列表。
- **整合包**：从 Modrinth 或 CurseForge 保存整合包文件，也可直接安装为独立实例，或导入本地 `.mrpack` 和 CurseForge `.zip`。CurseForge 在线查询和依赖下载需要 API Key。
- **资源**：「下载」可浏览 Modrinth 模组、资源包和光影包，并把选中文件保存到自选文件夹。从实例的资源页点「添加」会按该实例筛选并安装选中文件。需要前置时，可从「查看前置」跳转并自行下载。本地资源支持多文件导入、搜索、状态筛选、按名称 / 修改时间 / 大小排序、批量启停和清单复制。
- **存档**：从「实例 → 游戏文件」备份为 ZIP；恢复前可查看归档中的世界，选择恢复全部或其中一个。同名世界会另存为副本。
- **账号与服务器**：Microsoft、兼容 Yggdrasil 的第三方皮肤站与离线账号，支持皮肤头像、可旋转的 3D 预览、服务器收藏与状态查询（公告、版本、人数和延迟）。「联机」可编辑收藏或临时直连，并单独选择本次使用的实例。Microsoft 账号可上传 PNG 皮肤；皮肤站账号在站内修改外观。
- **诊断**：Java 检测、下载进度和任务历史；「游戏文件 → 日志与崩溃报告」可查看实例最近的启动日志、`latest.log` 和崩溃报告。预览支持刷新、搜索、警告 / 错误筛选、复制与导出，并保留对应堆栈上下文。

未指定 Java 时，先检查系统和已安装的运行时；找不到兼容版本才下载 Mojang 官方运行时。指定的 Java 不可用时会报告错误，保留用户的选择。

## 快速开始

1. 打开首页的「切换账号」，选择 Microsoft、第三方皮肤站或离线身份。
2. 在「下载 → 游戏版本」选择版本和加载器，安装实例。
3. 回到首页确认实例与账号，点击「启动游戏」。首次启动会检查并补齐必要文件。

「已缓存」只代表检测到游戏缓存，启动时仍会校验。失败后从任务中心查看详情、复制错误或重试；实例或账号已变化时，请检查设置后重新发起。

| 快捷键 | 操作 |
| --- | --- |
| Ctrl+K | 搜索实例 |
| Ctrl+J | 任务中心 |
| F2 | 重命名当前浏览实例 |
| Esc | 关闭弹窗或返回 |
| F1 | 关于与版本信息 |

## 开发

安装 [Go 1.27.3 或更新版本](https://go.dev/dl/)，在仓库根目录运行 `go run .`。构建、测试与发布见[开发指南](docs/development.md)。

## 反馈与贡献

欢迎通过 [Issues](https://github.com/CAUCraft/LXMCL/issues) 报告问题，附上「关于」中的版本、系统、复现步骤及相关日志；提交前移除个人凭据。功能改动请附必要验证，贡献流程与发布说明见[开发指南](docs/development.md)，模块边界见[架构](docs/architecture.md)。

## 许可

Copyright © 2026 CAUCRAFT and LXMCL contributors.

LXMCL 当前源码与文档按 [GPLv3（仅第 3 版）](LICENSE) 发布。此前以 [MIT](licenses/lxmcl-legacy-mit.txt) 发布的版本仍可按原许可使用。[第三方组件](THIRD_PARTY_NOTICES.txt)和[美术素材](ui/assets/README.md)各自沿用原许可。

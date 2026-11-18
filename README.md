# LXMCL

CAUCRAFT 的 Minecraft Java 版启动器，使用 Go、MyGo 和 Lobe MyGo 构建原生桌面界面。

## 下载

Windows x64：从 [GitHub Actions](https://github.com/CAUCraft/LXMCL/actions/workflows/ci.yml?query=branch%3Amain) 的成功构建下载开发版。可在「设置 → 关于」检查正式版和查看更新说明。正式版发布后见 [GitHub Releases](https://github.com/CAUCraft/LXMCL/releases)。

面向 Minecraft Java 版 1.20.1 及之后的正式版本。Windows x64 可按游戏元数据自动安装所需 Java，也可在设置中指定已有 Java。

- **实例**：安装原版、Fabric、Forge、NeoForge 或 Quilt。在「实例 → 概览 → 加载器」切换已有实例的加载器与构建版本。
- **整合包**：从 Modrinth、CurseForge 或本地归档安装。选择已有实例即可预览并应用更新，保留世界和玩家修改的文件。CurseForge 在线功能需要 API Key。
- **服主订阅**：在「下载 → 整合包 → 服主订阅」输入地址。安装后从实例概览检查更新。发布方法见[服主整合包订阅](docs/development.md#服主整合包订阅)。
- **模组与资源**：从 Modrinth 安装到实例或保存到文件夹，可排队下载、查看前置和重试。在「下载任务」调整顺序，重启后可继续未完成的队列。实例资源页支持导入、启停、筛选和按文件内容检查模组更新。
- **复制、导出与备份**：从实例菜单复制或导出 `.mrpack`，可勾选资源、配置和存档。世界也可单独备份为 ZIP，恢复时保留已有同名世界。
- **账号与联机**：支持 Microsoft、兼容 Yggdrasil 的皮肤站与离线账号。可预览皮肤、上传 Microsoft 账号皮肤、收藏服务器或直接输入地址联机。
- **诊断**：从失败通知打开对应任务，复制错误详情。游戏日志标出已识别的 Java、内存与模组依赖错误，可跳回实例处理。

未指定 Java 时，先检查系统和已安装的运行时；找不到兼容版本才下载 Mojang 官方运行时。指定的 Java 不可用时会报告错误，保留用户的选择。

## 开发

安装 [Go 1.27.3 或更新版本](https://go.dev/dl/)，在仓库根目录运行 `go run .`。构建、测试与发布见[开发指南](docs/development.md)。

## 许可

Copyright © 2026 CAUCRAFT and LXMCL contributors.

LXMCL 当前源码与文档按 [GPLv3（仅第 3 版）](LICENSE) 发布。此前以 [MIT](licenses/lxmcl-legacy-mit.txt) 发布的版本仍可按原许可使用。[第三方组件](THIRD_PARTY_NOTICES.txt)和[美术素材](ui/assets/README.md)各自沿用原许可。

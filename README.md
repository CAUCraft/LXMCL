# LXMCL

CAUCRAFT 的 Minecraft Java 版启动器，使用 Go、MyGo 和 Lobe MyGo 构建原生桌面界面。

## 下载

Windows x64：从 [GitHub Actions](https://github.com/CAUCraft/LXMCL/actions/workflows/ci.yml?query=branch%3Amain) 的成功构建下载开发版。可在「设置 → 关于」检查正式版和查看更新说明。正式版发布后见 [GitHub Releases](https://github.com/CAUCraft/LXMCL/releases)。

面向 Minecraft Java 版 1.20.1 及之后的正式版本。Windows x64 可按游戏元数据自动安装所需 Java，也可在设置中指定已有 Java。

- **实例**：从「下载 → 游戏版本」选择 Minecraft 版本和加载器，可直接安装原版、Fabric、Forge、NeoForge 或 Quilt。在已有实例的「概览 → 加载器」中可切换加载器及其构建版本，下次启动时校验所需文件。
- **整合包**：从 Modrinth 或 CurseForge 保存整合包文件，也可直接安装为独立实例，或导入本地 `.mrpack` 和 CurseForge `.zip`。在整合包预览中选择已有实例，可先核对新增、替换、移除和保留的文件，再应用更新。更新保留世界和本地修改。服主可发布[订阅清单](docs/development.md#服主整合包订阅)，玩家在「下载 → 整合包 → 服主订阅」安装后，可从实例概览检查更新。CurseForge 在线查询和依赖下载需要 API Key。
- **资源**：在「下载」浏览 Modrinth 模组、资源包和光影包，将文件保存到自选文件夹，或加入队列，在「下载 → 任务」逐项下载、查看结果和重试。从实例的「资源」页可安装兼容文件，按文件名和启用状态筛选本地资源。在实例的模组页点「检查更新」，可按文件内容识别 Modrinth 版本，勾选并更新兼容版本。更新保留原文件名和停用状态。需要前置时，可从「查看前置」选择下载。
- **复制与导出**：从实例菜单创建副本或导出可再次导入的 `.mrpack`。模组与资源包、配置与脚本、世界存档可分别勾选。
- **存档**：备份为 ZIP，恢复时保留已有同名世界。
- **账号与联机**：支持 Microsoft、兼容 Yggdrasil 的皮肤站和离线账号，可预览 3D 皮肤。Microsoft 账号可上传 PNG 皮肤，皮肤站账号在站内更换外观。在「联机」输入地址即可使用当前启动实例直连，也可收藏常用服务器并查询状态。
- **诊断**：检测 Java，查看下载进度和游戏日志。日志窗口标出 Java 版本、堆内存或模组依赖错误，附上报错原文和对应实例的设置入口。从失败通知可以打开对应任务并复制错误详情。

未指定 Java 时，先检查系统和已安装的运行时；找不到兼容版本才下载 Mojang 官方运行时。指定的 Java 不可用时会报告错误，保留用户的选择。

## 开发

安装 [Go 1.27.3 或更新版本](https://go.dev/dl/)，在仓库根目录运行 `go run .`。构建、测试与发布见[开发指南](docs/development.md)。

## 许可

Copyright © 2026 CAUCRAFT and LXMCL contributors.

LXMCL 当前源码与文档按 [GPLv3（仅第 3 版）](LICENSE) 发布。此前以 [MIT](licenses/lxmcl-legacy-mit.txt) 发布的版本仍可按原许可使用。[第三方组件](THIRD_PARTY_NOTICES.txt)和[美术素材](ui/assets/README.md)各自沿用原许可。

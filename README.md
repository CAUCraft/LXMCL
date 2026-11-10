# LXMCL

CAUCRAFT 的 Minecraft Java 版启动器，使用 Go、MyGo 和 Lobe MyGo 构建原生桌面界面。

## 下载

Windows x64：从 [GitHub Actions](https://github.com/CAUCraft/LXMCL/actions/workflows/ci.yml?query=branch%3Amain) 的成功构建下载开发版。正式版发布后见 [GitHub Releases](https://github.com/CAUCraft/LXMCL/releases)。

面向 Minecraft Java 版 1.20.5 及之后的正式版本，需自备兼容 Java。

- **实例**：从「下载 → 游戏版本」选择 Minecraft 版本及原版或 Fabric，直接创建实例并下载游戏。实例页可切换版本、校验文件和配置启动设置。
- **资源**：从「下载」或实例的资源页进入 Modrinth，按目标实例筛选模组、资源包和光影包。模组自动下载必要依赖，本地资源可导入、停用或恢复。
- **存档**：备份为 ZIP，恢复时保留已有同名世界。
- **账号与服务器**：Microsoft、兼容 Yggdrasil 的第三方皮肤站与离线账号，支持皮肤头像、可旋转的 3D 预览、服务器收藏和直连。Microsoft 账号可上传 PNG 皮肤；皮肤站账号在站内修改外观。
- **诊断**：Java 检测、下载进度、任务历史和游戏日志。

Forge／NeoForge／Quilt、整合包导入和 Java 自动安装尚未接入。

## 开发

安装 [Go 1.27.3 或更新版本](https://go.dev/dl/)，在仓库根目录运行 `go run .`。构建、测试与发布见[开发指南](docs/development.md)。

## 许可

Copyright © 2026 CAUCRAFT and LXMCL contributors.

LXMCL 当前源码与文档按 [GPLv3（仅第 3 版）](LICENSE) 发布。此前以 [MIT](licenses/lxmcl-legacy-mit.txt) 发布的版本仍可按原许可使用。[第三方组件](THIRD_PARTY_NOTICES.txt)和[美术素材](ui/assets/README.md)各自沿用原许可。

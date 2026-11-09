# LXMCL

CAUCRAFT 的 Minecraft Java 版启动器，使用 Go、MyGo 和 Lobe MyGo 构建原生桌面界面。

## 下载

Windows x64：从 [GitHub Actions](https://github.com/CAUCraft/LXMCL/actions/workflows/ci.yml?query=branch%3Amain) 的成功构建下载开发版。正式版发布后见 [GitHub Releases](https://github.com/CAUCraft/LXMCL/releases)。

LXMCL 支持独立实例、原版游戏下载与校验、Java 选择和启动日志。当前主要支持 1.20.5 及之后的原版正式版本，需自备匹配的 Java（例如 1.21.1 使用 Java 21）。暂仅支持离线单人游玩。

## 开发

安装 [Go 1.27.3 或更新版本](https://go.dev/dl/)，在仓库根目录运行 `go run .`。构建、测试与发布见[开发指南](docs/development.md)。

## 许可

LXMCL 源码与文档采用 [MIT 许可证](LICENSE)。[第三方组件](THIRD_PARTY_NOTICES.txt)和[美术素材](ui/assets/README.md)各自沿用原许可。

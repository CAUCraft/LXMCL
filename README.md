# LXMCL

CAUCRAFT 的 Minecraft Java 版启动器。当前版本使用 **Go、MyGo 和 Lobe MyGo**：桌面界面由 GPU 原生绘制，使用 Lobe 的标签页、按钮、表单、弹窗和进度条。无需 Rust、Node.js 或 WebView。

- 创建、选择、搜索、重命名和移除实例配置，每个实例使用独立游戏目录。
- 从 Mojang 下载并校验原版游戏文件、运行库及资源。已下载并校验的文件可复用，下载可以取消。
- 检测本机 Java、配置最大内存、启动客户端、查看进程状态和打开日志目录。

游戏流程优先支持 Windows x64 上的 **1.20.5 及之后的原版正式版本**。请准备匹配游戏版本的 Java（例如 1.21.1 使用 Java 21）。当前提供离线单人游玩；Microsoft 登录、正版联机、Mod 加载器和 Java 自动安装未接入。

安装 [Go 1.27.3 或以上](https://go.dev/dl/) 后，在仓库根目录运行：

```powershell
go run .
# 自动编译并重启的开发模式
go tool mygo dev
```

更多命令、数据目录和验证方式见[开发指南](docs/development.md)。代码边界见[架构](docs/architecture.md)，界面约定见 [UI Skill](.agents/skills/lxmcl-ui/SKILL.md)。

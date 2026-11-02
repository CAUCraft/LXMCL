# LXMCL

面向 CAUCRAFT 的 Minecraft 启动器，使用 **Rust + Slint** 构建原生桌面界面。通过插件装配业务服务、功能扩展和完整界面 Shell。

## 项目状态

项目处于开发阶段。已接入本机设置持久化、实例配置创建与重命名、Java 检测与路径验证、游戏目录打开和只读目录检查；支持原生四主 Tab、皮肤预览、可选动态壁纸与离线帮助。

**认证、游戏下载与安装、Java 安装、游戏启动、联机及网络诊断目前仍是演示流程。** 创建实例仅保存配置，不安装游戏文件。

插件当前在编译时链接，由 `product.toml` 选择已注册的提供者；完整 Shell 可替换。默认 Shell 的扩展动作集中在「设置 → 扩展」。运行时安装插件和各页面扩展槽位尚未实现。

当前优先完善通用启动器基础。社团服务器、公告与专属入服流程后续作为插件实现，不在默认界面提供演示入口。

## 运行

当前主要开发环境为 Windows x64 / MSVC。安装 Rust、Visual Studio C++ Build Tools / Windows SDK 与 Git LFS：

```powershell
git lfs install
git clone https://github.com/CAUCraft/LXMCL.git
cd LXMCL
git lfs pull
cargo run --locked
```

工具链和依赖由仓库锁定。静态背景随仓库提供；七段可选视频由 Git LFS 管理，缺少视频仍能运行。动态壁纸需要 PATH 中或可执行文件旁的 FFmpeg。素材来源与许可范围见[素材说明](ui/assets/README.md)。

## 开发

- [开发指南](docs/development.md)：环境、源码地图、数据目录与检查方式。
- [架构设计](docs/architecture.md)：当前实现、前后端边界与插件架构迁移。
- [插件开发](docs/plugins.md)：现有 SDK、提供者、动作贡献和 Shell 替换。
- [UI Skill](.agents/skills/lxmcl-ui/SKILL.md)：日光 04 的完整设计规范、交互要求和参考原型。
- [协作约定](AGENTS.md)：代码与文档维护规则。

应用逻辑与新增工具使用 Rust，界面使用 Slint。Skill 中的 HTML 原型仅供设计参考，不参与应用运行。

# 开发指南

LXMCL 是 Rust + Slint 原生桌面程序。业务与插件设计见[架构设计](architecture.md)，当前 SDK 用法见[插件开发](plugins.md)。全部 UI 规范与参考原型位于 [UI Skill](../.agents/skills/lxmcl-ui/SKILL.md)。

## 环境与运行

主要开发环境为 Windows x64 / MSVC，需要 Visual Studio C++ Build Tools 和 Windows SDK。其他平台的窗口、路径与媒体行为仍需单独验证。

[rust-toolchain.toml](../rust-toolchain.toml) 固定 Rust 1.92.0，包含 `rustfmt`、`clippy`、`rust-src` 与 `rust-analyzer`。VS Code 安装 rust-analyzer 扩展并打开仓库根目录，使用项目工具链配套的分析器；无需设置个人机器上的绝对路径。依赖由 Cargo.lock 锁定，Slint 当前为 1.18.1。

```powershell
git lfs install
git lfs pull
cargo run --locked
```

静态资源可直接运行；动态壁纸另需 FFmpeg。视频来源与生成方式见[素材说明](../ui/assets/README.md)。自维护应用逻辑和新增工具使用 Rust；设计原型不属于应用运行依赖。

## 代码地图

| 位置 | 当前职责 |
|---|---|
| [launcher-runtime](../crates/launcher-runtime/src/lib.rs) | 工厂、依赖拓扑、服务与贡献注册、作用域清理、有界 worker |
| [launcher-api](../crates/launcher-api/src/lib.rs) | 设置、实例、Java、Shell 契约，以及语义 UI 贡献 |
| [launcher-settings](../crates/launcher-settings/src/lib.rs)、[launcher-instances](../crates/launcher-instances/src/lib.rs)、[launcher-java](../crates/launcher-java/src/lib.rs) | JSON 设置、JSON 实例记录、本机 Java 提供者 |
| [main.rs](../src/main.rs)、[composition.rs](../src/composition.rs)、[product.toml](../product.toml) | 产品装配、默认 Shell、窗口与退出生命周期 |
| [shell_plugins.rs](../src/shell_plugins.rs)、[shell_extensions.rs](../src/shell_extensions.rs) | 工具插件及贡献到 Slint 的适配 |
| [application](../src/application/mod.rs) | 应用插件、实例重命名用例、后台写入与退出 Drain，不依赖 Slint |
| [presentation/rename.rs](../src/presentation/rename.rs) | 重命名表单会话、请求关联、冲突恢复与展示投影 |
| [settings_bridge.rs](../src/settings_bridge.rs)、[instances_bridge.rs](../src/instances_bridge.rs)、[java_bridge.rs](../src/java_bridge.rs) | 尚未迁移的设置、实例加载 / 创建与 Java 后台 I/O |
| [controller.rs](../src/controller.rs)、[state.rs](../src/state.rs) | 展示模型、导航、演示状态与现有业务调用 |
| [contracts.rs](../src/contracts.rs)、[preview_services.rs](../src/preview_services.rs)、[journey.rs](../src/journey.rs) | 准备契约、演示实现和向导适配 |
| [ui/](../ui) | Slint Shell、页面、模型与共享控件 |
| [wallpaper.rs](../src/wallpaper.rs)、[skin_preview.rs](../src/skin_preview.rs) | 可选视频与皮肤渲染 |
| [examples/](../examples) | 独立 Cargo 工作区：替换设置提供者、独立产品窗口 |

实例重命名已贯通独立应用用例与 Rust 展示层：按捕获的 ID / 修订保存，失败保留草稿；关闭表单后写入继续，结果不改变当前选择或导航。其余流程沿架构设计逐项迁移，不继续把文件操作和业务规则加入大型 controller。

实例列表区分启动选择与浏览详情：单击选择、Enter / 双击管理、F2 重命名。详情与目录操作绑定查看的实例，返回保留搜索和滚动。默认联机页仅显示未启用状态，社团服务留给后续插件；首次向导只走通用本地流程。

## 当前能力

| 真实实现 | 边界 |
|---|---|
| 设置与界面偏好保存 | 两份数据分别落盘，不是跨文件事务 |
| 实例配置列表、创建、选择与重命名 | 只保存配置；不安装游戏文件；删除入口仍为演示 |
| Java 候选检测、手动验证、选择保存 | 运行本机 Java 探测，单次超时 5 秒；不安装 Java、不修改系统环境 |
| 打开实例目录、扩展只读目录检查 | 检查真实路径；批量检查最多 100 个实例，不判断游戏完整性 |
| 原生窗口、皮肤、可选壁纸、帮助与搜索 | Windows 窗控合入导航顶栏，其他平台保留系统装饰；轻透表面由应用绘制 |

认证、游戏下载 / 安装、Java 安装、资源管理、游戏启动、服务器、联机房间、网络诊断仍是演示。来源标记与错误恢复入口应跟随真实服务接入逐项更新。

## 产品配置与本机数据

默认产品启用 `settings-json`、`instances-json`、`java-local`、`local-tools`、`launcher-application`、`slint-shell` 六个提供者。`product.toml` 只能选择已链接并注册的工厂；程序从可执行文件目录向上最多六层查找该文件。

| 数据 | Windows 默认位置 |
|---|---|
| 业务设置 | `%LOCALAPPDATA%\org.caucraft.lxmcl\launcher-settings.json` |
| 实例记录 | `%LOCALAPPDATA%\org.caucraft.lxmcl\instances.json` |
| Shell 外观偏好 | `%LOCALAPPDATA%\LXMCL\ui-preferences.json` |
| 新用户的游戏根目录 | 产品数据目录下的 `games`，已保存值保持不变 |

产品 ID 决定业务数据目录，改 ID 不会自动迁移旧数据。路径策略由产品负责；不要把开发者的绝对路径写入共享配置。

## 按改动验证

纯文档只检查链接、格式及忽略规则。Rust 改动选择相关 crate 的测试；公共契约、依赖或产品装配改动再覆盖受影响消费者。常用命令：

```powershell
cargo fmt --all --check
cargo test --locked -p launcher-runtime
cargo clippy --workspace --locked --all-targets -- -D warnings
cargo build --locked
```

需要完整回归时用 `cargo test --workspace --locked`。示例是独立工作区，API 变动时单独检查：

```powershell
cargo check --manifest-path examples/minimal-launcher/Cargo.toml --locked
```

Windows 链接前退出运行中的 lxmcl.exe。改 Slint 后核对涉及的原生窗口操作；编译成功不代表交互或流畅度通过。慢速媒体测试只在相关改动时显式运行，需 FFmpeg 与 LFS 视频。

开发模式提供两项有限探针，输出放在已忽略的 `target/`：

```powershell
target/debug/lxmcl.exe --ui-capture target/ui-capture-day04
target/debug/lxmcl.exe --ui-benchmark target/ui-benchmark-day04.json
```

截图探针导出十幅原生渲染画面后退出，包含实例概览与重命名对话框；后两项使用仓库中首条实例，建议提供隔离的验收数据。探针不提交表单或保存偏好，不覆盖系统标题栏或真实键盘输入。导航测量不是鼠标到屏幕延迟或游戏 FPS。实际执行结果在交付说明 / PR 中报告；本机过程日志与导出存放 `.local/` 或 `target/`，不纳入公开文档。

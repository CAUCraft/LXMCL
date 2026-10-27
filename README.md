# LXMCL

面向 CAUCRAFT 的 Minecraft 启动器，使用 **Rust + Slint** 开发原生桌面界面。目前是可运行的前端交互预览，采用「先前端、后接服务」的开发方式。

项目自己维护的应用逻辑与新增工具统一使用 Rust；Slint 是允许的 UI 描述层。第三方依赖的实现语言不受限制。历史 Python、PowerShell、HTML / JS 设计工具保留作追溯，应用运行和编译不依赖这些脚本。

## 当前可以体验

- 经典 / 像素玻璃两种风格，浅深主题平滑切换，像素玻璃导航融合与分离。
- 七段循环动态壁纸、暂停与静态回退；孪枭双层皮肤预览、主页跟随鼠标侧身和转头。
- 游戏、资源、账号、下载任务和社团联机的演示交互。
- 四步首次游玩向导、启动准备清单、六种检查场景及失败重试。
- 八篇离线教程、F1 上下文帮助、Ctrl+K 聚合搜索、删除游戏名称确认。

**登录、游戏下载、Java 安装、游戏启动和网络诊断尚未接真实服务。** 演示数据退出后重置；目录检查仅校验 Windows 路径格式。点击「保存设置」会写入本机 `%LOCALAPPDATA%\LXMCL\ui-preferences.json`，不会修改指定游戏目录。

## 获取与运行

当前验证平台为 Windows x64 / MSVC。需要 Rust（工具链由 `rust-toolchain.toml` 固定为 1.92.0）与 Visual Studio C++ Build Tools / Windows SDK；Slint 固定为 1.18.1。

```powershell
git lfs install
git clone https://github.com/CAUCraft/LXMCL.git
cd LXMCL
git lfs pull
cargo run --locked
```

七段 MP4 通过 Git LFS 管理。直接下载仓库 ZIP 可能不包含视频实体；推荐使用 Git 克隆并运行 `git lfs pull`。视频不是编译输入，未获取视频时仍可显示仓库内的封面和静态背景。

动态壁纸需要本机 PATH 中的 FFmpeg，或可执行文件旁的 `ffmpeg.exe`。仓库不附带 FFmpeg 二进制。独立分发时将视频放到可执行文件旁的 `assets/wallpapers/`；开发模式自动查找项目内的 `ui/assets/wallpapers/`。当前使用 CPU 解码、GPU 上传，尚未实现硬件解码。

## 文档与接力开发

| 入口 | 内容 |
|---|---|
| [UI 文档索引](docs/ui/README.md) | 当前规范、状态与参考来源 |
| [信息架构与需求映射](docs/ui/information-architecture.md) | 功能分组、入口、原需求编号与优先级 |
| [视觉与动效规范](docs/ui/visual-design.md) | 像素玻璃、主题、孪枭、壁纸、性能约束 |
| [按钮与新手流程](docs/ui/interaction.md) | 按钮责任、错误恢复、教程与防呆 |
| [接口与状态约定](docs/ui/service-contracts.md) | Rust 契约、目标快照、幂等、后续服务划分 |
| [开发交接](docs/development.md) | 代码入口、能力边界、下一步与验证方法 |
| [验证记录](docs/validation.md) | 已完成检查与待复验项 |
| [素材说明](ui/assets/README.md) | 来源、原件保护、双层皮肤与视频 |
| [LXMCL UI Skill](.agents/skills/lxmcl-ui/SKILL.md) | 供后续开发者 / Agent 使用的项目设计技能 |

`AGENTS.md` 是仓库协作入口。Skill 随仓库一起版本管理，可在支持项目技能的工具中使用 `$lxmcl-ui`；其他工具可直接读取其 `SKILL.md`。

早期 Figma / HTML 三套方向稿位于 [设计归档](design/README.md)。它们不是当前 Slint 界面的截图，也不再作为实现依据。

## 验证

```powershell
cargo fmt --check
cargo clippy --locked --all-targets -- -D warnings
cargo test --locked
```

默认使用 FemtoVG / OpenGL，并保留软件渲染回退。性能测量、可选的媒体集成测试及人工检查步骤见 [开发交接](docs/development.md)。仓库不包含构建产物、个人偏好或原始视频下载目录。

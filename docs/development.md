# 开发交接

基线日期：2026-10-27。先阅读 [UI 索引](ui/README.md) 和 [验证记录](validation.md)，再按要改的功能定位代码。

## 代码地图

| 文件 | 责任 |
|---|---|
| `src/main.rs` / `build.rs` | 窗口、控制器与动效生命周期；Slint 编译和 Windows 图标 |
| `src/controller.rs` / `src/state.rs` | 旧演示数据、页面回调、模型增量同步、偏好保存 |
| `src/contracts.rs` / `src/preview_services.rs` | 不依赖 Slint 的准备流程契约与内存演示服务 |
| `src/journey.rs` / `src/help.rs` | 向导 / 检查 / 帮助适配，以及离线教程和别名检索 |
| `ui/app.slint` | 窗口、导航、快捷键、搜索与弹窗 |
| `ui/models.slint` / `ui/guidance-models.slint` | Slint 展示模型与回调 |
| `ui/components.slint` / `ui/theme.slint` | 共享控件与主题 token |
| `ui/pages/home.slint` / `home-games.slint` | 首页、我的游戏、社团联机 |
| `ui/pages/library.slint` / `settings.slint` | 资源、下载、账号、设置、诊断 |
| `ui/pages/guidance.slint` | 首次游玩向导、准备检查、帮助阅读 |
| `src/pixel_motion.rs` / `ui/pixel-glass.slint` | 像素玻璃材质及导航融合 |
| `src/skin_preview.rs` / `src/mascot_motion.rs` | 双层皮肤渲染、鼠标跟随 |
| `src/wallpaper.rs` / `ui/wallpaper-picker.slint` | 视频管线、暂停、封面和选择器 |
| `src/perf_probe.rs` | debug 构建内的有限性能测量 |

现有页面编号 0–8 为日常入口，9 为向导，10 为准备检查。教程 ID 固定为 `start`、`club`、`account`、`java`、`storage`、`resources`、`network`、`launch`；改变展示标题时不要破坏 ID 关联。当前搜索仍使用特殊整数 ID 路由，改为类型化动作是后续工作。

## 真实状态与模拟状态

真实可用：原生 UI、皮肤渲染、主题 / 导航动画、FFmpeg 视频、关键词搜索、路径格式校验，以及点击保存后的界面偏好落盘。偏好文件默认位于 `%LOCALAPPDATA%\LXMCL\ui-preferences.json`。

仅演示：账号、游戏实例、资源目录、下载任务、Java 准备、启动、服务器和诊断。它们不操作真实游戏文件。向导保留本次运行中的草稿，没有跨重启恢复；尚未自动识别首次启动。文件存在、权限和空间都没有由路径格式校验验证。

准备服务测试已约束目标快照、配置修订、幂等请求和旧结果失效。当前 `PreparationPort` 是同步的内存端口；真实 I/O 需要工作线程 / 调度桥，再通过 Slint 事件循环更新状态。不要直接把同步下载放进 UI 回调。真实实例修订应为单调递增版本，当前仅为内存指纹；持久幂等、任务取消与事件流尚未实现。

## 建议接续顺序

1. 先补 [待复验项](validation.md)，根据实际窗口反馈收敛密度、换行、焦点与 DPI 适配。
2. 设置持久化与实例仓库 → Java 检测 / 安装计划 → 独立游戏目录和创建流程。
3. 账号状态机与社团认证协议 → 服务器清单 → 真实启动会话。
4. 持久下载及依赖计划 → 资源操作 → 备份 / 删除恢复 → 日志和分服务诊断。

真实服务仍需确认社团认证协议、服务器版本清单、下载 / 皮肤站 API、更新清单与签名机制。它们不能从演示账号或示例地址推断。完整未来边界见 [接口设计](ui/service-contracts.md)。

## 构建与验证

工具链与首次运行见 [README](../README.md)。Windows 链接前关闭当前运行的 `lxmcl.exe`，避免文件锁。首次或修改 Slint 后生成 / 编译可能较慢；上轮全量构建约 10 分钟，这不是界面运行帧率。

```powershell
cargo fmt --check
cargo test --locked --lib
cargo clippy --locked --all-targets -- -D warnings
cargo test --locked
cargo build --locked
```

按改动选验证范围，不必每次纯文档修改都重新构建。状态 / 契约改动验证目标固定、未知不放行、旧报告拒绝、重复提交与重试；视觉修改用实际窗口验证，不能只以编译通过代表布局正确。

可选性能和媒体检查（`--ignored` 显式执行平时跳过的慢 / 环境相关测试）：

```powershell
cargo test --locked benchmark_tracking_frames -- --ignored --nocapture
cargo test --locked decoder_loops_pauses_and_resumes_without_an_unbounded_queue -- --ignored --nocapture
target/debug/lxmcl.exe --ui-benchmark target/ui-benchmark.json
```

媒体测试需要 FFmpeg 和 LFS 视频。性能探针有限执行 27 次程序化切页后退出，不保存偏好；报告分别记录回调、开始绘制和 CPU 提交，不能解释成鼠标到屏幕的延迟。现有报告见 `docs/performance/`。

## 资产与历史工具

当前 UI 编译输入、Logo、皮肤、封面及图标均在仓库中。七段 MP4 由 `.gitattributes` 指定 Git LFS；更新视频后检查 `git lfs status`，不要把 LFS 指针误当成可播放文件。FFmpeg 独立安装，不依赖私人下载目录。

`scripts/create-glass-assets.py` 与 `scripts/prepare-wallpapers.ps1` 为已有生成工具，后续有需要再迁移为 Rust；不新增第二套语言工具链。普通界面开发不需要运行它们。`design/archive/v0.1/` 中的 HTML、JS、Python 与 Figma 插件仅保留历史，不应重新进入产品运行路径。

项目技能位于 [.agents/skills/lxmcl-ui](../.agents/skills/lxmcl-ui/SKILL.md)，从 [AGENTS.md](../AGENTS.md) 可以发现。改设计时更新对应规范；仅当形成可复用决策或约束时更新 Skill，避免复制整份文档产生两套标准。

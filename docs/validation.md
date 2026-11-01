# 验证记录与待复验项

本文件区分历史测试、当前检查和仍未完成的人工验证。历史基线为 2026-10-27，最新修复复验为 2026-11-01；不能把 v0.1 Figma / HTML 渲染检查当作当前 Slint 验收。

## 本机 Java 检测与选择（2026-11-01）

### 实现

- `launcher-api` 增加 `JavaCandidate`、`JavaProbeFailure`、`JavaScanResult`、`JavaError`、`JavaRuntimeService` 与 `JavaRuntime` 服务键。
- 新增 `crates/launcher-java`（`java-local`）：发现保存路径、`JAVA_HOME`、`PATH` 与少量常见安装目录；实际启动 `java -XshowSettings:properties -version`，不经 shell，单次探测 5 秒超时。
- 候选按规范化路径去重；单个失败进入 `failures`，不影响其他候选。手动路径复用同一探测。
- `src/java_bridge.rs` 用独立工作线程执行检测 / 验证；关闭时设置取消标志并等待当前探测结束。
- 设置页新增真实检测、候选列表、手动路径验证、使用自动选择；保存写回 `LauncherSettings.java_path`，成功后更新控制器持有的有效设置。

### 验证

| 检查 | 结果 |
|---|---|
| `cargo fmt --all --check` | 通过 |
| `cargo test --workspace --locked` | 通过：38 项通过，2 项需 FFmpeg / 手动计时的测试 ignored |
| `cargo clippy --workspace --locked --all-targets -- -D warnings` | 通过 |
| Java 输出解析测试 | OpenJDK 21 属性、Java 8 `1.8.0_402` 版本行与主版本解析通过 |
| 窗口操作：检测 | 本机检测到 6 个候选、0 个失败；包含 BellSoft 21.0.10、Oracle 25.0.4 / 17.0.12 等真实结果 |
| 窗口操作：选择候选 | 选择 BellSoft 候选后，`launcher-settings.json` 写入对应 `java_path` |
| 窗口操作：手动路径 | 输入 `C:\Program Files\Java\jdk-17\bin\java.exe`，真实探测通过并保存；JSON 更新为该路径 |
| 窗口操作：重启恢复 | 重启后设置页显示该 `jdk-17` 路径；退出码 0 |
| 窗口操作：实例切换不重置 Java | 游戏目录只读从 `java-instance-1` 更新到 `java-instance-2`；用户填写的 Java 目录 `D:\CustomJava` 保持；设置的 `java_path` 未变化 |

### 限制

- 仅以 Windows 为验证平台；常见安装目录是有限列表，不递归扫描全部磁盘。
- 未实现 Java 下载 / 安装、完整游戏兼容规则或自动选择策略；`None` 只表示交给后续自动选择。
- 探测顺序执行，单次 5 秒超时；退出最多等待当前进程探测结束。
- 不修改系统 `JAVA_HOME` 或 `PATH`，不执行 shell 拼接命令。
- 厂商 / 架构字段仅在 Java 输出实际提供时展示。

## 实例仓库首个闭环（2026-11-01）

### 实现

- `launcher-api` 增加 `InstanceRecord`、`NewInstance`、`InstanceError`、`InstanceRepository` 与 `Instances` 服务键；沿用现有 `LaunchContext` 的整数实例 ID 与 `u64` 修订。
- 新增 `crates/launcher-instances`（`instances-json`）：单个 JSON 文件、同目录临时文件 + `persist` 替换；`apply` 只注册服务。
- 创建分配 `max(id)+1` 与 `<game_root>/instance-<id>`，修订从 1 开始；重命名保持 ID / 目录，修订递增。
- `src/instances_bridge.rs` 用单工作线程执行列表 / 创建 / 重命名；队列容量 1，关闭时先写完已接受命令，并把最后写入错误返回给 `join`。
- “我的游戏”接入创建、列表、选择、重命名；空仓库显示空状态；新实例内容为空，不生成演示模组或下载任务。
- 向导的游戏目录只读展示当前实例路径；列表、下拉框、搜索选择统一经状态刷新。切换实例只更新游戏目录，不重置用户单独填写的 Java 目录。
- 设置桥 `join` 现在返回最后保存错误（`FinalSave`），主程序仍无条件执行宿主清理。

### 验证

| 检查 | 结果 |
|---|---|
| `cargo fmt --all --check` | 通过 |
| `cargo test --workspace --locked` | 通过：36 项通过，2 项需 FFmpeg / 手动计时的测试 ignored |
| `cargo clippy --workspace --locked --all-targets -- -D warnings` | 通过 |
| 实例仓库测试 | 创建 → 重命名 → 新实例重载：ID 1、修订 1 → 2、目录不变；损坏 JSON 返回 `Parse` 且原字节不变 |
| 设置退出错误测试 | 最后一次保存失败时 `join` 返回 `FinalSave("disk full")` |
| 实例退出错误测试 | 最后一次创建失败时 `join` 返回 `FinalWrite("disk full")` |
| 窗口操作：向导目录与实例切换 | 游戏目录只读并随实例从 `java-instance-1` 更新到 `java-instance-2`；用户填写的 Java 目录 `D:\CustomJava` 在切换后保持；设置中的 `java_path` 未变化 |
| 窗口操作：创建 / 重命名 | JSON 写入 `验收实例`（修订 1），重命名后为 `验收实例改名`（修订 2），目录 `D:\Games\LXMCL\instance-1` |
| 窗口操作：关窗 / 重启 | 两次退出码均为 0；重启后“我的游戏”显示 `验收实例改名` 与 `修订 2 · D:\Games\LXMCL\instance-1` |
| 窗口操作：空仓库 | 空数据目录启动，“我的游戏”显示“还没有实例记录”；退出码 0 |

### 限制

- 本轮不支持导入、复制、删除记录 / 文件、自动下载或创建真实游戏目录。
- 游戏文件安装、账号、下载、Java 下载 / 安装和启动仍保留演示标识；Java 检测见上一节，创建记录只代表配置已保存。
- JSON 仓库使用单进程工作线程写保护，不做跨进程文件锁。
- 未增加第二种实例存储后端、通用任务总线或事件溯源。

## 插件框架 Phase 1 加固（2026-11-01，第二轮）

本轮只改现有插件基础设施与设置桥，不新增示例、存储框架、可选依赖或访问授权。

### 修复

- **设置退出 flush**：两份线程桥关闭邮箱后拒绝新请求，但保留最后一个已接受的保存快照；工作线程写完才退出。保存失败仍如实回调。未改正式文件写入路径，继续由现有 JSON / TOML provider 落盘。
- **插件清理**：`Context` 用关闭状态和一把锁保护服务名与清理闭包；`dispose` 在锁外逆序执行闭包，再在注册表锁外移除并析构服务。关闭后 `provide` / `on_dispose` 返回 `ContextClosed`。
- **统一逆序清理**：正常退出与启动失败共用 `dispose_started`；失败插件先执行自身清理并释放实例，再清理已启动插件。服务注册失败或重复时，服务也被带出锁外析构。
- **装配检查**：实例化时固定 descriptor；`apply` 后确认声明提供的服务已按预期类型注册；必需依赖仍由 Kahn 拓扑排序保证提供者先启动。

### 验证

| 检查 | 结果 |
|---|---|
| `cargo fmt --all --check` | 通过 |
| `cargo test --workspace --locked` | 通过：32 项通过，2 项需 FFmpeg / 手动计时的测试 ignored（runtime 4、settings 3、库 8、二进制 17） |
| `cargo clippy --workspace --locked --all-targets -- -D warnings` | 通过 |
| 最小示例 `cargo test --locked` / clippy / fmt | 通过：4 项、clippy 0 告警、格式通过 |
| 主程序保存后立即关窗（窗口操作） | 编辑目录后点击保存，约 100 ms 后关窗；退出码 0，JSON 已写入新目录并保留 `java_path` |
| 主程序重启读回 | 设置页输入框读回 `flush-after`；关窗退出码 0 |
| 最小示例保存后立即关窗 | 退出码 0，JSON 写入 `min-flush-after` 并保留 `java_path` |
| 最小示例重启读回 | 控制台初始目录为 `min-flush-after`；退出码 0 |

### 已知边界

- 本轮未改正式文件写入方式；如果后续需要原子替换，应引入成熟实现，而不是在本轮自建存储层。
- 未增加可选依赖、逐次访问授权、热重载、沙箱或通用任务总线。
- 关闭后不再接受新保存；被合并掉的旧快照不会单独写盘，最终以最新快照为准。

## 插件框架 Phase 1 修复复验（2026-11-01）

本轮修复第一轮验收的 5 项问题，并补做保存 / 重启操作。验收数据放在 `target/acceptance/`，通过独立 `LOCALAPPDATA` 隔离，不触碰本机真实用户配置。

### 修复

- **独立示例构建**：生成 `examples/minimal-launcher/Cargo.lock`；确认入口包含 `slint::include_modules!()`，Slint 关闭默认特性时启用 `compat-1-2`；两个示例补跑 rustfmt。示例 `target/` 加入 `.gitignore`。
- **示例退出**：`SettingsWorker::join` 先显式关闭邮箱并唤醒工作线程，再等待线程；不再依赖发送端是否恰好全部释放。
- **插件实例生命周期**：`PluginHost` 保存 `Box<dyn Plugin>` 直到 `shutdown()`；新增 `Drop` 兜底，提前返回时仍按相反顺序清理并释放实例。
- **Java 路径保留**：LXMCL 保存设置时使用启动时读到的 `initial.java_path`，不再硬编码 `None`。
- **设置有界队列**：两份线程桥改为邮筒（`Mutex` + `Condvar`）。待保存快照只保留最新一个（正在执行一个 + 等待一个），读取队列上限 4；连续点击不追加完整任务。

### 验证

| 检查 | 结果 |
|---|---|
| `cargo fmt --all --check` | 通过 |
| `cargo check --workspace --locked` | 通过 |
| `cargo build --locked` | 通过 |
| `cargo test --locked`（主工作区） | 通过：库测试 8 项、二进制测试 16 项；2 项需 FFmpeg / 手动计时的测试保持 ignored |
| `cargo clippy --locked --all-targets -- -D warnings`（根工作区与两个示例） | 通过 |
| `cargo check --manifest-path examples/minimal-launcher/Cargo.toml --locked` | 通过 |
| `cargo test --manifest-path examples/minimal-launcher/Cargo.toml --locked` | 通过：4 项（队列合并、join 关闭、桥释放、最新快照） |
| 两个示例的 `cargo fmt --check` | 通过 |
| `cargo test --manifest-path examples/custom-settings/Cargo.toml --locked` | 通过：4 项（JSON/TOML 读写、缺失文件默认值、损坏文件不覆盖） |
| 主程序 `--ui-benchmark` | 27 次程序化切页后自行退出，退出码 0；OpenGL 后端 |
| 主程序保存 / 重启（窗口操作） | 把目录改为 `games-main-edited` 后保存；JSON 仍保留 `java_path`；重启后输入框读回同一路径；正常关窗退出码 0 |
| 最小示例保存 / 重启（窗口操作） | 把目录改为 `min-games-edited` 后保存；JSON 保留 `java_path`；重启控制台打印同一路径；退出码 0 |
| 最小示例换 TOML provider | 只改 `product.toml` 的 provider / file_name；新 TOML 写入 `min-toml-edited` 且保留 `java_path`；原 JSON 哈希不变；重启控制台读回新值；退出码 0 |
| 两产品数据隔离 | 主程序写 `%LOCALAPPDATA%\org.caucraft.lxmcl\`；示例写 `%LOCALAPPDATA%\dev.example.minimal-launcher\`，互不影响 |

### 已知边界

- 本轮只验证设置服务的保存 / 重启与插件生命周期；游戏、Java、下载、账号、诊断仍是演示数据，没有真实安装或启动。
- 队列合并的语义是“最新快照获胜”：连续点击时被覆盖的旧回调不会执行，最终由最新回调更新状态。测试覆盖了写入次数上限与最新值。
- TOML provider 在 `java_path` 为 `None` 时省略该字段（TOML 没有 null），已有单元测试覆盖。

## 插件框架 Phase 1（2026-10-28）

以下为当时记录；未执行项已由上一节补跑，这里保留原文以区分历史。

| 检查 | 状态 |
|---|---|
| 代码编写 | 已完成：`crates/launcher-runtime`、`crates/launcher-api`、`crates/launcher-settings`、产品装配（`src/main.rs`、`src/composition.rs`、`src/settings_bridge.rs`、`src/controller.rs` 保存流改写）、`product.toml`、`examples/custom-settings`、`examples/minimal-launcher` |
| `cargo fmt --all --check` | **未执行**：本会话内命令行工具被环境阻断，待恢复后补跑 |
| `cargo check --workspace --locked` / `cargo clippy --locked --all-targets -- -D warnings` | **未执行**（同上）；期间以编辑器内 rust-analyzer 诊断做静态核对，全部已知告警已消除（unused import ×3、超宽行），但**这不等于编译通过** |
| `cargo build --locked`（主工作区） | **未执行**（同上） |
| 示例构建与运行（`--manifest-path` 指向 `examples/minimal-launcher`） | **未执行** |
| 窗口操作（保存设置 → JSON 文件落盘 → 重启读回；最小示例换 provider；两产品数据隔离；退出清理） | **未执行，不宣称通过**，待人工验收 |
| 损坏设置文件行为 | 代码路径为明确报错且不写文件（`SettingsError::Parse`），但未实测 |

未执行的原因如实记录：本会话中执行命令的网络分类服务持续不可用（cargo、WebFetch 均被阻断）。上述命令在环境恢复后按序补跑并更新本表；在补跑完成前，Phase 1 只能视为「已写完、未编译验证」。

## 前端上一轮验证

| 检查 | 结果与范围 |
|---|---|
| Rust / Slint 编译 | 最后一次 `cargo build` 成功，约 9 分 42 秒 |
| Clippy | 最后一次 `cargo clippy --all-targets -- -D warnings` 通过 |
| 准备服务与帮助测试 | 最后一次 `cargo test --lib`：8 项通过 |
| 全量测试 | 较早快照：6 项库测试 + 12 项二进制功能测试通过，2 项环境 / 性能测试默认忽略；后续新增两项库测试及 UI 修正 |
| 实际窗口操作 | 已走过四步向导、缺 Java 检查、准备清单、模拟准备 / 启动、相关教程、返回检查和暗色帮助阅读 |
| 最后修正的视觉复验 | 未完成；用户按 Escape 停止了自动操作 |

最后一轮修正包括向导玩法卡片宽度、账号管理入口位置、路径说明布局、阻断项优先排序、检查项高度、禁用文字颜色与根焦点下 F1 处理。它们已编译，但尚未完成修正后窗口验收。

## 接力时优先复验

- 向导四步在最小窗口下无截断，账号管理返回能继续，路径错误可就地改正；草稿在本次运行保留。
- 准备页的被阻止 / 未知 / 待处理项优先显示；六种演示场景给出正确动作；不就绪时不能模拟启动。
- 确认清单后更改游戏、账号或配置，应拒绝旧结果；失败重试不能重复执行，重查不会丢失已模拟准备的 Java 状态。
- 根窗口与子控件分别持有焦点时验证 F1；同时检查 Ctrl+K、Tab / Shift+Tab、Enter、Esc、关闭浮层后的焦点。
- 经典 / 像素玻璃 × 浅 / 深色下核对禁用文字、错误提示、弹窗、搜索与视频背景可读性。
- 导航快速连点、主题连续反转、孪枭跟随 / 回正、视频暂停 / 切换 / 最小化，以及退出后 FFmpeg 回收。

尚未做完整验收：100% / 125% / 150% 系统缩放、读屏、减少动画 / 透明度设置、跟随系统主题、Linux / macOS、打包安装流程。部分设置尚未实现，不能将上述清单写成已支持能力。

## 性能证据

[performance/](performance/) 保留历史 JSON 原始报告。孪枭 360 姿态测量的 CPU 光栅化 p50 约 1.814 ms、p95 约 2.674 ms；只包括 CPU 贴图和光栅化，不包括 GPU 上传或系统输入 / 显示。视频 24 fps 和导航动画混合的帧间隔不能当作稳定 UI FPS。性能回归应在相同机器、后端、窗口与场景下比较。

## 仓库整理检查（2026-10-27）

- `cargo fmt --check` 通过；应用行为未改动，仅整理部分文件末尾空行。
- `cargo test --locked --lib` 再次通过，8 项测试、0 失败。
- 项目 Skill 通过 skill-creator 的 `quick_validate.py`；技能元数据已生成。
- 当前及归档 Markdown 的本地链接检查通过。
- 孪枭贴图 SHA-256 与原始 RGBA 文件记录一致。
- 七段壁纸共约 181.18 MiB，采用 Git LFS；普通 Git 保留封面，缺视频时仍可编译和回退。

本轮未重新进行人工窗口操作；上一轮待复验项继续保留。

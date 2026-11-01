# 启动器平台架构（Phase 1）

本文件描述插件框架第一阶段的实现：目标、角色、依赖方向、生命周期、线程模型与产品装配方式。基线日期 2026-10-28。与 Cordis 的关系见 [Cordis 对照](#cordis-对照与查阅状态)——**本框架只借鉴其概念，不声称实现完整 Cordis 兼容**。

## 目标与不目标

Phase 1 让第三方用「装配插件 + 配置产品 + 替换界面」的方式组装自己的启动器：

- 产品装配入口：`product.toml` 声明产品身份与启用的插件。
- 服务依赖解析：宿主按「插件声明的 requires/provides」完成整体装配解析后统一启动。
- 设置服务贯通：插件提供设置服务 → Slint 界面通过契约消费 → 落盘保存、重启后读回。
- 实例记录贯通：插件提供实例仓库 → Slint 界面创建、列选、重命名 → JSON 持久化、重启后读回。
- 独立最小启动器示例：证明第三方能不携带 LXMCL 的控制器、品牌素材与演示状态复用 SDK 与插件。

Phase 1 明确**不包含**：真实账号认证、Java 安装、游戏下载、游戏启动、实例文件安装、模组管理。这些只作为「未来契约归属」记录（见文末），不写占位 trait 或空插件。

## 五个角色与依赖方向

| 角色 | 落地 | 职责 | 禁止 |
|---|---|---|---|
| 1. 运行时（Runtime） | `crates/launcher-runtime` | 插件目录、服务注册表、依赖解析、装配启动、退出清理 | 不知道 Minecraft、Slint、LXMCL、账号，不知道任何业务服务名 |
| 2. API | `crates/launcher-api` | 业务能力契约：服务 trait、请求与结果类型、服务键 | 不依赖任何提供者实现，不依赖 Slint |
| 3. 提供者插件 | `crates/launcher-settings`（`settings-json`）、`crates/launcher-instances`（`instances-json`） | 实现服务契约并注册到上下文；I/O 在 load/save 或仓库方法中 | 不写 UI；不直接调用其他插件的具体实现（只依赖契约） |
| 4. 消费者 / Shell | `src/controller.rs`、`src/settings_bridge.rs`、`src/instances_bridge.rs`、`examples/minimal-launcher` | 调用服务契约渲染界面 | 不直接读配置文件 |
| 5. 产品 | `src/main.rs` + `src/composition.rs` + `product.toml`（LXMCL）；`examples/minimal-launcher`（最小示例） | 装配：注册工厂、定位配置、启动宿主、运行 Shell | 不是 Runtime 的特例——LXMCL 与最小示例是平级的两个产品 |

依赖方向（箭头指向被依赖方）：

```
产品(5) ──→ Shell(4) ──→ API(2) ──→ Runtime(1)
产品(5) ──→ 插件(3) ──→ API(2)
API(2)  ──→ Runtime(1)   （仅为 ServiceKey 等服务键类型）
```

Runtime 不依赖任何业务契约；API 不依赖提供者与 UI。内核没有任何 `match plugin_name` 的业务分支——新插件通过 `PluginCatalog::register` 的**公开工厂注册**进入，内核代码不变。

## 代码布局

```
crates/launcher-runtime/   插件宿主（catalog/context/error/host/key/plugin/product/registry）
crates/launcher-api/       业务契约（LauncherSettings、SettingsService、Settings 服务键）
crates/launcher-settings/  JSON 设置提供者插件（settings-json）
crates/launcher-instances/ JSON 实例仓库插件（instances-json）
src/main.rs                LXMCL 入口：装配 → 运行 → 退出清理
src/composition.rs         LXMCL 产品装配（定位 product.toml、注册工厂、运行 Shell）
src/settings_bridge.rs     LXMCL Shell 的设置线程桥
src/instances_bridge.rs    LXMCL Shell 的实例仓库工作线程桥
examples/custom-settings/  第三方 TOML 设置提供者插件示例（settings-toml-example）
examples/minimal-launcher/ 独立最小启动器示例（自带 Slint 窗口与线程桥）
examples/*/product.toml    示例产品装配配置
```

`examples/` 是独立 Cargo 工作区（各自带空 `[workspace]` 表），用 `cargo run --manifest-path examples/minimal-launcher/Cargo.toml` 单独构建，避免拖进主工作区。

## 服务注册与类型擦除

- 服务键 `ServiceKey`：关联类型 `Service`（可为 trait 对象）+ 常量 `NAME`（如 `"launcher.settings"`）。注册表以服务名字符串索引，以 `TypeId` 校验类型一致。
- 存储采用双层 `Arc`：`Arc::new(Arc<K::Service>)` 擦成 `Arc<dyn Any + Send + Sync>`；取出时 downcast 回外层再克隆内层。全程零 unsafe。
- 插件通过 `Context::provide::<K>(service)` 登记服务；消费者通过 `PluginHost::service::<K>()` 取用。同一服务名重复提供、同类型不匹配都是装配错误，直接失败，不做兜底。

## 实例记录闭环（2026-11-01）

- `InstanceRecord` 只保存稳定整数 ID、名称、版本、加载器、独立游戏目录和修订号；沿用现有 `LaunchContext` 的 `instance_id` / `instance_revision`，不重做 ID 体系。
- `InstanceRepository` 只提供 `list` / `create` / `rename`；`instances-json` 插件用单个 JSON 文件持久化，`apply` 只注册服务。
- 创建分配 `max(id)+1` 与 `<game_root>/instance-<id>`，修订从 1 开始；重命名保持 ID 与目录不变，修订递增 1。
- 仓库方法由 `src/instances_bridge.rs` 的单工作线程执行；队列容量 1，关闭时先写完已接受命令，并把最后写入错误返回给 `join`。文件损坏返回 `Parse`，不覆盖原文件。
- 向导的游戏目录来自当前实例记录，只读展示；列表、下拉框和搜索选择都经同一状态刷新。切换实例只更新游戏目录，不重置用户单独填写的 Java 目录。
- “我的游戏”空仓库显示空状态；创建只写配置记录，不写演示模组、下载任务或游戏文件。

## 插件生命周期

```
Registered → Starting → Active → Stopping → Stopped
                     ↘ Failed（启动失败）
```

- **编译期链接**：插件是普通 Rust crate，产品在装配处通过工厂实例化。没有 DLL / WASM / 热重载。
- **启动装配**（`PluginHost::start`）：实例化（未知 provider / 重复插件 ID 即失败）→ 固定每个实例的描述符 → 建立服务提供表（同服务重复提供即失败）→ Kahn 拓扑排序（依赖缺失 / 环形依赖即失败，环错误列出插件链）→ 依次 `apply(&Context)`。每个插件启动成功后还要确认其声明提供的服务已按预期类型实际注册，否则按启动失败处理。
- **Context 作用域**：每个插件一个独立 `Context`，记录它提供的服务与清理闭包（`on_dispose`）。内部锁只保护状态和容器；停止时先置关闭状态并取走清理闭包，清理闭包与服务析构都在锁外执行。关闭后 `provide` / `on_dispose` 返回 `ContextClosed`，不会在停止过程中新增资源。
- **失败原子性**：某插件 `apply` 失败或声明服务未注册时，先执行它自己的清理并释放实例，再按启动相反顺序清理已启动插件；所有清理都执行，第一个错误随 `PluginFailed` 上报。
- **退出清理**：`host.shutdown()` 按启动相反顺序清理全部插件（先消费者后提供者的自然序）：每个作用域先逆序执行清理闭包，再在注册表锁外移除并析构服务，最后释放插件实例；随后清空注册表。全部清理执行，首个错误以 `Dispose` 上报。产品提前返回而直接 drop 宿主时，`Drop` 走同一清理路径，不会跳过插件实例释放。

## 线程模型

- Slint 窗口与模型只在 UI 线程；设置读写在工作线程。
- Shell 桥（每个 Shell 自己实现一份）：有界邮筒（`Mutex` + `Condvar`）。Load 携带回复 sender；Save 携带**点击时快照**与完成回调。待保存槽位只保留最新一个快照，正在执行一个，新点击覆盖尚未开始的旧快照，不随点击次数增长。工作线程执行磁盘 I/O，完成回调经 `slint::invoke_from_event_loop` 回到 UI 线程执行。
- 启动首读用 `load_blocking`（窗口创建前完成，读操作仍在线程上）；读取排队上限 4，超过即返回 `Unavailable`。
- 顺序：`ui.run()` 返回 → 丢弃窗口 → `SettingsWorker::join()` 关闭邮箱并 join → `host.shutdown()`。关闭后不再接受新请求，但已接受的最后一个保存快照仍会写完；即使界面回调仍持有桥的克隆，工作线程也会在写完快照后退出。不在 UI 回调里 join，无全局轮询定时器，无无界队列。
- 保存结果与界面偏好保存结果合并为一条提示展示；工作线程不可用时回调仍被执行（返回 `Unavailable`），不静默。

## 产品配置与数据目录

- 所有产品使用统一文件名 `product.toml`；宿主从可执行文件目录向上有限层（6 层）查找，`cargo run` 与安装目录都能稳定命中。
- `[product]` 定义 `id` / `name`；产品 ID 决定用户数据命名空间（`%LOCALAPPDATA%\{净化后的 id}`），基准目录在装配处解析，不依赖启动时的随机工作目录。
- `[[plugins]]` 声明 `id` / `provider`；`[plugins.config]` 是该 provider 自己定义的强类型配置（如 JSON 提供者的 `file_name` + `defaults`）。配置解码错误带 provider 名上报。
- 工厂在注册处把相对路径（如相对的 `defaults.game_root`）解析成基于数据目录的完整路径，插件只拿到明确的最终路径。
- 界面偏好（Slint 控件状态）与业务设置（`SettingsService`）是两份数据：前者仍写 `%LOCALAPPDATA%\LXMCL\ui-preferences.json`，后者由设置插件写入产品数据目录，互不覆盖。

## 错误与失败语义

`RuntimeError` 覆盖：产品配置错误（带路径）、数据目录不可用、未知 provider、插件配置解码、重复插件 ID / provider / 服务、服务类型不匹配、服务缺失、依赖缺失、依赖环（带插件链）、插件启动失败（含清理错误）、清理失败。中文错误消息直接可读。按项目约定：无 unwrap、无静默兜底、无自动修复；设置文件损坏返回明确错误，不会当作首次运行或被覆盖。

## 界面替换的三种方式（第三方视角）

1. **只换配置**：改 `product.toml`（默认值、文件名、换 provider）——不动任何代码。
2. **换提供者**：实现新的 `SettingsService` 插件 crate，在产品装配处注册工厂，改 `provider` 字段——Shell 窗口代码不变（`minimal-launcher` 的 JSON/TOML 双注册即为此演示）。
3. **换 Shell / 新产品**：复制 `examples/minimal-launcher` 的装配模板，写自己的 Slint 窗口与线程桥，只消费 `launcher-api` 契约。

## 未来契约归属（不在 Phase 1 实现）

完整服务划分见 [接口与状态约定](../ui/service-contracts.md) 的后续契约表。与插件框架的对应关系：账号认证、Java 准备、下载、启动等将各自成为 `launcher-api` 里的服务契约（新的 ServiceKey + trait），由对应提供者插件实现，消费端 Shell 不变。`SettingsPort`（有效值、草稿、继承覆盖、目录检测）是设置契约的后续演进，其落点仍由设置插件承担，本阶段的 `SettingsService` 是其最小前身。本阶段不写这些 trait 的空壳。

## Cordis 对照与查阅状态

概念来源：任务说明中对 [cordiverse/cordis](https://github.com/cordiverse/cordis) 的 Context 注入、Disposable 作用域、启动顺序语义的描述，以及 deepseek-harness 文档（framework / service / practice / config 章节）与 [XMCL](https://github.com/Voxelum/x-minecraft-launcher)（[业务划分](https://xmcl.app/en/core/)）作为启动器领域参考。

**查阅状态（如实）**：本会话内网络抓取工具不可用，上述上游仓库与文档站点**未能实际打开核对版本、commit 或具体路径**；本框架的实现依据是任务说明中转述的语义，而非对上游源码的直接对照。因此：

- 借鉴的概念：Context 注入服务、Disposable 作用域与逆序清理、装配整体解析后再启动的顺序。
- 已知差异（设计取舍，非实现遗漏）：编译期链接注册，无装饰器 / 自动扫描 / 依赖注入宏；生命周期只覆盖启动装配与退出清理，没有运行期重载 / 卸载；错误是普通 `Result` 传播而非事件流；无沙箱隔离声明——插件与宿主同进程同权限。
- 不声称实现完整 Cordis 兼容；后续接手者应自行核对上游（cordiverse/cordis、deepseek-harness 文档）后再补充本节。

# 前后端接口、状态与数据约定

更新：2026-10-27。以 [Rust 契约](../../src/contracts.rs) 和 [演示服务](../../src/preview_services.rs) 为已实现基线；下文明确标记的后续设计尚未接入真实服务。

## 当前接口边界

### 边界与调用方式

Slint 只负责展示与用户输入 → Rust 控制器处理导航 / 草稿 / 展示状态 → Rust 应用服务端口接收命令并返回结构化结果 → 真实后端接认证、磁盘、下载、进程。桌面程序内部无需先搭 HTTP 服务或监听网络端口。

当前可编译的最小契约在 `src/contracts.rs`，演示服务在 `src/preview_services.rs`，Slint 适配在 `src/journey.rs`。`PreparationPort` 提供 `check`、`apply_plan`、`launch`；当前实现仅内存操作。真实实现必须在工作线程执行，通过 Slint 事件循环回传；不能把真实阻塞下载塞进现在的 UI 回调。

| 已有 Rust 类型 | 责任与字段约束 |
|---|---|
| `LaunchContext` | 固定实例 ID、名称、配置修订、账号 ID、版本 / 加载器、游戏与 Java 目录、启动目标；无令牌 |
| `CheckReport` | 报告 ID + 目标快照 + 来源（Preview / Live）+ 检查项 + 可选准备计划；当前五项均是必需项 |
| `CheckItem` | 稳定 code、标题、Ready / NeedsAction / Blocked / Unknown、说明、关联教程 ID |
| `PreparationPlan` / `PlannedChange` | 清单 ID、各项动作、来源、目标路径、可空下载字节数；None 显示待估算 |
| `ApplyPlanRequest` | 幂等请求 ID、报告 ID、清单 ID、目标快照；确认后才提交 |
| `LaunchRequest` / `LaunchReceipt` | 请求 ID、报告 ID及目标；回执明确 Preview / Live，不能等同于已进入服务器 |
| `ServiceError` | 稳定错误码、用户说明、是否可重试、教程 ID；不让界面解析日志字符串来判断错误类型 |

当前演示配置修订使用内存指纹；真实实例服务须维护单调递增的修订号。检查开始清除旧结果；提交前再次比较目标与修订。同 ID 的成功重放返回同一结果；同请求 ID 对应不同内容时拒绝。失败重试沿用原请求 ID，不生成重复操作。真实服务还须在持久化层完成幂等与原子提交，不能只靠 UI 内存 Map。

### 完整服务划分（下表为后续契约设计，不宣称全部已实现）

| 端口 | 查询 / 命令 | 核心返回值 / 事件 | 对应需求 |
|---|---|---|---|
| AccountPort | 列表、开始认证、取消认证、刷新、切换角色、移除账号 | AccountSummary、AuthChallenge、AuthState；令牌只在服务内部 | 基础 1 / 11；高级 7 |
| InstancePort | 列表、预览创建 / 导入、提交计划、复制、删除预览、删除、导出 | InstanceSummary、Revision、ChangePlan、TaskId | 基础 2 / 3 / 6；高级 3 |
| RuntimePort | 扫描 Java、按实例解析需求、安装计划、手动选择 | JavaCandidate、CompatibilityResult、RuntimePlan | 基础 2.1；高级 2 |
| LaunchPort | 检查、准备、启动、停止、读取进程状态 | CheckReport、PreparationPlan、LaunchSession、退出结果 | 基础 3 / 8；高级 2(3) |
| ContentPort | 多源搜索、详情、兼容文件、依赖计划、导入识别、启停、删除 / 更新计划 | ContentKind、Origin、Compatibility、DependencyGraph、ChangePlan | 基础 4–7；高级 4 |
| TaskPort | 列表、订阅、暂停、继续、取消、重试失败项、清除完成记录 | TaskSnapshot、StageProgress、TaskEvent | 基础 5；高级 4 |
| ServerPort | 社团清单、详情、延迟、匹配实例、更新差异、联机房间 | ServerRequirements、AccountPolicy、MatchResult、RoomState | 基础 8；高级 5 |
| AppearancePort | 皮肤站资源、皮肤 / 披风预览、应用、恢复 | SkinAsset、ModelType、Layers、ApplyResult | 基础 4 / 6.2 / 10 |
| SettingsPort | 读有效值、保存草稿、逐项覆盖 / 恢复继承、检测目录、代理策略 | EffectiveSettings、SettingOrigin、ValidationIssue | 基础 9 / 10 / 12；高级 2 / 3 / 8 |
| DiagnosticsPort | 游戏检查、分服务连通性、日志流、崩溃摘要、诊断包预览 / 导出 | Finding、EndpointResult、LogEvent、ExportManifest | 基础 14 / 15；高级 1；辅助功能 |
| UpdatePort | 检查启动器更新、下载、校验、准备退出、重启恢复 | UpdateInfo、TaskId、RestartPlan | 基础 13 |
| AutomationPort | 深度链接解析、CLI / MCP 命令、扩展能力与权限 | TypedIntent、Capability、OperationReceipt | 高级 6；命令自动化 |

### 通用数据规则

- 实例、账号、服务器、资源、任务、操作和计划分别使用稳定 ID；不以列表序号或展示名称标识实体。演示数据保留现有整数 ID，服务接入时可换不透明 ID，不影响页面标签。
- 资源种类用枚举：Mod / Modpack / ResourcePack / ShaderPack / World / Skin / Cape。来源用结构化数据：平台项目 ID、文件 ID、哈希或 UserImported；自行导入不靠名称猜线上来源。
- 兼容性包括 Compatible / Incompatible(reason) / Unknown(reason)，与“已安装”“可更新”分开。完整冲突预测做不到时明确不确定性。
- 时间统一服务时间戳，界面按本地时区格式化；容量用整数 bytes，延迟可空；未知不填 0。路径在本地契约用 PathBuf，外部协议须规定编码 / 平台，不把敏感绝对路径默认上传。
- 全局设置与实例覆盖分离：返回 value + source(global / instance / recommended)；恢复继承是删除覆盖项，不能复制一次全局值后伪装继承。
- 搜索结果采用 ResultId + Kind + Title + Subtitle + Target + TypedAction；服务器、实例、教程和设置各自携带动作，不靠负数 ID 推断类别。当前演示搜索路由仍保留旧整数方式，后续替换为此类型。
- 错误至少区分 AuthExpired、EntitlementRequired、VersionMismatch、JavaMissing、DependencyMissing、NetworkTimeout、HashMismatch、DiskFull、PermissionDenied、InstanceBusy、PlanExpired、Cancelled、Unsupported。展示说明与技术详情分离，保留 relatedHelpId。

### 长任务与事件流

建议使用 `CommandEnvelope { schema_version, request_id, target, expected_revision, payload }`，返回 Accepted(operation_id) 或 Rejected(ServiceError)。长任务通过 `EventEnvelope { operation_id, target, sequence, occurred_at, payload }` 更新；同操作序号单调递增，忽略重复与乱序旧事件。关闭页面只取消订阅，不能取消后台任务。重新进入先取快照，再从快照序号订阅增量。

任务状态：Queued → Running(stage) ↔ Paused；Running 可进入 WaitingForUser、Failed、Cancelled、Succeeded。阶段分 Resolving / Downloading / Verifying / Installing。结束状态不能被迟到进度“复活”；重试产生新 attempt，仍保留稳定任务 ID 和原目标。没有总量时显示不定进度；100% 下载完成不等于安装完成。

启动状态：Idle → Checking → NeedsAction / WaitingConfirmation / Ready → Preparing → Checking → Ready → Spawning → Running → Exited。真实进程创建后才进入 Running；是否入服由可验证事件决定，不能靠固定计时器。取消检查与取消下载、停止游戏是不同命令。进程运行期间禁用会破坏该实例的修改；需要终止游戏时单独确认。

确认边界：安装 / 更新 / 删除 / 自定义执行命令先 `plan` 再 `commit`。计划包含新增、替换、删除、保留内容及有效修订；文件落盘前做最终校验，发生冲突返回 PlanExpired。外部深度链接和 MCP 也走同一层，不直接调用未经确认的文件操作。

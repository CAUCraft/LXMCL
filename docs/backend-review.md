# 后端架构进度审查

审查日期：2026-11-11。对象：`codex/backend-decoupling`，基于已合入 `main` 的 [PR #1](https://github.com/CAUCraft/LXMCL/pull/1)，继续完成任务依赖和应用用例装配的解耦。

核心后端分层、日志与 JSON 文案目录已经接入实际调用路径。任务记录已移除资源模块依赖，游戏与账号授权用例均支持显式传输注入；集成测试直接验证公开用例的实际装配。配置、实例 ID、目录、任务历史和凭据格式保持兼容。

## 已完成的职责划分

| 范围 | 当前实现及依据 | 完成情况 |
| --- | --- | --- |
| 应用编排 | [`Game.Execute`](../internal/launcher/launch_operation.go) 接收提交时快照，协调授权、安装、一次修复和启动；`Game.Transport` 注入安装与启动准备，`Store.Accounts` 注入账号授权。 | 已接通公开用例；业务包不反向导入 `launcher` 或桌面。 |
| 配置与实例 | [`config.Store`](../internal/config/store.go) 管理配置与旧版迁移；[`instance_ids.go`](../internal/config/instance_ids.go) 管理编号；[`installation.go`](../internal/config/installation.go) 准备安装草稿。 | 已实现；保留 ID、目录、修订与旧数据。 |
| 游戏与 Java | [`minecraft.Runtime`](../internal/minecraft/runtime.go) 管理安装、校验和启动；[`javaruntime.Manager`](../internal/javaruntime/manager.go) 管理选择与安装，缓存验证和清单解析独立维护。 | 已实现；Java 自动安装限 Windows x64。 |
| 账号协议与凭据 | [`account.Client`](../internal/account/client.go) 管理协议；[`Vault`](../internal/account/vault.go) 管理加密凭据；[`account_commit.go`](../internal/launcher/account_commit.go) 和 [`account_authorize.go`](../internal/launcher/account_authorize.go) 统一提交与授权用例。 | 已收口；部分成功显式返回，不承诺跨文件原子性。 |
| 平台、资源与整合包 | `catalog` 管理元数据，`distribution` 管理下载缓存，`content` 管理实例资源，`modpack` 管理计划和安装事务；游戏安装与配置提交通过回调注入。 | 已实现；无反向依赖应用层，沿用原数据格式。 |
| 任务、服务器与存档 | [`tasks.Journal`](../internal/tasks/journal.go) 管理历史；[`server.Client`](../internal/server/client.go) 管理查询；`saves` 分离计划、归档和发布。 | 已实现；任务资源类型保存为字符串，由应用层解释和校验。 |
| 文件与传输 | `storage.PublishNew` / `ReplaceFile` 明确区分禁止覆盖与允许替换；`transfer.Artifact` 封装摘要与大小约束，`Retry` 管理有限重试。 | 已实现；成功提交后保留成功结果。 |
| 日志与国际化 | [`log`](../internal/log/game.go) 接管游戏和安装器日志创建及游戏日志尾部预览；[`i18n`](../internal/i18n/i18n.go) 使用嵌入的中英 JSON 键值目录、回退和错误链。 | 已接入；桌面标签、应用级日志轮转尚未实现。 |

本轮用 `go list -f '{{.ImportPath}} {{join .Imports " "}}' ./internal/...` 检查实际依赖。所有生产后端包均无 UI 依赖，其他业务包不导入 `launcher`。`launcher` 不再导入文件系统、子进程、归档、摘要和 JSON 编解码包；保留的跨模块调用是应用流程的一部分。原生窗口、依赖栈和品牌资源保持原状。

## 前序已合入的改进

1. Java 验证显式接收预期清单。安装使用官方目录给出的描述，缓存发现使用原目录名保存的摘要；两者共用摘要、大小、JSON 与结构验证入口。临时目录不再绕过清单摘要检查，发布前仍验证实际文件并探测 Java 版本。保留公开接口、旧缓存布局、系统 Java 优先和显式路径失败不回退。
2. `transfer.Retry` 保留成功尝试的结果，失败时才优先返回取消原因。文件已经发布后发生取消，不再把此次提交改报失败；提交前取消仍由实际写入操作检查。
3. 账号提交结果分开表达最新内存凭据、加密落盘和配置提交。保存失败保留凭据；OAuth 轮换后的中途失败不丢令牌，皮肤站不恢复旧令牌。每次授权都重试保存，有效缓存会话或新的认证网络错误均不能跳过之前失败的持久化。
4. 两种账号的移除统一为先凭据后配置。删除已完成时清空内存；配置失败保留原选择和移除重试入口。登录弹窗直接重试保存已验证结果，浏览器授权完成时若其他任务忙碌，不并发写入旧快照。

## 本次解耦轮次

账号提交的收口标准已满足：桌面登录与移除、启动授权、外观刷新接入共享用例，配置失败返回原快照，最新凭据与实际完成的操作保留；两提供者的保存及删除失败回归通过。两份文件之间仍可能部分成功，这由结果与错误显式表达，未引入通用事务或账号注册框架。

**任务记录与进度已解耦。** [`tasks.Resource.Kind`](../internal/tasks/record.go) 保存稳定字符串，应用层重试时转换为 `content.Kind` 并检查实例兼容性。`tasks` 已移除对 `content` 的依赖，只使用进度的 Minecraft 运行时因此不再间接引入资源下载模块。旧 JSON、未知资源标识和任务身份原样保留；未知类型不能重试实例安装。

**游戏用例装配已收口。** `Game.Transport` 贯穿游戏与 Java 安装、整合包预览和依赖解析、文件下载及皮肤站 JVM 准备。私有测试安装入口已移除，原有整合包、Forge / NeoForge、资源安装及文件夹导出测试直接调用公开用例，覆盖下载校验、取消、配置失败回滚和缓存修复。CurseForge API 密钥继续只发到官方 API；主加载器选择在 Forge / NeoForge 集成用例中验证。

**账号授权装配已收口。** `Store.Accounts` 提供 Microsoft 刷新和皮肤站验证，`Game.Execute` 沿用传入 Store 的账号客户端。公开用例覆盖轮换后的失败、网络与保存同时失败、恢复存储后的重试和档案提交；凭据保持封装，配置模块继续只保存展示与选择信息。

后端职责划分和本次识别的装配缺口已落实。`minecraft` 对 `account.LaunchIdentity` 的依赖保留经过校验的身份契约；任务历史负责存储标识，应用层解释重试规则。目录、账号、服务器和传输模块保留各自的可注入接口，后续功能按现有扩展约定进入所属模块。

## 后续功能与实机验收

**按需求扩展：** 应用级结构化日志与轮转、桌面语言切换、非 Windows 受管 Java、断点恢复调度。当前日志仅覆盖游戏和加载器安装器；语言在进程启动时选择，桌面标签仍为中文。这些能力尚未实现，不算本轮已完成项。

## 验证范围

Java / 传输轮次仅新增两项回归：下载完成后将清单替换为同长度合法 JSON 时拒绝发布；重试函数在成功或失败返回时同时取消的结果。两项均在修复前复现失败，修复后通过。其余复用现有缓存复用与修复、自选 Java、取消清理、Forge / NeoForge、资源发布和启动流程测试。

该轮次在本机 Windows x64 通过以下七个包的测试及 `go vet`：

```text
./internal/javaruntime ./internal/transfer ./internal/minecraft
./internal/distribution ./internal/content ./internal/catalog ./internal/launcher
```

Java / 传输实现 `9e23ffc` 的 [CI](https://github.com/CAUCraft/LXMCL/actions/runs/42972836159) 已通过 Linux 全部 `internal` 包测试，以及 Windows 全量测试、静态检查、便携版与安装包打包；标签发布步骤按预期跳过。

后续账号收口仅新增两个应用用例回归，共用协议夹具，覆盖两提供者的凭据保存失败、配置保存失败、重试及删除后配置失败；同时扩展原有 Microsoft 协议用例检查中途失败保留轮换令牌，扩展原有登录弹窗用例检查忙碌时保留候选供保存重试。有效缓存会话不能绕过保存也在上述用例中检查。本机 `go test ./...`、`go vet ./...`、格式、文案键及本地敏感文件检查通过；最终跨平台构建状态记录在 PR 检查中。

任务依赖解耦轮次扩展原有旧历史读写及实例重试用例，覆盖未知类型保留、旧资源字段往返和未知类型拒绝重试。本机 Windows 全量 `go test ./...`、`go vet ./...` 通过；`go list` 确认 Minecraft 的传递依赖已移除 `content`、`distribution` 和 `catalog`。

游戏用例装配轮次复用原有集成测试，删除测试中手工组装的运行时和安装回调；文件夹导出从空缓存下载真实夹具字节。公开入口的全量 `go test ./...`、`go vet ./...` 及差异格式检查在本机 Windows 通过。

账号授权装配轮次增加两个公开用例回归，复用现有协议夹具，覆盖轮换后的失败与重存、双重错误链、启动中止和配置快照归属。本机 Windows 全量 `go test ./...`、`go vet ./...`、格式及本地敏感文件检查通过。任务轮次 `19aca34` 与游戏轮次 `1169d41` 的远端 CI 已通过 Linux 后端测试和 Windows 测试、静态检查及打包；账号轮次的跨平台结果以分支 CI 为准。

仍需单独实测的发布条件：真实在线登录与进入 Minecraft、Windows FAT32 / exFAT 和网络共享上的文件发布，以及批量下载新增 `Sync` 的性能。已有 Java 探测和替身进程测试只证明进程与参数流程，不证明游戏可玩。

文件与目录事务继续采用[架构文档](architecture.md)中的范围：同一实例操作由应用串行，存档恢复允许保留已发布世界，资源启停不承诺跨进程原子性，文件发布不承诺突然断电时目录持久性。游戏日志路径检查也不是对任意外部导入历史的实例来源鉴权。

# 后端架构进度审查

审查日期：2026-11-11。对象：`codex/backend-modules`，本轮实现提交为 `01032fa`、`9e23ffc`，[PR #1](https://github.com/CAUCraft/LXMCL/pull/1) 合入目标为 `main`。

核心后端分层已经落地，日志与 JSON 文案目录已接入实际调用路径。账号提交流程仍有跨层编排与部分失败语义待收口，因此不能把本次结果表述为所有后端工作均已结束。本审查按实际职责、依赖和调用方判断完成度，不按文件数量或主观百分比计进度。

## 已完成的职责划分

| 范围 | 当前实现及依据 | 完成情况 |
| --- | --- | --- |
| 应用编排 | [`Game.Execute`](../internal/launcher/launch_operation.go) 接收提交时快照，协调授权、安装、一次修复和启动；业务包不反向导入 `launcher` 或桌面。 | 核心分层已完成；账号提交见下文。 |
| 配置与实例 | [`config.Store`](../internal/config/store.go) 管理配置与旧版迁移；[`instance_ids.go`](../internal/config/instance_ids.go) 管理编号；[`installation.go`](../internal/config/installation.go) 准备安装草稿。 | 已实现；保留 ID、目录、修订与旧数据。 |
| 游戏与 Java | [`minecraft.Runtime`](../internal/minecraft/runtime.go) 管理安装、校验和启动；[`javaruntime.Manager`](../internal/javaruntime/manager.go) 管理选择与安装，缓存验证和清单解析独立维护。 | 已实现；Java 自动安装限 Windows x64。 |
| 账号协议与凭据 | [`account.Client`](../internal/account/client.go) 管理协议；[`Vault`](../internal/account/vault.go) 管理凭据，Windows 使用 DPAPI；[`identity.go`](../internal/account/identity.go) 提供校验后的启动身份。 | 协议与存储已分离；跨文件提交仍待收口。 |
| 平台、资源与整合包 | `catalog` 管理元数据，`distribution` 管理下载缓存，`content` 管理实例资源，`modpack` 管理计划和安装事务；游戏安装与配置提交通过回调注入。 | 已实现；无反向依赖应用层，沿用原数据格式。 |
| 任务、服务器与存档 | [`tasks.Journal`](../internal/tasks/journal.go) 管理历史；[`server.Client`](../internal/server/client.go) 管理查询；`saves` 分离计划、归档和发布。 | 已实现；任务资源类型仍有轻量依赖，见下文。 |
| 文件与传输 | `storage.PublishNew` / `ReplaceFile` 明确区分禁止覆盖与允许替换；`transfer.Artifact` 封装摘要与大小约束，`Retry` 管理有限重试。 | 已实现；本轮修复成功提交后迟到取消的结果。 |
| 日志与国际化 | [`log`](../internal/log/game.go) 接管游戏和安装器日志创建及游戏日志尾部预览；[`i18n`](../internal/i18n/i18n.go) 使用嵌入的中英 JSON 键值目录、回退和错误链。 | 已接入；桌面标签、应用级日志轮转尚未实现。 |

本轮用 `go list -f '{{.ImportPath}} {{join .Imports " "}}' ./internal/...` 检查实际依赖。所有生产后端包均无 UI 依赖，其他业务包不导入 `launcher`。`launcher` 不再导入文件系统、子进程、归档、摘要和 JSON 编解码包；保留的跨模块调用是应用流程的一部分。原生窗口、依赖栈和品牌资源保持原状。

## 本轮完成

1. Java 验证显式接收预期清单。安装使用官方目录给出的描述，缓存发现使用原目录名保存的摘要；两者共用摘要、大小、JSON 与结构验证入口。临时目录不再绕过清单摘要检查，发布前仍验证实际文件并探测 Java 版本。保留公开接口、旧缓存布局、系统 Java 优先和显式路径失败不回退。
2. `transfer.Retry` 保留成功尝试的结果，失败时才优先返回取消原因。文件已经发布后发生取消，不再把此次提交改报失败；提交前取消仍由实际写入操作检查。
3. 更正 `LaunchResult` 注释中的持久化承诺：`CredentialsChanged` 代表 Microsoft 凭据已在内存刷新，保存失败时返回错误并清空游戏会话；`StateChanged` 才代表配置已保存。本轮仅澄清现状，未改变账号流程。

## 剩余工作与优先级

**下一轮优先：统一账号提交用例。** [`desktop_accounts.go`](../desktop_accounts.go) 和 [`desktop_station.go`](../desktop_station.go) 仍决定凭据与配置的保存顺序，启动刷新则在 [`launch_auth.go`](../internal/launcher/launch_auth.go)。Microsoft 凭据保存成功、配置保存失败时，两份文件可能处于不同阶段；刷新后凭据保存失败时，内存会保留新凭据但不能启动游戏。这些路径目前没有统一的结果契约及针对保存失败的回归。

收口标准：由应用层账号用例统一保存顺序，结果分别表达内存刷新、凭据持久化和账号选择提交，桌面仅负责后台调度和展示结果。明确各阶段失败后可重试的状态，保留可能已轮换的刷新凭据；复用协议测试，仅增加必要的保存失败回归。不能靠分别写两个文件就宣称跨文件原子提交，也不需要账号提供者注册框架。

**次要依赖整理：任务记录与进度。** [`tasks.Record.Resource`](../internal/tasks/record.go) 使用 `content.Kind`，使只需要 `tasks.Progress` 的 Minecraft 运行时也间接依赖资源下载模块。目前没有循环依赖或行为缺陷；若后续需要独立运行时或任务历史，可用稳定的任务资源标识收窄依赖，同时保留旧 JSON。无需单为缩短依赖图增加通用事件总线。

**按需求扩展：** 应用级结构化日志与轮转、桌面语言切换、非 Windows 受管 Java、断点恢复调度。当前日志仅覆盖游戏和加载器安装器；语言在进程启动时选择，桌面标签仍为中文。这些能力尚未实现，不算本轮已完成项。

## 验证范围

本轮仅新增两项回归：下载完成后将清单替换为同长度合法 JSON 时拒绝发布；重试函数在成功或失败返回时同时取消的结果。两项均在修复前复现失败，修复后通过。其余复用现有缓存复用与修复、自选 Java、取消清理、Forge / NeoForge、资源发布和启动流程测试。

本机 Windows x64 通过以下七个包的测试及 `go vet`：

```text
./internal/javaruntime ./internal/transfer ./internal/minecraft
./internal/distribution ./internal/content ./internal/catalog ./internal/launcher
```

Go 格式、JSON 文案键、本地敏感文件和差异空白检查通过。实现提交 `9e23ffc` 的 [CI](https://github.com/CAUCraft/LXMCL/actions/runs/42972836159) 已通过 Linux 全部 `internal` 包测试，以及 Windows 全量测试、静态检查、便携版与安装包打包；标签发布步骤按预期跳过。此后本轮仅补充审查文档和契约注释。

仍需单独实测的发布条件：真实在线登录与进入 Minecraft、Windows FAT32 / exFAT 和网络共享上的文件发布，以及批量下载新增 `Sync` 的性能。已有 Java 探测和替身进程测试只证明进程与参数流程，不证明游戏可玩。

文件与目录事务继续采用[架构文档](architecture.md)中的范围：同一实例操作由应用串行，存档恢复允许保留已发布世界，资源启停不承诺跨进程原子性，文件发布不承诺突然断电时目录持久性。游戏日志路径检查也不是对任意外部导入历史的实例来源鉴权。

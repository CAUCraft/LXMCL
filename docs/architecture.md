# 架构

LXMCL 的主包绘制 MyGo 原生窗口，持有当前页面、表单草稿和运行中任务。桌面提交操作时捕获目标快照，由后台处理文件、网络与子进程，再回到 UI 线程更新界面。

## 模块

| 目录 | 职责 |
| --- | --- |
| [launcher](../internal/launcher) | 组合安装、授权、启动与重试用例，向桌面提供统一入口 |
| [config](../internal/config) 和 [instance](../internal/instance) | 读取和迁移配置，管理实例 ID、修订、启动选择与参数校验 |
| [account](../internal/account) | Microsoft 和皮肤站协议、外观与凭据存储 |
| [minecraft](../internal/minecraft) 和 [javaruntime](../internal/javaruntime) | 查询版本，安装并校验游戏、加载器及 Java，组装参数并启动进程 |
| [catalog](../internal/catalog) 和 [distribution](../internal/distribution) | 查询项目、版本与依赖，按平台规则下载、校验、缓存或导出文件 |
| [content](../internal/content) 和 [modpack](../internal/modpack) | 导入与启停实例资源，安装、导出和更新整合包 |
| [server](../internal/server) 和 [saves](../internal/saves) | 规范化联机地址，查询服务器状态，备份与恢复世界 |
| [tasks](../internal/tasks) 和 [log](../internal/log) | 记录任务结果、进度与目标，管理游戏和安装器日志 |
| [transfer](../internal/transfer)、[storage](../internal/storage)、[process](../internal/process) 和 [i18n](../internal/i18n) | 提供校验、重试、文件发布、平台进程属性和后端文案 |

领域包独立于桌面和 `launcher`。网络扩展通过用例注入传输，`launcher.Game.Transport` 负责安装与启动准备，`launcher.Store.Accounts` 负责账号授权。各协议模块管理来源、凭据、超时和重定向。

## 实例与启动

- `config.Store` 优先读取 `launcher.json`。首次迁移读取旧配置并保留原文件、实例 ID 和游戏目录。移除实例只移除配置。
- 修改配置前调用 `State.Clone`。实例修改校验 ID 与修订，`PrepareInstallation` 分配新目录并保留已有启动选择。调用方串行提交配置。
- `LaunchRequest.Snapshot` 捕获实例、设置、账号和服务器。`Game.Execute` 完成授权、必要的安装与启动，已安装实例遇到可修复的文件错误时重试一次。`Running` 表示 Java 进程已创建，游戏可玩性通过进入游戏验证。
- 显式 Java 路径优先。自动模式先检查已有运行时，Windows x64 缺少兼容版本时按 Mojang 元数据安装。发布前校验运行时清单与文件，并探测实际 Java 版本。
- Windows 使用 DPAPI 保存账号凭据，其他平台仅保留当前会话。登录和移除分别提交凭据与配置，`AccountCommit` 报告每一步的结果。令牌轮换后保留最新内存会话，保存失败可重试。

## 下载与文件

- `catalog` 返回带来源、大小和摘要的文件描述。`distribution` 管理缓存与导出，`content` 按实例检查资源兼容性和归档内容。普通资源安装选中的文件，整合包按清单下载必需依赖。
- `storage.PublishNew` 在目标目录准备临时文件，校验、同步并关闭后发布，同名文件会保留。`ReplaceFile` 用于可替换的缓存和配置。取消或校验失败时清理本次临时文件。
- 新实例在同级临时目录安装，发布目录后提交配置。原地更新将清单与实例的 `.lxmcl-pack.json` 比对，应用前复核差异。提交失败时回滚文件，恢复失败时报告备份位置。
- 存档应在游戏停止写入后备份。恢复先解压并校验整份 ZIP，再逐个发布世界。同名世界自动加后缀，发布阶段中断时保留已恢复的世界。

## 任务与诊断

`tasks.Run` 管理一次任务的进度回调，`Journal` 串行写入历史。重试实例任务时核对原实例 ID、修订、游戏版本和相关账号。队列绑定提交时的资源版本与目标，每个文件单独记入历史。资源重试沿用该版本和目标。失败通知通过记录 ID 打开对应详情。整合包更新重试时校验原归档的 SHA-256，重新预览当前文件差异。

游戏日志位于实例的 `launcher-logs/`，加载器安装日志位于数据目录的 `cache/installer-logs/`。界面预览游戏日志尾部，完整内容可从日志窗口打开。

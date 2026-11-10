# 开发指南

在 Windows x64 上安装 Go 1.27.3 或更新版本。MyGo 与 Lobe MyGo 的版本由 `go.mod` 固定。应用运行不依赖 Git LFS 视频。

```powershell
go run .
go tool mygo dev
go test ./...
go vet ./...
$env:NSISCONFDIR = (Resolve-Path packaging).Path
go tool mygo build -platform windows/amd64
```

`go tool mygo dev` 会在 Go 文件改动后重启应用，默认使用单独的开发数据目录。用 `go run . -data-dir .local/mygo-check` 可指定测试目录。代码分层见[架构](architecture.md)，界面约定见 [UI Skill](../.agents/skills/lxmcl-ui/SKILL.md)。

后端文案集中在 [简体中文](../internal/i18n/locales/zh-CN.json)和[英文](../internal/i18n/locales/en.json) JSON。新增文案时在两份文件中使用同一个 `模块.含义` 键，保持格式占位符一致；需要保留错误原因时使用 `i18n.Error` 和 `%w`，进度与标签使用 `i18n.Text`。默认使用简体中文。启动前设置 `$env:LXMCL_LOCALE = 'en'` 可查看英文后端消息，桌面界面标签仍使用中文。缺少译文时回退到简体中文。

## 数据

默认根目录为 `%LOCALAPPDATA%\org.caucraft.lxmcl`：`launcher.json` 保存设置和实例，`tasks.json` 保存最近 24 条任务，`cache` 保存共享的客户端、库和资源。首次迁移会读取旧版配置。存档、游戏设置和 `launcher-logs` 位于各实例目录；移除实例配置不会删除这些文件。

实例可继承或覆盖全局 Java 与内存，并指定启动分辨率或全屏。检测 Java 读取实际版本元数据，只检查本地运行时；游戏启动或加载器安装缺少 Java 时，Windows x64 会根据元数据中的组件和最低版本下载 Mojang 官方运行时。缓存位于 `cache/java/windows-x64/`，文件与清单按官方 SHA-1 和大小校验，实际运行 Java 探测版本后才发布安装目录。指定 Java 路径时不会自动替换。复制实例配置只复制版本和启动设置，并分配新目录。

「下载 → 游戏版本」从官方目录选 Minecraft 和加载器版本。原版不需要额外配置，Fabric、Forge、NeoForge 和 Quilt 会分别列出可选版本，预览版单独标注。「安装实例」会保存配置并下载游戏文件，不启动游戏，也不切换已有的启动目标。失败或取消后可在下载任务中重试。

「下载 → 整合包」从 Modrinth 或 CurseForge 选项目和版本，也可用系统文件窗口导入 Modrinth `.mrpack` 或 CurseForge 导出的 `.zip`。确认名称后下载必需的客户端依赖并应用包内文件；选装文件和服务器专用文件默认跳过。整合包与游戏文件校验完毕后才登记新实例。取消或失败后，可返回安装页重试，已校验的下载可以复用。

整合包按清单安装指定版本的 Fabric、Forge、NeoForge 或 Quilt。Forge 和 NeoForge 使用经过校验的官方客户端安装器，并复用上述 Java 解析与安装流程。网络不可用时可在「设置」指定兼容 Java 后重试。Java 版本要求以 Minecraft 元数据为准，安装器日志位于数据目录的 `cache/installer-logs/`。

后端扩展中，平台文件下载和摘要缓存由 `internal/distribution` 管理，整合包预览、清单与安装事务由 `internal/modpack` 管理。安装通过 `InstallOptions` 注入游戏安装和配置提交，依赖查询通过 `Resolver` 注入，不需要导入 `launcher` 或改全局 HTTP 客户端。可用本地归档和可控传输验证取消、归档变更、依赖缺项及提交失败回滚。

项目搜索、版本查询、前置浏览和图标读取由 `internal/catalog.Client` 提供，通过 `Transport` 注入测试传输。目录模块不读写配置或实例文件；可下载版本须包含有效来源、大小和摘要。前置浏览保持只读，作者指定版本不会被自动替换，普通资源安装不会自动下载前置。CurseForge 依赖解析通过 `CurseForgeResolver` 接入整合包安装，API 密钥只发送到 CurseForge 官方 API。

NeoForge 官方 Maven 请求失败时，启动器会尝试 NeoForge CDN。NeoForge、Forge 和 Fabric Maven 文件可回退到 BMCLAPI 镜像，始终沿用加载器元数据或官方 Maven 提供的校验值。校验地址遇到临时网络错误时会限次重试。所有来源都无法提供匹配文件时，任务失败并保留可复用的已校验缓存。

CurseForge 在线查询和下载依赖需要 [CurseForge API Key](https://console.curseforge.com/)。复制 [`.env.example`](../.env.example) 为 `.env`，在 `LXMCL_CURSEFORGE_API_KEY=` 后填入密钥并重启。启动器依次检查数据目录、当前工作目录和程序目录，使用第一个包含密钥的 `.env`。进程环境变量 `LXMCL_CURSEFORGE_API_KEY` 或 `CURSEFORGE_API_KEY` 优先于文件；页面输入只在本次运行中覆盖。不要提交含密钥的 `.env`。如果作者未开放某个必需文件的第三方下载，安装会报告文件并停止。

本地 API 密钥放在 `.env`，仓库模板只保留空变量。CI 所需凭据通过 GitHub Secrets 或运行时令牌提供。Microsoft 和皮肤站会话由启动器管理，在 Windows 上使用系统加密存储；公开的 Microsoft OAuth 客户端 ID 不属于密钥。CI 在构建前检查本地配置和凭据文件是否被 Git 跟踪。

「下载」里的模组、资源包、光影包和整合包可以直接保存到自选文件夹，不需要创建实例。独立下载只保存选中文件并校验摘要，同名文件不会被覆盖。模组、资源包和光影包任务重试时保留原文件夹和版本。

从实例的「资源」页点「添加」会绑定该实例。Modrinth 按实例的游戏版本及 Fabric、Forge、NeoForge 或 Quilt 筛选。单个模组、资源包和光影包只保存或安装选中文件；必要前置在「查看前置」中列出，点击可打开项目或作者指定的版本。安装时仍检查文件 SHA-512 和对应的 JAR 或 ZIP 内容。任务重试绑定原实例、配置修订和资源版本。启动时按原流程校验并补齐缺失的游戏文件；显式安装整合包仍按其清单处理。

存档备份位于数据目录的 `backups/`，恢复支持包含 `世界名/level.dat` 的 ZIP，不覆盖已有世界。

Microsoft 登录在系统浏览器授权，随后检查 Xbox、Java 版权益和档案。账号协议和外观操作由 `internal/account` 管理，可通过 `account.Client.Transport` 注入测试传输，不需要改全局 HTTP 客户端。`account.Vault` 在 Windows 使用 DPAPI 加密凭据，保留已有文件名和格式；其他平台仅保留当前会话。更换构建的公开应用 ID 时，旧账号需要重新登录。

点击顶栏头像进入独立账号中心。拖动 3D 角色可旋转，也可用视口下方的按钮转向背面或重置。选择 64 × 64 或 64 × 32 的 PNG 可在上传前预览；已拥有的披风可直接试穿在同一角色上。点击应用后才会更新 Minecraft Java 版档案。离线账号的孪枭头像和模型仅在启动器内显示。

点击账号列表的「＋」→「第三方皮肤站」，填写支持密码登录的 HTTPS 站点或 Yggdrasil API（例如 `https://littleskin.cn/api/yggdrasil`），登录并选定角色。加入联机时使用配置了同一认证服务的服务器。皮肤和披风在站内修改，返回启动器后可刷新 3D 预览。启动器不保存密码；Windows 加密保存会话，其他平台退出后需重新登录。首次用皮肤站账号启动时会下载并校验 authlib-injector，文件保存在共享缓存。

## 验证与发布

[CI](https://github.com/CAUCraft/LXMCL/actions) 在 Windows 上运行测试和打包，在 Linux 上运行全部 `internal` 包的测试。自动安装 Java 的平台测试在 Windows x64 运行，清单校验等测试可跨平台运行。新增后端行为须覆盖任务取消、失败后无成品发布及旧数据兼容等相关边界；可使用可控的 HTTP transport 验证文件字节与摘要。用 `$env:LXMCL_LOCALE = 'en'; go test ./...` 检查英文目录。UI 测试使用软件渲染，可通过 `LXMCL_UI_CAPTURE` 保存截图；系统窗控、输入法及实际进入游戏需要另行验证。

Windows 构建产物保留 14 天，包含便携版、安装包、许可归档、对应源码和 SHA-256 清单。源码包包含构建所需的 Go 依赖，解压后按上方命令构建。`NSISCONFDIR` 让安装器使用 zlib。更新 `mygo.json` 版本后推送对应的 `v主.次.修订` 标签，CI 通过后生成 Release 草稿。更新依赖时同步核对[第三方许可](../THIRD_PARTY_NOTICES.txt)。

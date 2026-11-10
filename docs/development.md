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

## 数据

默认根目录为 `%LOCALAPPDATA%\org.caucraft.lxmcl`：`launcher.json` 保存设置和实例，`tasks.json` 保存最近 24 条任务，`cache` 保存共享的客户端、库和资源。首次迁移会读取旧版配置。存档、游戏设置和 `launcher-logs` 位于各实例目录；移除实例配置不会删除这些文件。

实例可继承或覆盖全局 Java 与内存，并指定启动分辨率或全屏。检测 Java 读取实际版本元数据；复制实例配置只复制版本和启动设置，并分配新目录。

Fabric 加载器与库来自 Fabric Meta 和 Maven；Modrinth 文件按 SHA-512 校验。本地模组和资源包分别需要 `fabric.mod.json` 和 `pack.mcmeta`。存档备份位于数据目录的 `backups/`，恢复支持包含 `世界名/level.dat` 的 ZIP，不覆盖已有世界。

Microsoft 登录在系统浏览器授权，随后检查 Xbox、Java 版权益和档案。Windows 使用 DPAPI 加密刷新凭据，其他平台仅保留当前会话。更换构建的公开应用 ID 时，旧账号需要重新登录。

点击顶栏头像进入独立账号中心。拖动 3D 角色可旋转，也可用视口下方的按钮转向背面或重置。选择 64 × 64 或 64 × 32 的 PNG 可在上传前预览；已拥有的披风可直接试穿在同一角色上。点击应用后才会更新 Minecraft Java 版档案。离线账号的孪枭头像和模型仅在启动器内显示。

点击账号列表的「＋」→「第三方皮肤站」，填写支持密码登录的 HTTPS 站点或 Yggdrasil API（例如 `https://littleskin.cn/api/yggdrasil`），登录并选定角色。加入联机时使用配置了同一认证服务的服务器。皮肤和披风在站内修改，返回启动器后可刷新 3D 预览。启动器不保存密码；Windows 加密保存会话，其他平台退出后需重新登录。首次用皮肤站账号启动时会下载并校验 authlib-injector，文件保存在共享缓存。

## 验证与发布

[CI](https://github.com/CAUCraft/LXMCL/actions) 在 Windows 上运行测试和打包，在 Linux 上运行启动器核心测试。UI 测试使用软件渲染，可通过 `LXMCL_UI_CAPTURE` 保存截图；系统窗控、输入法及实际进入游戏需要另行验证。

Windows 构建产物保留 14 天，包含便携版、安装包、许可归档、对应源码和 SHA-256 清单。源码包包含构建所需的 Go 依赖，解压后按上方命令构建。`NSISCONFDIR` 让安装器使用 zlib。更新 `mygo.json` 版本后推送对应的 `v主.次.修订` 标签，CI 通过后生成 Release 草稿。更新依赖时同步核对[第三方许可](../THIRD_PARTY_NOTICES.txt)。

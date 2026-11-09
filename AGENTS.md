# LXMCL 协作约定

修改前读 [README](README.md) 和[开发指南](docs/development.md)，检查源码与未提交变更。[架构](docs/architecture.md)记录当前边界。

- 启动器使用 Go、MyGo 原生窗口和 Lobe MyGo 组件。保持无 WebView、无前端打包链。
- UI 修改先读 [lxmcl-ui Skill](.agents/skills/lxmcl-ui/SKILL.md)。优先直接使用 Lobe 组件，只对布局、品牌和必要反馈做少量调整。
- 文件、网络和子进程操作在后台执行。进度、错误和结果关联到提交时的实例；浏览详情不改变启动目标。
- 保留实例 ID、游戏目录、旧数据以及原 Logo 与孪枭皮肤。下载完成须有经过校验的实际文件；Java 进程启动与游戏可玩是不同的验证。
- 截图、测试数据、过程日志和产物写入 `target/` 或 `.local/`。按改动运行必要的检查，并说明实测范围。

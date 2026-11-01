# LXMCL 协作入口

本项目是 Rust + Slint 桌面启动器。修改前先看 [README](README.md) 与 [开发交接](docs/development.md)。

- 修改 UI、动效、新手流程或前后端展示契约时，读取项目技能 [.agents/skills/lxmcl-ui/SKILL.md](.agents/skills/lxmcl-ui/SKILL.md)，再按需读它链接的规范。
- 自己维护的应用逻辑和新增工具使用 Rust；允许 Slint。第三方实现语言不限。历史设计脚本不进入应用运行路径。
- 当前游戏 / 认证 / 下载 / 诊断是演示。保留演示标识，不能把定时器结果描述成真实扫描或真实服务成功。
- 当前产品方向为像素玻璃与经典风格，不添加航空 / 航天主题。孪枭使用双层 `ui/assets/luanxiao.png`，原始品牌素材保持原样。
- 性能优先：动画只追最新目标，静止停表；列表增量更新；阻塞 I/O 留给工作线程。
- 保留已有未提交修改。按改动范围验证，并在 [验证记录](docs/validation.md) 中如实区分编译、测试和人工窗口检查。
- 优先参考成熟开源实现，围绕实际需求开发；避免多层兜底、提前抽象、重复测试和新增不必要的示例。
- 每完成一个功能模块并通过必要验证，提交一次对应改动，不必再次询问；用户明确要求暂不提交时遵从。提交信息遵循 [.agents/skills/lxmcl-commit-msg/SKILL.md](.agents/skills/lxmcl-commit-msg/SKILL.md)，先看暂存区和最近提交风格，默认使用一行英文。只读审查不创建提交；保留无关修改与暂存状态，不自动推送。

历史 Figma / HTML 位于 `design/archive/v0.1/`，不是当前运行界面的设计基线。素材更新遵守 Git LFS 约定；不要提交 `target/`、私人偏好、令牌或本机下载目录。

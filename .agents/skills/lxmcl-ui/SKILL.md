---
name: lxmcl-ui
description: 实施和审查 LXMCL 的 Go、MyGo 与 Lobe MyGo 原生桌面界面。
---

# LXMCL UI

先读 [开发指南](../../../docs/development.md)，使用 [Lobe MyGo 组件文档](https://mygo.lobehub.com/skills.md)选择控件，优先沿用组件的样式、键盘行为和默认动效。

- 保留 LXMCL 原 Logo 和河谷首页。主页、版本、联机和设置居中放在 42 DIP 合并顶栏；任务快捷入口和账号位于系统窗控旁。品牌和空白处可拖动，交互控件保持可点击。
- 首页使用完整实例集合。浏览实例详情不更换启动选择。保存、启动和目录动作使用捕获的实例目标。
- 表单用 Lobe Input、Select 和 Modal，任务使用 Progress，错误使用 Alert。页面不复刻组件或引入网页运行链。减少动态效果跟随系统设置。
- 首次运行、空实例、下载失败及 Java 不可用需提供可操作的反馈。未接入的功能如实标记。
- 首页集中于实例选择和启动。实例列表把搜索、筛选与新建放在同一工具栏；常用动作直接可达，其他操作收进菜单。首页和实例详情使用同一启动按钮与状态。任务重试只绑定原实例和修订。设置用标签与控件对齐的行，只有有修改时显示应用和还原。说明只保留状态、错误和后果。交互参考 [PCL](https://github.com/Meloong-Git/PCL)、[HMCL](https://docs.hmcl.net/launcher/gui-features.html) 和 [SJMCL](https://github.com/UNIkeEN/SJMCL)，不引入其代码或美术。
- 实例详情用「概览／启动设置／资源／游戏文件」分组。启动设置默认继承全局；草稿按实例和修订隔离。资源下载和存档操作显示在任务中心。Java 错误引导至实际生效的设置。
- 顶栏头像打开独立账号中心。左侧浏览账号，右侧用原生 3D 视口预览皮肤和披风；更改需显式应用。Microsoft 登录在系统浏览器完成，对话框只显示设备代码和操作按钮，任务重试不跨身份。
- 用 MyGo headless 测试验证导航和弹窗；窗控、DPI、中文输入和真实启动需要独立验收。截图放进 `target/`。

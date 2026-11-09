---
name: lxmcl-ui
description: 实施和审查 LXMCL 的 Go、MyGo 与 Lobe MyGo 原生桌面界面。
---

# LXMCL UI

先读 [开发指南](../../../docs/development.md)，使用 [Lobe MyGo 组件文档](https://mygo.lobehub.com/skills.md)选择控件，优先沿用组件的样式、键盘行为和默认动效。

- 保留 LXMCL 原 Logo 和河谷首页。主导航提供主页、版本、联机和设置。Windows 合并顶栏保留系统窗控，品牌和空白处可拖动，交互控件保持可点击。
- 首页使用完整实例集合。浏览实例详情不更换启动选择。保存、启动和目录动作使用捕获的实例目标。
- 表单用 Lobe Input、Select 和 Modal，任务使用 Progress，错误使用 Alert。页面不复刻组件或引入网页运行链。减少动态效果跟随系统设置。
- 首次运行、空实例、下载失败及 Java 不可用需提供可操作的反馈。未接入的功能如实标记。
- 用 MyGo headless 测试验证导航和弹窗；窗控、DPI、中文输入和真实启动需要独立验收。截图放进 `target/`。

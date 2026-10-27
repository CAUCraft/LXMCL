# LXMCL · UI 设计评审包

> 历史归档：2026-10-26 的 Figma / HTML 方案。下文保留当时的交付说明；当前 Slint 实现已继续迭代，请从 [当前 UI 文档](../../../docs/ui/README.md) 开始。这里的截图不代表当前程序。

**先打开 [index.html](index.html)**。无需安装依赖，可直接用浏览器打开；选择顶部风格和页面进行评审。所有游戏、网络、下载和登录行为均为交互演示，不调用后端。

## 内容

- `信息架构与交互规范.md`：功能分类、原需求映射、关键路径、三版差异、组件与异常态规范。
- `screens/glass-home.png`：液态玻璃首页。
- `screens/classic-home.png`：经典桌面首页。
- `screens/terminal-home.png`：极简深色首页。
- `screens/*-overview.jpg`：每种风格的 11 画面总览。
- `screens/*.svg`：33 份矢量画稿；PNG 仅用于预览。
- `figma-plugin/`：准备好的原生图层生成插件。
- `design-data.json`：共享场景数据；`build_design.py` / `render_qa.py` 用于重新生成。

## Figma 当前状态

已通过 Figma 桌面端完成原生绘制：[打开 LXMCL 设计文件](https://www.figma.com/design/wDdnB7HQaIpiE8rzkwU4VQ/LXMCL)。包含三种风格各 11 个界面，以及 1 张功能分类与交互路径图。绘制过程使用本地 Figma 插件，不调用 Figma AI。

已在实际文件核对：33 个 UI 画板、1865 个原生文本图层、248 个按钮实例、1166 个矢量图层、409 条跨页面导航；52 个当前页面入口无需重复跳转。字体为 Noto Sans SC。按钮母组件共 130 个，导航检查无错误。

设计以 Minecraft 启动器为主题，不添加航空图形或行业视觉隐喻。

可选的本地开发插件路径（需要 Figma 桌面端）：

1. 打开一个 Figma Design 文件。
2. 进入 Plugins → Development → Import plugin from manifest，选择此文件夹下的 `figma-plugin/manifest.json`。
3. 运行 `LXMCL — Three UI Directions`，保留需要的主题，点击“生成可编辑设计”。
4. 默认写入当前页面的新区域；勾选独立页面时会新增三页。已有内容不会被删除或覆盖。
5. 核对中文字体、按钮、玻璃效果和原型导航，再保存到目标文件。

插件不依赖私有插件元数据；无网络请求，设计数据全部随包提供。已有文件可使用插件底部的“检查图层与导航”“补充功能地图”和画板定位功能，避免重复生成。

## 验证与限制

- 33 个画面已渲染并进行文字宽度、画板边界检查；检查记录见 `qa-layout.json`。
- 插画由矢量路径构成，不是把整个界面压成截图；SVG 导入也可作为静态矢量备用方式。
- Figma 插件提供原生文本、形状、按钮组件、颜色样式、主要页面跳转；整个画板不是完整 Auto Layout，也没有实现所有状态变量。
- 浏览器交互验证受当前浏览器访问限制，已完成静态检查；真实点击与键盘链路还需在用户浏览器复核。
- HTML 支持主要路径与说明弹窗，不能启动 MC、安装模组、登录账号或执行系统清理。
- 方案评审后再细化全套二级页面、可用性测试和 Slint 适配。

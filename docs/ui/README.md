# LXMCL 当前 UI 文档

基线：2026-10-27 的 Rust + Slint 前端。需求输入来自《中国民航 MC 启动器研发 LXMCL》；这里只维护可随仓库交接的设计结果，原 Obsidian 需求文件未改动。

| 要解决的问题 | 阅读文档 |
|---|---|
| 功能放在哪里，是否漏了需求 | [信息架构与需求映射](information-architecture.md) |
| 新控件应该长什么样、怎样动 | [视觉与动效](visual-design.md) |
| 按钮点完发生什么，新手出错怎样恢复 | [交互与帮助](interaction.md) |
| 前后端传什么、哪些结果可以信任 | [接口与状态](service-contracts.md) |
| 从哪里开始改、哪些地方尚未完成 | [开发交接](../development.md) |
| 哪些结论有验证依据 | [验证记录](../validation.md) |

实际 Rust 类型以 `src/contracts.rs` 为准；视觉 token 以 `ui/theme.slint` 为准。需求映射与未来端口描述包含尚未实现的目标，不能据此宣称后端能力已经完成。界面演示必须保留演示标识。

当前有「经典 / 像素玻璃」两种风格，每种支持浅 / 深色；[v0.1 归档](../../design/archive/v0.1/README.md) 的三套 Figma 方案已经被后续 Slint 迭代替代。当前方向强调 Minecraft 像素与玻璃质感，不采用航空或航天主题。

竞品证据及采用理由集中在 [交互规范](interaction.md)；来源为 XMCL、SJMCL 官方文档和发布记录，未安装它们进行本机实测。不要将它们的完整技术栈或品牌资产带入本项目。

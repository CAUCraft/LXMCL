# 资源与渲染约束

外观、布局和动效只维护于[最终规范](REDESIGN.md)。本文保留素材与渲染边界，不保留旧侧栏尺寸、经典色板或像素玻璃视觉基线。

## 原件与背景

原 Logo 与 [luanxiao.png](../../../../ui/assets/luanxiao.png) 保持字节不变，派生资源另存。当前皮肤为 64×64 RGBA、经典 4 px 手臂。旧 club-skin 不是透明皮肤，不按白色抠图。首页不显示孪枭，账号页保留双层预览与原图头像裁取。

默认使用[日光河谷](../../../../ui/assets/daylight-river.png)，cover 裁切；旧 [scenic-home.png](../../../../ui/assets/scenic-home.png)及 [Rust 生成器](../../../../tools/scenic-background.rs)可作静态回退。不能用整张设计 UI 作背景。

七段视频与 LFS 原件保留，来源与分发见[素材说明](../../../../ui/assets/README.md)。缺文件、FFmpeg 或解码失败使用静态回退；动态壁纸不成为编译或首屏依赖。

## 皮肤与媒体

[skin_preview.rs](../../../../src/skin_preview.rs)保留基础与外层模型、最近邻采样、深度和背面剔除。帽层外扩 0.5 模型单位，身体与四肢外扩 0.25；头部两层共用颈部转轴，透明像素跳过。不能把当前二值 alpha 能力描述为完整半透明、披风与 slim 支持。历史 [mascot_motion.rs](../../../../src/mascot_motion.rs)跟随算法不构成新首页需求；不可见停止渲染，不抢占控件事件。

[wallpaper.rs](../../../../src/wallpaper.rs)后台 FFmpeg 管线只保留最新帧，暂停 / 最小化停解码，切换 / 退出清理子进程，UI 不同步等待回收；Windows 子进程隐藏窗口。当前不能称为硬件解码或零复制上传。

## 成本与验证

颜色集中于 [theme.slint](../../../../ui/theme.slint)，控件集中于 [components.slint](../../../../ui/components.slint)。兼容像素玻璃代码存在不表示恢复侧栏，也不表示系统透窗或实时模糊已实现。

默认 FemtoVG / OpenGL，保留合理软件回退。列表保留模型身份、增量更新；主题与弹窗不重建全部列表。运动只追随最新目标、闲置停表，不可见媒体停更或降频。手动减少动态效果和读取系统设置分别报告。

CPU 皮肤光栅化、开始绘制、提交耗时不是端到端输入延迟或游戏 FPS。截图与性能输出放入 `target/`，检查结论在交付说明 / PR 中报告。没有端到端测量时不推断流畅度。

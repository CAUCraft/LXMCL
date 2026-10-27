# 美术素材

美术部提供的三份素材均按原始文件复制，未改动其像素或路径：

| 项目文件 | 原始素材 | 使用位置 |
| --- | --- | --- |
| `launcher-logo.png` | 美术部 `Logo_128x128.png` | 侧栏、窗口图标 |
| `club-logo.svg` | 美术部 `logo (1).svg` | 社团卡片、联机页 |
| `luanxiao.png` | 美术部补充的 `luanxiao.png` | 孪枭的双层立体皮肤 |
| `club-skin.png` | 早期提供的扁平化皮肤 | 保留参考，已不参与渲染 |

`launcher.ico` 为原始 PNG 添加 ICO 容器，用于 Windows 可执行文件图标，未缩放原图。`club-logo-preview.png` 是检查 SVG 时使用的预览，程序直接使用原始 SVG。`hero-background.svg` 是代码绘制的渐变与圆形背景。

`glass-wallpaper-*.svg` 是像素玻璃主题的背景；`glass-hero-*.svg` 是保留作对比的旧版圆角材质，当前界面不再使用。玻璃阶梯边缘位于 `ui/pixel-glass.slint`，融合动效位于 `src/pixel_motion.rs`。`../icons/pixel/` 下是原生 16×16 网格图标；SVG 可由 `scripts/create-glass-assets.py` 重建，没有改绘或替换社团原始标识。

当前皮肤是 64 × 64 RGBA PNG。`src/skin_preview.rs` 渲染六个基础部件与六个外层，保留原始透明度；帽层外扩 0.5 模型单位，衣服和四肢外扩 0.25，头部双层同步转动。披风尚未接入。标准贴图坐标参考：[skinview3d 模型定义](https://github.com/bs-community/skinview3d/blob/master/src/model.ts)。

`wallpapers/` 包含用户提供的七段 Minecraft 视频的 720p / 24 fps 无音轨副本和封面，由已有的 `scripts/prepare-wallpapers.ps1` 生成，原始视频未修改。MP4 使用 Git LFS；JPG 封面直接进入 Git。开发者运行 `git lfs pull` 获取视频，无需访问提供者的本地下载路径。FFmpeg 运行依赖见 [项目说明](../../README.md)。

| 播放副本 | 原始提供文件名 |
|---|---|
| `cabin.mp4` | `Minecraft Overgrown Cabin_2_prob4.mp4` |
| `castle.mp4` | `Minecraft castle Fixed_2_prob4.mp4` |
| `aquarium.mp4` | `Minecraft Soothing Scenes – Relaxing Aquarium.mp4` |
| `falling-snow.mp4` | `Minecraft Soothing Scenes – Relaxing Falling Snow.mp4` |
| `fireplace.mp4` | `Minecraft Soothing Scenes – Relaxing Fireplace.mp4` |
| `swamp.mp4` | `Minecraft Soothing Scenes – Relaxing Rainy Swamp.mp4` |
| `serene-snow.mp4` | `Soothing Minecraft – Serene Snow.mp4` |

这里记录素材来源，不将外部视频或美术部作品声明为程序代码原创。当前仓库尚未选定统一许可证。

孪枭原始文件 SHA-256：`FE277C62DE9915ADCFB984562BB7E9E399C5D5BEA8DA8567CD7E0E62FCC68F61`。素材维护时可以据此确认没有意外改写原件。旧 `club-skin.png` 虽使用 PNG 扩展名，实际是 JPEG，不能用于需要透明通道的双层渲染。

# 插件开发上手（Phase 1）

面向第三方：用本框架写一个提供者插件，或组装自己的启动器。工具链 Rust 1.92（`rust-toolchain.toml` 固定），Slint 1.18.1。先读 [架构总览](../architecture/launcher-platform.md) 的角色与依赖方向，再按本文动手。

## 路线 A：给现有产品写一个服务提供者

以一个 TOML 设置提供者为例（完整代码见 `examples/custom-settings`，无任何 Slint 依赖）：

**1. 新建 crate，只依赖契约包**

```toml
[dependencies]
launcher-api = { path = "../crates/launcher-api" }   # 已发布后改为版本号
serde = { version = "1", features = ["derive"] }
toml = "0.9"
```

**2. 实现插件与服务**

```rust
use launcher_api::{
    Context, LauncherSettings, Plugin, PluginDescriptor, RuntimeError,
    ServiceRequirement, Settings, SettingsError, SettingsService,
};

pub struct TomlSettingsPlugin { path: std::path::PathBuf, defaults: LauncherSettings }

impl Plugin for TomlSettingsPlugin {
    fn descriptor(&self) -> PluginDescriptor {
        PluginDescriptor {
            id: "settings-toml-example",                       // 产品装配里的插件 ID
            provides: vec![ServiceRequirement::of::<Settings>()], // 提供设置服务
            requires: vec![],                                  // 按服务契约声明依赖
        }
    }

    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
        // apply 不做磁盘 I/O；只登记服务与清理。
        ctx.provide::<Settings>(std::sync::Arc::new(TomlSettings {
            path: self.path.clone(), defaults: self.defaults.clone(),
        }))
    }
}

impl SettingsService for TomlSettings {
    fn load(&self) -> Result<LauncherSettings, SettingsError> { /* 读文件、解析 */ }
    fn save(&self, settings: &LauncherSettings) -> Result<(), SettingsError> { /* 写文件 */ }
}
```

规则：I/O 只发生在 `load` / `save`（工作线程调用）；插件不写 UI；不调用其他插件的具体实现，只依赖契约；错误如实返回（文件损坏返回解析错误，不当首次运行，不自动修复）。

**3. 在产品装配处注册工厂**（LXMCL 在 `src/composition.rs` 的 `plugin_catalog`）：

```rust
catalog.register("settings-toml-example", move |config| {
    let config: TomlSettingsConfig = config.decode()?;   // [plugins.config] 的强类型解码
    Ok(Box::new(TomlSettingsPlugin::new(root.join(&config.file_name), config.defaults)))
})?;
```

**4. 在 `product.toml` 里启用**：

```toml
[[plugins]]
id = "settings"
provider = "settings-toml-example"

[plugins.config]
file_name = "launcher-settings.toml"

[plugins.config.defaults]
game_root = 'D:\Games\MyLauncher'
```

**5. 编译**：`cargo build --locked`。替换 provider 后 Shell 窗口代码一行不改。

## 路线 B：组装你自己的启动器

以 `examples/minimal-launcher` 为模板（它验证了不携带 LXMCL 控制器 / 品牌素材 / 演示状态也能跑通）：

1. 新建独立工作区的 crate（带空 `[workspace]` 表）。
2. 依赖：`launcher-runtime`、`launcher-api`、你选的提供者插件、`slint`（示例用 `=1.18.1`，关闭默认特性时需显式启用 `std/backend-winit/renderer-femtovg/compat-1-2`）+ `slint-build`。提交独立工作区的 `Cargo.lock`，才能用 `--locked` 构建。
3. `main` 装配顺序（照抄 `examples/minimal-launcher/src/main.rs`）：
   定位 `product.toml`（可执行文件目录向上查找）→ 解析数据目录 → `PluginCatalog` 注册工厂 → `PluginHost::start` → `host.service::<Settings>()` 取服务 → 自己写一份设置线程桥（参考示例的 `settings_bridge.rs`：有界邮筒、待保存快照合并、关闭后写完最后快照、`invoke_from_event_loop` 回 UI）→ 建窗口、绑回调 → `ui.run()` → 丢窗口 → join 线程（join 关闭新请求并 flush）→ `host.shutdown()`。
4. Shell 只调契约：`load_blocking` 取初值，`save_async` 传点击时的快照，回调里更新状态文本。

运行：`cargo run --manifest-path examples/minimal-launcher/Cargo.toml --locked`（控制台会打印插件状态行）。

## 调试与常见装配错误

错误消息即诊断：未知 provider = 工厂没注册或 `provider` 拼错；重复服务 = 两个插件提供同一服务键；依赖缺失 / 环 = descriptor 的 requires/provides 不一致；配置解码错误带 provider 名 = `[plugins.config]` 字段与该插件的 config 结构体不符。产品 ID 改变后数据目录随之改变（`%LOCALAPPDATA%\{id}`），旧配置不会迁移。

## Phase 1 的边界（不要误当能力）

- 编译期链接：加插件 = 加依赖 + 注册工厂 + 改配置 + 编译。没有运行期加载 / 热更新。
- 只有设置一个服务契约。账号、下载、Java、启动、实例、模组的契约尚未设计（归属见 [接口与状态约定](../ui/service-contracts.md)）。
- 插件与宿主同进程同权限，无沙箱；只依赖宿主自身的装配错误检查。
- 插件间协作只能通过服务契约；依赖解析在启动时完成，不支持运行期动态发现。

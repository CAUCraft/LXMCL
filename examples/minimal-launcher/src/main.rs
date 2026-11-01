//! 最小启动器示例：第三方产品的完整装配模板。
//!
//! 演示不依赖 LXMCL 的控制器、品牌素材或演示逻辑，只用
//! launcher-runtime / launcher-api / launcher-settings（以及一个
//! 第三方插件示例 custom-settings）组装一个能跑的最小启动器。
//! 控制台输出本产品的插件状态，方便观察宿主行为。

mod settings_bridge;
slint::include_modules!();

use custom_settings::{TomlSettingsConfig, TomlSettingsPlugin};
use launcher_api::{LauncherSettings, Settings};
use launcher_runtime::{PluginCatalog, PluginHost, ProductConfig, RuntimeError};
use launcher_settings::{JsonSettingsConfig, JsonSettingsPlugin};
use slint::ComponentHandle;
use std::error::Error;
use std::path::{Path, PathBuf};
use std::sync::Arc;

fn main() -> Result<(), Box<dyn Error>> {
    let product = ProductConfig::load(&locate_product_config()?)?;
    let data_root = data_root(&product.product.id)?;
    let mut catalog = PluginCatalog::new();

    // 注册 JSON 提供者（launcher-settings）。工厂只做参数解析与路径解析。
    {
        let root = data_root.clone();
        catalog.register("settings-json", move |config| {
            let config: JsonSettingsConfig = config.decode()?;
            let mut defaults = config.defaults;
            if defaults.game_root.is_relative() {
                defaults.game_root = root.join(&defaults.game_root);
            }
            Ok(Box::new(JsonSettingsPlugin::new(
                root.join(&config.file_name),
                defaults,
            )))
        })?;
    }
    // 注册第三方 TOML 提供者（custom-settings）。工厂形状与上面完全一致；
    // product.toml 里换 provider 字段即可切换，窗口代码不变。
    {
        let root = data_root.clone();
        catalog.register("settings-toml-example", move |config| {
            let config: TomlSettingsConfig = config.decode()?;
            let mut defaults = config.defaults;
            if defaults.game_root.is_relative() {
                defaults.game_root = root.join(&defaults.game_root);
            }
            Ok(Box::new(TomlSettingsPlugin::new(
                root.join(&config.file_name),
                defaults,
            )))
        })?;
    }

    let host = PluginHost::start(&catalog, &product.plugins)?;
    for status in host.plugin_states() {
        println!(
            "[host] 插件 {}（{}）：{:?}，提供 {:?}",
            status.id, status.provider, status.state, status.provides
        );
    }

    // Shell 只通过服务契约取用设置，不读配置文件。
    let settings = host.service::<Settings>()?;
    let (bridge, mut worker) = settings_bridge::SettingsBridge::start(Arc::clone(&settings))?;
    let initial = bridge.load_blocking()?;
    println!("[shell] 初始游戏目录：{}", initial.game_root.display());

    let ui = MinimalWindow::new()?;
    ui.set_product_name(product.product.name.as_str().into());
    ui.set_game_root(initial.game_root.display().to_string().into());
    ui.set_status("设置已读取".into());
    {
        let weak = ui.as_weak();
        let bridge = bridge.clone();
        // 界面没有 Java 路径编辑入口，保存时沿用已加载的值，避免整条设置被覆盖。
        let java_path = initial.java_path.clone();
        ui.on_save(move || {
            let Some(ui) = weak.upgrade() else { return };
            // 保存点击时的快照；结果回到界面线程后更新状态行。
            let snapshot = LauncherSettings {
                game_root: PathBuf::from(ui.get_game_root().trim()),
                java_path: java_path.clone(),
            };
            ui.set_status("正在保存…".into());
            let weak = ui.as_weak();
            bridge.save_async(snapshot, move |result| {
                let Some(ui) = weak.upgrade() else { return };
                match result {
                    Ok(()) => ui.set_status("已保存".into()),
                    Err(error) => ui.set_status(format!("保存失败：{error}").into()),
                }
            });
        });
    }

    ui.show()?;
    let run_result = ui.run();
    drop(ui);
    // 界面销毁会释放回调里持有的克隆；join 还会显式关闭邮箱，
    // 因此即使仍有克隆，工作线程也会在完成当前任务后退出。
    drop(bridge);
    worker.join()?;
    let shutdown = host.shutdown();
    run_result?;
    shutdown?;
    Ok(())
}

/// 从可执行文件目录向上有限层查找 `product.toml`。
fn locate_product_config() -> Result<PathBuf, RuntimeError> {
    let exe = std::env::current_exe().map_err(|error| RuntimeError::ProductConfig {
        path: "product.toml".into(),
        reason: format!("无法确定程序位置：{error}"),
    })?;
    let mut dir = exe
        .parent()
        .map_or_else(|| PathBuf::from("."), Path::to_path_buf);
    for _ in 0..6 {
        let candidate = dir.join("product.toml");
        if candidate.is_file() {
            return Ok(candidate);
        }
        if !dir.pop() {
            break;
        }
    }
    Err(RuntimeError::ProductConfig {
        path: "product.toml".into(),
        reason: "未找到产品配置：可执行文件目录及其上层都没有 product.toml".into(),
    })
}

/// 产品 ID 决定用户数据命名空间；基准目录在装配处解析。
fn data_root(product_id: &str) -> Result<PathBuf, RuntimeError> {
    let base = std::env::var_os("LOCALAPPDATA").ok_or(RuntimeError::DataDirUnavailable)?;
    let namespace: String = product_id
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() || matches!(c, '.' | '-') {
                c
            } else {
                '-'
            }
        })
        .collect();
    Ok(PathBuf::from(base).join(namespace))
}

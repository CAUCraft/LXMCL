//! LXMCL 产品装配：定位产品配置、注册插件工厂、启动宿主并运行 Slint 界面。
//!
//! 这里是产品（第 5 个角色）的代码：只做装配与消费，不实现任何服务。
use crate::controller;
use crate::instances_bridge::InstanceBridge;
use crate::java_bridge::JavaBridge;
use crate::settings_bridge::SettingsBridge;
use crate::{AppWindow, Theme, mascot_motion, pixel_motion, wallpaper};
use launcher_api::{Instances, JavaRuntime, Settings};
use launcher_instances::{JsonInstancesConfig, JsonInstancesPlugin};
use launcher_java::LocalJavaPlugin;
use launcher_runtime::{PluginCatalog, PluginHost, ProductConfig, RuntimeError};
use launcher_settings::{JsonSettingsConfig, JsonSettingsPlugin};
use slint::ComponentHandle;
use std::error::Error;
use std::path::{Path, PathBuf};
use std::sync::Arc;

/// 读取 LXMCL 的产品配置。
pub fn load_product() -> Result<ProductConfig, RuntimeError> {
    ProductConfig::load(&locate_product_config()?)
}

/// 组装插件目录并启动宿主。
pub fn start_host(product: &ProductConfig) -> Result<PluginHost, RuntimeError> {
    let data_root = data_root(&product.product.id)?;
    let catalog = plugin_catalog(&data_root)?;
    PluginHost::start(&catalog, &product.plugins)
}

/// 从可执行文件目录向上有限层查找 `product.toml`，
/// `cargo run`（target/debug）与安装目录都能稳定找到同一份配置。
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

/// 产品 ID 决定用户数据命名空间；基准目录在装配处解析，不依赖启动位置。
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

/// LXMCL 的插件目录：JSON 设置与 JSON 实例仓库。
fn plugin_catalog(data_root: &Path) -> Result<PluginCatalog, RuntimeError> {
    let mut catalog = PluginCatalog::new();
    let root = data_root.to_path_buf();
    let instance_root = root.clone();
    catalog.register("settings-json", move |config| {
        let config: JsonSettingsConfig = config.decode()?;
        let mut defaults = config.defaults;
        // 相对的游戏目录相对产品数据目录解析。
        if defaults.game_root.is_relative() {
            defaults.game_root = root.join(&defaults.game_root);
        }
        Ok(Box::new(JsonSettingsPlugin::new(
            root.join(&config.file_name),
            defaults,
        )))
    })?;
    catalog.register("instances-json", move |config| {
        let config: JsonInstancesConfig = config.decode()?;
        Ok(Box::new(JsonInstancesPlugin::new(
            instance_root.join(&config.file_name),
        )))
    })?;
    catalog.register("java-local", move |_config| {
        Ok(Box::new(LocalJavaPlugin::new()))
    })?;
    Ok(catalog)
}

/// 运行 Slint Shell：从设置服务取初始值，保存走工作线程桥。
/// 返回前窗口已关闭、设置工作线程已结束；宿主清理由调用方执行。
pub fn run_shell(host: &PluginHost) -> Result<(), Box<dyn Error>> {
    let settings = host.service::<Settings>()?;
    let (bridge, mut worker) = SettingsBridge::start(Arc::clone(&settings))?;
    let initial = bridge.load_blocking()?;
    let instances = host.service::<Instances>()?;
    let (instance_bridge, mut instance_worker) = InstanceBridge::start(Arc::clone(&instances))?;
    let initial_instances = instance_bridge.load_blocking()?;
    let java = host.service::<JavaRuntime>()?;
    let (java_bridge, mut java_worker) = JavaBridge::start(Arc::clone(&java))?;

    let ui = AppWindow::new()?;
    let timer = controller::connect(
        &ui,
        bridge,
        initial,
        instance_bridge,
        initial_instances,
        java_bridge,
    );
    let _motion = pixel_motion::connect(&ui);
    let _mascot = mascot_motion::connect(&ui);
    let _wallpaper = wallpaper::connect(&ui);
    #[cfg(debug_assertions)]
    let _probe = crate::perf_probe::install_if_requested(&ui);
    ui.show()?;
    let weak = ui.as_weak();
    // Sync once after the first layout; later geometry changes track themselves.
    slint::Timer::single_shot(std::time::Duration::from_millis(32), move || {
        if let Some(ui) = weak.upgrade() {
            ui.invoke_sync_nav();
            ui.global::<Theme>().set_animations_ready(true);
        }
    });

    let run_result = ui.run();
    drop(ui);
    drop(timer);
    // 两个工作线程都要收尾；先各自 join，再按顺序上报，避免一个失败跳过另一个。
    let settings_join = worker.join();
    let instances_join = instance_worker.join();
    let java_join = java_worker.join();
    settings_join?;
    instances_join?;
    java_join?;
    Ok(run_result?)
}

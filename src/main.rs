#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]
slint::include_modules!();
mod composition;
mod controller;
mod instances_bridge;
mod journey;
mod mascot_motion;
#[cfg(debug_assertions)]
mod perf_probe;
mod pixel_motion;
mod settings_bridge;
mod skin_preview;
mod state;
mod wallpaper;
use std::error::Error;

fn main() -> Result<(), Box<dyn Error>> {
    let product = composition::load_product()?;
    let host = composition::start_host(&product)?;
    // 界面关闭（或出错）后仍要执行宿主清理，两条结果都如实上报。
    let shell = composition::run_shell(&host);
    let shutdown = host.shutdown();
    shell?;
    shutdown?;
    Ok(())
}

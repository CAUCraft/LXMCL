#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]
use slint::ComponentHandle;
slint::include_modules!();
mod controller;
mod journey;
mod mascot_motion;
#[cfg(debug_assertions)]
mod perf_probe;
mod pixel_motion;
mod skin_preview;
mod state;
mod wallpaper;
fn main() -> Result<(), slint::PlatformError> {
    let ui = AppWindow::new()?;
    let _timer = controller::connect(&ui);
    let _motion = pixel_motion::connect(&ui);
    let _mascot = mascot_motion::connect(&ui);
    let _wallpaper = wallpaper::connect(&ui);
    #[cfg(debug_assertions)]
    let _probe = perf_probe::install_if_requested(&ui);
    ui.show()?;
    let weak = ui.as_weak();
    // Sync once after the first layout; later geometry changes track themselves.
    slint::Timer::single_shot(std::time::Duration::from_millis(32), move || {
        if let Some(ui) = weak.upgrade() {
            ui.invoke_sync_nav();
            ui.global::<Theme>().set_animations_ready(true);
        }
    });
    ui.run()
}

//! Opt-in, finite UI benchmark. Never starts during ordinary app use.
use crate::{AppState, AppWindow, Theme};
use slint::{ComponentHandle, RenderingState, Timer, TimerMode};
use std::{
    cell::RefCell,
    path::PathBuf,
    rc::Rc,
    time::{Duration, Instant},
};

#[derive(Default)]
struct Samples {
    api: String,
    pending: Option<Instant>,
    frame_start: Option<Instant>,
    last_frame: Option<Instant>,
    dispatch_to_render: Vec<f64>,
    submit: Vec<f64>,
    intervals: Vec<f64>,
    callback: Vec<f64>,
}
fn summary(values: &[f64]) -> serde_json::Value {
    let mut v = values.to_vec();
    v.sort_by(f64::total_cmp);
    if v.is_empty() {
        return serde_json::json!({"samples": 0});
    }
    serde_json::json!({"samples": v.len(), "p50_ms": v[v.len()/2],
        "p95_ms": v[((v.len() as f64 * 0.95).ceil() as usize - 1).min(v.len()-1)],
        "max_ms": v[v.len()-1]})
}

pub fn install_if_requested(ui: &AppWindow) -> Option<Rc<Timer>> {
    let mut args = std::env::args();
    if !args.any(|a| a == "--ui-benchmark") {
        return None;
    }
    let output = PathBuf::from(args.next().unwrap_or_else(|| "ui-benchmark.json".into()));
    ui.global::<Theme>().set_pixel_glass(true);
    let samples = Rc::new(RefCell::new(Samples::default()));
    let data = samples.clone();
    let supported = ui
        .window()
        .set_rendering_notifier(move |state, api| {
            let mut s = data.borrow_mut();
            match state {
                RenderingState::RenderingSetup => s.api = format!("{api:?}"),
                RenderingState::BeforeRendering => {
                    let now = Instant::now();
                    if let Some(t) = s.pending.take() {
                        s.dispatch_to_render
                            .push(t.elapsed().as_secs_f64() * 1000.0);
                    }
                    if let Some(previous) = s.last_frame {
                        let ms = now.duration_since(previous).as_secs_f64() * 1000.0;
                        // Exclude deliberate idle gaps between navigation trials.
                        if ms < 100.0 {
                            s.intervals.push(ms);
                        }
                    }
                    s.last_frame = Some(now);
                    s.frame_start = Some(now);
                }
                RenderingState::AfterRendering => {
                    if let Some(t) = s.frame_start.take() {
                        s.submit.push(t.elapsed().as_secs_f64() * 1000.0);
                    }
                }
                _ => {}
            }
        })
        .is_ok();
    let timer = Rc::new(Timer::default());
    let weak_timer = Rc::downgrade(&timer);
    let weak = ui.as_weak();
    let mut index = 0;
    timer.start(TimerMode::Repeated, Duration::from_millis(600), move || {
        let Some(ui) = weak.upgrade() else { return; };
        if index >= 27 {
            if let Some(t) = weak_timer.upgrade() { t.stop(); }
            let s = samples.borrow();
            let report = serde_json::json!({
                "graphics_api": s.api, "render_notifier_available": supported,
                "scenario": "27 programmatic page navigations, 1180x700 logical px, pixel glass, 280ms morph; NOT OS input-to-photon latency",
                "dispatch_to_before_render": summary(&s.dispatch_to_render),
                "cpu_render_submission": summary(&s.submit),
                "active_frame_interval_under_100ms": summary(&s.intervals),
                "navigation_callback": summary(&s.callback),
            });
            if let Some(parent) = output.parent() && !parent.as_os_str().is_empty() { let _ = std::fs::create_dir_all(parent); }
            if let Err(e) = std::fs::write(&output, serde_json::to_vec_pretty(&report).unwrap()) { eprintln!("benchmark report: {e}"); }
            let _ = slint::quit_event_loop();
            return;
        }
        if index % 9 == 0 { ui.global::<Theme>().set_dark(index < 9); ui.invoke_sync_theme(); }
        samples.borrow_mut().pending = Some(Instant::now());
        let started = Instant::now();
        ui.global::<AppState>().invoke_navigate((index + 1) % 9);
        samples.borrow_mut().callback.push(started.elapsed().as_secs_f64() * 1000.0);
        index += 1;
    });
    Some(timer)
}

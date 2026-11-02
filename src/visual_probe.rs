//! Explicit debug-only native render capture; never starts during ordinary use.
//! PNG encoding and filesystem access run on a bounded background worker.
use crate::{AppState, AppWindow, Theme};
use launcher_runtime::{BoundedWorker, ShutdownPolicy, WorkerExit, WorkerStatus};
use slint::{ComponentHandle, LogicalSize, Model, Timer, TimerMode};
use std::{io, path::PathBuf, rc::Rc, sync::mpsc, time::Duration};

struct Save {
    path: PathBuf,
    width: u32,
    height: u32,
    rgba: Vec<u8>,
}

/// Keep this guard alive until the shell's event loop returns.
/// Early window closure still drains accepted PNG writes.
pub struct VisualProbe {
    timer: Rc<Timer>,
    worker: Option<BoundedWorker<Save>>,
}

impl Drop for VisualProbe {
    fn drop(&mut self) {
        self.timer.stop();
        if let Some(worker) = self.worker.take()
            && worker.shutdown(ShutdownPolicy::Drain) == WorkerExit::Panicked
        {
            eprintln!("ui-capture: PNG worker panicked");
        }
    }
}

struct Case {
    name: &'static str,
    width: f32,
    height: f32,
    page: i32,
    category: i32,
    dark: bool,
}
const CASES: [Case; 10] = [
    Case {
        name: "01-home-light-1200x700",
        width: 1200.,
        height: 700.,
        page: 0,
        category: 0,
        dark: false,
    },
    Case {
        name: "02-versions-light-1200x700",
        width: 1200.,
        height: 700.,
        page: 1,
        category: 0,
        dark: false,
    },
    Case {
        name: "03-settings-java-light-1200x700",
        width: 1200.,
        height: 700.,
        page: 6,
        category: 2,
        dark: false,
    },
    Case {
        name: "04-settings-extensions-light-1200x700",
        width: 1200.,
        height: 700.,
        page: 6,
        category: 6,
        dark: false,
    },
    Case {
        name: "05-settings-appearance-light-1200x700",
        width: 1200.,
        height: 700.,
        page: 6,
        category: 4,
        dark: false,
    },
    Case {
        name: "06-settings-general-light-960x560",
        width: 960.,
        height: 560.,
        page: 6,
        category: 0,
        dark: false,
    },
    Case {
        name: "07-home-light-960x560",
        width: 960.,
        height: 560.,
        page: 0,
        category: 0,
        dark: false,
    },
    Case {
        name: "08-home-dark-1200x700",
        width: 1200.,
        height: 700.,
        page: 0,
        category: 0,
        dark: true,
    },
    Case {
        name: "09-instance-overview-light-960x560",
        width: 960.,
        height: 560.,
        page: 1,
        category: 0,
        dark: false,
    },
    Case {
        name: "10-rename-light-960x560",
        width: 960.,
        height: 560.,
        page: 1,
        category: 0,
        dark: false,
    },
];

fn show_case(ui: &AppWindow, case: &Case) {
    ui.window()
        .set_size(LogicalSize::new(case.width, case.height));
    ui.global::<Theme>().set_dark(case.dark);
    ui.invoke_sync_theme();
    let app = ui.global::<AppState>();
    if !app.get_modal().is_empty() {
        app.invoke_close_modal();
    }
    app.set_settings_category(case.category);
    app.set_instance_detail_open(false);
    app.invoke_navigate(case.page);
    if matches!(
        case.name,
        "09-instance-overview-light-960x560" | "10-rename-light-960x560"
    ) && let Some(instance) = app.get_installed_instances().row_data(0)
    {
        app.invoke_open_instance(instance.id);
        if case.name == "10-rename-light-960x560" {
            app.invoke_open_modal("rename".into(), instance.id);
        }
    }
}

/// `--ui-capture <directory>` captures the finite read-only cases in `CASES`.
/// Call before `ui.run()` on the UI thread, after controllers are connected.
/// Logical window dimensions appear in filenames; PNG dimensions are physical
/// pixels at the existing OS scale. This does not change system DPI or preferences.
pub fn install_if_requested(ui: &AppWindow) -> io::Result<Option<VisualProbe>> {
    let mut args = std::env::args_os().skip(1);
    if !args.any(|argument| argument == "--ui-capture") {
        return Ok(None);
    }
    let directory = PathBuf::from(args.next().ok_or_else(|| {
        io::Error::new(
            io::ErrorKind::InvalidInput,
            "--ui-capture requires an output directory",
        )
    })?);
    let (completed, results) = mpsc::channel();
    // At most CASES.len() images exist in the finite probe. The UI never waits
    // for encoding or filesystem access, even when storage is slow.
    let worker = BoundedWorker::spawn("ui-capture-png", CASES.len(), move |save: Save, _| {
        let outcome = (|| -> Result<(), String> {
            if let Some(parent) = save.path.parent() {
                std::fs::create_dir_all(parent).map_err(|error| error.to_string())?;
            }
            image::save_buffer_with_format(
                &save.path,
                &save.rgba,
                save.width,
                save.height,
                image::ColorType::Rgba8,
                image::ImageFormat::Png,
            )
            .map_err(|error| error.to_string())
        })();
        match &outcome {
            Ok(()) => println!(
                "ui-capture: saved {} ({}x{} physical pixels)",
                save.path.display(),
                save.width,
                save.height
            ),
            Err(error) => eprintln!("ui-capture: {}: {error}", save.path.display()),
        }
        let _ = completed.send(outcome.is_ok());
    })?;
    let sender = worker.sender();
    let timer = Rc::new(Timer::default());
    let weak_timer = Rc::downgrade(&timer);
    let weak_ui = ui.as_weak();
    let app = ui.global::<AppState>();
    app.set_wallpaper_enabled(false);
    app.set_wallpaper_paused(true);
    ui.global::<Theme>().set_pixel_glass(false);
    show_case(ui, &CASES[0]);
    println!(
        "ui-capture: starting {} native render snapshots in {}",
        CASES.len(),
        directory.display()
    );
    let mut next_case = 0;
    let mut submitted = 0;
    let mut returned = 0;
    let mut saved = 0;
    let mut capture_failures = 0;
    timer.start(TimerMode::Repeated, Duration::from_millis(350), move || {
        for success in results.try_iter() {
            returned += 1;
            saved += usize::from(success);
        }
        let worker_failed = sender.status() == WorkerStatus::Panicked;
        if worker_failed || (next_case == CASES.len() && returned == submitted) {
            if let Some(timer) = weak_timer.upgrade() { timer.stop(); }
            eprintln!("ui-capture: finished; {saved}/{} PNGs saved, {capture_failures} snapshot/queue failures, {} save failures, worker_panicked={worker_failed}", CASES.len(), returned - saved);
            if let Err(error) = slint::quit_event_loop() { eprintln!("ui-capture: cannot quit event loop: {error}"); }
            return;
        }
        if next_case == CASES.len() { return; }
        let Some(ui) = weak_ui.upgrade() else {
            if let Some(timer) = weak_timer.upgrade() { timer.stop(); }
            return;
        };
        let case = &CASES[next_case];
        match ui.window().take_snapshot() {
            Ok(pixels) => {
                let save = Save { path: directory.join(format!("{}.png", case.name)), width: pixels.width(), height: pixels.height(), rgba: pixels.as_bytes().to_vec() };
                match sender.try_submit(save) {
                    Ok(()) => submitted += 1,
                    Err(_) => { capture_failures += 1; eprintln!("ui-capture: {}: PNG worker busy or closed", case.name); }
                }
            }
            Err(error) => { capture_failures += 1; eprintln!("ui-capture: {}: snapshot failed: {error}", case.name); }
        }
        next_case += 1;
        if let Some(case) = CASES.get(next_case) { show_case(&ui, case); }
    });
    Ok(Some(VisualProbe {
        timer,
        worker: Some(worker),
    }))
}

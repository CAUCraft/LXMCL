//! Local video wallpaper. Decoding is isolated from the UI and retains one frame.
use crate::{AppState, AppWindow, WallpaperItem};
use slint::{
    ComponentHandle, Image, ModelRc, Rgb8Pixel, SharedPixelBuffer, Timer, TimerMode, VecModel,
};
use std::{
    cell::RefCell,
    io::Read,
    path::PathBuf,
    process::{Child, Command, Stdio},
    rc::Rc,
    sync::{
        Arc, Mutex,
        atomic::{AtomicBool, Ordering},
    },
    thread::{self, JoinHandle},
    time::{Duration, Instant},
};

const WIDTH: u32 = 1280;
const HEIGHT: u32 = 720;
const PERIOD: Duration = Duration::from_micros(41_667);
const CLIPS: [(&str, &str); 7] = [
    ("cabin", "林间小屋"),
    ("castle", "月下城堡"),
    ("aquarium", "珊瑚水族箱"),
    ("falling-snow", "雪夜街灯"),
    ("fireplace", "暖冬壁炉"),
    ("swamp", "雨落沼泽"),
    ("serene-snow", "雪地营火"),
];

fn asset_dir() -> PathBuf {
    let beside_exe = std::env::current_exe()
        .unwrap_or_default()
        .with_file_name("assets")
        .join("wallpapers");
    if beside_exe.is_dir() {
        beside_exe
    } else {
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("ui/assets/wallpapers")
    }
}

#[derive(Default)]
struct FrameSlot {
    pixels: Option<Vec<u8>>,
    failed: bool,
}

struct Decoder {
    child: Arc<Mutex<Child>>,
    stop: Arc<AtomicBool>,
    paused: Arc<AtomicBool>,
    slot: Arc<Mutex<FrameSlot>>,
    worker: Option<JoinHandle<()>>,
}

impl Decoder {
    fn start(path: PathBuf) -> std::io::Result<Self> {
        let bundled = std::env::current_exe()
            .unwrap_or_default()
            .with_file_name("ffmpeg.exe");
        let mut command = Command::new(if bundled.is_file() {
            bundled
        } else {
            PathBuf::from("ffmpeg")
        });
        command
            .args([
                "-hide_banner",
                "-loglevel",
                "error",
                "-nostdin",
                "-stream_loop",
                "-1",
                "-threads",
                "2",
                "-i",
            ])
            .arg(path)
            .args([
                "-map", "0:v:0", "-an", "-sn", "-dn", "-threads", "1", "-f", "rawvideo",
                "-pix_fmt", "rgb24", "pipe:1",
            ])
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::null());
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            command.creation_flags(0x08000000); // CREATE_NO_WINDOW
        }
        let mut child = command.spawn()?;
        let mut output = child.stdout.take().expect("piped video output");
        let stop = Arc::new(AtomicBool::new(false));
        let paused = Arc::new(AtomicBool::new(false));
        let slot = Arc::new(Mutex::new(FrameSlot::default()));
        let (worker_stop, worker_paused, worker_slot) =
            (stop.clone(), paused.clone(), slot.clone());
        let worker = thread::spawn(move || {
            while !worker_stop.load(Ordering::Relaxed) {
                if worker_paused.load(Ordering::Relaxed) {
                    thread::park();
                    continue;
                }
                let started = Instant::now();
                let mut pixels = vec![0; (WIDTH * HEIGHT * 3) as usize];
                if output.read_exact(&mut pixels).is_err() {
                    if !worker_stop.load(Ordering::Relaxed) {
                        worker_slot.lock().unwrap().failed = true;
                    }
                    break;
                }
                worker_slot.lock().unwrap().pixels = Some(pixels);
                thread::park_timeout(PERIOD.saturating_sub(started.elapsed()));
            }
        });
        Ok(Self {
            child: Arc::new(Mutex::new(child)),
            stop,
            paused,
            slot,
            worker: Some(worker),
        })
    }
    fn pause(&self, value: bool) {
        self.paused.store(value, Ordering::Relaxed);
        if let Some(worker) = &self.worker {
            worker.thread().unpark();
        }
    }
}

impl Drop for Decoder {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Relaxed);
        if let Some(worker) = &self.worker {
            worker.thread().unpark();
        }
        // Kill breaks a blocked pipe read. Reaping/joining never blocks the UI.
        let _ = self.child.lock().unwrap().kill();
        let child = self.child.clone();
        let worker = self.worker.take();
        thread::spawn(move || {
            let _ = child.lock().unwrap().wait();
            if let Some(worker) = worker {
                let _ = worker.join();
            }
        });
    }
}

pub struct Player {
    decoder: Option<Decoder>,
    timer: Timer,
    hidden: bool,
}

fn run_timer(player: &Rc<RefCell<Player>>, ui: &AppWindow) {
    let weak_player = Rc::downgrade(player);
    let weak_ui = ui.as_weak();
    let state = player.borrow();
    let paused = state.hidden || ui.global::<AppState>().get_wallpaper_paused();
    if let Some(decoder) = &state.decoder {
        decoder.pause(paused);
    }
    state.timer.stop();
    if paused || state.decoder.is_none() {
        return;
    }
    state.timer.start(TimerMode::Repeated, PERIOD, move || {
        let (Some(player), Some(ui)) = (weak_player.upgrade(), weak_ui.upgrade()) else {
            return;
        };
        let mut state = player.borrow_mut();
        let Some(decoder) = &state.decoder else {
            return;
        };
        let (frame, failed) = {
            let mut slot = decoder.slot.lock().unwrap();
            (slot.pixels.take(), slot.failed)
        };
        if let Some(frame) = frame {
            ui.global::<AppState>()
                .set_wallpaper_frame(Image::from_rgb8(
                    SharedPixelBuffer::<Rgb8Pixel>::clone_from_slice(&frame, WIDTH, HEIGHT),
                ));
        }
        if failed {
            state.timer.stop();
            state.decoder = None;
            ui.global::<AppState>()
                .set_wallpaper_status("视频暂时无法播放，已保留静态画面".into());
        }
    });
}

fn load(player: &Rc<RefCell<Player>>, ui: &AppWindow) {
    let app = ui.global::<AppState>();
    let id = app.get_wallpaper_index().clamp(0, CLIPS.len() as i32 - 1) as usize;
    let mut state = player.borrow_mut();
    state.timer.stop();
    state.decoder = None;
    app.set_wallpaper_status("静音循环 · 最小化时暂停".into());
    let dir = asset_dir();
    match Image::load_from_path(&dir.join(format!("{}.jpg", CLIPS[id].0))) {
        Ok(poster) => {
            app.set_wallpaper_frame(poster);
            app.set_wallpaper_ready(true);
        }
        Err(_) => {
            app.set_wallpaper_ready(false);
            app.set_wallpaper_status("壁纸文件缺失，已使用默认背景".into());
        }
    }
    if app.get_wallpaper_enabled() && app.get_wallpaper_ready() {
        match Decoder::start(dir.join(format!("{}.mp4", CLIPS[id].0))) {
            Ok(decoder) => state.decoder = Some(decoder),
            Err(_) => app.set_wallpaper_status("播放器不可用，已显示静态封面".into()),
        }
    }
    drop(state);
    run_timer(player, ui);
}

pub fn connect(ui: &AppWindow) -> Rc<RefCell<Player>> {
    let dir = asset_dir();
    let entries = CLIPS
        .iter()
        .enumerate()
        .map(|(id, (file, name))| WallpaperItem {
            id: id as i32,
            name: (*name).into(),
            poster: Image::load_from_path(&dir.join(format!("{file}.jpg"))).unwrap_or_default(),
        })
        .collect::<Vec<_>>();
    ui.global::<AppState>()
        .set_wallpapers(ModelRc::new(VecModel::from(entries)));
    let player = Rc::new(RefCell::new(Player {
        decoder: None,
        timer: Timer::default(),
        hidden: false,
    }));
    let weak = ui.as_weak();
    let state = player.clone();
    ui.global::<AppState>().on_select_wallpaper(move |index| {
        let Some(ui) = weak.upgrade() else {
            return;
        };
        let app = ui.global::<AppState>();
        if !(0..CLIPS.len() as i32).contains(&index) {
            return;
        }
        app.set_wallpaper_index(index);
        app.set_wallpaper_enabled(true);
        load(&state, &ui);
    });
    let weak = ui.as_weak();
    let state = player.clone();
    ui.global::<AppState>()
        .on_configure_wallpaper(move |enabled, paused| {
            let Some(ui) = weak.upgrade() else {
                return;
            };
            let app = ui.global::<AppState>();
            let reload = enabled != app.get_wallpaper_enabled();
            app.set_wallpaper_enabled(enabled);
            app.set_wallpaper_paused(paused);
            if reload {
                load(&state, &ui);
            } else {
                run_timer(&state, &ui);
            }
        });
    let weak = ui.as_weak();
    let state = player.clone();
    ui.global::<AppState>()
        .on_wallpaper_visibility(move |visible| {
            state.borrow_mut().hidden = !visible;
            if let Some(ui) = weak.upgrade() {
                run_timer(&state, &ui);
            }
        });
    load(&player, ui);
    player
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    #[ignore = "local FFmpeg integration; requires prepared wallpaper assets"]
    fn decoder_loops_pauses_and_resumes_without_an_unbounded_queue() {
        let fixture =
            PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("target/wallpaper-loop-smoke.mp4");
        let mut cmd = Command::new("ffmpeg");
        cmd.args(["-hide_banner", "-loglevel", "error", "-threads", "2", "-i"])
            .arg(asset_dir().join("cabin.mp4"))
            .args([
                "-t",
                "0.25",
                "-an",
                "-c:v",
                "libx264",
                "-preset",
                "ultrafast",
                "-threads",
                "2",
                "-y",
            ])
            .arg(&fixture);
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            cmd.creation_flags(0x08000000);
        }
        assert!(cmd.status().unwrap().success());
        let decoder = Decoder::start(fixture.clone()).unwrap();
        let deadline = Instant::now() + Duration::from_secs(3);
        let mut received = 0;
        while received < 20 && Instant::now() < deadline {
            if let Some(frame) = decoder.slot.lock().unwrap().pixels.take() {
                assert_eq!(frame.len(), (WIDTH * HEIGHT * 3) as usize);
                received += 1;
            }
            thread::sleep(Duration::from_millis(10));
        }
        // The fixture has six frames: receiving twenty proves EOF looping.
        assert!(received >= 20);
        decoder.pause(true);
        thread::sleep(Duration::from_millis(150));
        decoder.slot.lock().unwrap().pixels.take();
        thread::sleep(Duration::from_millis(150));
        assert!(decoder.slot.lock().unwrap().pixels.is_none());
        decoder.pause(false);
        thread::sleep(Duration::from_millis(150));
        assert!(decoder.slot.lock().unwrap().pixels.is_some());
        assert!(!decoder.slot.lock().unwrap().failed);
        drop(decoder);
        thread::sleep(Duration::from_millis(100));
        std::fs::remove_file(fixture).unwrap();
    }
}

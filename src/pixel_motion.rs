//! A tiny 4 px grid, rather than full-window blur, drives the liquid selection.
use crate::{AppState, AppWindow, Theme};
use slint::{ComponentHandle, Image, Rgba8Pixel, SharedPixelBuffer, Timer, TimerMode};
use std::{
    cell::RefCell,
    rc::Rc,
    time::{Duration, Instant},
};

const CELL: f32 = 4.0;
const WIDTH: f32 = 168.0;
const HEIGHT: f32 = 40.0;
const DURATION: Duration = Duration::from_millis(280);

fn rounded_box(x: f32, y: f32, cx: f32, cy: f32, w: f32, h: f32) -> f32 {
    let r = 8.0_f32.min(w / 2.0).min(h / 2.0);
    let dx = (x - cx).abs() - w / 2.0 + r;
    let dy = (y - cy).abs() - h / 2.0 + r;
    dx.max(0.0).hypot(dy.max(0.0)) + dx.max(dy).min(0.0) - r
}

fn smooth_union(a: f32, b: f32, k: f32) -> f32 {
    let h = (0.5 + 0.5 * (b - a) / k).clamp(0.0, 1.0);
    b + (a - b) * h - k * h * (1.0 - h)
}

pub fn frame(
    from: f32,
    to: f32,
    progress: f32,
    darkness: f32,
) -> (f32, SharedPixelBuffer<Rgba8Pixel>) {
    let p = progress.clamp(0.0, 1.0);
    let settled = p >= 1.0 || (from - to).abs() < 1.0;
    let top = ((if settled { to } else { from.min(to) }) / CELL).floor() * CELL;
    let bottom = if settled {
        to + HEIGHT
    } else {
        from.max(to) + HEIGHT
    };
    let cols = (WIDTH / CELL) as usize;
    let rows = ((bottom - top) / CELL).ceil() as usize;
    let mut mask = vec![false; cols * rows];
    // Grow a receiving lobe, join through a neck, then pinch that neck apart.
    let receive = (p / 0.55).clamp(0.0, 1.0);
    let release = ((p - 0.30) / 0.65).clamp(0.0, 1.0);
    let neck = if (0.12..0.76).contains(&p) {
        36.0 * ((p - 0.12) / 0.64 * std::f32::consts::PI).sin()
    } else {
        0.0
    };
    for y in 0..rows {
        for x in 0..cols {
            let px = x as f32 * CELL + CELL / 2.0;
            let py = top + y as f32 * CELL + CELL / 2.0;
            let d = if settled {
                rounded_box(px, py, WIDTH / 2.0, to + HEIGHT / 2.0, WIDTH, HEIGHT)
            } else {
                let old = if release < 1.0 {
                    rounded_box(
                        px,
                        py,
                        WIDTH / 2.0,
                        from + HEIGHT / 2.0,
                        WIDTH * (1.0 - release * 0.72),
                        HEIGHT * (1.0 - release),
                    )
                } else {
                    10000.0
                };
                let new = if receive > 0.0 {
                    rounded_box(
                        px,
                        py,
                        WIDTH / 2.0,
                        to + HEIGHT / 2.0,
                        WIDTH * receive,
                        HEIGHT * receive,
                    )
                } else {
                    10000.0
                };
                let lobes = old.min(new);
                if neck >= CELL {
                    smooth_union(
                        lobes,
                        rounded_box(
                            px,
                            py,
                            WIDTH / 2.0,
                            (from + to + HEIGHT) / 2.0,
                            neck,
                            (to - from).abs() + 4.0,
                        ),
                        14.0,
                    )
                } else {
                    lobes
                }
            };
            mask[y * cols + x] = d <= 0.0;
        }
    }
    let mut pixels = SharedPixelBuffer::<Rgba8Pixel>::new(cols as u32, rows as u32);
    let rgba = pixels.make_mut_bytes();
    for y in 0..rows {
        for x in 0..cols {
            if !mask[y * cols + x] {
                continue;
            }
            let edge_top = y == 0 || !mask[(y - 1) * cols + x];
            let edge_side =
                x == 0 || x + 1 == cols || !mask[y * cols + x - 1] || !mask[y * cols + x + 1];
            let edge_bottom = y + 1 == rows || !mask[(y + 1) * cols + x];
            let (day, night) = if edge_top {
                ([255, 255, 255, 250], [178, 210, 247, 205])
            } else if edge_side || edge_bottom {
                ([215, 235, 255, 235], [119, 155, 205, 145])
            } else {
                ([248, 253, 255, 190], [90, 123, 169, 110])
            };
            let color: [u8; 4] = std::array::from_fn(|c| {
                (day[c] as f32 + (night[c] as f32 - day[c] as f32) * darkness.clamp(0., 1.)) as u8
            });
            rgba[(y * cols + x) * 4..(y * cols + x + 1) * 4].copy_from_slice(&color);
        }
    }
    (top, pixels)
}

struct Motion {
    from: f32,
    to: f32,
    start: Instant,
    initialized: bool,
}
impl Motion {
    fn progress(&self) -> f32 {
        (self.start.elapsed().as_secs_f32() / DURATION.as_secs_f32()).min(1.0)
    }
    fn paint(&self, ui: &AppWindow, p: f32) {
        let (top, pixels) = frame(self.from, self.to, p, ui.global::<Theme>().get_darkness());
        let app = ui.global::<AppState>();
        app.set_nav_top(top);
        app.set_nav_height(pixels.height() as f32 * CELL);
        app.set_nav_image(Image::from_rgba8(pixels));
    }
}

pub fn connect(ui: &AppWindow) -> Rc<Timer> {
    let state = Rc::new(RefCell::new(Motion {
        from: 0.0,
        to: 0.0,
        start: Instant::now(),
        initialized: false,
    }));
    let timer = Rc::new(Timer::default());
    let weak = ui.as_weak();
    let anim = state.clone();
    let ticker = timer.clone();
    ui.global::<AppState>().on_move_nav(move |target| {
        // Layout has not been placed yet during initial component construction.
        if target <= 0.0 {
            return;
        }
        let Some(ui) = weak.upgrade() else { return };
        let mut m = anim.borrow_mut();
        if m.initialized && (m.to - target).abs() < 0.5 {
            return;
        }
        // A new click retargets the current transition; nothing is queued.
        let p = m.progress();
        m.from = if m.initialized {
            m.from + (m.to - m.from) * p
        } else {
            target
        };
        m.to = target;
        m.initialized = true;
        m.start = Instant::now();
        ticker.stop();
        if !ui.global::<Theme>().get_pixel_glass() || (m.from - target).abs() < 1.0 {
            m.from = target;
            m.paint(&ui, 1.0);
            return;
        }
        m.paint(&ui, 0.0);
        let weak_ui = ui.as_weak();
        let weak_timer = Rc::downgrade(&ticker);
        let moving = anim.clone();
        ticker.start(TimerMode::Repeated, Duration::from_millis(16), move || {
            let Some(ui) = weak_ui.upgrade() else { return };
            let m = moving.borrow();
            let p = m.progress();
            m.paint(&ui, p);
            if p >= 1.0
                && let Some(t) = weak_timer.upgrade()
            {
                t.stop();
            }
        });
    });
    let weak = ui.as_weak();
    let ticker = timer.clone();
    ui.global::<AppState>().on_refresh_nav(move || {
        if let Some(ui) = weak.upgrade() {
            let m = state.borrow();
            if !m.initialized {
                return;
            }
            m.paint(&ui, if ticker.running() { m.progress() } else { 1.0 });
        }
    });
    timer
}

#[cfg(test)]
mod tests {
    use super::*;
    fn alpha_at(from: f32, to: f32, p: f32, x: usize, y: f32) -> u8 {
        let (top, image) = frame(from, to, p, 0.0);
        if y < top || y >= top + image.height() as f32 * CELL {
            return 0;
        }
        image.as_bytes()[(((y - top) / CELL) as usize * image.width() as usize + x) * 4 + 3]
    }
    #[test]
    fn neck_joins_then_separates_and_settles_at_destination() {
        assert_eq!(alpha_at(0.0, 92.0, 0.0, 21, 66.0), 0);
        assert!(alpha_at(0.0, 92.0, 0.40, 21, 66.0) > 0);
        assert_eq!(alpha_at(0.0, 92.0, 0.82, 21, 66.0), 0);
        assert_eq!(alpha_at(0.0, 92.0, 1.0, 21, 20.0), 0);
        assert!(alpha_at(0.0, 92.0, 1.0, 21, 112.0) > 0);
    }
    #[test]
    fn reversed_and_long_transitions_have_bounded_buffers() {
        for (from, to) in [(92.0, 0.0), (0.0, 600.0), (100.0, 100.0)] {
            for p in [0.0, 0.3, 0.6, 1.0] {
                let (_, image) = frame(from, to, p, 1.0);
                assert_eq!(image.width(), 42);
                assert!(image.height() <= 160);
                assert!(image.as_bytes().chunks_exact(4).any(|v| v[3] > 0));
            }
        }
    }
}

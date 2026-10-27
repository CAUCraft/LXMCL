//! Coalesce pointer targets; animate only while the home avatar is changing pose.
use crate::{
    AppState, AppWindow,
    skin_preview::{Pose, SkinPreview},
};
use slint::{ComponentHandle, Timer, TimerMode};
use std::{
    cell::RefCell,
    rc::Rc,
    time::{Duration, Instant},
};

struct Motion {
    pose: Pose,
    target: Pose,
    last_frame: Instant,
}

pub fn connect(ui: &AppWindow) -> Rc<Timer> {
    let skin = Rc::new(SkinPreview::new());
    let app = ui.global::<AppState>();
    let initial = skin.render_pose(Pose::default());
    app.set_mascot_home_image(initial.clone());
    app.set_mascot_image(initial);
    let weak = ui.as_weak();
    let account_skin = skin.clone();
    app.on_rotate_mascot(move |delta| {
        if let Some(ui) = weak.upgrade() {
            let app = ui.global::<AppState>();
            let angle = if delta == 0 {
                -24
            } else {
                (app.get_mascot_angle() + delta).rem_euclid(360)
            };
            app.set_mascot_angle(angle);
            app.set_mascot_image(account_skin.render(angle));
        }
    });

    let motion = Rc::new(RefCell::new(Motion {
        pose: Pose::default(),
        target: Pose::default(),
        last_frame: Instant::now(),
    }));
    let timer = Rc::new(Timer::default());
    let weak = ui.as_weak();
    let timer_weak = Rc::downgrade(&timer);
    app.on_track_mascot(move |x, y, active| {
        let (Some(ui), Some(timer)) = (weak.upgrade(), timer_weak.upgrade()) else {
            return;
        };
        let app = ui.global::<AppState>();
        let on_home = app.get_page() == 0;
        let target = if active && on_home && x.is_finite() && y.is_finite() {
            Pose::towards(x, y)
        } else {
            Pose::default()
        };
        let mut state = motion.borrow_mut();
        state.target = target;
        if !on_home {
            timer.stop();
            if state.pose != Pose::default() {
                state.pose = Pose::default();
                app.set_mascot_home_image(skin.render_pose(state.pose));
            }
            return;
        }
        if state.pose == target || timer.running() {
            return;
        }
        state.last_frame = Instant::now();
        drop(state);
        let weak = weak.clone();
        let timer_weak = Rc::downgrade(&timer);
        let motion = motion.clone();
        let skin = skin.clone();
        timer.start(TimerMode::Repeated, Duration::from_millis(16), move || {
            let (Some(ui), Some(timer)) = (weak.upgrade(), timer_weak.upgrade()) else {
                return;
            };
            let mut state = motion.borrow_mut();
            let now = Instant::now();
            let dt = now.duration_since(state.last_frame).as_secs_f32().min(0.05);
            state.last_frame = now;
            let target = state.target;
            let moving = state.pose.settle_towards(target, dt);
            ui.global::<AppState>()
                .set_mascot_home_image(skin.render_pose(state.pose));
            if !moving {
                timer.stop();
            }
        });
    });
    timer
}

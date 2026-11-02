//! 设置服务与 Slint 界面之间的线程桥。
//!
//! 界面回调把命令交给工作线程，工作线程执行真正的磁盘读写；
//! 结果回调通过 `slint::invoke_from_event_loop` 回到界面线程，
//! 回调体里可以安全地更新全局属性（约定：回调在 UI 线程执行）。
//!
//! 队列是有界的：待执行的保存最多一个快照，后续点击覆盖旧快照而不是追加；
//! 正在执行的保存最多一个。窗口关闭后不再接受新请求，但已接受的最后一份
//! 保存快照仍会写完，然后工作线程才退出。

use crate::state::Preferences;
use launcher_api::{LauncherSettings, SettingsError, SettingsService};
use std::collections::VecDeque;
use std::sync::{
    Arc, Condvar, Mutex, MutexGuard, PoisonError,
    mpsc::{self, Sender},
};
use std::thread::JoinHandle;

/// 启动时的读取不会很多；限制排队数量，避免异常调用无限占用内存。
const MAX_PENDING_LOADS: usize = 4;

/// 一次待执行的保存：快照与它对应的 UI 回调一起被新点击覆盖。
struct SaveJob {
    settings: LauncherSettings,
    preferences: Option<Preferences>,
    on_done: SaveCallback,
}

type SaveCallback = Box<dyn FnOnce(Result<(), SettingsError>, Result<(), String>) + Send>;

#[derive(Default)]
struct MailboxState {
    loads: VecDeque<Sender<Result<LauncherSettings, SettingsError>>>,
    save: Option<SaveJob>,
    closed: bool,
}

struct Mailbox {
    state: Mutex<MailboxState>,
    ready: Condvar,
}

fn lock<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
    // 锁中毒只可能来自某次操作中的 panic；继续清理比扩散 panic 更合理。
    mutex.lock().unwrap_or_else(PoisonError::into_inner)
}

impl Mailbox {
    fn close(&self) {
        let mut state = lock(&self.state);
        state.closed = true;
        // 关闭后拒绝新请求；已接受的最后一个保存快照保留，由工作线程写完。
        // 启动读取在退出时不再需要，等待中的读取直接失败。
        state.loads.clear();
        drop(state);
        self.ready.notify_all();
    }
}

/// 保存命令的入口。持有者全部释放时邮箱自动关闭。
struct BridgeCore {
    mailbox: Arc<Mailbox>,
}

impl Drop for BridgeCore {
    fn drop(&mut self) {
        self.mailbox.close();
    }
}

/// 发送命令用的把手，可克隆进界面回调。
#[derive(Clone)]
pub struct SettingsBridge {
    core: Arc<BridgeCore>,
}

/// `join` 的最后结果：线程异常，或退出 flush 的最后一次保存失败。
#[derive(Debug)]
pub enum SettingsWorkerError {
    FinalSave(String),
    Thread(std::io::Error),
}

impl std::fmt::Display for SettingsWorkerError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::FinalSave(message) => write!(f, "退出前最后一次设置保存失败：{message}"),
            Self::Thread(error) => write!(f, "设置工作线程异常退出：{error}"),
        }
    }
}

impl std::error::Error for SettingsWorkerError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::FinalSave(_) => None,
            Self::Thread(error) => Some(error),
        }
    }
}

/// 工作线程句柄：窗口关闭后由产品显式 join。
pub struct SettingsWorker {
    handle: Option<JoinHandle<()>>,
    mailbox: Arc<Mailbox>,
    last_save_error: Arc<Mutex<Option<String>>>,
}

impl SettingsBridge {
    pub fn start(service: Arc<dyn SettingsService>) -> std::io::Result<(Self, SettingsWorker)> {
        let mailbox = Arc::new(Mailbox {
            state: Mutex::new(MailboxState::default()),
            ready: Condvar::new(),
        });
        let worker_mailbox = Arc::clone(&mailbox);
        let last_save_error = Arc::new(Mutex::new(None));
        let worker_error = Arc::clone(&last_save_error);
        let handle = std::thread::Builder::new()
            .name("settings".into())
            .spawn(move || run_worker(service, worker_mailbox, worker_error))?;
        Ok((
            Self {
                core: Arc::new(BridgeCore {
                    mailbox: Arc::clone(&mailbox),
                }),
            },
            SettingsWorker {
                handle: Some(handle),
                mailbox,
                last_save_error,
            },
        ))
    }

    /// 启动时的首次读取：在窗口创建前完成，读操作仍由工作线程执行。
    pub fn load_blocking(&self) -> Result<LauncherSettings, SettingsError> {
        let (reply, result) = mpsc::channel();
        {
            let mut state = lock(&self.core.mailbox.state);
            if state.closed || state.loads.len() >= MAX_PENDING_LOADS {
                return Err(SettingsError::Unavailable);
            }
            state.loads.push_back(reply);
        }
        self.core.mailbox.ready.notify_one();
        result
            .recv()
            .map_err(|_| SettingsError::Unavailable)
            .and_then(|loaded| loaded)
    }

    /// 保存设置：只保留最新的待保存快照；结果回调在 UI 线程执行。
    ///
    /// 连续点击时，尚未开始的旧快照和旧回调会被新点击覆盖，因此队列长度
    /// 不随点击次数增长。关闭后调用会立即返回 `Unavailable`，不会排队。
    pub fn save_async(
        &self,
        settings: LauncherSettings,
        on_done: impl FnOnce(Result<(), SettingsError>) + Send + 'static,
    ) {
        let job = SaveJob {
            settings,
            preferences: None,
            on_done: Box::new(move |result, _preferences| on_done(result)),
        };
        self.enqueue_save(job);
    }

    /// Saves both click-time snapshots on the existing coalescing worker.
    /// They share one pending slot, but the two files are not an atomic transaction.
    pub fn save_with_preferences_async(
        &self,
        settings: LauncherSettings,
        preferences: Preferences,
        on_done: impl FnOnce(Result<(), SettingsError>, Result<(), String>) + Send + 'static,
    ) {
        self.enqueue_save(SaveJob {
            settings,
            preferences: Some(preferences),
            on_done: Box::new(on_done),
        });
    }

    fn enqueue_save(&self, mut job: SaveJob) {
        let replaced = {
            let mut state = lock(&self.core.mailbox.state);
            if state.closed {
                drop(state);
                // 工作线程已停止：直接在调用线程报告失败。
                (job.on_done)(
                    Err(SettingsError::Unavailable),
                    Err("设置工作线程不可用".into()),
                );
                return;
            }
            let mut replaced = state.save.take();
            // A Java-only update must not discard the latest accepted UI preferences.
            if job.preferences.is_none() {
                job.preferences = replaced
                    .as_mut()
                    .and_then(|previous| previous.preferences.take());
            }
            state.save = Some(job);
            replaced
        };
        // Callback captures may own resources; dispose of superseded jobs outside the lock.
        drop(replaced);
        self.core.mailbox.ready.notify_one();
    }

    #[cfg(test)]
    fn is_closed(&self) -> bool {
        lock(&self.core.mailbox.state).closed
    }
}

fn run_worker(
    service: Arc<dyn SettingsService>,
    mailbox: Arc<Mailbox>,
    last_save_error: Arc<Mutex<Option<String>>>,
) {
    enum Job {
        Load(Sender<Result<LauncherSettings, SettingsError>>),
        Save(SaveJob),
    }

    let mut last_preferences_error = None;
    loop {
        let job = {
            let mut state = lock(&mailbox.state);
            loop {
                if let Some(reply) = state.loads.pop_front() {
                    break Job::Load(reply);
                }
                if let Some(job) = state.save.take() {
                    break Job::Save(job);
                }
                if state.closed {
                    return;
                }
                state = mailbox
                    .ready
                    .wait(state)
                    .unwrap_or_else(PoisonError::into_inner);
            }
        };
        match job {
            Job::Load(reply) => {
                // 读取方可能已放弃等待；发送失败不影响后续命令。
                let _ = reply.send(service.load());
            }
            Job::Save(SaveJob {
                settings,
                preferences,
                on_done,
            }) => {
                let result = service.save(&settings);
                let preferences_result = preferences
                    .as_ref()
                    .map(|preferences| preferences.save().map_err(|error| error.to_string()));
                let failure = save_failures(
                    &result,
                    preferences_result.as_ref(),
                    &mut last_preferences_error,
                );
                *lock(&last_save_error) = failure.clone();
                let preferences_result = preferences_result.unwrap_or(Ok(()));
                // 窗口可能已关闭；无法回传时至少把失败原因写入日志。
                if slint::invoke_from_event_loop(move || on_done(result, preferences_result))
                    .is_err()
                {
                    match failure {
                        Some(error) => eprintln!("设置保存失败且结果无法回传：{error}"),
                        None => eprintln!("设置保存结果无法回传：Slint 事件循环已退出"),
                    }
                }
            }
        }
    }
}

fn save_failures(
    settings: &Result<(), SettingsError>,
    preferences: Option<&Result<(), String>>,
    last_preferences_error: &mut Option<String>,
) -> Option<String> {
    if let Some(result) = preferences {
        *last_preferences_error = result.as_ref().err().cloned();
    }
    let mut errors = Vec::new();
    if let Err(error) = settings {
        errors.push(format!("游戏设置：{error}"));
    }
    if let Some(error) = last_preferences_error {
        errors.push(format!("界面偏好：{error}"));
    }
    (!errors.is_empty()).then(|| errors.join("；"))
}

impl SettingsWorker {
    /// 等待工作线程退出。在 `ui.run()` 返回并丢弃界面后调用。
    ///
    /// 这里显式关闭邮箱：不再接受新请求，但会完成最后一个已接受的保存快照。
    /// 即使某个界面回调仍意外持有 `SettingsBridge` 克隆，工作线程也会在
    /// 写完快照后退出，join 不会永久等待。
    pub fn join(&mut self) -> Result<(), SettingsWorkerError> {
        self.mailbox.close();
        let thread_result = match self.handle.take() {
            Some(handle) => handle.join().map_err(|_| {
                SettingsWorkerError::Thread(std::io::Error::other("设置工作线程异常退出"))
            }),
            None => Ok(()),
        };
        thread_result?;
        match lock(&self.last_save_error).take() {
            Some(message) => Err(SettingsWorkerError::FinalSave(message)),
            None => Ok(()),
        }
    }
}

impl Drop for SettingsWorker {
    fn drop(&mut self) {
        // Startup/window errors must flush accepted settings just like normal exit.
        if let Err(error) = self.join() {
            eprintln!("{error}");
        }
    }
}

/// 结果回调里展示提示用：不依赖 `State`（它不是 `Send`），
/// 定时器只清除内容未变的那条提示，不会覆盖后续新提示。
pub fn toast_detached(ui: &slint::Weak<crate::AppWindow>, message: String) {
    // 调用方保证在 UI 线程。
    if let Some(ui) = ui.upgrade() {
        use slint::ComponentHandle;
        let app = ui.global::<crate::AppState>();
        app.set_toast(message.into());
        let generation = app.get_toast_generation().wrapping_add(1);
        app.set_toast_generation(generation);
        let weak = ui.as_weak();
        slint::Timer::single_shot(std::time::Duration::from_secs(5), move || {
            if let Some(ui) = weak.upgrade()
                && ui.global::<crate::AppState>().get_toast_generation() == generation
            {
                ui.global::<crate::AppState>().set_toast("".into());
            }
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;

    struct FakeService;

    impl SettingsService for FakeService {
        fn load(&self) -> Result<LauncherSettings, SettingsError> {
            Ok(LauncherSettings {
                game_root: "loaded".into(),
                java_path: None,
            })
        }

        fn save(&self, _settings: &LauncherSettings) -> Result<(), SettingsError> {
            Ok(())
        }
    }

    fn service() -> Arc<dyn SettingsService> {
        Arc::new(FakeService)
    }

    fn settings(root: &str) -> LauncherSettings {
        LauncherSettings {
            game_root: root.into(),
            java_path: None,
        }
    }

    struct FailingService;

    impl SettingsService for FailingService {
        fn load(&self) -> Result<LauncherSettings, SettingsError> {
            Ok(settings("loaded"))
        }

        fn save(&self, _settings: &LauncherSettings) -> Result<(), SettingsError> {
            Err(SettingsError::Encode("disk full".into()))
        }
    }

    #[test]
    fn join_returns_the_final_save_error() {
        let (bridge, mut worker) = SettingsBridge::start(Arc::new(FailingService)).unwrap();
        bridge.save_async(settings("bad"), |_| {});
        match worker.join() {
            Err(SettingsWorkerError::FinalSave(message)) => {
                assert!(message.contains("disk full"));
            }
            Err(other) => panic!("expected final save error, got {other}"),
            Ok(()) => panic!("expected final save error, got success"),
        }
    }

    struct SlowService {
        saves: Arc<std::sync::atomic::AtomicUsize>,
        gate: Arc<(Mutex<bool>, Condvar)>,
        last: Arc<Mutex<Option<LauncherSettings>>>,
    }

    impl SettingsService for SlowService {
        fn load(&self) -> Result<LauncherSettings, SettingsError> {
            Ok(settings("loaded"))
        }

        fn save(&self, settings: &LauncherSettings) -> Result<(), SettingsError> {
            use std::sync::atomic::Ordering;
            let call = self.saves.fetch_add(1, Ordering::SeqCst) + 1;
            if call == 1 {
                // 第一次写入卡住，让后续点击都落在“待保存”槽位里。
                let (lock, ready) = &*self.gate;
                let mut released = lock.lock().unwrap();
                while !*released {
                    released = ready.wait(released).unwrap();
                }
            }
            *self.last.lock().unwrap() = Some(settings.clone());
            Ok(())
        }
    }

    #[test]
    fn rapid_saves_coalesce_into_one_pending_write() {
        use std::sync::atomic::{AtomicUsize, Ordering};
        let saves = Arc::new(AtomicUsize::new(0));
        let gate = Arc::new((Mutex::new(false), Condvar::new()));
        let last = Arc::new(Mutex::new(None));
        let service = Arc::new(SlowService {
            saves: Arc::clone(&saves),
            gate: Arc::clone(&gate),
            last: Arc::clone(&last),
        });
        let (bridge, mut worker) = SettingsBridge::start(service).unwrap();
        bridge.save_async(settings("first"), |_| {});
        for _ in 0..200 {
            if saves.load(Ordering::SeqCst) >= 1 {
                break;
            }
            std::thread::sleep(Duration::from_millis(5));
        }
        assert_eq!(saves.load(Ordering::SeqCst), 1, "第一次保存应已开始");
        for index in 2..=20 {
            bridge.save_async(settings(&format!("save-{index}")), |_| {});
        }
        {
            let (lock, ready) = &*gate;
            *lock.lock().unwrap() = true;
            ready.notify_all();
        }
        for _ in 0..200 {
            if saves.load(Ordering::SeqCst) >= 2 {
                break;
            }
            std::thread::sleep(Duration::from_millis(5));
        }
        drop(bridge);
        worker.join().unwrap();
        assert_eq!(
            saves.load(Ordering::SeqCst),
            2,
            "正在执行一个 + 待保存一个，不能随点击次数增长"
        );
        assert_eq!(
            last.lock()
                .unwrap()
                .as_ref()
                .unwrap()
                .game_root
                .to_string_lossy(),
            "save-20"
        );
    }

    #[test]
    fn join_flushes_last_accepted_save_and_rejects_new_requests() {
        use std::sync::atomic::{AtomicUsize, Ordering};
        let saves = Arc::new(AtomicUsize::new(0));
        let gate = Arc::new((Mutex::new(false), Condvar::new()));
        let last = Arc::new(Mutex::new(None));
        let service = Arc::new(SlowService {
            saves: Arc::clone(&saves),
            gate: Arc::clone(&gate),
            last: Arc::clone(&last),
        });
        let (bridge, mut worker) = SettingsBridge::start(service).unwrap();
        bridge.save_async(settings("first"), |_| {});
        for _ in 0..200 {
            if saves.load(Ordering::SeqCst) >= 1 {
                break;
            }
            std::thread::sleep(Duration::from_millis(5));
        }
        assert_eq!(saves.load(Ordering::SeqCst), 1, "第一次保存应已开始");
        bridge.save_async(settings("second"), |_| {});

        let (joined, join_done) = mpsc::channel();
        let joiner = std::thread::spawn(move || {
            let _ = joined.send(worker.join());
        });
        for _ in 0..200 {
            if bridge.is_closed() {
                break;
            }
            std::thread::sleep(Duration::from_millis(5));
        }
        assert!(bridge.is_closed(), "join 应先关闭邮箱，等待已接受的写入");

        let (rejected, rejection) = mpsc::channel();
        bridge.save_async(settings("third"), move |result| {
            let _ = rejected.send(result);
        });
        assert!(matches!(
            rejection
                .recv_timeout(Duration::from_secs(2))
                .expect("关闭后的保存应立刻回调"),
            Err(SettingsError::Unavailable)
        ));

        {
            let (lock, ready) = &*gate;
            *lock.lock().unwrap() = true;
            ready.notify_all();
        }
        join_done
            .recv_timeout(Duration::from_secs(5))
            .expect("join 应在写完最后快照后完成")
            .expect("join 不应失败");
        joiner.join().unwrap();
        assert_eq!(
            saves.load(Ordering::SeqCst),
            2,
            "只写正在执行的和最后一份快照"
        );
        assert_eq!(
            last.lock()
                .unwrap()
                .as_ref()
                .unwrap()
                .game_root
                .to_string_lossy(),
            "second"
        );
    }

    #[test]
    fn pending_saves_keep_only_the_latest_snapshot() {
        let mailbox = Arc::new(Mailbox {
            state: Mutex::new(MailboxState::default()),
            ready: Condvar::new(),
        });
        let bridge = SettingsBridge {
            core: Arc::new(BridgeCore {
                mailbox: Arc::clone(&mailbox),
            }),
        };
        bridge.save_async(settings("first"), |_| {});
        bridge.save_async(settings("second"), |_| {});
        bridge.save_async(settings("third"), |_| {});
        let state = lock(&mailbox.state);
        assert_eq!(
            state
                .save
                .as_ref()
                .map(|job| job.settings.game_root.to_string_lossy().into_owned()),
            Some("third".to_string())
        );
        assert_eq!(state.loads.len(), 0);
    }

    #[test]
    fn combined_snapshots_coalesce_together_and_java_updates_retain_preferences() {
        let mailbox = Arc::new(Mailbox {
            state: Mutex::new(MailboxState::default()),
            ready: Condvar::new(),
        });
        let bridge = SettingsBridge {
            core: Arc::new(BridgeCore {
                mailbox: Arc::clone(&mailbox),
            }),
        };
        let first = Preferences {
            folder: "first-ui".into(),
            ..Preferences::default()
        };
        let latest = Preferences {
            folder: "latest-ui".into(),
            memory: 8,
            ..Preferences::default()
        };
        bridge.save_with_preferences_async(settings("first-business"), first, |_, _| {});
        bridge.save_with_preferences_async(settings("latest-business"), latest, |_, _| {});
        bridge.save_async(settings("java-only"), |_| {});
        let state = lock(&mailbox.state);
        let pending = state.save.as_ref().unwrap();
        assert_eq!(pending.settings.game_root.to_string_lossy(), "java-only");
        let preferences = pending.preferences.as_ref().unwrap();
        assert_eq!(preferences.folder, "latest-ui");
        assert_eq!(preferences.memory, 8);
        assert!(state.loads.is_empty());
    }

    #[test]
    fn preferences_failure_survives_business_only_success_until_preferences_retry() {
        let mut previous = None;
        let failed = Err("preferences disk full".to_string());
        let failure = save_failures(&Ok(()), Some(&failed), &mut previous).unwrap();
        assert!(failure.contains("preferences disk full"));
        assert!(
            save_failures(&Ok(()), None, &mut previous)
                .unwrap()
                .contains("preferences disk full")
        );
        let both = save_failures(
            &Err(SettingsError::Encode("settings disk full".into())),
            None,
            &mut previous,
        )
        .unwrap();
        assert!(both.contains("preferences disk full") && both.contains("settings disk full"));
        assert!(save_failures(&Ok(()), Some(&Ok(())), &mut previous).is_none());
    }

    #[test]
    fn closed_mailbox_rejects_both_snapshots_without_writing_preferences() {
        let (bridge, mut worker) = SettingsBridge::start(service()).unwrap();
        worker.join().unwrap();
        let (reply, received) = mpsc::channel();
        bridge.save_with_preferences_async(
            settings("closed"),
            Preferences::default(),
            move |business, preferences| {
                reply.send((business, preferences)).unwrap();
            },
        );
        let (business, preferences) = received.recv_timeout(Duration::from_secs(2)).unwrap();
        assert!(matches!(business, Err(SettingsError::Unavailable)));
        assert!(preferences.is_err());
    }

    #[test]
    fn join_finishes_even_while_a_bridge_clone_is_alive() {
        let (bridge, mut worker) = SettingsBridge::start(service()).unwrap();
        let (done, finished) = mpsc::channel();
        let waiter = std::thread::spawn(move || {
            let _ = done.send(worker.join());
        });
        finished
            .recv_timeout(Duration::from_secs(5))
            .expect("设置工作线程应在 join 关闭邮箱后退出")
            .expect("join 报告工作线程异常");
        waiter.join().unwrap();
        assert!(bridge.load_blocking().is_err());
    }

    #[test]
    fn dropping_the_bridge_stops_the_worker() {
        let (bridge, mut worker) = SettingsBridge::start(service()).unwrap();
        drop(bridge);
        worker.join().expect("工作线程应正常退出");
    }
}

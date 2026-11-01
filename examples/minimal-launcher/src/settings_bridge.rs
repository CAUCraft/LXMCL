//! 设置服务与 Slint 界面之间的线程桥（minimal-launcher 自己的一份实现）。
//!
//! 每个 Shell 写自己的桥；框架不提供共享抽象。契约与 LXMCL 相同：
//! 界面回调发命令进通道，工作线程做磁盘读写，结果回调经
//! `slint::invoke_from_event_loop` 回到界面线程执行。
//!
//! 队列有界：待保存快照最多一个（新点击覆盖旧快照），正在执行的保存最多一个。
//! 关闭窗口后不再接受新请求，但已接受的最后一份快照仍会写完，然后线程退出。

use launcher_api::{LauncherSettings, SettingsError, SettingsService};
use std::collections::VecDeque;
use std::sync::{
    Arc, Condvar, Mutex, MutexGuard, PoisonError,
    mpsc::{self, Sender},
};
use std::thread::JoinHandle;

const MAX_PENDING_LOADS: usize = 4;

struct SaveJob {
    settings: LauncherSettings,
    on_done: Box<dyn FnOnce(Result<(), SettingsError>) + Send>,
}

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
    mutex.lock().unwrap_or_else(PoisonError::into_inner)
}

impl Mailbox {
    fn close(&self) {
        let mut state = lock(&self.state);
        state.closed = true;
        // 已接受的最后一个保存快照保留；退出时不再等待启动读取。
        state.loads.clear();
        drop(state);
        self.ready.notify_all();
    }
}

struct BridgeCore {
    mailbox: Arc<Mailbox>,
}

impl Drop for BridgeCore {
    fn drop(&mut self) {
        self.mailbox.close();
    }
}

#[derive(Clone)]
pub struct SettingsBridge {
    core: Arc<BridgeCore>,
}

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

    /// 启动时的首次读取：窗口创建前完成，读操作仍由工作线程执行。
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

    /// 保存设置：只保留最新的待保存快照；关闭后调用立即返回 `Unavailable`。
    pub fn save_async(
        &self,
        settings: LauncherSettings,
        on_done: impl FnOnce(Result<(), SettingsError>) + Send + 'static,
    ) {
        let job = SaveJob {
            settings,
            on_done: Box::new(on_done),
        };
        {
            let mut state = lock(&self.core.mailbox.state);
            if state.closed {
                drop(state);
                (job.on_done)(Err(SettingsError::Unavailable));
                return;
            }
            state.save = Some(job);
        }
        self.core.mailbox.ready.notify_one();
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
                let _ = reply.send(service.load());
            }
            Job::Save(SaveJob { settings, on_done }) => {
                let result = service.save(&settings);
                let failure = result.as_ref().err().map(ToString::to_string);
                *lock(&last_save_error) = failure.clone();
                if slint::invoke_from_event_loop(move || on_done(result)).is_err() {
                    match failure {
                        Some(error) => eprintln!("设置保存失败且结果无法回传：{error}"),
                        None => eprintln!("设置保存结果无法回传：Slint 事件循环已退出"),
                    }
                }
            }
        }
    }
}

impl SettingsWorker {
    /// 等待工作线程退出。在 `ui.run()` 返回并丢弃界面后调用。
    ///
    /// 先关闭邮箱拒绝新请求，再写完最后一个已接受的保存快照；
    /// 即使界面回调仍持有桥的克隆也不会永久等待。
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

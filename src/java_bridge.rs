//! Java 检测与 Slint 界面之间的工作线程桥。
//!
//! 检测/探测不在 UI 线程执行；邮箱容量为 1，界面同一时间只发起一个操作。
//! 退出时设置取消标志，正在进行的检测会在当前进程探测超时后停止。

use launcher_api::{JavaCandidate, JavaError, JavaRuntimeService, JavaScanResult};
use std::path::PathBuf;
use std::sync::{
    Arc,
    atomic::{AtomicBool, Ordering},
    mpsc::{self, SyncSender, TrySendError},
};
use std::thread::JoinHandle;

enum Command {
    Detect {
        saved: Option<PathBuf>,
        on_done: Box<dyn FnOnce(Result<JavaScanResult, JavaError>) + Send>,
    },
    Probe {
        path: PathBuf,
        on_done: Box<dyn FnOnce(Result<JavaCandidate, JavaError>) + Send>,
    },
    Shutdown,
}

#[derive(Clone)]
pub struct JavaBridge {
    commands: SyncSender<Command>,
}

pub struct JavaWorker {
    handle: Option<JoinHandle<()>>,
    commands: SyncSender<Command>,
    cancelled: Arc<AtomicBool>,
}

impl JavaBridge {
    pub fn start(service: Arc<dyn JavaRuntimeService>) -> std::io::Result<(Self, JavaWorker)> {
        let (commands, pending) = mpsc::sync_channel(1);
        let cancelled = Arc::new(AtomicBool::new(false));
        let worker_cancelled = Arc::clone(&cancelled);
        let handle = std::thread::Builder::new()
            .name("java".into())
            .spawn(move || run_worker(service, pending, worker_cancelled))?;
        Ok((
            Self {
                commands: commands.clone(),
            },
            JavaWorker {
                handle: Some(handle),
                commands,
                cancelled,
            },
        ))
    }

    pub fn detect_async(
        &self,
        saved: Option<PathBuf>,
        on_done: impl FnOnce(Result<JavaScanResult, JavaError>) + Send + 'static,
    ) {
        let command = Command::Detect {
            saved,
            on_done: Box::new(on_done),
        };
        match self.commands.try_send(command) {
            Ok(()) => {}
            Err(TrySendError::Full(command)) => reject(command, JavaError::Busy),
            Err(TrySendError::Disconnected(command)) => reject(command, JavaError::Unavailable),
        }
    }

    pub fn probe_async(
        &self,
        path: PathBuf,
        on_done: impl FnOnce(Result<JavaCandidate, JavaError>) + Send + 'static,
    ) {
        let command = Command::Probe {
            path,
            on_done: Box::new(on_done),
        };
        match self.commands.try_send(command) {
            Ok(()) => {}
            Err(TrySendError::Full(command)) => reject(command, JavaError::Busy),
            Err(TrySendError::Disconnected(command)) => reject(command, JavaError::Unavailable),
        }
    }
}

fn reject(command: Command, error: JavaError) {
    match command {
        Command::Detect { on_done, .. } => on_done(Err(error)),
        Command::Probe { on_done, .. } => on_done(Err(error)),
        Command::Shutdown => {}
    }
}

fn run_worker(
    service: Arc<dyn JavaRuntimeService>,
    pending: mpsc::Receiver<Command>,
    cancelled: Arc<AtomicBool>,
) {
    while let Ok(command) = pending.recv() {
        match command {
            Command::Detect { saved, on_done } => {
                let result = service.detect(saved.as_deref(), &cancelled);
                complete(on_done, Ok(result));
            }
            Command::Probe { path, on_done } => {
                complete(on_done, service.probe(&path));
            }
            Command::Shutdown => break,
        }
    }
}

fn complete<T: Send + 'static>(
    on_done: Box<dyn FnOnce(Result<T, JavaError>) + Send>,
    result: Result<T, JavaError>,
) {
    let failure = result.as_ref().err().map(ToString::to_string);
    if slint::invoke_from_event_loop(move || on_done(result)).is_err() {
        match failure {
            Some(error) => eprintln!("Java 操作失败且结果无法回传：{error}"),
            None => eprintln!("Java 操作结果无法回传：Slint 事件循环已退出"),
        }
    }
}

impl JavaWorker {
    /// 设置取消标志，写完已接受命令后关闭线程。
    pub fn join(&mut self) -> std::io::Result<()> {
        self.cancelled.store(true, Ordering::SeqCst);
        let _ = self.commands.send(Command::Shutdown);
        match self.handle.take() {
            Some(handle) => handle
                .join()
                .map_err(|_| std::io::Error::other("Java 工作线程异常退出")),
            None => Ok(()),
        }
    }
}

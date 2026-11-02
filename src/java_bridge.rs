//! Java 检测与 Slint 界面之间的工作线程桥。
//!
//! 检测/探测不在 UI 线程执行；邮箱容量为 1，界面同一时间只发起一个操作。
//! 退出时设置取消标志，正在进行的检测会在当前进程探测超时后停止。

use launcher_api::{JavaCandidate, JavaError, JavaRuntimeService, JavaScanResult};
use launcher_runtime::{BoundedWorker, ShutdownPolicy, SubmitError, WorkerExit, WorkerSender};
use std::path::PathBuf;
use std::sync::Arc;

enum Command {
    Detect {
        saved: Option<PathBuf>,
        on_done: Box<dyn FnOnce(Result<JavaScanResult, JavaError>) + Send>,
    },
    Probe {
        path: PathBuf,
        on_done: Box<dyn FnOnce(Result<JavaCandidate, JavaError>) + Send>,
    },
}

#[derive(Clone)]
pub struct JavaBridge {
    commands: WorkerSender<Command>,
}

pub struct JavaWorker {
    worker: Option<BoundedWorker<Command>>,
}

impl JavaBridge {
    pub fn start(service: Arc<dyn JavaRuntimeService>) -> std::io::Result<(Self, JavaWorker)> {
        let worker = BoundedWorker::spawn("java", 1, move |command, cancelled| match command {
            Command::Detect { saved, on_done } => {
                complete(on_done, Ok(service.detect(saved.as_deref(), cancelled)));
            }
            Command::Probe { path, on_done } => complete(on_done, service.probe(&path)),
        })?;
        Ok((
            Self {
                commands: worker.sender(),
            },
            JavaWorker {
                worker: Some(worker),
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
        match self.commands.try_submit(command) {
            Ok(()) => {}
            Err(SubmitError::Busy(command)) => reject(command, JavaError::Busy),
            Err(SubmitError::Closed(command)) => reject(command, JavaError::Unavailable),
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
        match self.commands.try_submit(command) {
            Ok(()) => {}
            Err(SubmitError::Busy(command)) => reject(command, JavaError::Busy),
            Err(SubmitError::Closed(command)) => reject(command, JavaError::Unavailable),
        }
    }
}

fn reject(command: Command, error: JavaError) {
    match command {
        Command::Detect { on_done, .. } => on_done(Err(error)),
        Command::Probe { on_done, .. } => on_done(Err(error)),
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
    /// 取消检测并丢弃等待中的只读探测，等待正在执行的探测按超时收尾。
    pub fn join(&mut self) -> std::io::Result<()> {
        match self
            .worker
            .take()
            .map(|worker| worker.shutdown(ShutdownPolicy::Cancel))
        {
            Some(WorkerExit::Panicked) => Err(std::io::Error::other("Java 工作线程异常退出")),
            _ => Ok(()),
        }
    }
}

//! 实例仓库与 Slint 界面之间的工作线程桥。
//!
//! 文件 I/O 只在工作线程执行，结果经 `slint::invoke_from_event_loop` 回到 UI。
//! 邮箱容量为 1，界面同时只允许一个实例操作；队列满时明确返回 `Busy`，
//! 不把点击无限追加到后台。

use launcher_api::{InstanceError, InstanceRecord, InstanceRepository, NewInstance};
use std::sync::{
    Arc, Mutex, MutexGuard, PoisonError,
    mpsc::{self, SyncSender, TrySendError},
};
use std::thread::JoinHandle;

enum Command {
    Load(mpsc::Sender<Result<Vec<InstanceRecord>, InstanceError>>),
    Create {
        draft: NewInstance,
        on_done: Box<dyn FnOnce(Result<InstanceRecord, InstanceError>) + Send>,
    },
    Rename {
        id: i32,
        name: String,
        on_done: Box<dyn FnOnce(Result<InstanceRecord, InstanceError>) + Send>,
    },
    Shutdown,
}

/// 可克隆进界面回调的发送端。
#[derive(Clone)]
pub struct InstanceBridge {
    commands: SyncSender<Command>,
}

/// `join` 的最后结果：线程异常，或退出前最后一次实例写入失败。
#[derive(Debug)]
pub enum InstanceWorkerError {
    FinalWrite(String),
    Thread(std::io::Error),
}

impl std::fmt::Display for InstanceWorkerError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::FinalWrite(message) => write!(f, "退出前最后一次实例写入失败：{message}"),
            Self::Thread(error) => write!(f, "实例工作线程异常退出：{error}"),
        }
    }
}

impl std::error::Error for InstanceWorkerError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::FinalWrite(_) => None,
            Self::Thread(error) => Some(error),
        }
    }
}

/// 实例工作线程；窗口关闭后由产品显式 `join`。
pub struct InstanceWorker {
    handle: Option<JoinHandle<()>>,
    commands: SyncSender<Command>,
    last_write_error: Arc<Mutex<Option<String>>>,
}

fn lock<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
    mutex.lock().unwrap_or_else(PoisonError::into_inner)
}

impl InstanceBridge {
    pub fn start(service: Arc<dyn InstanceRepository>) -> std::io::Result<(Self, InstanceWorker)> {
        let (commands, pending) = mpsc::sync_channel(1);
        let last_write_error = Arc::new(Mutex::new(None));
        let worker_error = Arc::clone(&last_write_error);
        let handle = std::thread::Builder::new()
            .name("instances".into())
            .spawn(move || run_worker(service, pending, worker_error))?;
        Ok((
            Self {
                commands: commands.clone(),
            },
            InstanceWorker {
                handle: Some(handle),
                commands,
                last_write_error,
            },
        ))
    }

    /// 启动时的首次列表读取；窗口创建前由工作线程完成。
    pub fn load_blocking(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
        let (reply, result) = mpsc::channel();
        self.commands
            .send(Command::Load(reply))
            .map_err(|_| InstanceError::Unavailable)?;
        result
            .recv()
            .map_err(|_| InstanceError::Unavailable)
            .and_then(|loaded| loaded)
    }

    pub fn create_async(
        &self,
        draft: NewInstance,
        on_done: impl FnOnce(Result<InstanceRecord, InstanceError>) + Send + 'static,
    ) {
        let command = Command::Create {
            draft,
            on_done: Box::new(on_done),
        };
        match self.commands.try_send(command) {
            Ok(()) => {}
            Err(TrySendError::Full(command)) => reject(command, InstanceError::Busy),
            Err(TrySendError::Disconnected(command)) => reject(command, InstanceError::Unavailable),
        }
    }

    pub fn rename_async(
        &self,
        id: i32,
        name: String,
        on_done: impl FnOnce(Result<InstanceRecord, InstanceError>) + Send + 'static,
    ) {
        let command = Command::Rename {
            id,
            name,
            on_done: Box::new(on_done),
        };
        match self.commands.try_send(command) {
            Ok(()) => {}
            Err(TrySendError::Full(command)) => reject(command, InstanceError::Busy),
            Err(TrySendError::Disconnected(command)) => reject(command, InstanceError::Unavailable),
        }
    }
}

fn reject(command: Command, error: InstanceError) {
    match command {
        Command::Create { on_done, .. } | Command::Rename { on_done, .. } => on_done(Err(error)),
        Command::Load(_) | Command::Shutdown => {}
    }
}

fn run_worker(
    service: Arc<dyn InstanceRepository>,
    pending: mpsc::Receiver<Command>,
    last_write_error: Arc<Mutex<Option<String>>>,
) {
    while let Ok(command) = pending.recv() {
        match command {
            Command::Load(reply) => {
                let _ = reply.send(service.list());
            }
            Command::Create { draft, on_done } => {
                let result = service.create(&draft);
                record_write(&last_write_error, &result);
                complete(on_done, result);
            }
            Command::Rename { id, name, on_done } => {
                let result = service.rename(id, &name);
                record_write(&last_write_error, &result);
                complete(on_done, result);
            }
            Command::Shutdown => break,
        }
    }
}

fn record_write(
    last_write_error: &Mutex<Option<String>>,
    result: &Result<InstanceRecord, InstanceError>,
) {
    *lock(last_write_error) = result.as_ref().err().map(ToString::to_string);
}

fn complete(
    on_done: Box<dyn FnOnce(Result<InstanceRecord, InstanceError>) + Send>,
    result: Result<InstanceRecord, InstanceError>,
) {
    let failure = result.as_ref().err().map(ToString::to_string);
    if slint::invoke_from_event_loop(move || on_done(result)).is_err() {
        match failure {
            Some(error) => eprintln!("实例操作失败且结果无法回传：{error}"),
            None => eprintln!("实例操作结果无法回传：Slint 事件循环已退出"),
        }
    }
}

impl InstanceWorker {
    /// 写完已接受的命令后关闭工作线程。关闭后不再接受新的实例操作。
    pub fn join(&mut self) -> Result<(), InstanceWorkerError> {
        let _ = self.commands.send(Command::Shutdown);
        let thread_result = match self.handle.take() {
            Some(handle) => handle.join().map_err(|_| {
                InstanceWorkerError::Thread(std::io::Error::other("实例工作线程异常退出"))
            }),
            None => Ok(()),
        };
        thread_result?;
        match lock(&self.last_write_error).take() {
            Some(message) => Err(InstanceWorkerError::FinalWrite(message)),
            None => Ok(()),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    struct FailingRepo;

    impl InstanceRepository for FailingRepo {
        fn list(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
            Ok(Vec::new())
        }

        fn create(&self, _draft: &NewInstance) -> Result<InstanceRecord, InstanceError> {
            Err(InstanceError::Invalid("disk full".into()))
        }

        fn rename(&self, _id: i32, _name: &str) -> Result<InstanceRecord, InstanceError> {
            Err(InstanceError::Invalid("disk full".into()))
        }
    }

    #[test]
    fn join_returns_the_final_instance_write_error() {
        let (bridge, mut worker) = InstanceBridge::start(Arc::new(FailingRepo)).unwrap();
        bridge.create_async(
            NewInstance {
                name: "失败实例".into(),
                version: "1.21.1".into(),
                loader: "Fabric".into(),
                game_root: PathBuf::from("D:\\Games"),
            },
            |_| {},
        );
        match worker.join() {
            Err(InstanceWorkerError::FinalWrite(message)) => {
                assert!(message.contains("disk full"));
            }
            Err(other) => panic!("expected final write error, got {other}"),
            Ok(()) => panic!("expected final write error, got success"),
        }
    }
}

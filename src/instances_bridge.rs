//! 实例仓库与 Slint 界面之间的工作线程桥。
//!
//! 文件 I/O 只在工作线程执行，结果经 `slint::invoke_from_event_loop` 回到 UI。
//! 邮箱容量为 1，界面同时只允许一个实例操作；队列满时明确返回 `Busy`，
//! 不把点击无限追加到后台。

use launcher_api::{InstanceError, InstanceRecord, InstanceRepository, NewInstance};
use launcher_runtime::{BoundedWorker, ShutdownPolicy, SubmitError, WorkerExit, WorkerSender};
use std::sync::{Arc, Mutex, MutexGuard, PoisonError, mpsc};

enum Command {
    Load(mpsc::Sender<Result<Vec<InstanceRecord>, InstanceError>>),
    Create {
        draft: NewInstance,
        on_done: Box<dyn FnOnce(Result<InstanceRecord, InstanceError>) + Send>,
    },
}

/// 可克隆进界面回调的发送端。
#[derive(Clone)]
pub struct InstanceBridge {
    commands: WorkerSender<Command>,
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
    worker: Option<BoundedWorker<Command>>,
    last_write_error: Arc<Mutex<Option<String>>>,
}

fn lock<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
    mutex.lock().unwrap_or_else(PoisonError::into_inner)
}

impl InstanceBridge {
    pub fn start(service: Arc<dyn InstanceRepository>) -> std::io::Result<(Self, InstanceWorker)> {
        let last_write_error = Arc::new(Mutex::new(None));
        let worker_error = Arc::clone(&last_write_error);
        let worker = BoundedWorker::spawn("instances", 1, move |command, _cancelled| {
            run_command(&*service, command, &worker_error);
        })?;
        let commands = worker.sender();
        Ok((
            Self { commands },
            InstanceWorker {
                worker: Some(worker),
                last_write_error,
            },
        ))
    }
    /// 启动时的首次列表读取；窗口创建前由工作线程完成。
    pub fn load_blocking(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
        let (reply, result) = mpsc::channel();
        self.commands
            .try_submit(Command::Load(reply))
            .map_err(|error| match error {
                SubmitError::Busy(_) => InstanceError::Busy,
                SubmitError::Closed(_) => InstanceError::Unavailable,
            })?;
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
        match self.commands.try_submit(command) {
            Ok(()) => {}
            Err(SubmitError::Busy(command)) => reject(command, InstanceError::Busy),
            Err(SubmitError::Closed(command)) => reject(command, InstanceError::Unavailable),
        }
    }
}

fn reject(command: Command, error: InstanceError) {
    match command {
        Command::Create { on_done, .. } => on_done(Err(error)),
        Command::Load(_) => {}
    }
}

fn run_command(
    service: &dyn InstanceRepository,
    command: Command,
    last_write_error: &Mutex<Option<String>>,
) {
    match command {
        Command::Load(reply) => {
            let _ = reply.send(service.list());
        }
        Command::Create { draft, on_done } => {
            let result = service.create(&draft);
            record_write(last_write_error, &result);
            complete(on_done, result);
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
        if let Some(worker) = self.worker.take()
            && worker.shutdown(ShutdownPolicy::Drain) == WorkerExit::Panicked
        {
            return Err(InstanceWorkerError::Thread(std::io::Error::other(
                "实例工作线程异常退出",
            )));
        }
        match lock(&self.last_write_error).take() {
            Some(message) => Err(InstanceWorkerError::FinalWrite(message)),
            None => Ok(()),
        }
    }
}

impl Drop for InstanceWorker {
    fn drop(&mut self) {
        if let Err(error) = self.join() {
            eprintln!("{error}");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use std::sync::atomic::{AtomicUsize, Ordering};

    struct FailingRepo;

    impl InstanceRepository for FailingRepo {
        fn list(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
            Ok(Vec::new())
        }

        fn create(&self, _draft: &NewInstance) -> Result<InstanceRecord, InstanceError> {
            Err(InstanceError::Invalid("disk full".into()))
        }

        fn rename(
            &self,
            _id: i32,
            _expected_revision: u64,
            _name: &str,
        ) -> Result<InstanceRecord, InstanceError> {
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

    #[test]
    fn shutdown_closes_all_bridge_clones() {
        let (bridge, mut worker) = InstanceBridge::start(Arc::new(FailingRepo)).unwrap();
        let cloned = bridge.clone();
        assert!(bridge.load_blocking().unwrap().is_empty());
        worker.join().unwrap();
        for sender in [bridge, cloned] {
            assert!(matches!(
                sender.load_blocking(),
                Err(InstanceError::Unavailable)
            ));
            let (reply, received) = mpsc::channel();
            sender.create_async(
                NewInstance {
                    name: "closed".into(),
                    version: "1.21.1".into(),
                    loader: "Fabric".into(),
                    game_root: PathBuf::from("unused"),
                },
                move |result| {
                    reply.send(result).unwrap();
                },
            );
            assert!(matches!(
                received.recv().unwrap(),
                Err(InstanceError::Unavailable)
            ));
        }
    }

    struct CountingRepo(Arc<AtomicUsize>);
    impl InstanceRepository for CountingRepo {
        fn list(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
            Ok(Vec::new())
        }
        fn create(&self, draft: &NewInstance) -> Result<InstanceRecord, InstanceError> {
            self.0.fetch_add(1, Ordering::SeqCst);
            Ok(InstanceRecord {
                id: 1,
                name: draft.name.clone(),
                version: draft.version.clone(),
                loader: draft.loader.clone(),
                game_directory: draft.game_root.clone(),
                revision: 1,
            })
        }
        fn rename(
            &self,
            _id: i32,
            _expected_revision: u64,
            _name: &str,
        ) -> Result<InstanceRecord, InstanceError> {
            unreachable!()
        }
    }

    #[test]
    fn dropping_worker_drains_accepted_write_on_early_return() {
        let writes = Arc::new(AtomicUsize::new(0));
        let (bridge, worker) =
            InstanceBridge::start(Arc::new(CountingRepo(writes.clone()))).unwrap();
        bridge.create_async(
            NewInstance {
                name: "saved".into(),
                version: "1.21.1".into(),
                loader: "Fabric".into(),
                game_root: PathBuf::from("D:\\Games"),
            },
            |_| {},
        );
        drop(worker);
        assert_eq!(writes.load(Ordering::SeqCst), 1);
        assert!(matches!(
            bridge.load_blocking(),
            Err(InstanceError::Unavailable)
        ));
    }
}

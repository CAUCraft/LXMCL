//! 实例写入用例；不依赖界面或具体存储提供者。
use launcher_api::{
    Application, ApplicationService, ApplicationSubmitError, Context, InstanceRepository,
    Instances, Plugin, PluginDescriptor, RenameCompleted, RenameInstance, RequestId, RuntimeError,
    ServiceRequirement,
};
use launcher_runtime::{BoundedWorker, ShutdownPolicy, SubmitError, WorkerExit, WorkerSender};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};

struct RenameJob {
    request_id: RequestId,
    command: RenameInstance,
    on_done: Box<dyn FnOnce(RenameCompleted) + Send>,
}

pub struct LauncherApplication {
    sender: WorkerSender<RenameJob>,
    worker: Mutex<Option<BoundedWorker<RenameJob>>>,
    next_request: AtomicU64,
}

impl LauncherApplication {
    pub fn new(repository: Arc<dyn InstanceRepository>, capacity: usize) -> std::io::Result<Self> {
        let worker = BoundedWorker::spawn(
            "launcher-application",
            capacity,
            move |job: RenameJob, _| {
                let command = job.command;
                let result = repository.rename(
                    command.instance_id,
                    command.expected_revision,
                    &command.name,
                );
                let current = if matches!(
                    &result,
                    Err(launcher_api::InstanceError::RevisionConflict { .. })
                ) {
                    repository.list().ok().and_then(|records| {
                        records
                            .into_iter()
                            .find(|record| record.id == command.instance_id)
                    })
                } else {
                    None
                };
                (job.on_done)(RenameCompleted {
                    request_id: job.request_id,
                    instance_id: command.instance_id,
                    result,
                    current,
                });
            },
        )?;
        Ok(Self {
            sender: worker.sender(),
            worker: Mutex::new(Some(worker)),
            next_request: AtomicU64::new(1),
        })
    }

    pub fn shutdown(&self) -> WorkerExit {
        let worker = self.worker.lock().unwrap().take();
        worker.map_or(WorkerExit::Completed, |worker| {
            worker.shutdown(ShutdownPolicy::Drain)
        })
    }
}

impl ApplicationService for LauncherApplication {
    fn rename(
        &self,
        command: RenameInstance,
        on_done: Box<dyn FnOnce(RenameCompleted) + Send>,
    ) -> Result<RequestId, ApplicationSubmitError> {
        let request_id = RequestId(self.next_request.fetch_add(1, Ordering::Relaxed));
        self.sender
            .try_submit(RenameJob {
                request_id,
                command,
                on_done,
            })
            .map_err(|error| match error {
                SubmitError::Busy(_) => ApplicationSubmitError::Busy,
                SubmitError::Closed(_) => ApplicationSubmitError::Closed,
            })?;
        Ok(request_id)
    }
}

impl Drop for LauncherApplication {
    fn drop(&mut self) {
        self.shutdown();
    }
}

#[derive(Default)]
pub struct ApplicationPlugin;

impl ApplicationPlugin {
    pub fn new() -> Self {
        Self
    }
}

impl Plugin for ApplicationPlugin {
    fn descriptor(&self) -> PluginDescriptor {
        PluginDescriptor {
            id: "launcher-application",
            provides: vec![ServiceRequirement::of::<Application>()],
            requires: vec![ServiceRequirement::of::<Instances>()],
        }
    }

    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
        let application = Arc::new(
            LauncherApplication::new(ctx.get::<Instances>()?, 8).map_err(|error| {
                RuntimeError::PluginFailed {
                    id: "launcher-application".into(),
                    reason: error.to_string(),
                }
            })?,
        );
        let cleanup = application.clone();
        ctx.on_dispose(move || match cleanup.shutdown() {
            WorkerExit::Completed => Ok(()),
            WorkerExit::Panicked => Err(RuntimeError::Dispose {
                plugin: "launcher-application".into(),
                reason: "application worker panicked".into(),
            }),
        })?;
        ctx.provide::<Application>(application)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use launcher_api::{InstanceError, InstanceRecord, NewInstance};
    use std::sync::mpsc;
    use std::time::Duration;

    struct BlockingRepository {
        started: mpsc::Sender<std::thread::ThreadId>,
        release: Mutex<mpsc::Receiver<()>>,
    }
    impl InstanceRepository for BlockingRepository {
        fn list(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
            unreachable!()
        }
        fn create(&self, _: &NewInstance) -> Result<InstanceRecord, InstanceError> {
            unreachable!()
        }
        fn rename(
            &self,
            id: i32,
            revision: u64,
            name: &str,
        ) -> Result<InstanceRecord, InstanceError> {
            self.started.send(std::thread::current().id()).unwrap();
            self.release.lock().unwrap().recv().unwrap();
            Ok(InstanceRecord {
                id,
                revision: revision + 1,
                name: name.into(),
                version: "1.21.1".into(),
                loader: "Fabric".into(),
                game_directory: "games".into(),
            })
        }
    }

    #[test]
    fn accepted_work_is_async_rejected_work_has_no_callback_and_shutdown_drains() {
        let (started_tx, started_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel();
        let app = LauncherApplication::new(
            Arc::new(BlockingRepository {
                started: started_tx,
                release: Mutex::new(release_rx),
            }),
            1,
        )
        .unwrap();
        let (done_tx, done_rx) = mpsc::channel();
        let command = RenameInstance {
            instance_id: 7,
            expected_revision: 3,
            name: "renamed".into(),
        };
        let output = done_tx.clone();
        let first = app
            .rename(
                command.clone(),
                Box::new(move |result| {
                    output.send((std::thread::current().id(), result)).unwrap();
                }),
            )
            .unwrap();
        let worker_thread = started_rx.recv_timeout(Duration::from_secs(2)).unwrap();
        assert_ne!(worker_thread, std::thread::current().id());
        let output = done_tx.clone();
        let second = app
            .rename(
                command.clone(),
                Box::new(move |result| {
                    output.send((std::thread::current().id(), result)).unwrap();
                }),
            )
            .unwrap();
        let rejected = Arc::new(std::sync::atomic::AtomicBool::new(false));
        let flag = rejected.clone();
        assert_eq!(
            app.rename(
                command.clone(),
                Box::new(move |_| {
                    flag.store(true, Ordering::Relaxed);
                })
            ),
            Err(ApplicationSubmitError::Busy)
        );
        release_tx.send(()).unwrap();
        release_tx.send(()).unwrap();
        assert_eq!(app.shutdown(), WorkerExit::Completed);
        for expected in [first, second] {
            let (thread, done) = done_rx.recv_timeout(Duration::from_secs(2)).unwrap();
            assert_eq!(thread, worker_thread);
            assert_eq!(done.request_id, expected);
            assert_eq!(done.result.unwrap().revision, 4);
        }
        let flag = rejected.clone();
        assert_eq!(
            app.rename(
                command,
                Box::new(move |_| {
                    flag.store(true, Ordering::Relaxed);
                })
            ),
            Err(ApplicationSubmitError::Closed)
        );
        assert!(!rejected.load(Ordering::Relaxed));
    }

    #[test]
    fn drop_drains_accepted_writes() {
        let (started_tx, started_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel();
        let app = LauncherApplication::new(
            Arc::new(BlockingRepository {
                started: started_tx,
                release: Mutex::new(release_rx),
            }),
            1,
        )
        .unwrap();
        let (done_tx, done_rx) = mpsc::channel();
        app.rename(
            RenameInstance {
                instance_id: 1,
                expected_revision: 1,
                name: "saved".into(),
            },
            Box::new(move |done| {
                done_tx.send(done).unwrap();
            }),
        )
        .unwrap();
        started_rx.recv_timeout(Duration::from_secs(2)).unwrap();
        release_tx.send(()).unwrap();
        drop(app);
        assert_eq!(done_rx.try_recv().unwrap().result.unwrap().name, "saved");
    }
}

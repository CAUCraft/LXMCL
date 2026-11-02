//! A bounded single-thread executor. Capacity counts queued jobs, excluding the running job.
//! Cancellation is cooperative: a handler must check its token during long operations.
use std::collections::VecDeque;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Condvar, Mutex};
use std::thread::{self, JoinHandle};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ShutdownPolicy {
    Drain,
    Cancel,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum WorkerExit {
    Completed,
    Panicked,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum WorkerStatus {
    Running,
    Closing,
    Completed,
    Panicked,
}
#[derive(Debug, PartialEq, Eq)]
pub enum SubmitError<T> {
    Busy(T),
    Closed(T),
}

struct State<T> {
    queue: VecDeque<T>,
    closing: bool,
    status: WorkerStatus,
}
struct Shared<T> {
    state: Mutex<State<T>>,
    ready: Condvar,
    cancelled: AtomicBool,
    capacity: usize,
}
pub struct WorkerSender<T: Send + 'static> {
    shared: Arc<Shared<T>>,
}
impl<T: Send + 'static> Clone for WorkerSender<T> {
    fn clone(&self) -> Self {
        Self {
            shared: Arc::clone(&self.shared),
        }
    }
}

pub struct BoundedWorker<T: Send + 'static> {
    shared: Arc<Shared<T>>,
    thread: Option<JoinHandle<()>>,
}

impl<T: Send + 'static> BoundedWorker<T> {
    /// Creates one worker. Zero capacity is invalid; no user handler runs under a lock.
    pub fn spawn(
        name: impl Into<String>,
        capacity: usize,
        mut handler: impl FnMut(T, &AtomicBool) + Send + 'static,
    ) -> std::io::Result<Self> {
        if capacity == 0 {
            return Err(std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                "worker capacity must be positive",
            ));
        }
        let shared = Arc::new(Shared {
            state: Mutex::new(State {
                queue: VecDeque::new(),
                closing: false,
                status: WorkerStatus::Running,
            }),
            ready: Condvar::new(),
            cancelled: AtomicBool::new(false),
            capacity,
        });
        let worker = Arc::clone(&shared);
        let thread = thread::Builder::new().name(name.into()).spawn(move || {
            let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
                loop {
                    let job = {
                        let mut state = worker.state.lock().unwrap();
                        while state.queue.is_empty() && !state.closing {
                            state = worker.ready.wait(state).unwrap();
                        }
                        match state.queue.pop_front() {
                            Some(job) => job,
                            None => break,
                        }
                    };
                    handler(job, &worker.cancelled);
                }
            }));
            let abandoned = {
                let mut state = worker
                    .state
                    .lock()
                    .unwrap_or_else(std::sync::PoisonError::into_inner);
                state.closing = true;
                state.status = if result.is_ok() {
                    WorkerStatus::Completed
                } else {
                    WorkerStatus::Panicked
                };
                std::mem::take(&mut state.queue)
            };
            drop(abandoned);
        })?;
        Ok(Self {
            shared,
            thread: Some(thread),
        })
    }

    pub fn sender(&self) -> WorkerSender<T> {
        WorkerSender {
            shared: Arc::clone(&self.shared),
        }
    }
    pub fn try_submit(&self, job: T) -> Result<(), SubmitError<T>> {
        self.sender().try_submit(job)
    }
    pub fn cancel(&self) {
        self.sender().cancel();
    }
    pub fn status(&self) -> WorkerStatus {
        self.sender().status()
    }
    /// Drain executes every accepted queued job; Cancel drops queued jobs and signals the running job.
    /// A running handler must return before shutdown can finish.
    pub fn shutdown(mut self, policy: ShutdownPolicy) -> WorkerExit {
        self.close(policy)
    }

    fn close(&mut self, policy: ShutdownPolicy) -> WorkerExit {
        let abandoned = {
            let mut state = self
                .shared
                .state
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner);
            state.closing = true;
            if state.status == WorkerStatus::Running {
                state.status = WorkerStatus::Closing;
            }
            if policy == ShutdownPolicy::Cancel {
                self.cancel();
                std::mem::take(&mut state.queue)
            } else {
                VecDeque::new()
            }
        };
        self.shared.ready.notify_all();
        drop(abandoned);
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
        }
        if self.status() == WorkerStatus::Panicked {
            WorkerExit::Panicked
        } else {
            WorkerExit::Completed
        }
    }
}
impl<T: Send + 'static> Drop for BoundedWorker<T> {
    fn drop(&mut self) {
        self.close(ShutdownPolicy::Cancel);
    }
}

impl<T: Send + 'static> WorkerSender<T> {
    pub fn try_submit(&self, job: T) -> Result<(), SubmitError<T>> {
        let mut state = self
            .shared
            .state
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if state.closing {
            return Err(SubmitError::Closed(job));
        }
        if state.queue.len() >= self.shared.capacity {
            return Err(SubmitError::Busy(job));
        }
        state.queue.push_back(job);
        self.shared.ready.notify_one();
        Ok(())
    }

    /// Marks the cooperative cancellation token; it remains set for this worker's lifetime.
    pub fn cancel(&self) {
        self.shared.cancelled.store(true, Ordering::Release);
    }
    pub fn status(&self) -> WorkerStatus {
        self.shared
            .state
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .status
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::mpsc;
    use std::time::Duration;

    #[test]
    fn bounds_queue_returns_rejected_job_and_drains_all_accepted_jobs() {
        let (started_tx, started_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel();
        let seen = Arc::new(Mutex::new(Vec::new()));
        let output = seen.clone();
        let worker = BoundedWorker::spawn("drain-test", 1, move |job: u32, _| {
            if job == 1 {
                started_tx.send(()).unwrap();
                release_rx.recv().unwrap();
            }
            output.lock().unwrap().push(job);
        })
        .unwrap();
        let sender = worker.sender();
        sender.try_submit(1).unwrap();
        started_rx.recv_timeout(Duration::from_secs(2)).unwrap();
        sender.try_submit(2).unwrap();
        assert_eq!(sender.try_submit(3), Err(SubmitError::Busy(3)));
        let closing = std::thread::spawn(move || worker.shutdown(ShutdownPolicy::Drain));
        while sender.status() == WorkerStatus::Running {
            std::thread::yield_now();
        }
        assert_eq!(sender.clone().try_submit(4), Err(SubmitError::Closed(4)));
        release_tx.send(()).unwrap();
        assert_eq!(closing.join().unwrap(), WorkerExit::Completed);
        assert_eq!(*seen.lock().unwrap(), [1, 2]);
        assert_eq!(sender.status(), WorkerStatus::Completed);
    }

    #[test]
    fn cancel_signals_running_job_and_discards_queue() {
        let (started_tx, started_rx) = mpsc::channel();
        let seen = Arc::new(Mutex::new(Vec::new()));
        let output = seen.clone();
        let worker = BoundedWorker::spawn("cancel-test", 1, move |job: u32, token| {
            started_tx.send(()).unwrap();
            while !token.load(Ordering::Acquire) {
                std::thread::yield_now();
            }
            output.lock().unwrap().push(job);
        })
        .unwrap();
        worker.try_submit(1).unwrap();
        started_rx.recv_timeout(Duration::from_secs(2)).unwrap();
        worker.try_submit(2).unwrap();
        assert_eq!(
            worker.shutdown(ShutdownPolicy::Cancel),
            WorkerExit::Completed
        );
        assert_eq!(*seen.lock().unwrap(), [1]);
    }

    #[test]
    fn handler_panic_closes_every_sender_and_reports_failure() {
        let worker =
            BoundedWorker::spawn("panic-test", 1, |_: (), _| panic!("handler failed")).unwrap();
        let sender = worker.sender();
        sender.try_submit(()).unwrap();
        assert_eq!(worker.shutdown(ShutdownPolicy::Drain), WorkerExit::Panicked);
        assert_eq!(sender.status(), WorkerStatus::Panicked);
        assert_eq!(sender.try_submit(()), Err(SubmitError::Closed(())));
    }
}

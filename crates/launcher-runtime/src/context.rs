use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use crate::error::RuntimeError;
use crate::key::ServiceKey;
use crate::registry::ServiceRegistry;

pub(crate) type DisposeFn = Box<dyn FnOnce() -> Result<(), RuntimeError> + Send>;

#[derive(Default)]
struct ScopeState {
    services: Vec<&'static str>,
    disposals: Vec<DisposeFn>,
    closed: bool,
}

/// 插件私有作用域：记录它提供的服务与注册的清理操作，停止时整体回收。
pub(crate) struct PluginScope {
    plugin_id: &'static str,
    registry: Arc<ServiceRegistry>,
    state: Mutex<ScopeState>,
}

/// 插件在启动时收到的上下文。通过它提供服务、按契约获取其他服务、
/// 登记退出清理，除此之外接触不到宿主内部。
#[derive(Clone)]
pub struct Context {
    scope: Arc<PluginScope>,
}

fn lock<T>(p: &Mutex<T>) -> MutexGuard<'_, T> {
    // 锁中毒只可能来自某次操作中的 panic；中断清理比扩散 panic 更合理。
    p.lock().unwrap_or_else(PoisonError::into_inner)
}

impl Context {
    pub(crate) fn new(plugin_id: &'static str, registry: Arc<ServiceRegistry>) -> Self {
        Self {
            scope: Arc::new(PluginScope {
                plugin_id,
                registry,
                state: Mutex::new(ScopeState::default()),
            }),
        }
    }

    /// 向宿主注册本插件提供的服务。同一服务不能由两个插件同时提供。
    pub fn provide<K: ServiceKey>(&self, service: Arc<K::Service>) -> Result<(), RuntimeError> {
        let mut state = lock(&self.scope.state);
        if state.closed {
            drop(state);
            return Err(RuntimeError::ContextClosed {
                plugin: self.scope.plugin_id,
            });
        }
        // 状态锁与注册表锁只保护容器；失败时把服务带出锁外再析构。
        match self
            .scope
            .registry
            .provide::<K>(service, self.scope.plugin_id)
        {
            Ok(()) => {
                state.services.push(K::NAME);
                Ok(())
            }
            Err((service, error)) => {
                drop(state);
                drop(service);
                Err(error)
            }
        }
    }

    /// 按服务契约获取其他插件提供的服务。依赖必须写在描述符里，
    /// 这样宿主才能保证提供者先启动。
    pub fn get<K: ServiceKey>(&self) -> Result<Arc<K::Service>, RuntimeError> {
        self.scope.registry.get::<K>()
    }

    /// 登记退出清理操作，按登记的相反顺序执行。
    pub fn on_dispose(
        &self,
        cleanup: impl FnOnce() -> Result<(), RuntimeError> + Send + 'static,
    ) -> Result<(), RuntimeError> {
        let mut state = lock(&self.scope.state);
        if state.closed {
            drop(state);
            return Err(RuntimeError::ContextClosed {
                plugin: self.scope.plugin_id,
            });
        }
        state.disposals.push(Box::new(cleanup));
        Ok(())
    }

    /// 关闭作用域并回收资源。清理闭包在状态锁外逆序执行，随后在注册表锁外
    /// 移除并析构本插件提供的服务。重复调用是幂等的。
    pub(crate) fn dispose(&self) -> Result<(), RuntimeError> {
        let (disposals, services) = {
            let mut state = lock(&self.scope.state);
            if state.closed {
                return Ok(());
            }
            state.closed = true;
            (
                std::mem::take(&mut state.disposals),
                std::mem::take(&mut state.services),
            )
        };
        let mut first_error = None;
        for cleanup in disposals.into_iter().rev() {
            if let Err(error) = cleanup() {
                first_error = first_error.or(Some(error));
            }
        }
        self.scope.registry.remove_services(&services);
        first_error.map_or(Ok(()), Err)
    }

    pub(crate) fn provided_services(&self) -> Vec<&'static str> {
        lock(&self.scope.state).services.clone()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dispose_runs_in_reverse_once_and_rejects_reentry() {
        let context = Context::new("probe", Arc::new(ServiceRegistry::default()));
        let order = Arc::new(Mutex::new(Vec::new()));
        let reentry = Arc::new(Mutex::new(None));

        let cleanup_context = context.clone();
        let first_order = Arc::clone(&order);
        let reentry_result = Arc::clone(&reentry);
        context
            .on_dispose(move || {
                first_order.lock().unwrap().push("first");
                *reentry_result.lock().unwrap() = Some(cleanup_context.on_dispose(|| Ok(())));
                Ok(())
            })
            .unwrap();

        let second_order = Arc::clone(&order);
        context
            .on_dispose(move || {
                second_order.lock().unwrap().push("second");
                Ok(())
            })
            .unwrap();

        context.dispose().unwrap();
        assert_eq!(*order.lock().unwrap(), vec!["second", "first"]);
        assert!(matches!(
            reentry.lock().unwrap().as_ref(),
            Some(Err(RuntimeError::ContextClosed { .. }))
        ));
        assert!(matches!(
            context.on_dispose(|| Ok(())),
            Err(RuntimeError::ContextClosed { .. })
        ));

        // 第二次 dispose 不再执行清理，也不会重新打开作用域。
        context.dispose().unwrap();
        assert_eq!(order.lock().unwrap().len(), 2);
    }
}

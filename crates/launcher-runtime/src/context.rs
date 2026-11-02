use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use crate::error::RuntimeError;
use crate::key::{ServiceKey, ServiceRequirement};
use crate::plugin::PluginDescriptor;
use crate::registry::ServiceRegistry;

pub(crate) type DisposeFn = Box<dyn FnOnce() -> Result<(), RuntimeError> + Send>;

#[derive(Default)]
struct ScopeState {
    contributions: Vec<(&'static str, String)>,
    services: Vec<ServiceRequirement>,
    disposals: Vec<DisposeFn>,
    closed: bool,
}

/// 插件私有作用域：记录它提供的服务与注册的清理操作，停止时整体回收。
pub(crate) struct PluginScope {
    owner: String,
    provides: Vec<ServiceRequirement>,
    requires: Vec<ServiceRequirement>,
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
    #[cfg(test)]
    pub(crate) fn new(plugin_id: &'static str, registry: Arc<ServiceRegistry>) -> Self {
        Self::with_owner(plugin_id, plugin_id.to_string(), registry)
    }

    #[cfg(test)]
    pub(crate) fn with_owner(
        plugin_id: &'static str,
        owner: String,
        registry: Arc<ServiceRegistry>,
    ) -> Self {
        Self::with_descriptor(
            PluginDescriptor {
                id: plugin_id,
                provides: vec![],
                requires: vec![],
            },
            owner,
            registry,
        )
    }

    pub(crate) fn with_descriptor(
        descriptor: PluginDescriptor,
        owner: String,
        registry: Arc<ServiceRegistry>,
    ) -> Self {
        Self {
            scope: Arc::new(PluginScope {
                owner,
                provides: descriptor.provides,
                requires: descriptor.requires,
                registry,
                state: Mutex::new(ScopeState::default()),
            }),
        }
    }

    /// Adds a typed contribution. IDs are unique within each extension point.
    pub fn contribute<K: crate::ContributionKey>(
        &self,
        id: impl Into<String>,
        order: i32,
        entry: K::Entry,
    ) -> Result<(), RuntimeError> {
        let id = id.into();
        let mut state = lock(&self.scope.state);
        if state.closed {
            drop(state);
            return Err(RuntimeError::ContextClosed {
                plugin: self.scope.owner.clone(),
            });
        }
        match self.scope.registry.contributions.insert::<K>(
            id.clone(),
            self.scope.owner.clone(),
            order,
            entry,
        ) {
            Ok(()) => {
                state.contributions.push((K::NAME, id));
                Ok(())
            }
            Err((entry, error)) => {
                drop(state);
                drop(entry);
                Err(error)
            }
        }
    }

    /// 向宿主注册本插件提供的服务。同一服务不能由两个插件同时提供。
    pub fn provide<K: ServiceKey>(&self, service: Arc<K::Service>) -> Result<(), RuntimeError> {
        let mut state = lock(&self.scope.state);
        if state.closed {
            drop(state);
            return Err(RuntimeError::ContextClosed {
                plugin: self.scope.owner.clone(),
            });
        }
        if !declares::<K>(&self.scope.provides) {
            drop(state);
            return Err(RuntimeError::UndeclaredService {
                plugin: self.scope.owner.clone(),
                service: K::NAME,
                operation: "provide",
            });
        }
        // 状态锁与注册表锁只保护容器；失败时把服务带出锁外再析构。
        match self.scope.registry.provide::<K>(service, &self.scope.owner) {
            Ok(()) => {
                state.services.push(ServiceRequirement::of::<K>());
                Ok(())
            }
            Err((service, error)) => {
                drop(state);
                drop(service);
                Err(RuntimeError::PluginFailed {
                    id: self.scope.owner.clone(),
                    reason: error.to_string(),
                })
            }
        }
    }

    /// 按服务契约获取其他插件提供的服务。依赖必须写在描述符里，
    /// 这样宿主才能保证提供者先启动。
    pub fn get<K: ServiceKey>(&self) -> Result<Arc<K::Service>, RuntimeError> {
        let state = lock(&self.scope.state);
        if state.closed {
            return Err(RuntimeError::ContextClosed {
                plugin: self.scope.owner.clone(),
            });
        }
        if !declares::<K>(&self.scope.requires) && !declares::<K>(&state.services) {
            return Err(RuntimeError::UndeclaredService {
                plugin: self.scope.owner.clone(),
                service: K::NAME,
                operation: "get",
            });
        }
        self.scope
            .registry
            .get::<K>()
            .map_err(|error| RuntimeError::PluginFailed {
                id: self.scope.owner.clone(),
                reason: error.to_string(),
            })
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
                plugin: self.scope.owner.clone(),
            });
        }
        state.disposals.push(Box::new(cleanup));
        Ok(())
    }

    /// 关闭作用域并回收资源。清理闭包在状态锁外逆序执行，随后在注册表锁外
    /// 移除并析构本插件提供的服务。重复调用是幂等的。
    pub(crate) fn dispose(&self) -> Result<(), RuntimeError> {
        let (disposals, services, contributions) = {
            let mut state = lock(&self.scope.state);
            if state.closed {
                return Ok(());
            }
            state.closed = true;
            (
                std::mem::take(&mut state.disposals),
                std::mem::take(&mut state.services),
                std::mem::take(&mut state.contributions),
            )
        };
        self.scope.registry.contributions.remove(&contributions);
        let mut first_error = None;
        for cleanup in disposals.into_iter().rev() {
            let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(cleanup))
                .unwrap_or_else(|_| {
                    Err(RuntimeError::Dispose {
                        plugin: self.scope.owner.clone(),
                        reason: "cleanup panicked".into(),
                    })
                });
            if let Err(error) = result {
                first_error = first_error.or(Some(error));
            }
        }
        let names: Vec<_> = services.iter().map(|service| service.service).collect();
        self.scope.registry.remove_services(&names);
        first_error.map_or(Ok(()), Err)
    }

    pub(crate) fn provided_services(&self) -> Vec<&'static str> {
        lock(&self.scope.state)
            .services
            .iter()
            .map(|service| service.service)
            .collect()
    }

    pub(crate) fn has_provided(&self, requirement: &ServiceRequirement) -> bool {
        let state = lock(&self.scope.state);
        !state.closed
            && state.services.iter().any(|provided| {
                provided.service == requirement.service
                    && provided.service_type == requirement.service_type
            })
    }
}

fn declares<K: ServiceKey>(requirements: &[ServiceRequirement]) -> bool {
    let expected = ServiceRequirement::of::<K>();
    requirements.iter().any(|requirement| {
        requirement.service == expected.service && requirement.service_type == expected.service_type
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Number;
    impl ServiceKey for Number {
        type Service = u32;
        const NAME: &'static str = "test.number";
    }
    struct WrongNumber;
    impl ServiceKey for WrongNumber {
        type Service = u64;
        const NAME: &'static str = Number::NAME;
    }

    fn scoped(
        registry: Arc<ServiceRegistry>,
        provides: Vec<ServiceRequirement>,
        requires: Vec<ServiceRequirement>,
    ) -> Context {
        Context::with_descriptor(
            PluginDescriptor {
                id: "factory-id",
                provides,
                requires,
            },
            "product-instance".into(),
            registry,
        )
    }

    #[test]
    fn undeclared_reads_and_writes_fail_even_when_service_exists() {
        let registry = Arc::new(ServiceRegistry::default());
        registry.provide::<Number>(Arc::new(7), "other").unwrap();
        let context = scoped(registry.clone(), vec![], vec![]);
        for error in [
            context.get::<Number>().unwrap_err(),
            context.provide::<WrongNumber>(Arc::new(9)).unwrap_err(),
        ] {
            assert!(matches!(error, RuntimeError::UndeclaredService { .. }));
            let message = error.to_string();
            assert!(message.contains("product-instance"));
            assert!(message.contains(Number::NAME));
        }
        assert_eq!(*registry.get::<Number>().unwrap(), 7);
    }

    #[test]
    fn declarations_match_both_name_and_type_and_own_service_requires_registration() {
        let registry = Arc::new(ServiceRegistry::default());
        let provider = scoped(
            registry.clone(),
            vec![ServiceRequirement::of::<Number>()],
            vec![],
        );
        assert!(matches!(
            provider.get::<Number>(),
            Err(RuntimeError::UndeclaredService { .. })
        ));
        assert!(matches!(
            provider.provide::<WrongNumber>(Arc::new(9)),
            Err(RuntimeError::UndeclaredService { .. })
        ));
        provider.provide::<Number>(Arc::new(7)).unwrap();
        assert_eq!(*provider.get::<Number>().unwrap(), 7);
        assert!(provider.has_provided(&ServiceRequirement::of::<Number>()));
        assert!(!provider.has_provided(&ServiceRequirement::of::<WrongNumber>()));
        let consumer = scoped(registry, vec![], vec![ServiceRequirement::of::<Number>()]);
        assert_eq!(*consumer.get::<Number>().unwrap(), 7);
        assert!(!consumer.has_provided(&ServiceRequirement::of::<Number>()));
        assert!(matches!(
            consumer.get::<WrongNumber>(),
            Err(RuntimeError::UndeclaredService { .. })
        ));
        let retained = consumer.get::<Number>().unwrap();
        consumer.dispose().unwrap();
        assert!(
            matches!(consumer.get::<Number>(), Err(RuntimeError::ContextClosed { plugin }) if plugin == "product-instance")
        );
        assert_eq!(*retained, 7);
        provider.dispose().unwrap();
        assert!(!provider.has_provided(&ServiceRequirement::of::<Number>()));
    }

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

#[cfg(test)]
mod panic_cleanup_tests {
    use super::*;
    #[test]
    fn cleanup_panic_does_not_skip_remaining_cleanup_or_resource_removal() {
        let registry = Arc::new(ServiceRegistry::default());
        let context = Context::new("cleanup", registry.clone());
        let done = Arc::new(std::sync::atomic::AtomicBool::new(false));
        let completed = done.clone();
        context
            .on_dispose(move || {
                completed.store(true, std::sync::atomic::Ordering::SeqCst);
                Ok(())
            })
            .unwrap();
        context.on_dispose(|| panic!("broken cleanup")).unwrap();
        assert!(matches!(
            context.dispose(),
            Err(RuntimeError::Dispose { .. })
        ));
        assert!(done.load(std::sync::atomic::Ordering::SeqCst));
        context.dispose().unwrap();
    }
}

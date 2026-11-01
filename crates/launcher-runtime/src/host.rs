use std::any::TypeId;
use std::collections::HashMap;
use std::sync::Arc;

use crate::catalog::PluginCatalog;
use crate::context::Context;
use crate::error::RuntimeError;
use crate::key::ServiceKey;
use crate::plugin::{Plugin, PluginDescriptor, PluginState};
use crate::product::PluginEntry;
use crate::registry::ServiceRegistry;

/// 启动前 / 启动中的插件：实例化时的描述符固定下来，后续不再询问插件。
struct PendingPlugin {
    entry_id: String,
    provider: String,
    descriptor: PluginDescriptor,
    plugin: Box<dyn Plugin>,
}

/// 已启动的插件：实例与作用域都留在宿主里，退出时先清理作用域再释放实例。
///
/// 插件可能把资源句柄放在自己的字段里，因此实例不能随 `start()` 返回而释放。
struct HostedPlugin {
    id: String,
    provider: String,
    state: PluginState,
    scope: Context,
    /// `None` 只在清理阶段出现。
    instance: Option<Box<dyn Plugin>>,
}

/// 插件状态快照，供产品展示与诊断。
#[derive(Clone, Debug)]
pub struct PluginStatus {
    pub id: String,
    pub provider: String,
    pub state: PluginState,
    pub provides: Vec<&'static str>,
}

/// 插件宿主：完整解析装配依赖后按顺序启动，退出时按相反顺序清理。
pub struct PluginHost {
    registry: Arc<ServiceRegistry>,
    plugins: Vec<HostedPlugin>,
}

impl PluginHost {
    /// 实例化所有插件、固定描述符、解析服务依赖并启动。
    ///
    /// 任何一个插件启动失败时，失败插件先释放自身资源与实例，再按启动的
    /// 相反顺序清理已启动插件，然后返回错误。
    pub fn start(catalog: &PluginCatalog, entries: &[PluginEntry]) -> Result<Self, RuntimeError> {
        let mut instantiated: Vec<PendingPlugin> = Vec::new();
        for entry in entries {
            let factory =
                catalog
                    .get(&entry.provider)
                    .ok_or_else(|| RuntimeError::UnknownProvider {
                        provider: entry.provider.clone(),
                    })?;
            let plugin = factory(&entry.config)?;
            if instantiated
                .iter()
                .any(|pending| pending.entry_id == entry.id)
            {
                return Err(RuntimeError::DuplicatePluginId {
                    id: entry.id.clone(),
                });
            }
            let descriptor = plugin.descriptor();
            instantiated.push(PendingPlugin {
                entry_id: entry.id.clone(),
                provider: entry.provider.clone(),
                descriptor,
                plugin,
            });
        }

        // 服务名 -> 提供者下标；同一服务不允许两个插件同时提供。
        let mut providers: HashMap<&'static str, (usize, TypeId)> = HashMap::new();
        for (index, pending) in instantiated.iter().enumerate() {
            for requirement in &pending.descriptor.provides {
                if let Some((_, expected)) = providers.get(requirement.service) {
                    return Err(if *expected != requirement.service_type {
                        RuntimeError::ServiceTypeMismatch {
                            service: requirement.service,
                        }
                    } else {
                        RuntimeError::DuplicateService {
                            service: requirement.service,
                            plugin: pending.entry_id.clone(),
                        }
                    });
                }
                providers.insert(requirement.service, (index, requirement.service_type));
            }
        }

        // Kahn 拓扑排序：提供者先于消费者。自给自足的服务不成边。
        let count = instantiated.len();
        let mut indegree = vec![0usize; count];
        let mut dependents: Vec<Vec<usize>> = vec![Vec::new(); count];
        for (index, pending) in instantiated.iter().enumerate() {
            for requirement in &pending.descriptor.requires {
                let Some(&(provider_index, provider_type)) = providers.get(requirement.service)
                else {
                    return Err(RuntimeError::DependencyMissing {
                        plugin: pending.entry_id.clone(),
                        service: requirement.service,
                    });
                };
                if provider_type != requirement.service_type {
                    return Err(RuntimeError::ServiceTypeMismatch {
                        service: requirement.service,
                    });
                }
                if provider_index != index {
                    indegree[index] += 1;
                    dependents[provider_index].push(index);
                }
            }
        }
        let mut order: Vec<usize> = (0..count).filter(|&i| indegree[i] == 0).collect();
        let mut cursor = 0;
        while cursor < order.len() {
            for &next in &dependents[order[cursor]] {
                indegree[next] -= 1;
                if indegree[next] == 0 {
                    order.push(next);
                }
            }
            cursor += 1;
        }
        if order.len() != count {
            let chain = (0..count)
                .filter(|index| indegree[*index] > 0)
                .map(|index| instantiated[index].entry_id.clone())
                .collect::<Vec<_>>()
                .join(" -> ");
            return Err(RuntimeError::ServiceCycle { chain });
        }

        let registry = Arc::new(ServiceRegistry::default());
        // 按启动顺序重排实例，之后逐条取走所有权；宿主负责让它活到关闭。
        let mut slots: Vec<Option<PendingPlugin>> = instantiated.into_iter().map(Some).collect();
        let mut ordered: Vec<PendingPlugin> = Vec::with_capacity(count);
        for &index in &order {
            if let Some(item) = slots[index].take() {
                ordered.push(item);
            }
        }
        let mut started: Vec<HostedPlugin> = Vec::with_capacity(count);
        for pending in ordered {
            let PendingPlugin {
                entry_id,
                provider,
                descriptor,
                mut plugin,
            } = pending;
            let plugin_id = descriptor.id;
            let context = Context::new(plugin_id, Arc::clone(&registry));
            let failure = match plugin.apply(&context) {
                Ok(()) => descriptor
                    .provides
                    .iter()
                    .find(|requirement| {
                        !registry.has_service(requirement.service, requirement.service_type)
                    })
                    .map(|requirement| RuntimeError::PluginFailed {
                        id: entry_id.clone(),
                        reason: format!(
                            "声明提供服务 {}，但 apply 后没有实际注册",
                            requirement.service
                        ),
                    }),
                Err(error) => Some(error),
            };
            if let Some(failure) = failure {
                let reason = failure.to_string();
                // 失败插件自身的清理与实例释放先完成，再清理它依赖的已启动插件。
                let failing = context.dispose().err();
                drop(plugin);
                let started_error = dispose_started(&mut started, &registry);
                let cleanup_error = failing.or(started_error);
                return Err(match cleanup_error {
                    None => RuntimeError::PluginFailed {
                        id: entry_id,
                        reason,
                    },
                    Some(dispose_error) => RuntimeError::PluginFailed {
                        id: entry_id,
                        reason: format!("{reason}；另外，清理阶段出错：{dispose_error}"),
                    },
                });
            }
            started.push(HostedPlugin {
                id: entry_id,
                provider,
                state: PluginState::Active,
                scope: context,
                instance: Some(plugin),
            });
        }

        Ok(Self {
            registry,
            plugins: started,
        })
    }

    /// 按服务契约获取服务。服务必须在装配描述中已由某插件提供。
    pub fn service<K: ServiceKey>(&self) -> Result<Arc<K::Service>, RuntimeError> {
        self.registry.get::<K>()
    }

    /// 当前所有插件的状态快照。
    pub fn plugin_states(&self) -> Vec<PluginStatus> {
        self.plugins
            .iter()
            .map(|plugin| PluginStatus {
                id: plugin.id.clone(),
                provider: plugin.provider.clone(),
                state: plugin.state,
                provides: plugin.scope.provided_services(),
            })
            .collect()
    }

    /// 停止所有插件：消费者先于提供者，逐个执行清理，再释放插件实例。
    /// 所有插件的清理都会执行；返回第一个错误（如有）。
    pub fn shutdown(mut self) -> Result<(), RuntimeError> {
        self.dispose_all()
    }

    fn dispose_all(&mut self) -> Result<(), RuntimeError> {
        dispose_started(&mut self.plugins, &self.registry).map_or(Ok(()), Err)
    }
}

impl Drop for PluginHost {
    /// 产品忘记调用 `shutdown`（例如提前返回或 panic）时仍要按相反顺序清理，
    /// 并释放插件实例；正常路径已清理过时这里不会重复执行清理操作。
    fn drop(&mut self) {
        let _ = self.dispose_all();
    }
}

/// 反向清理已启动插件。每个作用域先执行清理闭包，再在锁外析构服务，
/// 最后释放插件实例；所有清理都会执行，返回第一个错误（如有）。
fn dispose_started(
    started: &mut [HostedPlugin],
    registry: &ServiceRegistry,
) -> Option<RuntimeError> {
    let mut first_error = None;
    for plugin in started.iter_mut().rev() {
        plugin.state = PluginState::Stopping;
        if let Err(error) = plugin.scope.dispose() {
            first_error = first_error.or(Some(RuntimeError::Dispose {
                plugin: plugin.id.clone(),
                reason: error.to_string(),
            }));
        }
        drop(plugin.instance.take());
        plugin.state = PluginState::Stopped;
    }
    registry.clear();
    first_error
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::key::ServiceRequirement;
    use crate::product::PluginConfig;
    use std::sync::Mutex;
    use std::sync::atomic::{AtomicUsize, Ordering};

    struct LifetimeProbe {
        id: &'static str,
        applies: Arc<AtomicUsize>,
        drops: Arc<AtomicUsize>,
    }

    impl Plugin for LifetimeProbe {
        fn descriptor(&self) -> PluginDescriptor {
            PluginDescriptor {
                id: self.id,
                provides: Vec::new(),
                requires: Vec::new(),
            }
        }

        fn apply(&mut self, _ctx: &Context) -> Result<(), RuntimeError> {
            self.applies.fetch_add(1, Ordering::SeqCst);
            Ok(())
        }
    }

    impl Drop for LifetimeProbe {
        fn drop(&mut self) {
            self.drops.fetch_add(1, Ordering::SeqCst);
        }
    }

    fn start_probe() -> (PluginHost, Arc<AtomicUsize>, Arc<AtomicUsize>) {
        let applies = Arc::new(AtomicUsize::new(0));
        let drops = Arc::new(AtomicUsize::new(0));
        let mut catalog = PluginCatalog::new();
        let probe_applies = Arc::clone(&applies);
        let probe_drops = Arc::clone(&drops);
        catalog
            .register("probe", move |_config| {
                Ok(Box::new(LifetimeProbe {
                    id: "probe",
                    applies: Arc::clone(&probe_applies),
                    drops: Arc::clone(&probe_drops),
                }))
            })
            .unwrap();
        let entries = vec![PluginEntry {
            id: "probe".into(),
            provider: "probe".into(),
            config: PluginConfig::empty("probe"),
        }];
        let host = PluginHost::start(&catalog, &entries).unwrap();
        (host, applies, drops)
    }

    #[test]
    fn host_keeps_plugin_instance_alive_until_shutdown() {
        let (host, applies, drops) = start_probe();
        assert_eq!(applies.load(Ordering::SeqCst), 1);
        assert_eq!(
            drops.load(Ordering::SeqCst),
            0,
            "start 返回后实例不能提前释放"
        );
        host.shutdown().unwrap();
        assert_eq!(drops.load(Ordering::SeqCst), 1);
    }

    #[test]
    fn dropping_host_without_shutdown_still_releases_instances() {
        let (host, _, drops) = start_probe();
        drop(host);
        assert_eq!(drops.load(Ordering::SeqCst), 1);
    }

    trait TestService: Send + Sync {}

    struct TestServiceImpl {
        log: Arc<Mutex<Vec<&'static str>>>,
    }

    impl TestService for TestServiceImpl {}

    impl Drop for TestServiceImpl {
        fn drop(&mut self) {
            self.log.lock().unwrap().push("service-drop");
        }
    }

    struct TestKey;

    impl ServiceKey for TestKey {
        type Service = dyn TestService;
        const NAME: &'static str = "test.service";
    }

    struct Provider {
        log: Arc<Mutex<Vec<&'static str>>>,
    }

    impl Plugin for Provider {
        fn descriptor(&self) -> PluginDescriptor {
            PluginDescriptor {
                id: "provider",
                provides: vec![ServiceRequirement::of::<TestKey>()],
                requires: Vec::new(),
            }
        }

        fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
            let service: Arc<dyn TestService> = Arc::new(TestServiceImpl {
                log: Arc::clone(&self.log),
            });
            ctx.provide::<TestKey>(service)?;
            let log = Arc::clone(&self.log);
            ctx.on_dispose(move || {
                log.lock().unwrap().push("provider-cleanup");
                Ok(())
            })
        }
    }

    impl Drop for Provider {
        fn drop(&mut self) {
            self.log.lock().unwrap().push("provider-instance");
        }
    }

    struct Failing {
        log: Arc<Mutex<Vec<&'static str>>>,
    }

    impl Plugin for Failing {
        fn descriptor(&self) -> PluginDescriptor {
            PluginDescriptor {
                id: "failing",
                provides: Vec::new(),
                requires: vec![ServiceRequirement::of::<TestKey>()],
            }
        }

        fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
            let log = Arc::clone(&self.log);
            ctx.on_dispose(move || {
                log.lock().unwrap().push("failed-cleanup");
                Ok(())
            })?;
            Err(RuntimeError::PluginFailed {
                id: "failing".into(),
                reason: "boom".into(),
            })
        }
    }

    impl Drop for Failing {
        fn drop(&mut self) {
            self.log.lock().unwrap().push("failed-instance");
        }
    }

    #[test]
    fn failed_plugin_releases_itself_before_its_dependency() {
        let log = Arc::new(Mutex::new(Vec::new()));
        let mut catalog = PluginCatalog::new();
        let provider_log = Arc::clone(&log);
        let failing_log = Arc::clone(&log);
        catalog
            .register("provider", move |_config| {
                Ok(Box::new(Provider {
                    log: Arc::clone(&provider_log),
                }))
            })
            .unwrap();
        catalog
            .register("failing", move |_config| {
                Ok(Box::new(Failing {
                    log: Arc::clone(&failing_log),
                }))
            })
            .unwrap();
        let entries = vec![
            PluginEntry {
                id: "failing".into(),
                provider: "failing".into(),
                config: PluginConfig::empty("failing"),
            },
            PluginEntry {
                id: "provider".into(),
                provider: "provider".into(),
                config: PluginConfig::empty("provider"),
            },
        ];
        let error = match PluginHost::start(&catalog, &entries) {
            Ok(_) => panic!("失败的插件不应启动成功"),
            Err(error) => error,
        };
        assert!(matches!(
            error,
            RuntimeError::PluginFailed { ref id, .. } if id == "failing"
        ));
        assert_eq!(
            *log.lock().unwrap(),
            vec![
                "failed-cleanup",
                "failed-instance",
                "provider-cleanup",
                "service-drop",
                "provider-instance",
            ]
        );
    }
}

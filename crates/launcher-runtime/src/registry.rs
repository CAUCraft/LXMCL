use std::any::{Any, TypeId};
use std::collections::HashMap;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use crate::error::RuntimeError;
use crate::key::ServiceKey;

/// 类型擦除后的服务记录：外层 `Arc` 供多个消费者持有同一份服务实例。
pub(crate) struct ServiceRecord {
    /// `Arc<Arc<K::Service>>`，擦除为 `Arc<dyn Any + Send + Sync>`。
    service: Arc<dyn Any + Send + Sync>,
    service_type: TypeId,
}

#[derive(Default)]
pub(crate) struct ServiceRegistry {
    pub(crate) contributions: crate::contribution::ContributionRegistry,
    services: Mutex<HashMap<&'static str, ServiceRecord>>,
}

impl ServiceRegistry {
    /// 注册服务。失败时把服务交还调用方，让服务析构发生在注册表锁外。
    pub(crate) fn provide<K: ServiceKey>(
        &self,
        service: Arc<K::Service>,
        owner: &str,
    ) -> Result<(), (Arc<K::Service>, RuntimeError)> {
        let mut services = self.lock();
        if let Some(existing) = services.get(K::NAME) {
            let expected = TypeId::of::<K::Service>();
            return Err((
                service,
                if existing.service_type != expected {
                    RuntimeError::ServiceTypeMismatch { service: K::NAME }
                } else {
                    RuntimeError::DuplicateService {
                        service: K::NAME,
                        plugin: owner.to_string(),
                    }
                },
            ));
        }
        services.insert(
            K::NAME,
            ServiceRecord {
                service: Arc::new(service),
                service_type: TypeId::of::<K::Service>(),
            },
        );
        Ok(())
    }

    pub(crate) fn get<K: ServiceKey>(&self) -> Result<Arc<K::Service>, RuntimeError> {
        let service = {
            let services = self.lock();
            services
                .get(K::NAME)
                .ok_or(RuntimeError::ServiceMissing { service: K::NAME })?
                .service
                .clone()
        };
        service
            .downcast::<Arc<K::Service>>()
            .map(|outer| (*outer).clone())
            .map_err(|_| RuntimeError::ServiceTypeMismatch { service: K::NAME })
    }

    /// 锁外析构指定服务；调用方负责保证名称属于同一作用域。
    pub(crate) fn remove_services(&self, names: &[&'static str]) {
        let removed: Vec<ServiceRecord> = {
            let mut services = self.lock();
            names
                .iter()
                .filter_map(|name| services.remove(name))
                .collect()
        };
        drop(removed);
    }

    /// 清空注册表；服务析构在锁外执行。
    pub(crate) fn clear(&self) {
        let removed = {
            let mut services = self.lock();
            std::mem::take(&mut *services)
        };
        drop(removed);
    }

    fn lock(&self) -> MutexGuard<'_, HashMap<&'static str, ServiceRecord>> {
        // 锁中毒只可能来自某次操作中的 panic；中断清理比扩散 panic 更合理。
        self.services.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

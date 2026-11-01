use std::any::TypeId;

/// 一个服务的键：稳定名称 + 关联的 trait 类型。
///
/// 依赖按服务契约（名称与类型）解析，不按提供者插件的名字解析。
pub trait ServiceKey: Send + Sync + 'static {
    /// 服务的 trait 类型，例如 `dyn SettingsService`。
    type Service: ?Sized + Send + Sync + 'static;
    /// 服务名称，用于配置、依赖解析与错误展示。
    const NAME: &'static str;
}

/// 一条服务需求（提供或依赖），携带运行时类型标识用于一致性检查。
#[derive(Clone, Copy, Debug)]
pub struct ServiceRequirement {
    pub service: &'static str,
    pub service_type: TypeId,
}

impl ServiceRequirement {
    pub fn of<K: ServiceKey>() -> Self {
        Self {
            service: K::NAME,
            service_type: TypeId::of::<K::Service>(),
        }
    }
}

use std::fmt;

/// 宿主与装配过程中的错误。
///
/// 只覆盖真实会发生的情况：配置错误、依赖缺失、插件启动失败与清理失败。
/// 不做兜底恢复，配置写错就明确报错。
#[derive(Debug)]
pub enum RuntimeError {
    DuplicateContribution {
        point: &'static str,
        id: String,
    },
    ContributionTypeMismatch {
        point: &'static str,
    },
    /// 产品配置文件不存在、无法读取或不符合格式。
    ProductConfig {
        path: String,
        reason: String,
    },
    /// 用户数据目录不可用（例如 LOCALAPPDATA 环境变量未设置）。
    DataDirUnavailable,
    /// 产品配置引用了未注册的插件工厂。
    UnknownProvider {
        provider: String,
    },
    /// 插件配置无法解码为该插件声明的配置类型。
    PluginConfig {
        provider: String,
        reason: String,
    },
    /// 产品配置中同一个插件 ID 出现多次。
    DuplicatePluginId {
        id: String,
    },
    /// 同一插件工厂注册了两次。
    DuplicateProvider {
        provider: String,
    },
    /// 两个插件声明提供同一个服务。
    DuplicateService {
        service: &'static str,
        plugin: String,
    },
    /// 服务的注册类型与需求方的期望不一致。
    ServiceTypeMismatch {
        service: &'static str,
    },
    /// 没有任何插件提供所需服务。
    ServiceMissing {
        service: &'static str,
    },
    /// 服务名称与类型未在插件的对应声明中出现。
    UndeclaredService {
        plugin: String,
        service: &'static str,
        operation: &'static str,
    },
    /// 插件依赖的服务没有提供者。
    DependencyMissing {
        plugin: String,
        service: &'static str,
    },
    /// 插件依赖成环，无法确定启动顺序。
    ServiceCycle {
        chain: String,
    },
    /// 插件作用域已关闭，不能再注册服务或清理操作。
    ContextClosed {
        plugin: String,
    },
    /// 插件启动失败。
    PluginFailed {
        id: String,
        reason: String,
    },
    /// 退出清理阶段出错。
    Dispose {
        plugin: String,
        reason: String,
    },
}

impl fmt::Display for RuntimeError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::DuplicateContribution { point, id } => {
                write!(f, "duplicate contribution {point}/{id}")
            }
            Self::ContributionTypeMismatch { point } => {
                write!(f, "contribution type mismatch: {point}")
            }
            Self::ProductConfig { path, reason } => {
                write!(f, "产品配置 {path} 不可用：{reason}")
            }
            Self::DataDirUnavailable => {
                write!(f, "用户数据目录不可用：LOCALAPPDATA 未设置")
            }
            Self::UnknownProvider { provider } => {
                write!(f, "产品配置引用了未注册的插件工厂：{provider}")
            }
            Self::PluginConfig { provider, reason } => {
                write!(f, "插件 {provider} 的配置无法解析：{reason}")
            }
            Self::DuplicatePluginId { id } => {
                write!(f, "产品配置中插件 ID 重复：{id}")
            }
            Self::DuplicateProvider { provider } => {
                write!(f, "插件工厂重复注册：{provider}")
            }
            Self::DuplicateService { service, plugin } => {
                write!(f, "服务 {service} 已由其他插件提供，{plugin} 不能重复提供")
            }
            Self::ServiceTypeMismatch { service } => {
                write!(f, "服务 {service} 的注册类型与需求方期望的类型不一致")
            }
            Self::ServiceMissing { service } => {
                write!(f, "服务 {service} 没有提供者")
            }
            Self::UndeclaredService {
                plugin,
                service,
                operation,
            } => {
                write!(
                    f,
                    "插件 {plugin} 未声明 {operation} 服务 {service} 的名称与类型"
                )
            }
            Self::DependencyMissing { plugin, service } => {
                write!(f, "插件 {plugin} 依赖的服务 {service} 没有提供者")
            }
            Self::ServiceCycle { chain } => {
                write!(f, "插件服务依赖成环，无法确定启动顺序：{chain}")
            }
            Self::ContextClosed { plugin } => {
                write!(f, "插件 {plugin} 的作用域已关闭，不能再发现服务或注册资源")
            }
            Self::PluginFailed { id, reason } => {
                write!(f, "插件 {id} 启动失败：{reason}")
            }
            Self::Dispose { plugin, reason } => {
                write!(f, "插件 {plugin} 退出清理失败：{reason}")
            }
        }
    }
}

impl std::error::Error for RuntimeError {}

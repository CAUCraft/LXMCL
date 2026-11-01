use crate::context::Context;
use crate::error::RuntimeError;
use crate::key::ServiceRequirement;

/// 插件的自述：ID、提供哪些服务、依赖哪些服务。
#[derive(Clone, Debug)]
pub struct PluginDescriptor {
    /// 插件 ID，在产品装配内唯一，用于日志与错误展示。
    pub id: &'static str,
    pub provides: Vec<ServiceRequirement>,
    pub requires: Vec<ServiceRequirement>,
}

/// 插件实现接口。编译期链接：产品在装配处实例化插件并把实例交给宿主。
pub trait Plugin: Send + 'static {
    fn descriptor(&self) -> PluginDescriptor;
    /// 启动阶段执行：向上下文提供服务、登记清理。不做阻塞耗时工作。
    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError>;
}

/// 插件生命周期状态。
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PluginState {
    Registered,
    Starting,
    Active,
    Stopping,
    Stopped,
    Failed,
}

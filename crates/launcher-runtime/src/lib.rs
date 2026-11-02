//! 启动器插件宿主。
//!
//! 职责只有四件事：装配插件、登记服务、按依赖顺序启动、按相反顺序清理。
//! 宿主不认识 Minecraft、Slint、任何具体业务，也不知道 LXMCL 的存在；
//! 业务契约（例如设置服务）定义在 `launcher-api`，由插件提供、由产品消费。
//!
//! 与 Cordis（github.com/cordiverse/cordis）的关系：本模块参考了它的
//! Context 注入、Disposable 作用域与依赖先行的启动顺序，但实现是独立的
//! Rust 子集，只覆盖"启动装配 + 退出清理"，不声称 Cordis 兼容。

mod catalog;
mod context;
mod contribution;
mod error;
mod host;
mod key;
mod plugin;
mod product;
mod registry;
mod worker;

pub use catalog::{PluginCatalog, PluginFactory};
pub use context::Context;
pub use error::RuntimeError;
pub use host::{PluginHost, PluginStatus};
pub use key::{ServiceKey, ServiceRequirement};
pub use plugin::{Plugin, PluginDescriptor, PluginState};
pub use product::{PluginConfig, PluginEntry, ProductConfig, ProductIdentity};

pub use contribution::{Contribution, ContributionKey};
pub use worker::{
    BoundedWorker, ShutdownPolicy, SubmitError, WorkerExit, WorkerSender, WorkerStatus,
};

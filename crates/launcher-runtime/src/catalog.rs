use std::collections::HashMap;

use crate::error::RuntimeError;
use crate::plugin::Plugin;
use crate::product::PluginConfig;

/// 插件工厂：从产品配置里的 `[plugins.config]` 构造插件实例。
pub type PluginFactory =
    Box<dyn Fn(&PluginConfig) -> Result<Box<dyn Plugin>, RuntimeError> + Send + Sync>;

/// 插件工厂登记处。产品装配负责注册，宿主负责按配置实例化。
#[derive(Default)]
pub struct PluginCatalog {
    factories: HashMap<String, PluginFactory>,
}

impl PluginCatalog {
    pub fn new() -> Self {
        Self::default()
    }

    /// 注册一个插件工厂。工厂名对应产品配置里的 `provider` 字段。
    pub fn register<F>(&mut self, provider: &str, factory: F) -> Result<(), RuntimeError>
    where
        F: Fn(&PluginConfig) -> Result<Box<dyn Plugin>, RuntimeError> + Send + Sync + 'static,
    {
        if self.factories.contains_key(provider) {
            return Err(RuntimeError::DuplicateProvider {
                provider: provider.to_string(),
            });
        }
        self.factories
            .insert(provider.to_string(), Box::new(factory));
        Ok(())
    }

    pub(crate) fn get(&self, provider: &str) -> Option<&PluginFactory> {
        self.factories.get(provider)
    }
}

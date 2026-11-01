use std::path::Path;

use serde::Deserialize;
use toml::Table;

use crate::error::RuntimeError;

/// 产品身份：ID 决定用户数据目录命名，名字用于展示。
#[derive(Clone, Debug)]
pub struct ProductIdentity {
    pub id: String,
    pub name: String,
}

/// 插件自己的 `[plugins.config]` 原始表。由插件工厂调用 `decode` 解码。
#[derive(Clone, Debug)]
pub struct PluginConfig {
    table: Table,
    /// 配置来源的 provider 名，用于在解码错误里给出上下文。
    provider: String,
}

impl PluginConfig {
    /// 把配置解码为插件声明的配置类型。
    pub fn decode<T: serde::de::DeserializeOwned>(&self) -> Result<T, RuntimeError> {
        T::deserialize(self.table.clone()).map_err(|error| RuntimeError::PluginConfig {
            provider: self.provider.clone(),
            reason: error.to_string(),
        })
    }

    /// 仅供宿主生命周期测试构造空配置；产品路径始终从 TOML 解码。
    #[cfg(test)]
    pub(crate) fn empty(provider: &str) -> Self {
        Self {
            table: Table::new(),
            provider: provider.to_string(),
        }
    }
}

/// 产品配置里的一条插件装配项。
#[derive(Clone, Debug)]
pub struct PluginEntry {
    pub id: String,
    pub provider: String,
    pub config: PluginConfig,
}

/// 解析后的产品配置：产品身份 + 要启用的插件列表。
#[derive(Clone, Debug)]
pub struct ProductConfig {
    pub product: ProductIdentity,
    pub plugins: Vec<PluginEntry>,
}

#[derive(Deserialize)]
struct ProductFile {
    product: ProductIdentityFile,
    #[serde(default)]
    plugins: Vec<PluginEntryFile>,
}

#[derive(Deserialize)]
struct ProductIdentityFile {
    id: String,
    name: String,
}

#[derive(Deserialize)]
struct PluginEntryFile {
    id: String,
    provider: String,
    #[serde(default = "Table::new")]
    config: Table,
}

impl ProductConfig {
    /// 读取并校验产品配置。路径或格式问题会得到明确错误。
    pub fn load(path: &Path) -> Result<Self, RuntimeError> {
        let path_string = path.display().to_string();
        let text = std::fs::read_to_string(path).map_err(|error| RuntimeError::ProductConfig {
            path: path_string.clone(),
            reason: format!("无法读取文件：{error}"),
        })?;
        let file: ProductFile =
            toml::from_str(&text).map_err(|error| RuntimeError::ProductConfig {
                path: path_string.clone(),
                reason: format!("格式错误：{error}"),
            })?;
        if file.product.id.trim().is_empty() {
            return Err(RuntimeError::ProductConfig {
                path: path_string.clone(),
                reason: "[product].id 不能为空".to_string(),
            });
        }
        if file.product.name.trim().is_empty() {
            return Err(RuntimeError::ProductConfig {
                path: path_string.clone(),
                reason: "[product].name 不能为空".to_string(),
            });
        }
        let mut plugins = Vec::with_capacity(file.plugins.len());
        for (index, entry) in file.plugins.into_iter().enumerate() {
            if entry.id.trim().is_empty() {
                return Err(RuntimeError::ProductConfig {
                    path: path_string.clone(),
                    reason: format!("第 {} 条插件的 id 不能为空", index + 1),
                });
            }
            if entry.provider.trim().is_empty() {
                return Err(RuntimeError::ProductConfig {
                    path: path_string.clone(),
                    reason: format!("插件 {} 的 provider 不能为空", entry.id),
                });
            }
            if plugins
                .iter()
                .any(|existing: &PluginEntry| existing.id == entry.id)
            {
                return Err(RuntimeError::ProductConfig {
                    path: path_string.clone(),
                    reason: format!("插件 ID 重复：{}", entry.id),
                });
            }
            let plugin_provider = entry.provider.clone();
            plugins.push(PluginEntry {
                id: entry.id,
                provider: entry.provider,
                config: PluginConfig {
                    table: entry.config,
                    provider: plugin_provider,
                },
            });
        }
        Ok(Self {
            product: ProductIdentity {
                id: file.product.id,
                name: file.product.name,
            },
            plugins,
        })
    }
}

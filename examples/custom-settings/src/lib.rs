//! 第三方设置插件示例：TOML 文件设置提供者（插件 ID `settings-toml-example`）。
//!
//! 与 `launcher-settings` 的 JSON 提供者实现同一个 `SettingsService` 契约。
//! 产品装配负责解析完整文件路径；插件只保存构造参数，真正的读写
//! 发生在 `load` / `save`，因此 `apply` 阶段没有任何磁盘 I/O。

use launcher_api::{
    Context, LauncherSettings, Plugin, PluginDescriptor, RuntimeError, ServiceRequirement,
    Settings, SettingsError, SettingsService,
};
use std::path::PathBuf;
use std::sync::Arc;

/// 产品配置中 `[plugins.config]` 的解码结果。
#[derive(Clone, Debug, serde::Deserialize)]
pub struct TomlSettingsConfig {
    /// 设置文件名；由产品装配拼接数据目录得到完整路径。
    pub file_name: PathBuf,
    /// 文件不存在时使用的初始值。
    pub defaults: LauncherSettings,
}

pub struct TomlSettingsPlugin {
    path: PathBuf,
    defaults: LauncherSettings,
}

impl TomlSettingsPlugin {
    pub fn new(path: PathBuf, defaults: LauncherSettings) -> Self {
        Self { path, defaults }
    }
}

impl Plugin for TomlSettingsPlugin {
    fn descriptor(&self) -> PluginDescriptor {
        PluginDescriptor {
            id: "settings-toml-example",
            provides: vec![ServiceRequirement::of::<Settings>()],
            requires: vec![],
        }
    }

    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
        let service = TomlSettings::new(self.path.clone(), self.defaults.clone());
        ctx.provide::<Settings>(Arc::new(service))
    }
}

pub struct TomlSettings {
    path: PathBuf,
    defaults: LauncherSettings,
}

impl TomlSettings {
    pub fn new(path: PathBuf, defaults: LauncherSettings) -> Self {
        Self { path, defaults }
    }
}

impl SettingsService for TomlSettings {
    fn load(&self) -> Result<LauncherSettings, SettingsError> {
        match std::fs::read_to_string(&self.path) {
            Ok(text) => toml::from_str(&text).map_err(|error| SettingsError::Parse {
                path: self.path.clone(),
                message: error.to_string(),
            }),
            // 文件不存在时采用产品明确配置的初始值。
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(self.defaults.clone()),
            Err(error) => Err(SettingsError::Io {
                path: self.path.clone(),
                source: error,
            }),
        }
    }

    fn save(&self, settings: &LauncherSettings) -> Result<(), SettingsError> {
        // TOML 没有 null：java_path 为 None 时省略字段，不能交给序列化器报错。
        #[derive(serde::Serialize)]
        struct StoredSettings<'a> {
            game_root: &'a PathBuf,
            #[serde(skip_serializing_if = "Option::is_none")]
            java_path: Option<&'a PathBuf>,
        }
        let stored = StoredSettings {
            game_root: &settings.game_root,
            java_path: settings.java_path.as_ref(),
        };
        let text = toml::to_string_pretty(&stored)
            .map_err(|error| SettingsError::Encode(error.to_string()))?;
        if let Some(dir) = self.path.parent() {
            std::fs::create_dir_all(dir).map_err(|error| SettingsError::Io {
                path: dir.to_path_buf(),
                source: error,
            })?;
        }
        std::fs::write(&self.path, text).map_err(|error| SettingsError::Io {
            path: self.path.clone(),
            source: error,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_path(name: &str) -> PathBuf {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        std::env::temp_dir()
            .join(format!("lxmcl-toml-{name}-{}-{unique}", std::process::id()))
            .join("launcher-settings.toml")
    }

    fn cleanup(path: &Path) {
        if let Some(dir) = path.parent() {
            let _ = std::fs::remove_dir_all(dir);
        }
    }

    fn defaults() -> LauncherSettings {
        LauncherSettings {
            game_root: PathBuf::from("default-games"),
            java_path: None,
        }
    }

    #[test]
    fn missing_file_returns_configured_defaults() {
        let path = temp_path("missing");
        let service = TomlSettings::new(path.clone(), defaults());
        assert_eq!(service.load().unwrap(), defaults());
        cleanup(&path);
    }

    #[test]
    fn save_without_java_path_omits_none_and_reloads() {
        let path = temp_path("none");
        let settings = LauncherSettings {
            game_root: path.parent().unwrap().join("games"),
            java_path: None,
        };
        TomlSettings::new(path.clone(), defaults())
            .save(&settings)
            .unwrap();
        let text = std::fs::read_to_string(&path).unwrap();
        assert!(!text.contains("java_path"));
        let restarted = TomlSettings::new(path.clone(), defaults());
        assert_eq!(restarted.load().unwrap(), settings);
        cleanup(&path);
    }

    #[test]
    fn save_with_java_path_roundtrips_after_restart() {
        let path = temp_path("java");
        let settings = LauncherSettings {
            game_root: path.parent().unwrap().join("games"),
            java_path: Some(PathBuf::from(r"C:\Java\bin\java.exe")),
        };
        TomlSettings::new(path.clone(), defaults())
            .save(&settings)
            .unwrap();
        let restarted = TomlSettings::new(path.clone(), defaults());
        assert_eq!(restarted.load().unwrap(), settings);
        cleanup(&path);
    }

    #[test]
    fn corrupt_file_is_reported_without_overwriting() {
        let path = temp_path("corrupt");
        std::fs::create_dir_all(path.parent().unwrap()).unwrap();
        std::fs::write(&path, b"[not toml").unwrap();
        let service = TomlSettings::new(path.clone(), defaults());
        assert!(matches!(service.load(), Err(SettingsError::Parse { .. })));
        assert_eq!(std::fs::read(&path).unwrap(), b"[not toml");
        cleanup(&path);
    }
}

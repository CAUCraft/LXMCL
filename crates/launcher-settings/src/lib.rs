//! JSON 文件设置服务提供者（插件 ID `settings-json`）。
//!
//! 产品装配负责解析完整文件路径；插件只保存构造参数，真正的读写
//! 发生在 `load` / `save`，因此 `apply` 阶段没有任何磁盘 I/O。

use std::path::PathBuf;
use std::sync::Arc;

use launcher_api::{
    Context, LauncherSettings, Plugin, PluginDescriptor, RuntimeError, ServiceRequirement,
    Settings, SettingsError, SettingsService,
};
use serde::Deserialize;

/// 产品配置中 `[plugins.config]` 的解码结果。
#[derive(Clone, Debug, Deserialize)]
pub struct JsonSettingsConfig {
    /// 设置文件名；由产品装配拼接数据目录得到完整路径。
    pub file_name: PathBuf,
    /// 文件不存在时使用的初始值。
    pub defaults: LauncherSettings,
}

pub struct JsonSettingsPlugin {
    path: PathBuf,
    defaults: LauncherSettings,
}

impl JsonSettingsPlugin {
    pub fn new(path: PathBuf, defaults: LauncherSettings) -> Self {
        Self { path, defaults }
    }
}

impl Plugin for JsonSettingsPlugin {
    fn descriptor(&self) -> PluginDescriptor {
        PluginDescriptor {
            id: "settings-json",
            provides: vec![ServiceRequirement::of::<Settings>()],
            requires: vec![],
        }
    }

    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
        let service = JsonSettings::new(self.path.clone(), self.defaults.clone());
        ctx.provide::<Settings>(Arc::new(service))
    }
}

pub struct JsonSettings {
    path: PathBuf,
    defaults: LauncherSettings,
}

impl JsonSettings {
    pub fn new(path: PathBuf, defaults: LauncherSettings) -> Self {
        Self { path, defaults }
    }
}

impl SettingsService for JsonSettings {
    fn load(&self) -> Result<LauncherSettings, SettingsError> {
        match std::fs::read(&self.path) {
            Ok(bytes) => serde_json::from_slice(&bytes).map_err(|error| SettingsError::Parse {
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
        let bytes = serde_json::to_vec_pretty(settings)
            .map_err(|error| SettingsError::Encode(error.to_string()))?;
        if let Some(dir) = self.path.parent() {
            std::fs::create_dir_all(dir).map_err(|error| SettingsError::Io {
                path: dir.to_path_buf(),
                source: error,
            })?;
        }
        std::fs::write(&self.path, bytes).map_err(|error| SettingsError::Io {
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
            .join(format!("lxmcl-json-{name}-{}-{unique}", std::process::id()))
            .join("launcher-settings.json")
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
        let service = JsonSettings::new(path.clone(), defaults());
        assert_eq!(service.load().unwrap(), defaults());
        cleanup(&path);
    }

    #[test]
    fn save_then_reload_roundtrips_after_restart() {
        let path = temp_path("roundtrip");
        let settings = LauncherSettings {
            game_root: path.parent().unwrap().join("games"),
            java_path: Some(PathBuf::from(r"C:\Java\bin\java.exe")),
        };
        JsonSettings::new(path.clone(), defaults())
            .save(&settings)
            .unwrap();
        // 新实例读同一个文件，等价于重启后重新装配插件。
        let restarted = JsonSettings::new(path.clone(), defaults());
        assert_eq!(restarted.load().unwrap(), settings);
        cleanup(&path);
    }

    #[test]
    fn corrupt_file_is_reported_without_overwriting() {
        let path = temp_path("corrupt");
        std::fs::create_dir_all(path.parent().unwrap()).unwrap();
        std::fs::write(&path, b"{ not json").unwrap();
        let service = JsonSettings::new(path.clone(), defaults());
        assert!(matches!(service.load(), Err(SettingsError::Parse { .. })));
        assert_eq!(std::fs::read(&path).unwrap(), b"{ not json");
        cleanup(&path);
    }
}

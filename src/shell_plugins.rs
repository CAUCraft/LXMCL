//! Product tools are ordinary plugins. They contribute semantic actions, not widgets.
use launcher_api::{
    Context, InstanceRepository, Instances, Plugin, PluginDescriptor, RuntimeError,
    ServiceRequirement, SettingsSection, UiAction, UiCommand, UiContribution, UiContributions,
    UiPlacement, UiRoute, VersionSection,
};
use std::sync::{
    Arc,
    atomic::{AtomicBool, Ordering},
};

pub struct LocalToolsPlugin;

impl Plugin for LocalToolsPlugin {
    fn descriptor(&self) -> PluginDescriptor {
        PluginDescriptor {
            id: "local-tools",
            provides: vec![],
            requires: vec![ServiceRequirement::of::<Instances>()],
        }
    }

    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
        for (id, order, title, description, placement, route) in [
            (
                "lxmcl.instances",
                10,
                "实例配置",
                "创建、重命名与选择已保存的实例",
                UiPlacement::Versions,
                UiRoute::Versions(VersionSection::Installed),
            ),
            (
                "lxmcl.java",
                20,
                "本机 Java",
                "检测本机运行时，验证并保存 Java 路径",
                UiPlacement::Settings,
                UiRoute::Settings(SettingsSection::Java),
            ),
        ] {
            ctx.contribute::<UiContributions>(
                id,
                order,
                UiContribution {
                    title: title.into(),
                    description: description.into(),
                    placement,
                    action: UiAction::Navigate(route),
                },
            )?;
        }
        ctx.contribute::<UiContributions>(
            "lxmcl.inspect-directories",
            30,
            UiContribution {
                title: "检查实例目录".into(),
                description: "只读检查已保存实例的目录是否存在；不创建文件，也不判断游戏能否启动"
                    .into(),
                placement: UiPlacement::Extensions,
                action: UiAction::Command(Arc::new(InspectDirectories(ctx.get::<Instances>()?))),
            },
        )
    }
}

struct InspectDirectories(Arc<dyn InstanceRepository>);

impl UiCommand for InspectDirectories {
    fn execute(&self, cancelled: &AtomicBool) -> Result<String, String> {
        if cancelled.load(Ordering::Acquire) {
            return Err("目录检查已取消".into());
        }
        let records = self.0.list().map_err(|error| error.to_string())?;
        if records.is_empty() {
            return Ok("尚未创建实例配置。可在「版本」中创建。".into());
        }
        let mut lines = Vec::new();
        for record in records.iter().take(100) {
            if cancelled.load(Ordering::Acquire) {
                return Err("目录检查已取消".into());
            }
            let result = match std::fs::metadata(&record.game_directory) {
                Ok(meta) if meta.is_dir() => "目录存在".to_string(),
                Ok(_) => "路径不是目录".to_string(),
                Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                    "目录尚不存在".to_string()
                }
                Err(error) => format!("无法访问：{error}"),
            };
            lines.push(format!(
                "{}：{}\n{}",
                record.name,
                result,
                record.game_directory.display()
            ));
        }
        if records.len() > 100 {
            lines.push(format!("本次显示前 100 项，共 {} 项。", records.len()));
        }
        Ok(lines.join("\n\n"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use launcher_api::{InstanceError, InstanceRecord, NewInstance};
    use std::path::PathBuf;

    struct Repository(Vec<InstanceRecord>);
    impl InstanceRepository for Repository {
        fn list(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
            Ok(self.0.clone())
        }
        fn create(&self, _: &NewInstance) -> Result<InstanceRecord, InstanceError> {
            panic!("read-only audit must never create")
        }
        fn rename(&self, _: i32, _: u64, _: &str) -> Result<InstanceRecord, InstanceError> {
            panic!("read-only audit must never rename")
        }
    }
    fn record(id: i32, path: PathBuf) -> InstanceRecord {
        InstanceRecord {
            id,
            name: format!("实例{id}"),
            version: "1.21.1".into(),
            loader: "Vanilla".into(),
            game_directory: path,
            revision: 1,
        }
    }

    #[test]
    fn audit_reports_real_directory_state_without_creating_missing_paths() {
        let root =
            std::env::temp_dir().join(format!("lxmcl-directory-audit-{}", std::process::id()));
        std::fs::create_dir_all(&root).unwrap();
        let missing = root.join("missing-instance");
        let file = root.join("not-a-directory");
        std::fs::write(&file, "fixture").unwrap();
        let command = InspectDirectories(Arc::new(Repository(vec![
            record(1, root.clone()),
            record(2, missing.clone()),
            record(3, file.clone()),
        ])));
        let report = command.execute(&AtomicBool::new(false)).unwrap();
        assert!(report.contains("实例1：目录存在"));
        assert!(report.contains("实例2：目录尚不存在"));
        assert!(report.contains("实例3：路径不是目录"));
        assert!(!missing.exists());
        assert!(command.execute(&AtomicBool::new(true)).is_err());
        std::fs::remove_file(file).unwrap();
        std::fs::remove_dir(root).unwrap();
    }
}

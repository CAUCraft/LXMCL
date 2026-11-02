//! JSON 实例仓库提供者（插件 ID `instances-json`）。
//!
//! 产品装配负责解析完整文件路径；插件只保存构造参数，`apply` 不读写磁盘。
//! 列表、创建和重命名由产品边界放在工作线程调用。文件损坏时明确报错，
//! 不会静默当成空仓库覆盖。

use launcher_api::{
    Context, InstanceError, InstanceRecord, InstanceRepository, Instances, NewInstance, Plugin,
    PluginDescriptor, RuntimeError, ServiceRequirement,
};
use serde::{Deserialize, Serialize};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

#[derive(Clone, Debug, Deserialize)]
pub struct JsonInstancesConfig {
    /// 实例文件名；由产品装配拼接数据目录得到完整路径。
    pub file_name: PathBuf,
}

pub struct JsonInstancesPlugin {
    path: PathBuf,
}

impl JsonInstancesPlugin {
    pub fn new(path: PathBuf) -> Self {
        Self { path }
    }
}

impl Plugin for JsonInstancesPlugin {
    fn descriptor(&self) -> PluginDescriptor {
        PluginDescriptor {
            id: "instances-json",
            provides: vec![ServiceRequirement::of::<Instances>()],
            requires: vec![],
        }
    }

    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
        ctx.provide::<Instances>(Arc::new(JsonInstances::new(self.path.clone())))
    }
}

#[derive(Serialize, Deserialize)]
struct InstanceFile {
    instances: Vec<InstanceRecord>,
}

pub struct JsonInstances {
    path: PathBuf,
    writes: Mutex<()>,
}

impl JsonInstances {
    pub fn new(path: PathBuf) -> Self {
        Self {
            path,
            writes: Mutex::new(()),
        }
    }

    fn read(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
        match std::fs::read(&self.path) {
            Ok(bytes) => serde_json::from_slice::<InstanceFile>(&bytes)
                .map(|file| file.instances)
                .map_err(|error| InstanceError::Parse {
                    path: self.path.clone(),
                    message: error.to_string(),
                }),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(Vec::new()),
            Err(error) => Err(InstanceError::Io {
                path: self.path.clone(),
                source: error,
            }),
        }
    }

    fn write(&self, instances: &[InstanceRecord]) -> Result<(), InstanceError> {
        let bytes = serde_json::to_vec_pretty(&InstanceFile {
            instances: instances.to_vec(),
        })
        .map_err(|error| InstanceError::Encode(error.to_string()))?;
        let parent = self
            .path
            .parent()
            .filter(|path| !path.as_os_str().is_empty())
            .unwrap_or_else(|| Path::new("."));
        std::fs::create_dir_all(parent).map_err(|error| InstanceError::Io {
            path: parent.to_path_buf(),
            source: error,
        })?;
        // 同目录临时文件 + 关闭后替换，避免写到一半留下损坏仓库。
        let mut temp =
            tempfile::NamedTempFile::new_in(parent).map_err(|error| InstanceError::Io {
                path: parent.to_path_buf(),
                source: error,
            })?;
        temp.write_all(&bytes).map_err(|error| InstanceError::Io {
            path: self.path.clone(),
            source: error,
        })?;
        temp.as_file_mut()
            .sync_all()
            .map_err(|error| InstanceError::Io {
                path: self.path.clone(),
                source: error,
            })?;
        temp.persist(&self.path)
            .map_err(|error| InstanceError::Io {
                path: self.path.clone(),
                source: error.error,
            })?;
        Ok(())
    }
}

fn valid_name(
    name: &str,
    instances: &[InstanceRecord],
    except: Option<i32>,
) -> Result<String, InstanceError> {
    let name = name.trim();
    if name.is_empty() {
        return Err(InstanceError::Invalid("名称不能为空".into()));
    }
    if name.chars().count() > 32 {
        return Err(InstanceError::Invalid("名称不能超过 32 个字符".into()));
    }
    if instances
        .iter()
        .any(|instance| Some(instance.id) != except && instance.name.eq_ignore_ascii_case(name))
    {
        return Err(InstanceError::DuplicateName {
            name: name.to_string(),
        });
    }
    Ok(name.to_string())
}

fn next_id(instances: &[InstanceRecord]) -> Result<i32, InstanceError> {
    instances
        .iter()
        .map(|instance| instance.id)
        .max()
        .unwrap_or(0)
        .checked_add(1)
        .ok_or_else(|| InstanceError::Invalid("实例 ID 已用尽".into()))
}

impl InstanceRepository for JsonInstances {
    fn list(&self) -> Result<Vec<InstanceRecord>, InstanceError> {
        self.read()
    }

    fn create(&self, draft: &NewInstance) -> Result<InstanceRecord, InstanceError> {
        let _write = self.writes.lock().unwrap();
        let mut instances = self.read()?;
        let name = valid_name(&draft.name, &instances, None)?;
        let version = draft.version.trim();
        let loader = draft.loader.trim();
        if version.is_empty() || loader.is_empty() {
            return Err(InstanceError::Invalid("版本和加载器不能为空".into()));
        }
        if draft.game_root.as_os_str().is_empty() {
            return Err(InstanceError::Invalid("游戏根目录不能为空".into()));
        }
        let id = next_id(&instances)?;
        let record = InstanceRecord {
            id,
            name,
            version: version.to_string(),
            loader: loader.to_string(),
            game_directory: draft.game_root.join(format!("instance-{id}")),
            revision: 1,
        };
        instances.push(record.clone());
        self.write(&instances)?;
        Ok(record)
    }

    fn rename(
        &self,
        id: i32,
        expected_revision: u64,
        name: &str,
    ) -> Result<InstanceRecord, InstanceError> {
        let _write = self.writes.lock().unwrap();
        let mut instances = self.read()?;
        let current = instances
            .iter()
            .find(|instance| instance.id == id)
            .ok_or(InstanceError::NotFound { id })?;
        if current.revision != expected_revision {
            return Err(InstanceError::RevisionConflict {
                id,
                expected: expected_revision,
                actual: current.revision,
            });
        }
        let name = valid_name(name, &instances, Some(id))?;
        let instance = instances
            .iter_mut()
            .find(|instance| instance.id == id)
            .ok_or(InstanceError::NotFound { id })?;
        if instance.name == name {
            return Ok(instance.clone());
        }
        instance.name = name;
        instance.revision = instance
            .revision
            .checked_add(1)
            .ok_or_else(|| InstanceError::Invalid("实例修订号已用尽".into()))?;
        let record = instance.clone();
        self.write(&instances)?;
        Ok(record)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn draft(root: &Path) -> NewInstance {
        NewInstance {
            name: "生存实例".into(),
            version: "1.21.1".into(),
            loader: "Fabric".into(),
            game_root: root.to_path_buf(),
        }
    }

    #[test]
    fn create_rename_and_reload_keep_id_and_revision() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("instances.json");
        let games = dir.path().join("games");
        let service = JsonInstances::new(file.clone());
        assert!(service.list().unwrap().is_empty());

        let created = service.create(&draft(&games)).unwrap();
        assert_eq!(created.id, 1);
        assert_eq!(created.revision, 1);
        assert_eq!(created.game_directory, games.join("instance-1"));

        let renamed = service
            .rename(created.id, created.revision, "新名字")
            .unwrap();
        assert_eq!(renamed.name, "新名字");
        assert_eq!(renamed.revision, 2);
        assert_eq!(renamed.game_directory, created.game_directory);

        let restarted = JsonInstances::new(file);
        assert!(matches!(
            service.rename(created.id, created.revision, "旧请求"),
            Err(InstanceError::RevisionConflict { actual: 2, .. })
        ));
        assert_eq!(
            service
                .rename(renamed.id, renamed.revision, "新名字")
                .unwrap(),
            renamed
        );
        assert_eq!(restarted.list().unwrap(), vec![renamed]);
    }

    #[test]
    fn shared_repository_serializes_concurrent_writes() {
        let dir = tempfile::tempdir().unwrap();
        let service = Arc::new(JsonInstances::new(dir.path().join("instances.json")));
        let barrier = Arc::new(std::sync::Barrier::new(8));
        let threads: Vec<_> = (0..8)
            .map(|index| {
                let service = service.clone();
                let barrier = barrier.clone();
                let mut input = draft(dir.path());
                input.name = format!("instance-{index}");
                std::thread::spawn(move || {
                    barrier.wait();
                    service.create(&input).unwrap()
                })
            })
            .collect();
        for thread in threads {
            thread.join().unwrap();
        }
        let records = service.list().unwrap();
        assert_eq!(records.len(), 8);
        let mut ids: Vec<_> = records.iter().map(|record| record.id).collect();
        ids.sort();
        assert_eq!(ids, (1..=8).collect::<Vec<_>>());
    }

    #[test]
    fn corrupt_file_reports_and_is_not_overwritten() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("instances.json");
        std::fs::write(&file, b"{ not json").unwrap();
        let service = JsonInstances::new(file.clone());

        assert!(matches!(service.list(), Err(InstanceError::Parse { .. })));
        assert!(matches!(
            service.create(&draft(&dir.path().join("games"))),
            Err(InstanceError::Parse { .. })
        ));
        assert_eq!(std::fs::read(&file).unwrap(), b"{ not json");
    }
}

use launcher_api::InstanceRecord;
use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Clone)]
pub struct Game {
    pub id: i32,
    pub name: String,
    pub version: String,
    pub loader: String,
    pub game_directory: PathBuf,
    pub revision: u64,
    pub content: Vec<Content>,
}

impl From<InstanceRecord> for Game {
    fn from(record: InstanceRecord) -> Self {
        Self {
            id: record.id,
            name: record.name,
            version: record.version,
            loader: record.loader,
            game_directory: record.game_directory,
            revision: record.revision,
            content: Vec::new(),
        }
    }
}

#[derive(Clone)]
pub struct Content {
    pub id: i32,
    pub name: String,
    pub category: i32,
    pub source: String,
    pub enabled: bool,
}

#[derive(Clone)]
pub struct Resource {
    pub id: i32,
    pub name: &'static str,
    pub alias: &'static str,
    pub description: &'static str,
    pub category: i32,
    pub version: &'static str,
    pub loader: &'static str,
    pub source: &'static str,
}

impl Resource {
    pub fn compatible(&self, game: &Game) -> bool {
        self.version == game.version && (self.loader == "通用" || self.loader == game.loader)
    }
}

pub struct Download {
    pub id: i32,
    pub name: String,
    pub target: i32,
    pub target_name: String,
    pub content: Option<Content>,
    pub progress: i32,
    pub paused: bool,
    pub done: bool,
    pub join: bool,
}

#[derive(Clone, Serialize, Deserialize)]
#[serde(default)]
pub struct Preferences {
    pub dark: bool,
    pub reduce_motion: bool,
    pub pixel_glass: bool,
    pub wallpaper_index: i32,
    #[serde(default = "legacy_wallpaper_enabled")]
    pub wallpaper_enabled: bool,
    pub wallpaper_paused: bool,
    pub folder: String,
    pub memory: i32,
    pub auto_memory: bool,
    pub fullscreen: bool,
    pub proxy: i32,
    pub source: i32,
}

// Existing files without this field used the former dynamic-background default.
fn legacy_wallpaper_enabled() -> bool {
    true
}

impl Default for Preferences {
    fn default() -> Self {
        Self {
            dark: false,
            reduce_motion: false,
            pixel_glass: false,
            wallpaper_index: 0,
            wallpaper_enabled: false,
            wallpaper_paused: false,
            folder: "D:\\Games\\LXMCL".into(),
            memory: 4,
            auto_memory: true,
            fullscreen: false,
            proxy: 0,
            source: 0,
        }
    }
}

impl Preferences {
    pub fn path() -> PathBuf {
        std::env::var_os("LOCALAPPDATA")
            .map(PathBuf::from)
            .unwrap_or_else(std::env::temp_dir)
            .join("LXMCL")
            .join("ui-preferences.json")
    }
    pub fn load() -> Self {
        let mut preferences: Self = std::fs::read(Self::path())
            .ok()
            .and_then(|data| serde_json::from_slice(&data).ok())
            .unwrap_or_default();
        preferences.memory = preferences.memory.clamp(1, 32);
        preferences.wallpaper_index = preferences.wallpaper_index.clamp(0, 6);
        preferences.proxy = preferences.proxy.clamp(0, 2);
        preferences.source = preferences.source.clamp(0, 2);
        preferences
    }
    pub fn save(&self) -> Result<(), Box<dyn std::error::Error>> {
        let path = Self::path();
        std::fs::create_dir_all(path.parent().ok_or("配置目录不可用")?)?;
        std::fs::write(path, serde_json::to_vec_pretty(self)?)?;
        Ok(())
    }
}

pub struct State {
    pub games: Vec<Game>,
    pub selected: i32,
    pub inspected: i32,
    pub catalog: Vec<Resource>,
    pub downloads: Vec<Download>,
    pub accounts: Vec<(i32, String)>,
    pub account: i32,
    pub java_path: Option<PathBuf>,
    pub next_id: i32,
    pub pending: Option<(String, i32, i32, i32)>,
    pub launch: Option<i32>,
    pub diagnostic: Option<(i32, i32)>,
}

impl Default for State {
    fn default() -> Self {
        Self {
            games: vec![
                Game {
                    id: 10,
                    name: "Fabric 生存".into(),
                    version: "1.21.1".into(),
                    loader: "Fabric".into(),
                    game_directory: PathBuf::new(),
                    revision: 1,
                    content: vec![
                        Content {
                            id: 100,
                            name: "Fabric API".into(),
                            category: 0,
                            source: "Modrinth".into(),
                            enabled: true,
                        },
                        Content {
                            id: 101,
                            name: "本地示例模组".into(),
                            category: 0,
                            source: "本地导入".into(),
                            enabled: false,
                        },
                    ],
                },
                Game {
                    id: 20,
                    name: "原版生存".into(),
                    version: "1.21.1".into(),
                    loader: "原版".into(),
                    game_directory: PathBuf::new(),
                    revision: 1,
                    content: vec![],
                },
                Game {
                    id: 30,
                    name: "模组实验室".into(),
                    version: "1.20.1".into(),
                    loader: "Forge".into(),
                    game_directory: PathBuf::new(),
                    revision: 1,
                    content: vec![],
                },
            ],
            selected: 10,
            inspected: 10,
            catalog: vec![
                Resource {
                    id: 100,
                    name: "Fabric API",
                    alias: "前置",
                    description: "Fabric 模组常用的基础依赖。",
                    category: 0,
                    version: "1.21.1",
                    loader: "Fabric",
                    source: "Modrinth",
                },
                Resource {
                    id: 102,
                    name: "Sodium",
                    alias: "钠 优化 性能",
                    description: "改善游戏渲染性能，让探索更流畅。",
                    category: 0,
                    version: "1.21.1",
                    loader: "Fabric",
                    source: "Modrinth",
                },
                Resource {
                    id: 103,
                    name: "Iris Shaders",
                    alias: "鸢尾 光影",
                    description: "为 Fabric 游戏提供光影支持。",
                    category: 0,
                    version: "1.21.1",
                    loader: "Fabric",
                    source: "Modrinth",
                },
                Resource {
                    id: 104,
                    name: "JEI",
                    alias: "物品管理器 合成",
                    description: "查看物品与合成配方。",
                    category: 0,
                    version: "1.20.1",
                    loader: "Forge",
                    source: "CurseForge",
                },
                Resource {
                    id: 110,
                    name: "轻量整合包",
                    alias: "轻量 优化",
                    description: "整合包演示配置，包含基础优化资源。",
                    category: 1,
                    version: "1.21.1",
                    loader: "Fabric",
                    source: "演示资源",
                },
                Resource {
                    id: 120,
                    name: "简洁方块",
                    alias: "材质 资源包",
                    description: "清晰的方块纹理与柔和配色。",
                    category: 2,
                    version: "1.21.1",
                    loader: "通用",
                    source: "演示资源",
                },
                Resource {
                    id: 130,
                    name: "柔和光影",
                    alias: "光影 着色器",
                    description: "温和的光照与水面效果。",
                    category: 3,
                    version: "1.21.1",
                    loader: "Fabric",
                    source: "演示资源",
                },
                Resource {
                    id: 140,
                    name: "一起建造的世界",
                    alias: "地图 存档",
                    description: "用于演示存档管理的社区世界。",
                    category: 4,
                    version: "1.21.1",
                    loader: "通用",
                    source: "演示资源",
                },
            ],
            downloads: vec![],
            accounts: vec![(1, "本地玩家".into())],
            account: 1,
            java_path: None,
            next_id: 1000,
            pending: None,
            launch: None,
            diagnostic: None,
        }
    }
}

impl State {
    /// 用实例仓库的记录替换演示条目；空仓库就是空列表。
    pub fn replace_instances(&mut self, instances: Vec<InstanceRecord>) {
        self.games = instances.into_iter().map(Game::from).collect();
        self.selected = self.games.first().map_or(-1, |game| game.id);
        self.inspected = self.selected;
        self.downloads.clear();
        self.pending = None;
        self.launch = None;
    }

    /// 创建成功后更新单条记录并选择该实例；不补演示模组或下载任务。
    pub fn apply_instance(&mut self, record: InstanceRecord) {
        let id = record.id;
        self.update_instance(record);
        self.selected = id;
    }

    /// Apply a captured operation without changing the user's current selection.
    pub fn update_instance(&mut self, record: InstanceRecord) {
        let id = record.id;
        if let Some(game) = self.games.iter_mut().find(|game| game.id == id) {
            if record.revision < game.revision {
                return;
            }
            game.name = record.name;
            game.version = record.version;
            game.loader = record.loader;
            game.game_directory = record.game_directory;
            game.revision = record.revision;
        } else {
            self.games.push(Game::from(record));
        }
    }

    pub fn id(&mut self) -> i32 {
        let id = self.next_id;
        self.next_id += 1;
        id
    }
    pub fn game(&self) -> Option<&Game> {
        self.games.iter().find(|g| g.id == self.selected)
    }
    pub fn inspected_game(&self) -> Option<&Game> {
        self.games.iter().find(|g| g.id == self.inspected)
    }
    pub fn inspect(&mut self, id: i32) -> bool {
        if self.games.iter().any(|g| g.id == id) {
            self.inspected = id;
            true
        } else {
            false
        }
    }
    pub fn select(&mut self, id: i32) {
        if self.games.iter().any(|g| g.id == id) {
            self.selected = id;
        }
    }
    pub fn remove_game(&mut self, id: i32) {
        self.games.retain(|g| g.id != id);
        self.downloads.retain(|d| d.target != id);
        if self.selected == id {
            self.selected = self.games.first().map_or(-1, |g| g.id);
        }
        if self.inspected == id {
            self.inspected = -1;
        }
    }
    pub fn queue(&mut self, resource_id: i32, target: i32) -> Result<(), &'static str> {
        let resource = self
            .catalog
            .iter()
            .find(|r| r.id == resource_id)
            .ok_or("资源已不存在")?
            .clone();
        let game = self
            .games
            .iter()
            .find(|g| g.id == target)
            .ok_or("请先创建游戏")?;
        if !resource.compatible(game) {
            return Err("资源与目标游戏的版本或加载器不兼容");
        }
        if game.content.iter().any(|c| c.id == resource_id) {
            return Err("目标游戏已安装此资源");
        }
        if self.downloads.iter().any(|d| {
            d.target == target && d.content.as_ref().is_some_and(|c| c.id == resource_id) && !d.done
        }) {
            return Err("该资源已在下载队列中");
        }
        let target_name = game.name.clone();
        let id = self.id();
        self.downloads.push(Download {
            id,
            name: resource.name.into(),
            target,
            target_name,
            content: Some(Content {
                id: resource.id,
                name: resource.name.into(),
                category: resource.category,
                source: resource.source.into(),
                enabled: true,
            }),
            progress: 0,
            paused: false,
            done: false,
            join: false,
        });
        Ok(())
    }
    pub fn tick_downloads(&mut self) -> Vec<(String, bool, i32)> {
        let mut completed = vec![];
        for task in &mut self.downloads {
            if task.done || task.paused {
                continue;
            }
            task.progress = (task.progress + 2).min(100);
            if task.progress == 100 {
                task.done = true;
                if let Some(game) = self.games.iter_mut().find(|g| g.id == task.target) {
                    if let Some(content) = &task.content
                        && !game.content.iter().any(|c| c.id == content.id)
                    {
                        game.content.push(content.clone());
                    }
                    completed.push((task.name.clone(), task.join, task.target));
                }
            }
        }
        completed
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn inspecting_and_renaming_another_instance_preserves_launch_target() {
        let mut state = State::default();
        assert!(state.inspect(20));
        assert_eq!(state.game().unwrap().id, 10);
        assert_eq!(state.inspected_game().unwrap().id, 20);
        state.update_instance(InstanceRecord {
            id: 20,
            name: "Renamed B".into(),
            version: "1.21.1".into(),
            loader: "原版".into(),
            game_directory: "games/b".into(),
            revision: 2,
        });
        assert_eq!(state.selected, 10);
        assert_eq!(state.inspected, 20);
        assert_eq!(state.inspected_game().unwrap().name, "Renamed B");
        state.select(30);
        assert_eq!(state.inspected, 20);
    }
    #[test]
    fn older_preferences_get_wallpaper_defaults_and_new_choices_round_trip() {
        let mut p: Preferences = serde_json::from_str(r#"{"dark":true,"memory":8}"#).unwrap();
        assert!(p.dark && p.wallpaper_enabled && !p.wallpaper_paused);
        assert_eq!(p.wallpaper_index, 0);
        p.wallpaper_index = 6;
        p.wallpaper_paused = true;
        let restored: Preferences =
            serde_json::from_slice(&serde_json::to_vec(&p).unwrap()).unwrap();
        assert!(restored.wallpaper_paused);
        assert_eq!(restored.wallpaper_index, 6);
        assert_eq!(restored.memory, 8);
    }
    #[test]
    fn first_launch_is_static_and_saved_wallpaper_choices_are_preserved() {
        assert!(!Preferences::default().wallpaper_enabled);
        for enabled in [false, true] {
            let saved: Preferences = serde_json::from_str(&format!(
                r#"{{"wallpaper_enabled":{enabled},"wallpaper_index":4,"wallpaper_paused":true}}"#
            ))
            .unwrap();
            let restored: Preferences =
                serde_json::from_slice(&serde_json::to_vec(&saved).unwrap()).unwrap();
            assert_eq!(restored.wallpaper_enabled, enabled);
            assert_eq!(restored.wallpaper_index, 4);
            assert!(restored.wallpaper_paused);
        }
    }

    #[test]
    fn download_keeps_original_target_when_selection_changes() {
        let mut state = State::default();
        state.queue(102, 10).unwrap();
        state.select(20);
        for _ in 0..50 {
            state.tick_downloads();
        }
        assert!(state.games[0].content.iter().any(|c| c.id == 102));
        assert!(state.games[1].content.is_empty());
        assert_eq!(state.selected, 20);
    }
    #[test]
    fn paused_and_cancelled_tasks_do_not_install() {
        let mut state = State::default();
        state.queue(102, 10).unwrap();
        state.downloads[0].paused = true;
        for _ in 0..60 {
            state.tick_downloads();
        }
        assert_eq!(state.downloads[0].progress, 0);
        state.downloads.clear();
        state.tick_downloads();
        assert!(!state.games[0].content.iter().any(|c| c.id == 102));
    }
    #[test]
    fn resume_finishes_once_and_duplicate_queue_is_rejected() {
        let mut state = State::default();
        state.queue(102, 10).unwrap();
        assert!(state.queue(102, 10).is_err());
        state.downloads[0].paused = true;
        state.tick_downloads();
        state.downloads[0].paused = false;
        for _ in 0..100 {
            state.tick_downloads();
        }
        assert_eq!(
            state.games[0]
                .content
                .iter()
                .filter(|c| c.id == 102)
                .count(),
            1
        );
        assert!(state.queue(102, 10).is_err());
    }
    #[test]
    fn compatibility_and_missing_targets_are_rejected() {
        let mut state = State::default();
        assert!(state.queue(102, 20).is_err());
        assert!(state.queue(102, 999).is_err());
        assert!(state.queue(104, 10).is_err());
        assert!(state.queue(104, 30).is_ok());
    }
    #[test]
    fn deleting_game_cancels_its_tasks_and_keeps_stable_ids() {
        let mut state = State::default();
        state.queue(102, 10).unwrap();
        state.remove_game(10);
        assert!(state.downloads.is_empty());
        assert_eq!(state.selected, 20);
        state.select(30);
        assert_eq!(state.game().unwrap().name, "模组实验室");
        state.remove_game(20);
        state.remove_game(30);
        assert!(state.game().is_none());
    }
}

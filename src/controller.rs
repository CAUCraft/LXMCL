use crate::instances_bridge::InstanceBridge;
use crate::java_bridge::JavaBridge;
use crate::settings_bridge::SettingsBridge;
use crate::state::{Content, Game, Preferences, State};
use crate::{
    AccountItem, AppState, AppWindow, DownloadItem, GameItem, Guidance, InstanceUpdate,
    JavaCandidateItem, ModItem, ResourceItem, SearchItem, Theme,
};
use launcher_api::{InstanceRecord, JavaCandidate, JavaScanResult, LauncherSettings, NewInstance};
use slint::{Color, ComponentHandle, Model, ModelRc, SharedString, Timer, TimerMode, VecModel};
use std::{
    cell::RefCell,
    path::{Path, PathBuf},
    process::{Command, Stdio},
    rc::Rc,
    time::Duration,
};

fn model<T: Clone + 'static>(values: Vec<T>) -> ModelRc<T> {
    Rc::new(VecModel::from(values)).into()
}
// Keep model identities and untouched rows stable, including focused editors.
fn update_model<T: Clone + PartialEq + 'static>(
    existing: ModelRc<T>,
    values: Vec<T>,
) -> ModelRc<T> {
    if let Some(rows) = existing.as_any().downcast_ref::<VecModel<T>>()
        && rows.row_count() == values.len()
    {
        for (index, value) in values.into_iter().enumerate() {
            if rows.row_data(index).as_ref() != Some(&value) {
                rows.set_row_data(index, value);
            }
        }
        existing
    } else {
        model(values)
    }
}
fn tint(index: i32) -> Color {
    match index.rem_euclid(3) {
        0 => Color::from_rgb_u8(90, 126, 163),
        1 => Color::from_rgb_u8(89, 102, 164),
        _ => Color::from_rgb_u8(119, 132, 178),
    }
}
fn game_item(g: &Game) -> GameItem {
    let detail = if g.game_directory.as_os_str().is_empty() {
        format!("{} 项内容", g.content.len())
    } else {
        format!("修订 {} · {}", g.revision, g.game_directory.display())
    };
    GameItem {
        id: g.id,
        name: g.name.clone().into(),
        version: g.version.clone().into(),
        loader: g.loader.clone().into(),
        selector_label: format!("{} · {}", g.version, g.loader).into(),
        detail: detail.into(),
        tint: tint(g.id),
    }
}

fn installed_instance_items(games: &[Game]) -> Vec<GameItem> {
    games
        .iter()
        .map(|game| {
            let mut item = game_item(game);
            if games.iter().any(|other| {
                other.id != game.id && other.version == game.version && other.loader == game.loader
            }) {
                // IDs distinguish even imported instances with identical names and versions.
                item.selector_label = format!(
                    "{} · {} · {} (#{})",
                    game.version, game.loader, game.name, game.id
                )
                .into();
            }
            item
        })
        .collect()
}

fn filtered_game_items(games: &[Game], page: i32, filter: &str) -> Vec<GameItem> {
    let filter = filter.to_lowercase();
    games
        .iter()
        .filter(|game| {
            page != 1
                || format!("{} {} {}", game.name, game.version, game.loader)
                    .to_lowercase()
                    .contains(&filter)
        })
        .map(game_item)
        .collect()
}

fn open_directory(path: &Path) -> Result<(), String> {
    if path.as_os_str().is_empty() {
        return Err("实例尚未配置游戏目录".into());
    }
    let metadata = std::fs::metadata(path)
        .map_err(|error| format!("无法访问目录 {}：{error}", path.display()))?;
    if !metadata.is_dir() {
        return Err(format!("{} 不是文件夹", path.display()));
    }
    // Pass the actual instance path as one argument, without a command shell.
    #[cfg(target_os = "windows")]
    let mut command = Command::new("explorer.exe");
    #[cfg(target_os = "macos")]
    let mut command = Command::new("open");
    #[cfg(not(any(target_os = "windows", target_os = "macos")))]
    let mut command = Command::new("xdg-open");
    let status = command
        .arg(path)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .map_err(|error| format!("无法调用系统文件管理器：{error}"))?;
    if !status.success() {
        return Err(format!("系统文件管理器未能打开目录（{status}）"));
    }
    Ok(())
}

fn sync_guide_directory(ui: &AppWindow, game: &Game) {
    if game.game_directory.as_os_str().is_empty() {
        return;
    }
    ui.global::<Guidance>()
        .set_game_directory(game.game_directory.display().to_string().into());
}

fn open_instance_directory(ui: &AppWindow, s: &mut State, id: i32) {
    let Some(game) = s.games.iter().find(|game| game.id == id) else {
        toast(ui, s, "实例已不存在，请重新选择");
        return;
    };
    let path = game.game_directory.clone();
    let name = game.name.clone();
    let weak = ui.as_weak();
    if let Err(error) = std::thread::Builder::new()
        .name("open-game-folder".into())
        .spawn(move || {
            if let Err(reason) = open_directory(&path) {
                let message = format!("无法打开「{name}」的游戏文件夹：{reason}");
                let _ = weak.upgrade_in_event_loop(move |ui| {
                    crate::settings_bridge::toast_detached(&ui.as_weak(), message);
                });
            }
        })
    {
        toast(ui, s, format!("无法开始打开游戏文件夹：{error}"));
    }
}

fn inspect_instance(ui: &AppWindow, s: &mut State, id: i32, section: i32) {
    if s.inspect(id) {
        let app = ui.global::<AppState>();
        app.set_instance_detail_open(true);
        app.set_instance_section(section);
        app.set_game_tab(if section == 0 { 0 } else { section - 1 });
        app.set_page(1);
        app.set_search_open(false);
        ui.invoke_focus_main();
    }
}

fn instance_update(record: InstanceRecord) -> InstanceUpdate {
    InstanceUpdate {
        id: record.id,
        name: record.name.into(),
        version: record.version.into(),
        loader: record.loader.into(),
        game_directory: record.game_directory.display().to_string().into(),
        revision: record.revision.to_string().into(),
    }
}

fn instance_record(update: InstanceUpdate) -> Result<InstanceRecord, std::num::ParseIntError> {
    let revision = update.revision.parse::<u64>()?;
    Ok(InstanceRecord {
        id: update.id,
        name: update.name.to_string(),
        version: update.version.to_string(),
        loader: update.loader.to_string(),
        game_directory: PathBuf::from(update.game_directory.as_str()),
        revision,
    })
}

fn java_candidate_item(candidate: &JavaCandidate) -> JavaCandidateItem {
    JavaCandidateItem {
        path: candidate.path.display().to_string().into(),
        title: format!(
            "Java {} · {}",
            candidate.version.as_deref().unwrap_or("版本未知"),
            candidate.vendor.as_deref().unwrap_or("厂商未知")
        )
        .into(),
        detail: format!(
            "{} · {}",
            candidate
                .major
                .map(|major| format!("主版本 {major}"))
                .unwrap_or_else(|| "主版本未知".into()),
            candidate.architecture.as_deref().unwrap_or("架构未知")
        )
        .into(),
    }
}

fn java_candidate_items(scan: &JavaScanResult) -> Vec<JavaCandidateItem> {
    scan.candidates.iter().map(java_candidate_item).collect()
}

fn java_notes(scan: &JavaScanResult) -> String {
    scan.failures
        .iter()
        .map(|failure| format!("{}：{}", failure.path.display(), failure.reason))
        .collect::<Vec<_>>()
        .join("\n")
}

fn persist_java(ui: &AppWindow, settings: &SettingsBridge, path: Option<PathBuf>) {
    let app = ui.global::<AppState>();
    let path_text = path
        .as_ref()
        .map(|path| path.display().to_string())
        .unwrap_or_default();
    app.set_java_detecting(true);
    app.set_java_status("正在保存 Java 选择…".into());
    let weak = ui.as_weak();
    settings.save_async(
        LauncherSettings {
            game_root: PathBuf::from(app.get_folder().trim()),
            java_path: path,
        },
        move |result| {
            let Some(ui) = weak.upgrade() else { return };
            let (success, message) = match result {
                Ok(()) => (true, "Java 路径已保存".to_string()),
                Err(error) => (false, format!("Java 路径保存失败：{error}")),
            };
            ui.global::<AppState>()
                .invoke_java_saved(path_text.into(), success, message.into());
        },
    );
}
fn toast(ui: &AppWindow, _s: &mut State, message: impl Into<SharedString>) {
    let message: SharedString = message.into();
    crate::settings_bridge::toast_detached(&ui.as_weak(), message.to_string());
}
fn category_for_tab(tab: i32) -> i32 {
    match tab {
        0 => 0,
        1 => 2,
        2 => 3,
        3 => 4,
        4 => 1,
        _ => 0,
    }
}
fn refresh_downloads(ui: &AppWindow, s: &State) {
    let app = ui.global::<AppState>();
    let rows: Vec<DownloadItem> = s
        .downloads
        .iter()
        .map(|t| DownloadItem {
            id: t.id,
            name: t.name.clone().into(),
            detail: format!("安装到 {} · 模拟任务", t.target_name).into(),
            progress: t.progress,
            state: (if t.done {
                "已完成"
            } else if t.paused {
                "已暂停"
            } else {
                "下载中"
            })
            .into(),
            paused: t.paused,
            done: t.done,
        })
        .collect();
    let existing = app.get_downloads();
    if let Some(existing) = existing.as_any().downcast_ref::<VecModel<DownloadItem>>()
        && existing.row_count() == rows.len()
    {
        for (index, row) in rows.into_iter().enumerate() {
            if existing.row_data(index).as_ref() != Some(&row) {
                existing.set_row_data(index, row);
            }
        }
    } else {
        app.set_downloads(model(rows));
    }
    app.set_active_downloads(s.downloads.iter().filter(|t| !t.done).count() as i32);
}
pub(crate) fn refresh(ui: &AppWindow, s: &State) {
    let app = ui.global::<AppState>();
    app.set_games(update_model(
        app.get_games(),
        filtered_game_items(&s.games, app.get_page(), app.get_game_filter().as_str()),
    ));
    app.set_installed_instances(update_model(
        app.get_installed_instances(),
        installed_instance_items(&s.games),
    ));
    app.set_game_names(update_model(
        app.get_game_names(),
        s.games
            .iter()
            .map(|g| SharedString::from(g.name.as_str()))
            .collect(),
    ));
    app.set_current_game_index(
        s.games
            .iter()
            .position(|g| g.id == s.selected)
            .map_or(-1, |i| i as i32),
    );
    app.set_current_game(s.game().map(game_item).unwrap_or_else(|| GameItem {
        id: -1,
        name: "尚无实例".into(),
        version: "—".into(),
        loader: "请先创建实例".into(),
        selector_label: "尚无实例".into(),
        detail: "".into(),
        tint: tint(0),
    }));
    app.set_inspected_game(
        s.inspected_game()
            .map(game_item)
            .unwrap_or_else(|| GameItem {
                id: -1,
                ..Default::default()
            }),
    );
    app.set_inspected_directory(
        s.inspected_game()
            .map_or_else(String::new, |game| {
                game.game_directory.display().to_string()
            })
            .into(),
    );
    if let Some(game) = s.game() {
        sync_guide_directory(ui, game);
    }
    app.set_mods(update_model(
        app.get_mods(),
        s.inspected_game()
            .map(|g| {
                g.content
                    .iter()
                    .filter(|c| c.category == category_for_tab(app.get_game_tab()))
                    .map(|c| ModItem {
                        id: c.id,
                        name: c.name.clone().into(),
                        detail: (if c.enabled { "已启用" } else { "已停用" }).into(),
                        source: c.source.clone().into(),
                        enabled: c.enabled,
                        updatable: c.source != "本地导入",
                    })
                    .collect()
            })
            .unwrap_or_default(),
    ));
    let query = app.get_resource_query().to_lowercase();
    app.set_resources(update_model(
        app.get_resources(),
        s.catalog
            .iter()
            .filter(|r| r.category == app.get_resource_category())
            .filter(|r| {
                format!("{} {}", r.name, r.alias)
                    .to_lowercase()
                    .contains(&query)
            })
            .filter(|r| !app.get_compatible_only() || s.game().is_some_and(|g| r.compatible(g)))
            .map(|r| ResourceItem {
                id: r.id,
                name: r.name.into(),
                description: r.description.into(),
                detail: format!("{} / {} · 演示版本配置", r.version, r.loader).into(),
                source: r.source.into(),
                tint: tint(r.id),
                compatible: s.game().is_some_and(|g| r.compatible(g)),
                installed: s
                    .game()
                    .is_some_and(|g| g.content.iter().any(|c| c.id == r.id)),
            })
            .collect(),
    ));
    app.set_accounts(update_model(
        app.get_accounts(),
        s.accounts
            .iter()
            .map(|(id, name)| AccountItem {
                id: *id,
                name: name.clone().into(),
                kind: "本地预览身份".into(),
                active: *id == s.account,
            })
            .collect(),
    ));
    app.set_account_name(
        s.accounts
            .iter()
            .find(|(id, _)| *id == s.account)
            .map_or("本地玩家", |(_, n)| n)
            .into(),
    );
    refresh_downloads(ui, s);
}
fn open_modal(ui: &AppWindow, s: &mut State, kind: &str, id: i32) {
    if kind == "join" {
        let weak = ui.as_weak();
        Timer::single_shot(Duration::ZERO, move || {
            if let Some(ui) = weak.upgrade() {
                ui.global::<Guidance>().invoke_start_check(true);
            }
        });
        return;
    }
    let app = ui.global::<AppState>();
    let game = if matches!(kind, "delete" | "rename") {
        s.games.iter().find(|g| g.id == id)
    } else if kind == "import" {
        s.inspected_game()
    } else {
        s.game()
    }
    .map_or_else(|| "未选择游戏".to_string(), |g| g.name.clone());
    let (title,body)=match kind {
        "create"=>("创建实例","创建独立实例配置，游戏文件安装尚未接入。".into()),
        "rename"=>("重命名实例",format!("修改「{game}」的显示名称，不移动实例目录。")),
        "delete"=>("删除游戏",format!("将删除「{game}」的演示条目、资源条目和关联任务。\n\n输入完整游戏名称确认对象。当前不会删除磁盘文件；正式删除应另行展示存档保留与备份选项。")),
        "import"=>("导入资源 · 交互预览",format!("向「{game}」添加一个本地导入的演示条目。真实文件选择与导入尚未接入。")),
        "account"=>("添加本地账号","输入用于界面预览的角色名称。在线账号登录会在接入认证服务后提供。".into()),
        "install"=>{
            let Some(r)=s.catalog.iter().find(|r|r.id==id) else {return};
            ("确认安装资源",format!("{}\n\n目标游戏：{game}\n版本：{} / {}\n来源：{}\n\n确认后加入模拟下载队列，完成后出现在目标游戏中。",r.name,r.version,r.loader,r.source))
        },
        "notice"=>("社团公告","欢迎加入社团世界。\n\n此处将展示版本变更、维护通知和社区活动。当前没有连接公告服务。".into()),
        "room"=>("临时联机 · 演示","房间创建、房间码和连接服务尚未接入。当前不会创建真实房间。".into()),
        "java"=>("Java 环境管理","后续支持发现已有 Java、选择安装目录、下载与切换版本。\n\n当前展示 Java 21 示例配置，没有扫描或修改本机环境。".into()),
        "skin"=>("孪枭的皮肤与披风","当前使用美术部提供的孪枭原始 PNG，支持双层立体预览、鼠标跟随和手动旋转。\n\n基础层与帽层、衣服和四肢外层按原始透明度叠加。皮肤站绑定、应用到账号和披风切换将在后续接入。".into()),
        _=>return,
    };
    s.pending = Some((
        kind.into(),
        id,
        if kind == "import" {
            s.inspected
        } else {
            s.selected
        },
        category_for_tab(app.get_game_tab()),
    ));
    app.set_search_open(false);
    app.set_modal(kind.into());
    app.set_modal_title(title.into());
    app.set_modal_body(body.into());
    app.set_form_name(if kind == "rename" {
        game.into()
    } else {
        "".into()
    });
    app.set_form_error("".into());
    app.set_form_version(0);
    app.set_form_loader(0);
    app.set_instance_saving(false);
    ui.invoke_focus_modal();
}
fn close_modal(ui: &AppWindow, s: &mut State) {
    if ui.global::<AppState>().get_instance_saving()
        && ui.global::<AppState>().get_modal() != "rename"
    {
        return;
    }
    s.pending = None;
    ui.global::<AppState>().set_instance_saving(false);
    ui.global::<AppState>().set_modal("".into());
    ui.invoke_focus_main();
}
fn launch(ui: &AppWindow, s: &mut State) {
    if s.game().is_none() {
        toast(ui, s, "请先创建或选择实例");
        return;
    }
    let weak = ui.as_weak();
    Timer::single_shot(Duration::ZERO, move || {
        if let Some(ui) = weak.upgrade() {
            ui.global::<Guidance>().invoke_start_check(false);
        }
    });
}
fn confirm(ui: &AppWindow, s: &mut State, instances: &InstanceBridge) {
    let Some((kind, id, target, category)) = s.pending.clone() else {
        return;
    };
    let app = ui.global::<AppState>();
    let name = app.get_form_name().trim().to_owned();
    if ["create", "rename", "import", "account"].contains(&kind.as_str())
        && (name.is_empty() || name.chars().count() > 32)
    {
        app.set_form_error("请输入 1–32 个字符的名称。".into());
        return;
    }
    match kind.as_str() {
        "create" => {
            if s.games.iter().any(|g| g.name.eq_ignore_ascii_case(&name)) {
                app.set_form_error("已有同名游戏，请换一个名称。".into());
                return;
            }
            if let Err(reason) = lxmcl::contracts::validate_directory(app.get_folder().as_str()) {
                app.set_form_error(format!("游戏存放目录：{reason}").into());
                return;
            }
            let version = if app.get_form_version() == 0 {
                "1.21.1"
            } else {
                "1.20.1"
            };
            let loader = match app.get_form_loader() {
                0 => "Fabric",
                1 => "原版",
                _ => "Forge",
            };
            let draft = NewInstance {
                name,
                version: version.into(),
                loader: loader.into(),
                game_root: PathBuf::from(app.get_folder().trim()),
            };
            app.set_form_error("".into());
            app.set_instance_saving(true);
            let weak = ui.as_weak();
            instances.create_async(draft, move |result| {
                let Some(ui) = weak.upgrade() else { return };
                match result {
                    Ok(record) => ui
                        .global::<AppState>()
                        .invoke_instance_created(instance_update(record)),
                    Err(error) => ui
                        .global::<AppState>()
                        .invoke_instance_error(format!("创建失败：{error}").into()),
                }
            });
            return;
        }
        "delete" => {
            if s.games
                .iter()
                .find(|g| g.id == id)
                .is_none_or(|g| g.name != name)
            {
                app.set_form_error("名称不一致，请输入将删除的游戏完整名称。".into());
                return;
            }
            s.remove_game(id);
            toast(ui, s, "已删除演示游戏条目");
        }
        "import" => {
            let id = s.id();
            if let Some(g) = s.games.iter_mut().find(|g| g.id == target) {
                g.content.push(Content {
                    id,
                    name,
                    category,
                    source: "本地导入".into(),
                    enabled: true,
                });
                toast(ui, s, "已添加本地资源演示条目");
            }
        }
        "account" => {
            if s.accounts
                .iter()
                .any(|(_, n)| n.eq_ignore_ascii_case(&name))
            {
                app.set_form_error("已有同名账号，请换一个名称。".into());
                return;
            }
            let id = s.id();
            s.accounts.push((id, name));
            s.account = id;
            toast(ui, s, "已添加并切换到本地预览账号");
        }
        "install" => match s.queue(id, target) {
            Ok(()) => {
                app.set_page(4);
                toast(ui, s, "已加入模拟下载队列");
            }
            Err(error) => {
                app.set_form_error(error.into());
                return;
            }
        },
        _ => {}
    }
    close_modal(ui, s);
}
fn search(ui: &AppWindow, s: &State, query: &str) {
    let app = ui.global::<AppState>();
    app.set_search_query(query.into());
    app.set_search_index(0);
    let query = query.trim().to_lowercase();
    let mut results: Vec<SearchItem> = s
        .games
        .iter()
        .filter(|g| {
            format!("{} {}", g.name, g.version)
                .to_lowercase()
                .contains(&query)
        })
        .map(|g| SearchItem {
            id: g.id,
            name: g.name.clone().into(),
            detail: format!("{} / {}", g.version, g.loader).into(),
            category: "实例".into(),
        })
        .collect();
    for (id, name, detail) in [
        (-2, "联机", "服务器与联机房间"),
        (-3, "资源中心", "模组、整合包、资源包、光影、世界"),
        (-4, "下载任务", "暂停、继续与取消"),
        (-5, "账号与装扮", "切换账号与皮肤管理"),
        (-6, "设置", "Java、内存、目录、主题、代理"),
        (-7, "诊断与工具", "日志、网络与环境检查"),
        (-8, "帮助中心", "按任务查看使用说明"),
    ] {
        if format!("{name} {detail}").to_lowercase().contains(&query) {
            results.push(SearchItem {
                id,
                name: name.into(),
                detail: detail.into(),
                category: "功能".into(),
            });
        }
    }
    for (id, name, detail, category) in [(
        -101,
        "首次游玩向导",
        "新手 教程 入门 · 玩法、身份、目录与检查",
        "引导",
    )] {
        if format!("{name} {detail}").to_lowercase().contains(&query) {
            results.push(SearchItem {
                id,
                name: name.into(),
                detail: detail.into(),
                category: category.into(),
            });
        }
    }
    for index in lxmcl::help::search(&query) {
        let article = &lxmcl::help::ARTICLES[index];
        if article.id == "club" {
            continue;
        }
        results.push(SearchItem {
            id: -2000 - index as i32,
            name: article.title.into(),
            detail: article.summary.into(),
            category: "教程".into(),
        });
    }
    app.set_search_results(model(results));
}

pub fn connect(
    ui: &AppWindow,
    settings: SettingsBridge,
    initial: LauncherSettings,
    instances: InstanceBridge,
    initial_instances: Vec<InstanceRecord>,
    java: JavaBridge,
    application: std::sync::Arc<dyn launcher_api::ApplicationService>,
) -> Rc<Timer> {
    let state = Rc::new(RefCell::new({
        let mut state = State::default();
        state.replace_instances(initial_instances);
        state.java_path = initial.java_path.clone();
        state
    }));
    let rename =
        crate::presentation::rename::RenamePresentation::connect(ui, state.clone(), application);
    let p = Preferences::load();
    let app = ui.global::<AppState>();
    ui.global::<Theme>().set_dark(p.dark);
    ui.global::<Theme>().set_reduce_motion(p.reduce_motion);
    ui.global::<Theme>()
        .set_darkness(if p.dark { 1.0 } else { 0.0 });
    ui.global::<Theme>().set_pixel_glass(false);
    ui.invoke_sync_theme();
    // 游戏目录以业务设置（设置插件）为准；界面偏好里的 folder 仅作为旧字段继续写入。
    app.set_folder(initial.game_root.display().to_string().into());
    let initial_java = initial
        .java_path
        .as_ref()
        .map(|path| path.display().to_string())
        .unwrap_or_default();
    app.set_java_selected(initial_java.clone().into());
    app.set_java_input(initial_java.into());
    app.set_java_status("尚未检测本机 Java".into());
    app.set_memory(p.memory);
    app.set_auto_memory(p.auto_memory);
    app.set_fullscreen(p.fullscreen);
    app.set_proxy_mode(p.proxy);
    app.set_source_mode(p.source);
    app.set_wallpaper_index(p.wallpaper_index);
    app.set_wallpaper_enabled(p.wallpaper_enabled);
    app.set_wallpaper_paused(p.wallpaper_paused);
    crate::journey::connect(ui, state.clone());
    refresh(ui, &state.borrow());
    let demo_timer = Rc::new(Timer::default());
    macro_rules! bind {
        ($callback:ident, |$ui:ident,$s:ident $(,$arg:ident)*| $body:block)=>{{
            let weak=ui.as_weak();let data=state.clone();let ticker=demo_timer.clone();
            app.$callback(move |$($arg),*|{if let Some($ui)=weak.upgrade(){let $s=&mut *data.borrow_mut();$body refresh(&$ui,$s); wake_demo_timer($ui.as_weak(),data.clone(),ticker.clone());}});
        }};
    }
    macro_rules! bind_view {
        ($callback:ident, |$ui:ident,$s:ident $(,$arg:ident)*| $body:block)=>{{
            let weak=ui.as_weak();let data=state.clone();let ticker=demo_timer.clone();
            app.$callback(move |$($arg),*|{if let Some($ui)=weak.upgrade(){let $s=&mut *data.borrow_mut();$body wake_demo_timer($ui.as_weak(),data.clone(),ticker.clone());}});
        }};
    }
    let rename_navigation = rename.clone();
    bind!(on_navigate, |ui, s, page| {
        let app = ui.global::<AppState>();
        rename_navigation.dismiss();
        if app.get_modal() == "rename" {
            app.set_instance_saving(false);
        }
        if page == 8 {
            ui.global::<Guidance>().set_article(-1);
        }
        if (page == 7 || page == 8) && app.get_page() != 7 && app.get_page() != 8 {
            ui.global::<Guidance>().set_help_return_page(app.get_page());
        }
        if app.get_page() == 1 && page == 3 {
            app.set_resource_category(category_for_tab(app.get_game_tab()));
        }
        app.set_page(if page == 6 && app.get_settings_category() == 5 {
            5
        } else {
            page.clamp(0, 8)
        });
        s.pending = None;
        app.set_modal("".into());
    });
    bind!(on_choose_game, |ui, s, id| {
        if s.games.iter().any(|game| game.id == id) {
            s.select(id);
        } else {
            toast(&ui, s, "实例已不存在，请重新选择");
        }
    });
    bind!(on_open_game_settings, |ui, s| {
        if s.game().is_some() {
            s.inspect(s.selected);
            let app = ui.global::<AppState>();
            app.set_instance_detail_open(true);
            app.set_instance_section(5);
            app.set_game_tab(5);
            app.set_page(1);
            app.set_search_open(false);
            ui.invoke_focus_main();
        } else {
            toast(&ui, s, "请先创建或选择实例");
        }
    });
    bind!(
        on_extension_open_instance_settings,
        |ui, s, id, revision| {
            let valid = s
                .games
                .iter()
                .any(|game| game.id == id && game.revision.to_string() == revision.as_str());
            let app = ui.global::<AppState>();
            if valid {
                s.inspect(id);
                app.set_instance_detail_open(true);
                app.set_instance_section(5);
                app.set_game_tab(5);
                app.set_page(1);
                app.set_search_open(false);
            } else {
                app.set_extension_status("实例目标快照已过期，请重新选择实例后重试。".into());
            }
        }
    );
    bind_view!(on_open_game_folder, |ui, s| {
        open_instance_directory(&ui, s, s.selected);
    });
    bind_view!(on_open_instance_folder, |ui, s, id| {
        open_instance_directory(&ui, s, id);
    });
    bind!(on_open_instance, |ui, s, id| {
        inspect_instance(&ui, s, id, 0);
    });
    bind!(on_back_to_instances, |ui, s| {
        let app = ui.global::<AppState>();
        app.set_instance_detail_open(false);
        app.set_page(1);
        ui.invoke_focus_main();
        let _ = s;
    });
    bind!(on_launch_instance, |ui, s, id| {
        if s.games.iter().any(|game| game.id == id) {
            s.select(id);
            launch(&ui, s);
        }
    });
    bind!(on_choose_game_at, |ui, s, index| {
        if let Some(g) = s.games.get(index as usize) {
            let id = g.id;
            s.select(id);
        }
    });
    bind!(on_filter_games, |ui, s, query| {
        ui.global::<AppState>().set_game_filter(query);
        let _ = s;
    });
    bind!(on_switch_game_tab, |ui, s, tab| {
        ui.global::<AppState>().set_game_tab(tab);
        let _ = s;
    });
    {
        let weak = ui.as_weak();
        let data = state.clone();
        let rename = rename.clone();
        app.on_open_modal(move |kind, id| {
            let Some(ui) = weak.upgrade() else { return };
            rename.dismiss();
            let mut state = data.borrow_mut();
            if kind == "rename" {
                let Some(game) = state.games.iter().find(|game| game.id == id) else {
                    return;
                };
                rename.open(game.id, game.revision, game.name.clone());
            }
            open_modal(&ui, &mut state, kind.as_str(), id);
        });
    }
    {
        let weak = ui.as_weak();
        let data = state.clone();
        let instances = instances.clone();
        let ticker = demo_timer.clone();
        let rename = rename.clone();
        app.on_confirm_modal(move || {
            let Some(ui) = weak.upgrade() else { return };
            if ui.global::<AppState>().get_modal() == "rename" {
                rename.submit(&ui);
                return;
            }
            let mut s = data.borrow_mut();
            confirm(&ui, &mut s, &instances);
            refresh(&ui, &s);
            wake_demo_timer(ui.as_weak(), data.clone(), ticker.clone());
        });
    }
    {
        let weak = ui.as_weak();
        let data = state.clone();
        app.on_instance_created(move |update| {
            let Some(ui) = weak.upgrade() else { return };
            let app = ui.global::<AppState>();
            app.set_instance_saving(false);
            let mut s = data.borrow_mut();
            let record = match instance_record(update) {
                Ok(record) => record,
                Err(_) => {
                    app.set_form_error("实例修订无效，未更新当前记录。".into());
                    return;
                }
            };
            s.apply_instance(record);
            s.inspected = s.selected;
            s.pending = None;

            app.set_page(1);
            app.set_game_tab(0);
            app.set_instance_detail_open(true);
            app.set_instance_section(0);
            refresh(&ui, &s);
            toast(&ui, &mut s, "实例配置已保存");
            drop(s);
            app.set_modal("".into());
            ui.invoke_focus_main();
        });
    }
    {
        let weak = ui.as_weak();
        app.on_instance_error(move |message| {
            if let Some(ui) = weak.upgrade() {
                let app = ui.global::<AppState>();
                app.set_instance_saving(false);
                app.set_form_error(message);
            }
        });
    }

    {
        let weak = ui.as_weak();
        let data = state.clone();
        let java = java.clone();
        app.on_detect_java(move || {
            let Some(ui) = weak.upgrade() else { return };
            let app = ui.global::<AppState>();
            if app.get_java_detecting() {
                return;
            }
            let saved = data.borrow().java_path.clone();
            app.set_java_detecting(true);
            app.set_java_status("正在检测本机 Java…".into());
            app.set_java_notes("".into());
            let weak = ui.as_weak();
            java.detect_async(saved, move |result| {
                let Some(ui) = weak.upgrade() else { return };
                let app = ui.global::<AppState>();
                app.set_java_detecting(false);
                match result {
                    Ok(scan) => {
                        let count = scan.candidates.len();
                        let failed = scan.failures.len();
                        app.set_java_candidates(model(java_candidate_items(&scan)));
                        app.set_java_notes(java_notes(&scan).into());
                        app.set_java_status(
                            if count == 0 {
                                "未发现可用的 Java；可以手动填写 java.exe 路径。".to_string()
                            } else {
                                format!("检测完成：{count} 个可用，{failed} 个失败")
                            }
                            .into(),
                        );
                    }
                    Err(error) => {
                        app.set_java_candidates(model(Vec::<JavaCandidateItem>::new()));
                        app.set_java_status(format!("检测失败：{error}").into());
                    }
                }
            });
        });
    }
    {
        let weak = ui.as_weak();
        let settings = settings.clone();
        app.on_use_java(move |path| {
            let Some(ui) = weak.upgrade() else { return };
            persist_java(&ui, &settings, Some(PathBuf::from(path.as_str())));
        });
    }
    {
        let weak = ui.as_weak();
        let java = java.clone();
        app.on_verify_java(move || {
            let Some(ui) = weak.upgrade() else { return };
            let app = ui.global::<AppState>();
            if app.get_java_detecting() {
                return;
            }
            let input = app.get_java_input().trim().to_string();
            if input.is_empty() {
                app.set_java_status("请填写 java.exe 路径".into());
                return;
            }
            app.set_java_detecting(true);
            app.set_java_status("正在验证 Java…".into());
            app.set_java_notes("".into());
            let weak = ui.as_weak();
            java.probe_async(PathBuf::from(&input), move |result| {
                let Some(ui) = weak.upgrade() else { return };
                let app = ui.global::<AppState>();
                match result {
                    Ok(candidate) => {
                        let item = java_candidate_item(&candidate);
                        app.set_java_status(format!("验证通过：{}", item.title).into());
                        app.invoke_java_verified(item.path.clone(), item.title.clone());
                    }
                    Err(error) => {
                        app.set_java_detecting(false);
                        app.set_java_status(format!("验证失败：{error}").into());
                    }
                }
            });
        });
    }
    {
        let weak = ui.as_weak();
        let settings = settings.clone();
        app.on_auto_java(move || {
            let Some(ui) = weak.upgrade() else { return };
            if ui.global::<AppState>().get_java_detecting() {
                return;
            }
            persist_java(&ui, &settings, None);
        });
    }
    {
        let weak = ui.as_weak();
        let settings = settings.clone();
        app.on_java_verified(move |path, label| {
            let Some(ui) = weak.upgrade() else { return };
            ui.global::<AppState>()
                .set_java_status(format!("{label} 验证通过，正在保存…").into());
            persist_java(&ui, &settings, Some(PathBuf::from(path.as_str())));
        });
    }
    {
        let weak = ui.as_weak();
        let data = state.clone();
        app.on_java_saved(move |path, success, message| {
            let Some(ui) = weak.upgrade() else { return };
            let app = ui.global::<AppState>();
            app.set_java_detecting(false);
            app.set_java_status(message.clone());
            let mut s = data.borrow_mut();
            if success {
                s.java_path = if path.is_empty() {
                    None
                } else {
                    Some(PathBuf::from(path.as_str()))
                };
                app.set_java_selected(path.clone());
                app.set_java_input(path);
            }
            toast(&ui, &mut s, message);
        });
    }
    {
        let weak = ui.as_weak();
        let data = state.clone();
        let rename = rename.clone();
        app.on_close_modal(move || {
            let Some(ui) = weak.upgrade() else { return };
            if ui.global::<AppState>().get_modal() == "rename" {
                rename.dismiss();
            }
            close_modal(&ui, &mut data.borrow_mut());
        });
    }
    bind!(on_toggle_mod, |ui, s, id| {
        if let Some(c) = s
            .games
            .iter_mut()
            .find(|g| g.id == s.inspected)
            .and_then(|g| g.content.iter_mut().find(|c| c.id == id))
        {
            c.enabled = !c.enabled;
        }
        let _ = &ui;
    });
    bind_view!(on_update_mod, |ui, s, id| {
        let _ = id;
        toast(&ui, s, "演示资源已是当前示例版本，尚未连接更新服务");
    });
    bind!(on_filter_resources, |ui, s, query, category, compatible| {
        let app = ui.global::<AppState>();
        app.set_resource_query(query);
        app.set_resource_category(category);
        app.set_compatible_only(compatible);
        let _ = s;
    });
    bind!(on_install_resource, |ui, s, id| {
        open_modal(&ui, s, "install", id);
    });
    bind!(on_toggle_download, |ui, s, id| {
        if let Some(t) = s.downloads.iter_mut().find(|t| t.id == id && !t.done) {
            t.paused = !t.paused;
        }
        let _ = &ui;
    });
    bind!(on_view_download, |ui, s, id| {
        if let Some(task) = s.downloads.iter().find(|t| t.id == id) {
            let target = task.target;
            let category = task.content.as_ref().map_or(0, |c| c.category);
            s.inspect(target);
            let app = ui.global::<AppState>();
            app.set_game_tab(match category {
                1 => 4,
                2 => 1,
                3 => 2,
                4 => 3,
                _ => 0,
            });
            app.set_instance_detail_open(true);
            app.set_instance_section(if category == 1 {
                1
            } else {
                app.get_game_tab() + 1
            });

            app.set_page(1);
        }
    });
    bind!(on_cancel_download, |ui, s, id| {
        s.downloads.retain(|t| t.id != id);
        toast(&ui, s, "已取消任务，资源不会安装");
    });
    bind!(on_clear_downloads, |ui, s| {
        s.downloads.retain(|t| !t.done);
        toast(&ui, s, "已清除完成的任务");
    });
    bind_view!(on_launch_game, |ui, s| {
        launch(&ui, s);
    });
    bind!(on_select_account, |ui, s, id| {
        if s.accounts.iter().any(|(account, _)| *account == id) {
            s.account = id;
            toast(&ui, s, "已切换游戏账号");
        }
    });
    bind_view!(on_set_dark, |ui, s, dark| {
        ui.global::<Theme>().set_dark(dark);
        ui.invoke_sync_theme();
        toast(&ui, s, "外观已切换；点击保存设置可保留偏好");
    });
    bind_view!(on_set_pixel_glass, |ui, s, enabled| {
        ui.global::<Theme>().set_pixel_glass(enabled);
        toast(&ui, s, "主题已切换；在设置中保存可保留下次启动偏好");
    });
    // 保存设置：界面偏好写本机；游戏设置经桥发往设置插件，两条结果合并为一条提示。
    {
        let weak = ui.as_weak();
        let data = state.clone();
        let bridge = settings.clone();
        app.on_save_settings(move || {
            let Some(ui) = weak.upgrade() else { return };
            let s = &mut *data.borrow_mut();
            let app = ui.global::<AppState>();
            if app.get_instance_saving() || app.get_java_detecting() {
                toast(&ui, s, "请等待当前操作完成");
                return;
            }
            let p = Preferences {
                dark: ui.global::<Theme>().get_dark(),
                reduce_motion: ui.global::<Theme>().get_reduce_motion(),
                pixel_glass: ui.global::<Theme>().get_pixel_glass(),
                wallpaper_index: app.get_wallpaper_index(),
                wallpaper_enabled: app.get_wallpaper_enabled(),
                wallpaper_paused: app.get_wallpaper_paused(),
                folder: app.get_folder().trim().into(),
                memory: app.get_memory(),
                auto_memory: app.get_auto_memory(),
                fullscreen: app.get_fullscreen(),
                proxy: app.get_proxy_mode(),
                source: app.get_source_mode(),
            };
            if p.folder.is_empty() {
                toast(&ui, s, "请填写游戏存放目录");
                return;
            }
            // 绑定点击时的快照；结果经事件循环回到界面线程后合并展示。
            let weak = ui.as_weak();
            let java_path = s.java_path.clone();
            bridge.save_with_preferences_async(
                LauncherSettings {
                    game_root: p.folder.clone().into(),
                    java_path,
                },
                p,
                move |result, preferences_result| {
                    let prefs_note = match preferences_result {
                        Ok(()) => "界面偏好已保存到本机".to_string(),
                        Err(error) => format!("界面偏好保存失败：{error}"),
                    };
                    let settings_note = match result {
                        Ok(()) => "游戏设置已保存".to_string(),
                        Err(e) => format!("游戏设置保存失败：{e}"),
                    };
                    crate::settings_bridge::toast_detached(
                        &weak,
                        format!("{prefs_note}；{settings_note}"),
                    );
                },
            );
        });
    }
    {
        let weak = ui.as_weak();
        let data = state.clone();
        app.on_search(move |query| {
            if let Some(ui) = weak.upgrade() {
                search(&ui, &data.borrow(), query.as_str());
                let weak = ui.as_weak();
                Timer::single_shot(Duration::from_millis(0), move || {
                    if let Some(ui) = weak.upgrade() {
                        ui.invoke_focus_search_input();
                    }
                });
            }
        });
    }
    bind!(on_activate_search, |ui, s, id| {
        let app = ui.global::<AppState>();
        app.set_search_open(false);
        if id <= -2000 {
            ui.global::<Guidance>().invoke_open_article_at(-2000 - id);
        } else if id == -101 {
            ui.global::<Guidance>().invoke_open_guide();
        } else if id == -100 {
            let weak = ui.as_weak();
            Timer::single_shot(Duration::ZERO, move || {
                if let Some(ui) = weak.upgrade() {
                    ui.global::<Guidance>().invoke_start_check(true);
                }
            });
        } else if id < 0 {
            app.set_page(-id);
            if id == -8 {
                ui.global::<Guidance>().set_article(-1);
            }
        } else {
            s.inspect(id);
            app.set_page(1);
            app.set_instance_detail_open(true);
            app.set_instance_section(0);
        }
        ui.invoke_focus_main();
    });
    bind_view!(on_diagnose, |ui, s, kind| {
        let app = ui.global::<AppState>();
        if kind == 2 {
            app.set_diagnostic_text(format!("[前端预览] LXMCL 0.1.0\n[界面] {} 条实例记录，{} 个本地预览账号\n[任务] {} 项模拟下载\n\n这里只显示前端状态摘要。真实游戏日志、JVM 参数和错误分析尚未接入。",s.games.len(),s.accounts.len(),s.downloads.len()).into());
        } else {
            s.diagnostic = Some((kind, 0));
            app.set_diagnosing(true);
            app.set_diagnostic_text("正在演示检查过程…".into());
        }
    });
    wake_demo_timer(ui.as_weak(), state, demo_timer.clone());
    demo_timer
}

fn wake_demo_timer(weak: slint::Weak<AppWindow>, state: Rc<RefCell<State>>, timer: Rc<Timer>) {
    if timer.running() {
        return;
    }
    let weak_timer = Rc::downgrade(&timer);
    timer.start(TimerMode::Repeated,Duration::from_millis(200),move||{
        let Some(ui)=weak.upgrade() else{return};let mut s=state.borrow_mut();let app=ui.global::<AppState>();
        let active=s.downloads.iter().any(|d|!d.done && !d.paused);let completed=s.tick_downloads();
        for(name,join,target)in &completed{toast(&ui,&mut s,format!("{name}：模拟任务已完成"));if *join{s.select(*target);app.set_page(0);launch(&ui,&mut s);}}
        if !completed.is_empty(){refresh(&ui,&s);}else if active{refresh_downloads(&ui,&s);}
        if let Some(progress)=s.launch{let progress=progress+5;app.set_launch_progress(progress);app.set_launch_status((if progress<35{"检查账号与游戏配置（演示）…"}else if progress<70{"检查 Java 与资源（演示）…"}else{"准备启动（演示）…"}).into());if progress>=100{s.launch=None;app.set_launching(false);toast(&ui,&mut s,"启动流程演示完成，尚未接入游戏启动");}else{s.launch=Some(progress);}}
        if let Some((kind,step))=s.diagnostic{if step>=7{s.diagnostic=None;app.set_diagnosing(false);app.set_diagnostic_text((if kind==0{"环境检查 · 演示结果\n\n✓ 已选择游戏与账号\n✓ Java 21 示例配置匹配\n✓ 内存配置有效\n\n未扫描本机 Java 或验证真实游戏文件。"}else{"网络检查 · 演示结果\n\n此处将依次检查下载源、账号认证与服务器连接，并为失败项提供重试。\n\n当前没有发出网络请求，不代表真实网络状态。"}).into());}else{s.diagnostic=Some((kind,step+1));}}
        let busy = s.downloads.iter().any(|d| !d.done && !d.paused)
            || s.launch.is_some() || s.diagnostic.is_some();
        if !busy && let Some(timer) = weak_timer.upgrade() { timer.stop(); }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn instance_revision_roundtrips_without_truncating_u64_and_rejects_invalid_values() {
        let record = InstanceRecord {
            id: 99,
            name: "revision test".into(),
            version: "1.21.1".into(),
            loader: "Fabric".into(),
            game_directory: PathBuf::from("D:/Games/revision-test"),
            revision: u64::MAX,
        };
        let update = instance_update(record.clone());
        assert_eq!(instance_record(update.clone()).unwrap(), record);
        for invalid in ["-1", "18446744073709551616", "invalid", ""] {
            let mut broken = update.clone();
            broken.revision = invalid.into();
            assert!(instance_record(broken).is_err());
        }
    }

    #[test]
    fn home_selector_keeps_all_ids_when_versions_filter_hides_selected_instance() {
        let mut state = State::default();
        let visible = filtered_game_items(&state.games, 1, "原版");
        assert_eq!(
            visible.iter().map(|item| item.id).collect::<Vec<_>>(),
            vec![20]
        );

        let options = installed_instance_items(&state.games);
        assert_eq!(
            options.iter().map(|item| item.id).collect::<Vec<_>>(),
            vec![10, 20, 30]
        );
        state.select(options[2].id);
        assert_eq!(state.game().unwrap().id, 30);
        assert!(visible.iter().all(|item| item.id != state.selected));
        assert_eq!(filtered_game_items(&state.games, 0, "原版").len(), 3);
    }

    #[test]
    fn repeated_versions_and_names_have_distinct_selector_labels_and_targets() {
        let mut state = State::default();
        state.games[1].version = state.games[0].version.clone();
        state.games[1].loader = state.games[0].loader.clone();
        state.games[1].name = state.games[0].name.clone();
        state.games[0].game_directory = PathBuf::from("D:/Games/first");
        state.games[1].game_directory = PathBuf::from("D:/Games/second");
        let options = installed_instance_items(&state.games);
        assert_ne!(options[0].selector_label, options[1].selector_label);
        assert!(options[0].selector_label.contains("#10"));
        assert!(options[1].selector_label.contains("#20"));
        assert_eq!(options[2].selector_label, "1.20.1 · Forge");
        state.select(options[1].id);
        assert_eq!(
            state.game().unwrap().game_directory,
            PathBuf::from("D:/Games/second")
        );
        state.select(9999);
        assert_eq!(state.selected, 20);
    }

    #[test]
    fn folder_action_rejects_empty_missing_and_regular_file_paths() {
        assert!(open_directory(Path::new("")).is_err());
        // This source is a known existing regular file; no external process is started.
        let source = Path::new(env!("CARGO_MANIFEST_DIR")).join("src/controller.rs");
        assert!(open_directory(&source.join("missing-instance")).is_err());
        assert!(open_directory(&source).unwrap_err().contains("不是文件夹"));
    }
}

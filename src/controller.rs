use crate::state::{Content, Game, Preferences, State};
use crate::{
    AccountItem, AppState, AppWindow, DownloadItem, GameItem, Guidance, ModItem, ResourceItem,
    SearchItem, Theme,
};
use slint::{Color, ComponentHandle, Model, ModelRc, SharedString, Timer, TimerMode, VecModel};
use std::{
    cell::RefCell,
    rc::Rc,
    time::{Duration, Instant},
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
    GameItem {
        id: g.id,
        name: g.name.clone().into(),
        version: g.version.clone().into(),
        loader: g.loader.clone().into(),
        detail: format!("{} 项内容", g.content.len()).into(),
        tint: tint(g.id),
    }
}
fn toast(ui: &AppWindow, s: &mut State, message: impl Into<SharedString>) {
    ui.global::<AppState>().set_toast(message.into());
    s.toast_until = Some(Instant::now() + Duration::from_secs(5));
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
fn refresh(ui: &AppWindow, s: &State) {
    let app = ui.global::<AppState>();
    let filter = app.get_game_filter().to_lowercase();
    app.set_games(update_model(
        app.get_games(),
        s.games
            .iter()
            .filter(|g| {
                app.get_page() != 1
                    || format!("{} {} {}", g.name, g.version, g.loader)
                        .to_lowercase()
                        .contains(&filter)
            })
            .map(game_item)
            .collect(),
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
        name: "还没有游戏".into(),
        version: "—".into(),
        loader: "请先创建游戏".into(),
        detail: "".into(),
        tint: tint(0),
    }));
    app.set_mods(update_model(
        app.get_mods(),
        s.game()
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
    let game = if kind == "delete" {
        s.games.iter().find(|g| g.id == id)
    } else {
        s.game()
    }
    .map_or("未选择游戏", |g| g.name.as_str());
    let (title,body)=match kind {
        "create"=>("创建游戏","为游戏取一个容易辨认的名字，再选择版本和加载器。当前仅创建前端演示条目。".into()),
        "delete"=>("删除游戏",format!("将删除「{game}」的演示条目、资源条目和关联任务。\n\n输入完整游戏名称确认对象。当前不会删除磁盘文件；正式删除应另行展示存档保留与备份选项。")),
        "import"=>("导入资源 · 交互预览",format!("向「{game}」添加一个本地导入的演示条目。真实文件选择与导入尚未接入。")),
        "account"=>("添加本地账号","输入用于界面预览的角色名称。微软与社团账号登录会在接入认证服务后提供。".into()),
        "install"=>{
            let Some(r)=s.catalog.iter().find(|r|r.id==id) else {return};
            ("确认安装资源",format!("{}\n\n目标游戏：{game}\n版本：{} / {}\n来源：{}\n\n确认后加入模拟下载队列，完成后出现在目标游戏中。",r.name,r.version,r.loader,r.source))
        },
        "notice"=>("社团公告","欢迎加入社团世界。\n\n此处将展示版本变更、维护通知和社区活动。当前没有连接公告服务。".into()),
        "room"=>("临时联机","后续将在这里接入房间创建、房间码和连接状态。\n\n现在可先通过「社团联机 → 准备并进入」体验环境准备流程。".into()),
        "java"=>("Java 环境管理","后续支持发现已有 Java、选择安装目录、下载与切换版本。\n\n当前展示 Java 21 示例配置，没有扫描或修改本机环境。".into()),
        "skin"=>("孪枭的皮肤与披风","当前使用美术部提供的孪枭原始 PNG，支持双层立体预览、鼠标跟随和手动旋转。\n\n基础层与帽层、衣服和四肢外层按原始透明度叠加。皮肤站绑定、应用到账号和披风切换将在后续接入。".into()),
        _=>return,
    };
    s.pending = Some((
        kind.into(),
        id,
        s.selected,
        category_for_tab(app.get_game_tab()),
    ));
    app.set_search_open(false);
    app.set_modal(kind.into());
    app.set_modal_title(title.into());
    app.set_modal_body(body.into());
    app.set_form_name("".into());
    app.set_form_error("".into());
    app.set_form_version(0);
    app.set_form_loader(0);
    ui.invoke_focus_modal();
}
fn close_modal(ui: &AppWindow, s: &mut State) {
    s.pending = None;
    ui.global::<AppState>().set_modal("".into());
    ui.invoke_focus_main();
}
fn launch(ui: &AppWindow, s: &mut State) {
    if s.game().is_none() {
        toast(ui, s, "请先创建游戏");
        return;
    }
    let weak = ui.as_weak();
    Timer::single_shot(Duration::ZERO, move || {
        if let Some(ui) = weak.upgrade() {
            ui.global::<Guidance>().invoke_start_check(false);
        }
    });
}
fn confirm(ui: &AppWindow, s: &mut State) {
    let Some((kind, id, target, category)) = s.pending.clone() else {
        return;
    };
    let app = ui.global::<AppState>();
    let name = app.get_form_name().trim().to_owned();
    if ["create", "import", "account"].contains(&kind.as_str())
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
            let id = s.id();
            s.games.push(Game {
                id,
                name,
                version: (if app.get_form_version() == 0 {
                    "1.21.1"
                } else {
                    "1.20.1"
                })
                .into(),
                loader: match app.get_form_loader() {
                    0 => "Fabric",
                    1 => "原版",
                    _ => "Forge",
                }
                .into(),
                content: vec![],
            });
            s.selected = id;
            app.set_game_filter("".into());
            app.set_page(1);
            app.set_game_tab(0);
            toast(ui, s, "游戏已添加到演示列表");
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
            category: "游戏".into(),
        })
        .collect();
    for (id, name, detail) in [
        (-2, "社团联机", "服务器与环境准备"),
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
    for (id, name, detail, category) in [
        (
            -100,
            "CAUCRAFT 社团生存服",
            "民航 进服 联机 · 检查当前游戏与入服环境",
            "服务器",
        ),
        (
            -101,
            "首次游玩向导",
            "新手 教程 入门 · 玩法、身份、目录与检查",
            "引导",
        ),
    ] {
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
        results.push(SearchItem {
            id: -2000 - index as i32,
            name: article.title.into(),
            detail: article.summary.into(),
            category: "教程".into(),
        });
    }
    app.set_search_results(model(results));
}

pub fn connect(ui: &AppWindow) -> Timer {
    let state = Rc::new(RefCell::new(State::default()));
    let p = Preferences::load();
    let app = ui.global::<AppState>();
    ui.global::<Theme>().set_dark(p.dark);
    ui.global::<Theme>()
        .set_darkness(if p.dark { 1.0 } else { 0.0 });
    ui.global::<Theme>().set_pixel_glass(p.pixel_glass);
    ui.invoke_sync_theme();
    app.set_folder(p.folder.into());
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
    macro_rules! bind {
        ($callback:ident, |$ui:ident,$s:ident $(,$arg:ident)*| $body:block)=>{{
            let weak=ui.as_weak();let data=state.clone();
            app.$callback(move |$($arg),*|{if let Some($ui)=weak.upgrade(){let $s=&mut *data.borrow_mut();$body refresh(&$ui,$s);}});
        }};
    }
    macro_rules! bind_view {
        ($callback:ident, |$ui:ident,$s:ident $(,$arg:ident)*| $body:block)=>{{
            let weak=ui.as_weak();let data=state.clone();
            app.$callback(move |$($arg),*|{if let Some($ui)=weak.upgrade(){let $s=&mut *data.borrow_mut();$body}});
        }};
    }
    bind!(on_navigate, |ui, s, page| {
        let app = ui.global::<AppState>();
        if page == 8 {
            ui.global::<Guidance>().set_article(-1);
            ui.global::<Guidance>().set_help_return_page(app.get_page());
        }
        if app.get_page() == 1 && page == 3 {
            app.set_resource_category(category_for_tab(app.get_game_tab()));
        }
        app.set_page(page.clamp(0, 8));
        s.pending = None;
        app.set_modal("".into());
    });
    bind!(on_choose_game, |ui, s, id| {
        s.select(id);
        toast(&ui, s, "已切换当前游戏");
    });
    bind!(on_choose_game_at, |ui, s, index| {
        if let Some(g) = s.games.get(index as usize) {
            let id = g.id;
            s.select(id);
        }
        let _ = &ui;
    });
    bind!(on_filter_games, |ui, s, query| {
        ui.global::<AppState>().set_game_filter(query);
        let _ = s;
    });
    bind!(on_switch_game_tab, |ui, s, tab| {
        ui.global::<AppState>().set_game_tab(tab);
        let _ = s;
    });
    bind_view!(on_open_modal, |ui, s, kind, id| {
        open_modal(&ui, s, kind.as_str(), id);
    });
    bind!(on_confirm_modal, |ui, s| {
        confirm(&ui, s);
    });
    bind_view!(on_close_modal, |ui, s| {
        close_modal(&ui, s);
    });
    bind!(on_toggle_mod, |ui, s, id| {
        if let Some(c) = s
            .games
            .iter_mut()
            .find(|g| g.id == s.selected)
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
            s.select(target);
            let app = ui.global::<AppState>();
            app.set_game_tab(match category {
                1 => 4,
                2 => 1,
                3 => 2,
                4 => 3,
                _ => 0,
            });
            app.set_game_filter("".into());
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
    bind_view!(on_save_settings, |ui, s| {
        let app = ui.global::<AppState>();
        let p = Preferences {
            dark: ui.global::<Theme>().get_dark(),
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
        } else {
            match p.save() {
                Ok(()) => toast(&ui, s, "界面偏好已保存到本机"),
                Err(e) => toast(&ui, s, format!("保存失败：{e}")),
            }
        }
    });
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
            s.select(id);
            app.set_page(1);
            app.set_game_filter("".into());
        }
        ui.invoke_focus_main();
    });
    bind_view!(on_diagnose, |ui, s, kind| {
        let app = ui.global::<AppState>();
        if kind == 2 {
            app.set_diagnostic_text(format!("[前端预览] LXMCL 0.1.0\n[界面] {} 个游戏条目，{} 个本地预览账号\n[任务] {} 项模拟下载\n\n这里只显示前端状态摘要。真实游戏日志、JVM 参数和错误分析尚未接入。",s.games.len(),s.accounts.len(),s.downloads.len()).into());
        } else {
            s.diagnostic = Some((kind, 0));
            app.set_diagnosing(true);
            app.set_diagnostic_text("正在演示检查过程…".into());
        }
    });
    let timer = Timer::default();
    let weak = ui.as_weak();
    timer.start(TimerMode::Repeated,Duration::from_millis(200),move||{
        let Some(ui)=weak.upgrade() else{return};let mut s=state.borrow_mut();let app=ui.global::<AppState>();
        let active=s.downloads.iter().any(|d|!d.done && !d.paused);let completed=s.tick_downloads();
        for(name,join,target)in &completed{toast(&ui,&mut s,format!("{name}：模拟任务已完成"));if *join{s.select(*target);app.set_page(0);launch(&ui,&mut s);}}
        if !completed.is_empty(){refresh(&ui,&s);}else if active{refresh_downloads(&ui,&s);}
        if let Some(progress)=s.launch{let progress=progress+5;app.set_launch_progress(progress);app.set_launch_status((if progress<35{"检查账号与游戏配置（演示）…"}else if progress<70{"检查 Java 与资源（演示）…"}else{"准备启动（演示）…"}).into());if progress>=100{s.launch=None;app.set_launching(false);toast(&ui,&mut s,"启动流程演示完成，尚未接入游戏启动");}else{s.launch=Some(progress);}}
        if let Some((kind,step))=s.diagnostic{if step>=7{s.diagnostic=None;app.set_diagnosing(false);app.set_diagnostic_text((if kind==0{"环境检查 · 演示结果\n\n✓ 已选择游戏与账号\n✓ Java 21 示例配置匹配\n✓ 内存配置有效\n\n未扫描本机 Java 或验证真实游戏文件。"}else{"网络检查 · 演示结果\n\n此处将依次检查下载源、账号认证与社团服务器连接，并为失败项提供重试。\n\n当前没有发出网络请求，不代表真实网络状态。"}).into());}else{s.diagnostic=Some((kind,step+1));}}
        if s.toast_until.is_some_and(|until|Instant::now()>=until){app.set_toast("".into());s.toast_until=None;}
    });
    timer
}

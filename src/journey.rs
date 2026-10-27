//! Slint adapter for onboarding, contextual help and launch preparation.
use crate::{AppState, AppWindow, CheckRow, Guidance, GuideArticle, GuideStep, state::State};
use lxmcl::{
    contracts::*,
    help,
    preview_services::{PreviewPreparation, PreviewScenario},
};
use slint::{ComponentHandle, ModelRc, Timer, VecModel};
use std::{
    cell::RefCell,
    hash::{DefaultHasher, Hash, Hasher},
    rc::Rc,
    time::Duration,
};

#[derive(Default)]
struct Journey {
    port: PreviewPreparation,
    report: Option<CheckReport>,
    generation: u64,
    request: u64,
    apply_request: Option<RequestId>,
}

fn model<T: Clone + 'static>(values: Vec<T>) -> ModelRc<T> {
    Rc::new(VecModel::from(values)).into()
}

fn context(ui: &AppWindow, state: &State) -> Result<LaunchContext, String> {
    let guide = ui.global::<Guidance>();
    let app = ui.global::<AppState>();
    let game = state.game().ok_or("请先在“我的游戏”创建或选择一个游戏。")?;
    let (_, account) = state
        .accounts
        .iter()
        .find(|(id, _)| *id == state.account)
        .ok_or("请先选择一个账号。")?;
    // Temporary fingerprint of effective demo configuration. A real service
    // supplies a monotonic revision and revalidates immediately before commit.
    let mut revision = DefaultHasher::new();
    (
        app.get_memory(),
        app.get_auto_memory(),
        app.get_fullscreen(),
    )
        .hash(&mut revision);
    for item in &game.content {
        (item.id, item.enabled).hash(&mut revision);
    }
    Ok(LaunchContext {
        instance_id: game.id,
        instance_name: game.name.clone(),
        instance_revision: revision.finish(),
        account_id: state.account,
        account_name: account.clone(),
        version: game.version.clone(),
        loader: game.loader.clone(),
        game_directory: guide.get_game_directory().trim().into(),
        java_directory: guide.get_java_directory().trim().into(),
        target: if guide.get_club() {
            LaunchTarget::ClubServer {
                server_id: "caucraft-preview".into(),
            }
        } else {
            LaunchTarget::SinglePlayer
        },
    })
}

fn validate_paths(ui: &AppWindow) -> bool {
    let guide = ui.global::<Guidance>();
    let error = validate_directory(guide.get_game_directory().as_str())
        .err()
        .map(|e| format!("游戏目录：{e}"))
        .or_else(|| {
            validate_directory(guide.get_java_directory().as_str())
                .err()
                .map(|e| format!("Java 目录：{e}"))
        });
    guide.set_path_error(error.clone().unwrap_or_default().into());
    error.is_none()
}

fn display(ui: &AppWindow, report: &CheckReport) {
    let guide = ui.global::<Guidance>();
    let mut checks: Vec<_> = report.checks.iter().collect();
    checks.sort_by_key(|check| match check.state {
        CheckState::Blocked => 0,
        CheckState::Unknown => 1,
        CheckState::NeedsAction => 2,
        CheckState::Ready => 3,
    });
    let target = if matches!(report.context.target, LaunchTarget::SinglePlayer) {
        "本地游玩"
    } else {
        "CAUCRAFT 示例服务器"
    };
    guide.set_target_summary(
        format!(
            "{}  ·  {} / {}  ·  {}\n目标：{target}",
            report.context.instance_name,
            report.context.version,
            report.context.loader,
            report.context.account_name
        )
        .into(),
    );
    guide.set_checks(model(
        checks
            .into_iter()
            .map(|c| {
                let (status, severity) = match c.state {
                    CheckState::Ready => ("就绪 · 演示", 0),
                    CheckState::NeedsAction => ("需处理", 1),
                    CheckState::Blocked => ("被阻止", 2),
                    CheckState::Unknown => ("未确认", 3),
                };
                CheckRow {
                    title: c.title.clone().into(),
                    detail: c.explanation.clone().into(),
                    status: status.into(),
                    severity,
                    help_id: c.help_id.clone().into(),
                }
            })
            .collect(),
    ));
    guide.set_can_launch(report.can_launch());
    guide.set_has_plan(report.plan.is_some());
    guide.set_has_report(true);
    guide.set_show_plan(false);
    guide.set_plan_text(
        report
            .plan
            .as_ref()
            .map(|p| {
                p.changes
                    .iter()
                    .map(|c| {
                        format!(
                            "{}\n来源：{}\n位置：{}\n下载量：{}",
                            c.label,
                            c.source,
                            c.destination.display(),
                            c.download_bytes
                                .map_or("待估算".into(), |b| format!("{b} 字节"))
                        )
                    })
                    .collect::<Vec<_>>()
                    .join("\n\n")
            })
            .unwrap_or_default()
            .into(),
    );
    guide.set_check_message(
        if report.can_launch() {
            "演示检查已就绪，可以预演启动。"
        } else if report.plan.is_some() {
            "先核对准备清单；其他被阻止项仍需分别处理。"
        } else {
            "请按问题项打开教程，处理后重新检查。"
        }
        .into(),
    );
}

fn fail(ui: &AppWindow, message: impl Into<slint::SharedString>) {
    let guide = ui.global::<Guidance>();
    guide.set_busy(false);
    guide.set_can_launch(false);
    guide.set_check_message(message.into());
}

fn run_check(ui: &AppWindow, state: Rc<RefCell<State>>, data: Rc<RefCell<Journey>>, club: bool) {
    let guide = ui.global::<Guidance>();
    guide.set_club(club);
    guide.set_completed(false);
    guide.set_can_launch(false);
    guide.set_has_plan(false);
    guide.set_show_plan(false);
    guide.set_has_report(false);
    guide.set_checks(model(vec![]));
    guide.set_target_summary("正在读取当前游戏与账号…".into());
    guide.set_check_message("正在检查演示配置…".into());
    guide.set_busy(true);
    ui.global::<AppState>().set_modal("".into());
    ui.global::<AppState>().set_search_open(false);
    ui.global::<AppState>().set_toast("".into());
    ui.global::<AppState>().set_page(10);
    ui.invoke_focus_main();
    let generation = {
        let mut data = data.borrow_mut();
        data.generation += 1;
        data.report = None;
        data.apply_request = None;
        data.generation
    };
    let input = context(ui, &state.borrow());
    let weak = ui.as_weak();
    Timer::single_shot(Duration::from_millis(160), move || {
        let Some(ui) = weak.upgrade() else {
            return;
        };
        let mut data = data.borrow_mut();
        if data.generation != generation {
            return;
        }
        let result = input.map_err(|e| e.to_string()).and_then(|input| {
            if context(&ui, &state.borrow()).as_ref() != Ok(&input) {
                return Err("配置已变化，请重新检查。".into());
            }
            data.port.check(input).map_err(|e| e.message)
        });
        ui.global::<Guidance>().set_busy(false);
        match result {
            Ok(report) => {
                display(&ui, &report);
                data.report = Some(report);
            }
            Err(message) => fail(&ui, message),
        }
    });
}

fn open_article(ui: &AppWindow, index: usize) {
    let Some(article) = help::ARTICLES.get(index) else {
        return;
    };
    let app = ui.global::<AppState>();
    let guide = ui.global::<Guidance>();
    if app.get_page() != 8 {
        guide.set_help_return_page(app.get_page());
    }
    guide.set_article(index as i32);
    guide.set_article_title(article.title.into());
    guide.set_article_summary(article.summary.into());
    guide.set_article_caution(article.caution.into());
    guide.set_article_action(article.action.into());
    guide.set_article_steps(model(
        article
            .steps
            .iter()
            .map(|(title, detail)| GuideStep {
                title: (*title).into(),
                detail: (*detail).into(),
            })
            .collect(),
    ));
    app.set_page(8);
    ui.invoke_focus_main();
}

fn search_help(ui: &AppWindow, query: &str) {
    ui.global::<Guidance>().set_help_query(query.into());
    ui.global::<Guidance>().set_articles(model(
        help::search(query)
            .iter()
            .map(|&i| GuideArticle {
                id: i as i32,
                title: help::ARTICLES[i].title.into(),
                summary: help::ARTICLES[i].summary.into(),
            })
            .collect(),
    ));
}

pub fn connect(ui: &AppWindow, state: Rc<RefCell<State>>) {
    let data = Rc::new(RefCell::new(Journey::default()));
    let guide = ui.global::<Guidance>();
    guide.set_game_directory(ui.global::<AppState>().get_folder());
    guide.set_java_directory(
        format!(
            "{}\\runtimes",
            ui.global::<AppState>()
                .get_folder()
                .trim_end_matches(['\\', '/'])
        )
        .into(),
    );
    validate_paths(ui);
    search_help(ui, "");
    macro_rules! bind {
        ($callback:ident, |$ui:ident, $state:ident, $data:ident $(, $arg:ident)*| $body:block) => {{
            let weak = ui.as_weak(); let state = state.clone(); let data = data.clone();
            guide.$callback(move |$($arg),*| { if let Some($ui) = weak.upgrade() {
                let $state = state.clone(); let $data = data.clone(); $body
            }});
        }};
    }
    bind!(on_open_guide, |ui, _state, _data| {
        ui.global::<Guidance>().set_guide_started(true);
        ui.global::<AppState>().set_page(9);
        ui.global::<AppState>().set_search_open(false);
        ui.invoke_focus_main();
    });
    bind!(on_advance, |ui, state, _data, delta| {
        let guide = ui.global::<Guidance>();
        if delta > 0 && guide.get_step() == 2 && !validate_paths(&ui) {
            return;
        }
        if delta > 0 && guide.get_step() <= 1 && context(&ui, &state.borrow()).is_err() {
            guide.set_path_error("请先选择游戏与账号；也可以暂时退出向导，稍后继续。".into());
            return;
        }
        guide.set_path_error("".into());
        guide.set_step((guide.get_step() + delta).clamp(0, 3));
        ui.invoke_focus_main();
    });
    bind!(on_validate_paths, |ui, _state, _data| {
        validate_paths(&ui);
    });
    bind!(on_start_check, |ui, state, data, club| {
        run_check(&ui, state, data, club);
    });
    bind!(on_select_scenario, |ui, state, data, index| {
        let scenario = match index {
            1 => PreviewScenario::Ready,
            2 => PreviewScenario::ExpiredAccount,
            3 => PreviewScenario::Incompatible,
            4 => PreviewScenario::DownloadFailure,
            5 => PreviewScenario::Unknown,
            _ => PreviewScenario::MissingJava,
        };
        data.borrow_mut().port = PreviewPreparation::new(scenario);
        ui.global::<Guidance>().set_scenario(index);
        run_check(&ui, state, data, ui.global::<Guidance>().get_club());
    });
    bind!(on_apply_plan, |ui, state, data| {
        let guide = ui.global::<Guidance>();
        if guide.get_busy() || !guide.get_show_plan() {
            return;
        }
        let mut current = data.borrow_mut();
        let Some(report) = current.report.clone() else {
            return;
        };
        let Some(plan) = &report.plan else {
            return;
        };
        if context(&ui, &state.borrow()).as_ref() != Ok(&report.context) {
            guide.set_has_plan(false);
            fail(&ui, "目标或配置已变化，请重新检查后确认新清单。");
            return;
        }
        let request_id = match current.apply_request {
            Some(id) => id,
            None => {
                current.request += 1;
                let id = RequestId(current.request);
                current.apply_request = Some(id);
                id
            }
        };
        let request = ApplyPlanRequest {
            request_id,
            report_id: report.id,
            plan_id: plan.id,
            context: report.context,
        };
        let generation = current.generation;
        drop(current);
        guide.set_busy(true);
        guide.set_check_message("正在演示环境准备，可离开页面；任务目标保持不变。".into());
        let weak = ui.as_weak();
        Timer::single_shot(Duration::from_millis(350), move || {
            let Some(ui) = weak.upgrade() else {
                return;
            };
            let mut data = data.borrow_mut();
            if data.generation != generation {
                return;
            }
            if context(&ui, &state.borrow()).as_ref() != Ok(&request.context) {
                ui.global::<Guidance>().set_has_plan(false);
                fail(&ui, "配置已变化，本次准备未执行。请重新检查。");
                return;
            }
            ui.global::<Guidance>().set_busy(false);
            match data.port.apply_plan(request) {
                Ok(report) => {
                    display(&ui, &report);
                    data.report = Some(report);
                }
                Err(error) => fail(&ui, error.message),
            }
        });
    });
    bind!(on_launch_preview, |ui, state, data| {
        let guide = ui.global::<Guidance>();
        if guide.get_busy() || guide.get_completed() {
            return;
        }
        let mut data = data.borrow_mut();
        let Some(report) = data.report.clone() else {
            return;
        };
        if context(&ui, &state.borrow()).as_ref() != Ok(&report.context) {
            fail(&ui, "游戏、账号或配置已变化。请重新检查，不能沿用旧结果。");
            return;
        }
        data.request += 1;
        let request = LaunchRequest {
            request_id: RequestId(data.request),
            report_id: report.id,
            context: report.context,
        };
        match data.port.launch(request) {
            Ok(_) => {
                guide.set_completed(true);
                guide.set_guide_started(false);
                guide.set_can_launch(false);
                guide.set_check_message("启动预演完成。真实游戏进程与服务器尚未连接。".into());
            }
            Err(error) => fail(&ui, error.message),
        }
    });
    bind!(on_search_help, |ui, _state, _data, query| {
        search_help(&ui, query.as_str());
    });
    bind!(on_open_article, |ui, _state, _data, id| {
        if let Some(index) = help::ARTICLES.iter().position(|a| a.id == id.as_str()) {
            open_article(&ui, index);
        }
    });
    bind!(on_open_article_at, |ui, _state, _data, index| {
        if index >= 0 {
            open_article(&ui, index as usize);
        }
    });
    bind!(on_article_action_clicked, |ui, _state, _data| {
        let index = ui.global::<Guidance>().get_article();
        if let Some(article) = help::ARTICLES.get(index as usize) {
            ui.global::<AppState>().set_page(article.page);
            ui.invoke_focus_main();
        }
    });
    bind!(on_context_help, |ui, _state, _data| {
        let id = match ui.global::<AppState>().get_page() {
            1 | 3 => "resources",
            2 => "club",
            4 => "network",
            5 => "account",
            6 => "storage",
            7 | 10 => "launch",
            _ => "start",
        };
        if let Some(index) = help::ARTICLES.iter().position(|a| a.id == id) {
            open_article(&ui, index);
        }
    });
}

//! The Slint adapter renders plugin contributions with the host's controls.
use crate::{AppState, AppWindow, ExtensionItem};
use launcher_api::{
    MainArea, SettingsSection, UiAction, UiContributions, UiPlacement, UiRoute, VersionSection,
};
use launcher_runtime::{BoundedWorker, PluginHost, ShutdownPolicy, SubmitError, WorkerExit};
use slint::{ComponentHandle, ModelRc, VecModel};
use std::collections::HashMap;
use std::error::Error;
use std::sync::Arc;

type Command = Arc<dyn launcher_api::UiCommand>;

pub struct ExtensionWorker(Option<BoundedWorker<Command>>);

pub fn connect(ui: &AppWindow, host: &PluginHost) -> Result<ExtensionWorker, Box<dyn Error>> {
    let entries = host.contributions::<UiContributions>()?;
    let mut rows = Vec::new();
    let mut actions = HashMap::new();
    for entry in entries {
        let contribution = entry.value.as_ref();
        rows.push(ExtensionItem {
            id: entry.id.clone().into(),
            title: contribution.title.clone().into(),
            detail: contribution.description.clone().into(),
            status: entry.owner.into(),
            placement: placement_label(contribution.placement).into(),
            action_label: if matches!(contribution.action, UiAction::Navigate(_)) {
                "打开"
            } else {
                "执行"
            }
            .into(),
        });
        actions.insert(entry.id, contribution.action.clone());
    }
    let plugins = host.plugin_states();
    let plugin_summary = plugins
        .iter()
        .map(|plugin| format!("{} · {}", plugin.id, plugin.provider))
        .collect::<Vec<_>>()
        .join("\n");
    ui.global::<AppState>()
        .set_extensions(ModelRc::new(VecModel::from(rows)));
    ui.global::<AppState>().set_extension_status(
        format!("已装配 {} 个插件\n{}", plugins.len(), plugin_summary).into(),
    );

    let weak = ui.as_weak();
    let worker = BoundedWorker::spawn("shell-commands", 1, move |command: Command, cancelled| {
        let result =
            std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| command.execute(cancelled)))
                .unwrap_or_else(|_| Err("扩展执行异常，操作已停止".into()));
        let weak = weak.clone();
        let _ = slint::invoke_from_event_loop(move || {
            if let Some(ui) = weak.upgrade() {
                let app = ui.global::<AppState>();
                app.set_extension_busy(false);
                app.set_extension_status(
                    result
                        .unwrap_or_else(|error| format!("执行失败：{error}"))
                        .into(),
                );
            }
        });
    })?;
    let sender = worker.sender();
    let weak = ui.as_weak();
    ui.global::<AppState>().on_extension_action(move |id| {
        let Some(ui) = weak.upgrade() else {
            return;
        };
        let app = ui.global::<AppState>();
        let Some(action) = actions.get(id.as_str()) else {
            app.set_extension_status("扩展入口已失效，请重新打开应用".into());
            return;
        };
        match action {
            UiAction::Navigate(route) => navigate(&ui, route),
            UiAction::Command(command) => {
                if app.get_extension_busy() {
                    return;
                }
                match sender.try_submit(Arc::clone(command)) {
                    Ok(()) => {
                        app.set_extension_busy(true);
                        app.set_extension_status("正在执行…".into());
                    }
                    Err(SubmitError::Busy(_)) => {
                        app.set_extension_status("已有扩展操作正在执行".into())
                    }
                    Err(SubmitError::Closed(_)) => {
                        app.set_extension_status("扩展工作线程已关闭".into())
                    }
                }
            }
        }
    });
    Ok(ExtensionWorker(Some(worker)))
}

fn placement_label(placement: UiPlacement) -> &'static str {
    match placement {
        UiPlacement::Settings => "设置",
        UiPlacement::Extensions => "扩展",
        UiPlacement::Account => "账号",
        UiPlacement::Instance => "实例",
        UiPlacement::Versions => "版本",
        UiPlacement::Multiplayer => "联机",
    }
}

fn navigate(ui: &AppWindow, route: &UiRoute) {
    let app = ui.global::<AppState>();
    match route {
        UiRoute::Main(area) => app.invoke_navigate(match area {
            MainArea::Home => 0,
            MainArea::Versions => 1,
            MainArea::Multiplayer => 2,
            MainArea::Settings => 6,
        }),
        UiRoute::Versions(section) => {
            app.set_instance_detail_open(false);
            app.invoke_navigate(match section {
                VersionSection::Installed => 1,
                VersionSection::Resources => 3,
                VersionSection::Downloads => 4,
            });
        }
        UiRoute::Settings(section) => {
            let index = match section {
                SettingsSection::General => 0,
                SettingsSection::Game => 1,
                SettingsSection::Java => 2,
                SettingsSection::Network => 3,
                SettingsSection::Appearance => 4,
                SettingsSection::Accounts => 5,
                SettingsSection::Extensions => 6,
                SettingsSection::About => 7,
            };
            app.set_settings_category(index);
            app.invoke_navigate(if index == 5 { 5 } else { 6 });
        }
        UiRoute::Account => {
            app.set_settings_category(5);
            app.invoke_navigate(5);
        }
        UiRoute::InstanceSettings {
            instance_id,
            revision,
        } => app.invoke_extension_open_instance_settings(*instance_id, revision.to_string().into()),
    }
}

impl ExtensionWorker {
    pub fn join(&mut self) -> std::io::Result<()> {
        match self
            .0
            .take()
            .map(|worker| worker.shutdown(ShutdownPolicy::Cancel))
        {
            Some(WorkerExit::Panicked) => Err(std::io::Error::other("扩展工作线程异常退出")),
            _ => Ok(()),
        }
    }
}

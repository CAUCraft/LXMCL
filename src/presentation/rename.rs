use crate::{AppState, AppWindow, controller, state::State};
use launcher_api::{ApplicationService, InstanceError, RenameCompleted, RenameInstance, RequestId};
use slint::ComponentHandle;
use std::{
    cell::RefCell,
    collections::HashMap,
    rc::Rc,
    sync::{Arc, Mutex},
};

#[derive(Default)]
struct Forms {
    sequence: u64,
    current: Option<Form>,
    requests: HashMap<RequestId, (u64, i32)>,
}

struct Form {
    session: u64,
    instance_id: i32,
    revision: u64,
    request: Option<RequestId>,
    draft: String,
}

impl Forms {
    fn open(&mut self, instance_id: i32, revision: u64, name: String) {
        self.sequence += 1;
        self.current = Some(Form {
            session: self.sequence,
            instance_id,
            revision,
            request: None,
            draft: name,
        });
    }

    fn accept(&mut self, session: u64, instance_id: i32, request: RequestId) -> bool {
        self.requests.insert(request, (session, instance_id));
        if let Some(form) = &mut self.current
            && form.session == session
        {
            form.request = Some(request);
            return true;
        }
        false
    }

    fn finish(&mut self, request: RequestId, instance_id: i32) -> Option<bool> {
        let (session, id) = self.requests.remove(&request)?;
        if id != instance_id {
            return None;
        }
        Some(
            self.current
                .as_ref()
                .is_some_and(|form| form.session == session && form.request == Some(request)),
        )
    }
}

/// A form session is independent of navigation and of accepted application writes.
#[derive(Clone)]
pub struct RenamePresentation {
    forms: Rc<RefCell<Forms>>,
    application: Arc<dyn ApplicationService>,
    completed: Arc<Mutex<Vec<RenameCompleted>>>,
}

impl RenamePresentation {
    pub fn connect(
        ui: &AppWindow,
        state: Rc<RefCell<State>>,
        application: Arc<dyn ApplicationService>,
    ) -> Self {
        let presentation = Self {
            forms: Default::default(),
            application,
            completed: Default::default(),
        };
        let receiver = presentation.clone();
        let weak = ui.as_weak();
        // Only this notification crosses Slint; typed completions remain in Rust.
        ui.global::<AppState>().on_rename_completed(move || {
            let Some(ui) = weak.upgrade() else { return };
            let results = std::mem::take(&mut *receiver.completed.lock().unwrap());
            for result in results {
                receiver.complete(&ui, &state, result);
            }
        });
        presentation
    }

    pub fn open(&self, instance_id: i32, revision: u64, name: String) {
        self.forms.borrow_mut().open(instance_id, revision, name);
    }

    pub fn dismiss(&self) {
        self.forms.borrow_mut().current = None;
    }

    pub fn submit(&self, ui: &AppWindow) {
        let app = ui.global::<AppState>();
        let command = {
            let mut forms = self.forms.borrow_mut();
            let Some(form) = forms.current.as_mut() else {
                return;
            };
            if form.request.is_some() {
                return;
            }
            form.draft = app.get_form_name().trim().to_owned();
            if form.draft.is_empty() || form.draft.chars().count() > 32 {
                app.set_form_error("请输入 1–32 个字符的名称。".into());
                return;
            }
            (
                form.session,
                RenameInstance {
                    instance_id: form.instance_id,
                    expected_revision: form.revision,
                    name: form.draft.clone(),
                },
            )
        };
        app.set_form_error("".into());
        let completed = self.completed.clone();
        let weak = ui.as_weak();
        let instance_id = command.1.instance_id;
        // Release every RefCell borrow before invoking an external service, including rejection.
        let submitted = self.application.rename(
            command.1,
            Box::new(move |result| {
                let failure = result.result.as_ref().err().map(ToString::to_string);
                let failed_delivery = failure.clone();
                completed.lock().unwrap().push(result);
                if slint::invoke_from_event_loop(move || {
                    if let Some(ui) = weak.upgrade() {
                        ui.global::<AppState>().invoke_rename_completed();
                    } else if let Some(error) = failure {
                        eprintln!("实例重命名失败且窗口已关闭：{error}");
                    }
                })
                .is_err()
                {
                    if let Some(error) = failed_delivery {
                        eprintln!("实例重命名失败且结果无法回传：{error}");
                    } else {
                        eprintln!("重命名已完成，但界面事件循环已经退出");
                    }
                }
            }),
        );
        match submitted {
            Ok(request) => {
                let mut forms = self.forms.borrow_mut();
                if forms.accept(command.0, instance_id, request) {
                    app.set_instance_saving(true);
                    app.set_modal_body(
                        "正在保存实例名称。可以关闭此表单，已接受的保存会在后台继续。".into(),
                    );
                }
            }
            Err(error) => {
                app.set_instance_saving(false);
                app.set_form_error(format!("未提交重命名：{error}。草稿已保留，可重试。").into());
            }
        }
    }

    fn complete(&self, ui: &AppWindow, state: &Rc<RefCell<State>>, completed: RenameCompleted) {
        let app = ui.global::<AppState>();
        let active = {
            let mut forms = self.forms.borrow_mut();
            let Some(active) = forms.finish(completed.request_id, completed.instance_id) else {
                return;
            };
            active && app.get_modal() == "rename"
        };
        let message = match completed.result {
            Ok(record) => {
                let name = record.name.clone();
                state.borrow_mut().update_instance(record);
                if active {
                    self.dismiss();
                    state.borrow_mut().pending = None;
                    app.set_instance_saving(false);
                    app.set_modal("".into());
                    ui.invoke_focus_main();
                }
                format!("「{name}」已重命名")
            }
            Err(error) => {
                let conflict = matches!(error, InstanceError::RevisionConflict { .. });
                let revision = completed.current.as_ref().map(|record| record.revision);
                let current_summary = completed
                    .current
                    .as_ref()
                    .map(|record| format!("「{}」（修订 {}）", record.name, record.revision));
                if let Some(record) = completed.current {
                    state.borrow_mut().update_instance(record);
                }
                let message = if conflict && let Some(current) = current_summary {
                    format!("实例当前名称为{current}。草稿已保留，请核对后再次保存。")
                } else if conflict {
                    "暂时无法读取最新实例记录，草稿已保留，请重试。".to_owned()
                } else {
                    format!("实例 #{} 重命名失败：{error}", completed.instance_id)
                };
                if active {
                    if let Some(form) = &mut self.forms.borrow_mut().current {
                        form.request = None;
                        if let Some(revision) = revision {
                            form.revision = revision;
                        }
                    }
                    app.set_instance_saving(false);
                    app.set_modal_body("修改此实例的显示名称，不移动实例目录。".into());
                    app.set_form_error(message.clone().into());
                }
                message
            }
        };
        controller::refresh(ui, &state.borrow());
        crate::settings_bridge::toast_detached(&ui.as_weak(), message);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn completion_of_dismissed_form_cannot_claim_reopened_same_target() {
        let mut forms = Forms::default();
        forms.open(10, 1, "first draft".into());
        let session = forms.sequence;
        assert!(forms.accept(session, 10, RequestId(1)));
        forms.current = None;
        forms.open(10, 1, "later draft".into());
        assert_eq!(forms.finish(RequestId(1), 10), Some(false));
        assert_eq!(forms.current.as_ref().unwrap().draft, "later draft");
        assert_eq!(forms.finish(RequestId(1), 10), None);
    }

    #[test]
    fn accepted_request_survives_leaving_the_form() {
        let mut forms = Forms::default();
        forms.open(10, 1, "draft".into());
        assert!(forms.accept(forms.sequence, 10, RequestId(3)));
        forms.current = None;
        assert_eq!(forms.finish(RequestId(3), 10), Some(false));

        let mut state = State::default();
        state.select(20);
        state.update_instance(launcher_api::InstanceRecord {
            id: 10,
            revision: 2,
            name: "renamed".into(),
            version: "1.21.1".into(),
            loader: "Fabric".into(),
            game_directory: "captured-directory".into(),
        });
        assert_eq!(state.selected, 20);
        assert_eq!(
            state.games.iter().find(|game| game.id == 10).unwrap().name,
            "renamed"
        );
    }
}

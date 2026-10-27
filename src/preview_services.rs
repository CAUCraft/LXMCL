//! Deterministic in-memory fixtures. No authentication, downloads or disk writes.
use crate::contracts::*;
use std::{
    collections::{HashMap, HashSet},
    path::PathBuf,
};

#[derive(Clone, PartialEq, Eq)]
enum RequestBody {
    Apply(ApplyPlanRequest),
    Launch(LaunchRequest),
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum PreviewScenario {
    #[default]
    MissingJava,
    Ready,
    ExpiredAccount,
    Incompatible,
    DownloadFailure,
    Unknown,
}

#[derive(Default)]
pub struct PreviewPreparation {
    pub scenario: PreviewScenario,
    next_id: u64,
    current: Option<CheckReport>,
    download_failed_once: bool,
    prepared_java: HashSet<(PathBuf, String, String)>,
    requests: HashMap<RequestId, RequestBody>,
    applied: HashMap<RequestId, (ApplyPlanRequest, CheckReport)>,
    launched: HashMap<RequestId, (LaunchRequest, LaunchReceipt)>,
}

fn error(code: ErrorCode, message: &str, retryable: bool, help_id: &str) -> ServiceError {
    ServiceError {
        code,
        message: message.into(),
        retryable,
        help_id: help_id.into(),
    }
}

impl PreviewPreparation {
    fn reserve(&mut self, id: RequestId, body: RequestBody) -> Result<(), ServiceError> {
        if self
            .requests
            .get(&id)
            .is_some_and(|previous| previous != &body)
        {
            return Err(error(
                ErrorCode::RequestConflict,
                "请求编号已绑定其他内容，请重新检查后提交。",
                false,
                "launch",
            ));
        }
        self.requests.entry(id).or_insert(body);
        Ok(())
    }

    fn java_key(context: &LaunchContext) -> (PathBuf, String, String) {
        (
            context.java_directory.clone(),
            context.version.clone(),
            context.loader.clone(),
        )
    }
    pub fn new(scenario: PreviewScenario) -> Self {
        Self {
            scenario,
            ..Self::default()
        }
    }

    fn current_for(
        &self,
        report_id: u64,
        context: &LaunchContext,
    ) -> Result<&CheckReport, ServiceError> {
        self.current
            .as_ref()
            .filter(|r| r.id == report_id && &r.context == context)
            .ok_or_else(|| {
                error(
                    ErrorCode::StaleContext,
                    "游戏、账号或配置已经变化，请重新检查。",
                    true,
                    "launch",
                )
            })
    }
}

impl PreparationPort for PreviewPreparation {
    fn check(&mut self, context: LaunchContext) -> Result<CheckReport, ServiceError> {
        // Invalidate the previous report even when checking the new input fails.
        self.current = None;
        for path in [&context.game_directory, &context.java_directory] {
            validate_directory(&path.to_string_lossy())
                .map_err(|m| error(ErrorCode::InvalidPath, m, false, "storage"))?;
        }
        self.next_id += 1;
        let mut checks = vec![
            CheckItem {
                code: "account".into(),
                title: "账号与入服资格".into(),
                state: CheckState::Ready,
                explanation: "演示身份已选择；真实认证与服务器资格尚未验证。".into(),
                help_id: "account".into(),
            },
            CheckItem {
                code: "version".into(),
                title: "游戏版本与加载器".into(),
                state: CheckState::Ready,
                explanation: format!("{} / {} · 当前演示配置", context.version, context.loader),
                help_id: "club".into(),
            },
            CheckItem {
                code: "java".into(),
                title: "Java 运行环境".into(),
                state: CheckState::Ready,
                explanation: "示例环境匹配；实际版本由游戏与加载器元数据共同决定。".into(),
                help_id: "java".into(),
            },
            CheckItem {
                code: "storage".into(),
                title: "目录与可用空间".into(),
                state: CheckState::Ready,
                explanation: "路径格式正确；磁盘空间和写入权限使用演示结果。".into(),
                help_id: "storage".into(),
            },
            CheckItem {
                code: "files".into(),
                title: "游戏文件与资源依赖".into(),
                state: CheckState::Ready,
                explanation: "演示清单完整；尚未读取磁盘文件或检查模组冲突。".into(),
                help_id: "resources".into(),
            },
        ];
        let mut plan = None;
        if !self.prepared_java.contains(&Self::java_key(&context))
            && matches!(
                self.scenario,
                PreviewScenario::MissingJava | PreviewScenario::DownloadFailure
            )
        {
            checks[2].state = CheckState::NeedsAction;
            checks[2].explanation =
                "示例：缺少匹配的 Java。先查看来源和安装位置，再确认准备环境。".into();
            plan = Some(PreparationPlan {
                id: self.next_id,
                changes: vec![PlannedChange {
                    label: "安装匹配的 Zulu JDK（演示）".into(),
                    source: "Azul Zulu · 发行版与版本待真实元数据确认".into(),
                    destination: context.java_directory.clone(),
                    download_bytes: None,
                }],
            });
        }
        if self.scenario == PreviewScenario::ExpiredAccount {
            checks[0].state = CheckState::Blocked;
            checks[0].explanation =
                "示例：登录已过期。请在账号页重新认证；不会自动改用离线身份进入社团服。".into();
        }
        if self.scenario == PreviewScenario::Incompatible
            || (matches!(context.target, LaunchTarget::ClubServer { .. })
                && (context.version != "1.21.1" || context.loader != "Fabric"))
        {
            checks[1].state = CheckState::Blocked;
            checks[1].explanation =
                "示例服务器要求 1.21.1 / Fabric。请选择兼容游戏，或创建独立游戏；保留现有存档。"
                    .into();
        }
        if self.scenario == PreviewScenario::Unknown {
            checks[4].state = CheckState::Unknown;
            checks[4].explanation =
                "示例：无法获取校验结果。未知不等于通过；重试检查或查看帮助。".into();
        }
        let report = CheckReport {
            id: self.next_id,
            context,
            origin: DataOrigin::Preview,
            checks,
            plan,
        };
        self.current = Some(report.clone());
        Ok(report)
    }

    fn apply_plan(&mut self, request: ApplyPlanRequest) -> Result<CheckReport, ServiceError> {
        self.reserve(request.request_id, RequestBody::Apply(request.clone()))?;
        if let Some((original, result)) = self.applied.get(&request.request_id) {
            return if original == &request {
                Ok(result.clone())
            } else {
                Err(error(
                    ErrorCode::RequestConflict,
                    "同一请求编号对应了不同操作，请重新检查。",
                    false,
                    "launch",
                ))
            };
        }
        let mut report = self
            .current_for(request.report_id, &request.context)?
            .clone();
        if report.plan.as_ref().is_none_or(|p| p.id != request.plan_id) {
            return Err(error(
                ErrorCode::PlanNotFound,
                "准备清单已失效，请重新检查。",
                true,
                "launch",
            ));
        }
        if self.scenario == PreviewScenario::DownloadFailure && !self.download_failed_once {
            self.download_failed_once = true;
            return Err(error(
                ErrorCode::NetworkTimeout,
                "模拟下载超时。清单已保留，可以重试；没有安装任何文件。",
                true,
                "network",
            ));
        }
        for check in &mut report.checks {
            if check.state == CheckState::NeedsAction {
                check.state = CheckState::Ready;
                check.explanation = "环境准备演示完成。未下载或安装真实 Java。".into();
            }
        }
        self.prepared_java.insert(Self::java_key(&report.context));
        self.next_id += 1;
        report.id = self.next_id;
        report.plan = None;
        self.current = Some(report.clone());
        self.applied
            .insert(request.request_id, (request, report.clone()));
        Ok(report)
    }

    fn launch(&mut self, request: LaunchRequest) -> Result<LaunchReceipt, ServiceError> {
        self.reserve(request.request_id, RequestBody::Launch(request.clone()))?;
        if let Some((original, receipt)) = self.launched.get(&request.request_id) {
            return if original == &request {
                Ok(receipt.clone())
            } else {
                Err(error(
                    ErrorCode::RequestConflict,
                    "请求编号不能复用于另一个启动目标。",
                    false,
                    "launch",
                ))
            };
        }
        let report = self.current_for(request.report_id, &request.context)?;
        if !report.can_launch() {
            return Err(error(
                ErrorCode::NotReady,
                "仍有未完成的检查，请先处理列表中的问题。",
                false,
                "launch",
            ));
        }
        let receipt = LaunchReceipt {
            request_id: request.request_id,
            instance_id: request.context.instance_id,
            origin: DataOrigin::Preview,
        };
        self.launched
            .insert(request.request_id, (request, receipt.clone()));
        Ok(receipt)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn context() -> LaunchContext {
        LaunchContext {
            instance_id: 10,
            instance_name: "社团生存".into(),
            instance_revision: 1,
            account_id: 2,
            account_name: "玩家".into(),
            version: "1.21.1".into(),
            loader: "Fabric".into(),
            game_directory: "E:\\Games\\LXMCL".into(),
            java_directory: "E:\\Games\\Java".into(),
            target: LaunchTarget::SinglePlayer,
        }
    }
    #[test]
    fn blocks_unknown_expired_and_mismatched_environment() {
        for scenario in [
            PreviewScenario::Unknown,
            PreviewScenario::ExpiredAccount,
            PreviewScenario::Incompatible,
            PreviewScenario::MissingJava,
        ] {
            let mut port = PreviewPreparation::new(scenario);
            let report = port.check(context()).unwrap();
            assert!(!report.can_launch());
            assert_eq!(
                port.launch(LaunchRequest {
                    request_id: RequestId(1),
                    report_id: report.id,
                    context: context()
                })
                .unwrap_err()
                .code,
                ErrorCode::NotReady
            );
        }
    }
    #[test]
    fn plans_are_bound_to_the_target_and_retry_without_duplicate_changes() {
        let mut port = PreviewPreparation::new(PreviewScenario::DownloadFailure);
        let report = port.check(context()).unwrap();
        let request = ApplyPlanRequest {
            request_id: RequestId(1),
            report_id: report.id,
            plan_id: report.plan.unwrap().id,
            context: context(),
        };
        let mut wrong = request.clone();
        wrong.request_id = RequestId(99);
        wrong.context.instance_id = 20;
        assert_eq!(
            port.apply_plan(wrong).unwrap_err().code,
            ErrorCode::StaleContext
        );
        assert_eq!(
            port.apply_plan(request.clone()).unwrap_err().code,
            ErrorCode::NetworkTimeout
        );
        let prepared = port.apply_plan(request.clone()).unwrap();
        assert!(prepared.can_launch());
        assert_eq!(port.apply_plan(request).unwrap(), prepared);
    }
    #[test]
    fn stale_config_cannot_launch_and_duplicate_launch_is_idempotent() {
        let mut port = PreviewPreparation::new(PreviewScenario::Ready);
        let report = port.check(context()).unwrap();
        let mut changed = context();
        changed.instance_revision += 1;
        assert_eq!(
            port.launch(LaunchRequest {
                request_id: RequestId(1),
                report_id: report.id,
                context: changed
            })
            .unwrap_err()
            .code,
            ErrorCode::StaleContext
        );
        let request = LaunchRequest {
            request_id: RequestId(2),
            report_id: report.id,
            context: context(),
        };
        assert_eq!(
            port.launch(request.clone()).unwrap(),
            port.launch(request).unwrap()
        );
        let mut request = LaunchRequest {
            request_id: RequestId(2),
            report_id: report.id,
            context: context(),
        };
        request.context.account_id = 5;
        assert_eq!(
            port.launch(request).unwrap_err().code,
            ErrorCode::RequestConflict
        );
    }
    #[test]
    fn failed_recheck_invalidates_previous_ready_report() {
        let mut port = PreviewPreparation::new(PreviewScenario::Ready);
        let report = port.check(context()).unwrap();
        let mut invalid = context();
        invalid.java_directory = "relative".into();
        assert!(port.check(invalid).is_err());
        assert_eq!(
            port.launch(LaunchRequest {
                request_id: RequestId(1),
                report_id: report.id,
                context: context()
            })
            .unwrap_err()
            .code,
            ErrorCode::StaleContext
        );
    }
    #[test]
    fn validates_directory_without_rejecting_spaces_or_chinese() {
        for path in [
            "E:\\游戏目录\\LXMCL",
            "D:\\My Games\\LXMCL",
            "C:/Games/Java",
        ] {
            assert!(validate_directory(path).is_ok());
        }
        for path in [
            "",
            "D:",
            "D:\\",
            "Games",
            "E:\\x\\..\\y",
            "E:\\NUL",
            "E:\\bad?",
            "E:\\x.",
            "E:\\COM1.txt",
        ] {
            assert!(validate_directory(path).is_err(), "{path}");
        }
    }

    #[test]
    fn rechecking_keeps_prepared_environment_and_changed_runtime_needs_new_plan() {
        let mut port = PreviewPreparation::default();
        let report = port.check(context()).unwrap();
        port.apply_plan(ApplyPlanRequest {
            request_id: RequestId(1),
            report_id: report.id,
            plan_id: report.plan.unwrap().id,
            context: context(),
        })
        .unwrap();
        assert!(port.check(context()).unwrap().can_launch());
        let mut changed = context();
        changed.java_directory = "E:\\OtherJava".into();
        assert!(!port.check(changed).unwrap().can_launch());
    }

    #[test]
    fn request_ids_are_bound_even_after_failure_and_across_command_types() {
        let mut port = PreviewPreparation::new(PreviewScenario::DownloadFailure);
        let report = port.check(context()).unwrap();
        let request = ApplyPlanRequest {
            request_id: RequestId(1),
            report_id: report.id,
            plan_id: report.plan.unwrap().id,
            context: context(),
        };
        assert_eq!(
            port.apply_plan(request.clone()).unwrap_err().code,
            ErrorCode::NetworkTimeout
        );
        let mut other = request.clone();
        other.context.instance_id = 20;
        assert_eq!(
            port.apply_plan(other).unwrap_err().code,
            ErrorCode::RequestConflict
        );
        let prepared = port.apply_plan(request).unwrap();
        assert_eq!(
            port.launch(LaunchRequest {
                request_id: RequestId(1),
                report_id: prepared.id,
                context: context()
            })
            .unwrap_err()
            .code,
            ErrorCode::RequestConflict
        );
    }
}

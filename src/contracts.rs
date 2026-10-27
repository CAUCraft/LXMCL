//! UI-independent launch preparation port. Real implementations run off the UI
//! thread; tokens, process handles and filesystem operations stay in the service.
use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct RequestId(pub u64);

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub enum LaunchTarget {
    SinglePlayer,
    ClubServer { server_id: String },
}

/// Capture the target when the user starts an operation. Never resolve a running
/// operation against a later global selection.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct LaunchContext {
    pub instance_id: i32,
    pub instance_name: String,
    pub instance_revision: u64,
    pub account_id: i32,
    pub account_name: String,
    pub version: String,
    pub loader: String,
    pub game_directory: PathBuf,
    pub java_directory: PathBuf,
    pub target: LaunchTarget,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub enum CheckState {
    Ready,
    NeedsAction,
    Blocked,
    Unknown,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct CheckItem {
    pub code: String,
    pub title: String,
    pub state: CheckState,
    pub explanation: String,
    pub help_id: String,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct PlannedChange {
    pub label: String,
    pub source: String,
    pub destination: PathBuf,
    /// None means unknown, never zero bytes or a fabricated estimate.
    pub download_bytes: Option<u64>,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct PreparationPlan {
    pub id: u64,
    pub changes: Vec<PlannedChange>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub enum DataOrigin {
    Preview,
    Live,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct CheckReport {
    pub id: u64,
    pub context: LaunchContext,
    pub origin: DataOrigin,
    pub checks: Vec<CheckItem>,
    pub plan: Option<PreparationPlan>,
}

impl CheckReport {
    pub fn can_launch(&self) -> bool {
        !self.checks.is_empty() && self.checks.iter().all(|c| c.state == CheckState::Ready)
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub enum ErrorCode {
    StaleContext,
    InvalidPath,
    NotReady,
    PlanNotFound,
    NetworkTimeout,
    RequestConflict,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct ServiceError {
    pub code: ErrorCode,
    pub message: String,
    pub retryable: bool,
    pub help_id: String,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct ApplyPlanRequest {
    pub request_id: RequestId,
    pub report_id: u64,
    pub plan_id: u64,
    pub context: LaunchContext,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct LaunchRequest {
    pub request_id: RequestId,
    pub report_id: u64,
    pub context: LaunchContext,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct LaunchReceipt {
    pub request_id: RequestId,
    pub instance_id: i32,
    pub origin: DataOrigin,
}

/// Application boundary, not a TCP port or an HTTP API. The desktop adapter
/// marshals worker results onto Slint's event loop. The preview implementation
/// performs only in-memory work and can therefore be called synchronously.
pub trait PreparationPort {
    fn check(&mut self, context: LaunchContext) -> Result<CheckReport, ServiceError>;
    fn apply_plan(&mut self, request: ApplyPlanRequest) -> Result<CheckReport, ServiceError>;
    fn launch(&mut self, request: LaunchRequest) -> Result<LaunchReceipt, ServiceError>;
}

/// Syntax-only check for this Windows preview. Existence, space, write access,
/// canonicalization and directory overlap must be checked by the real service.
pub fn validate_directory(value: &str) -> Result<(), &'static str> {
    let value = value.trim();
    let bytes = value.as_bytes();
    if bytes.len() < 4
        || !bytes[0].is_ascii_alphabetic()
        || bytes[1] != b':'
        || !matches!(bytes[2], b'\\' | b'/')
    {
        return Err("请填写完整文件夹路径，例如 E:\\Games\\LXMCL；暂不支持网络目录。");
    }
    if value[3..].contains(['<', '>', ':', '"', '|', '?', '*'])
        || value.chars().any(char::is_control)
    {
        return Err("路径包含 Windows 不支持的字符，请修改后继续。");
    }
    let mut parts = 0;
    for part in value[3..].split(['\\', '/']).filter(|p| !p.is_empty()) {
        let stem = part.split('.').next().unwrap_or("").to_ascii_uppercase();
        let reserved = matches!(stem.as_str(), "CON" | "PRN" | "AUX" | "NUL")
            || (stem.len() == 4
                && (stem.starts_with("COM") || stem.starts_with("LPT"))
                && matches!(stem.as_bytes()[3], b'1'..=b'9'));
        if part == "." || part == ".." || part.ends_with([' ', '.']) || reserved {
            return Err("请选择普通文件夹，避免上级目录、保留名称或末尾的空格与句点。");
        }
        parts += 1;
    }
    if parts == 0 {
        return Err("请使用磁盘中的独立文件夹，不要直接选择磁盘根目录。");
    }
    Ok(())
}

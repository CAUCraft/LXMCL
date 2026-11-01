//! 启动器业务能力契约。
//!
//! 这里只定义服务 trait、请求与结果类型，不含任何提供者实现、UI 或
//! Slint 依赖。插件实现这些契约，产品与 Shell 消费这些契约。
//!
//! 重新导出插件开发所需的宿主类型，第三方插件只依赖本 crate 即可。

pub use launcher_runtime::{
    Context, Plugin, PluginDescriptor, RuntimeError, ServiceKey, ServiceRequirement,
};

use std::fmt;
use std::path::{Path, PathBuf};
use std::sync::atomic::AtomicBool;

/// 启动器全局设置的业务值。提供者负责落盘，界面不直接读写配置文件。
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct LauncherSettings {
    /// 游戏存放目录。
    pub game_root: PathBuf,
    /// Java 可执行文件；None 表示使用自动探测（后续服务决定）。
    pub java_path: Option<PathBuf>,
}

/// 设置服务的错误。文件损坏时明确报错，绝不静默当作首次运行覆盖。
#[derive(Debug)]
pub enum SettingsError {
    /// 设置文件存在但无法解析；本次未读取或写入任何值。
    Parse { path: PathBuf, message: String },
    /// 读取或写入设置文件失败。
    Io {
        path: PathBuf,
        source: std::io::Error,
    },
    /// 设置值无法序列化为提供者使用的格式。
    Encode(String),
    /// 后台设置服务不可用（例如工作线程已退出）。
    Unavailable,
}

impl fmt::Display for SettingsError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Parse { path, message } => {
                write!(
                    f,
                    "设置文件 {} 内容损坏：{}。为避免覆盖数据，本次未做任何修改。",
                    path.display(),
                    message
                )
            }
            Self::Io { path, source } => {
                write!(f, "设置文件 {} 读写失败：{}", path.display(), source)
            }
            Self::Encode(reason) => {
                write!(f, "设置无法序列化：{reason}")
            }
            Self::Unavailable => {
                write!(f, "设置服务不可用")
            }
        }
    }
}

impl std::error::Error for SettingsError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::Io { source, .. } => Some(source),
            _ => None,
        }
    }
}

/// 设置服务契约：读有效值与保存。只在工作线程调用，不做阻塞界面的事。
pub trait SettingsService: Send + Sync {
    fn load(&self) -> Result<LauncherSettings, SettingsError>;
    fn save(&self, settings: &LauncherSettings) -> Result<(), SettingsError>;
}

/// 设置服务的服务键。依赖按这个契约解析，不按提供者插件名解析。
pub struct Settings;

impl ServiceKey for Settings {
    type Service = dyn SettingsService;
    const NAME: &'static str = "launcher.settings";
}

/// 一个已保存的实例记录。只包含当前产品需要的稳定配置，不包含游戏文件状态。
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct InstanceRecord {
    /// 稳定实例 ID；沿用现有界面与 LaunchContext 的整数 ID。
    pub id: i32,
    pub name: String,
    pub version: String,
    pub loader: String,
    /// 实例独立目录。创建记录只保存路径，不创建或修改游戏文件。
    pub game_directory: PathBuf,
    /// 每次成功修改后单调递增。
    pub revision: u64,
}

/// 创建实例记录的输入。具体目录由提供者分配，界面不生成 ID。
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct NewInstance {
    pub name: String,
    pub version: String,
    pub loader: String,
    /// 全局游戏目录；提供者在其下分配实例目录。
    pub game_root: PathBuf,
}

/// 实例仓库错误。损坏文件明确报错，不静默当作空仓库覆盖。
#[derive(Debug)]
pub enum InstanceError {
    Parse {
        path: PathBuf,
        message: String,
    },
    Io {
        path: PathBuf,
        source: std::io::Error,
    },
    Encode(String),
    Invalid(String),
    DuplicateName {
        name: String,
    },
    NotFound {
        id: i32,
    },
    /// 工作线程忙，未接受本次请求。
    Busy,
    /// 工作线程已退出。
    Unavailable,
}

impl fmt::Display for InstanceError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Parse { path, message } => write!(
                f,
                "实例仓库 {} 内容损坏：{}。为避免覆盖数据，本次未做任何修改。",
                path.display(),
                message
            ),
            Self::Io { path, source } => {
                write!(f, "实例仓库 {} 读写失败：{}", path.display(), source)
            }
            Self::Encode(reason) => write!(f, "实例记录无法序列化：{reason}"),
            Self::Invalid(reason) => write!(f, "实例配置无效：{reason}"),
            Self::DuplicateName { name } => write!(f, "已有同名实例：{name}"),
            Self::NotFound { id } => write!(f, "实例 {id} 不存在"),
            Self::Busy => write!(f, "已有实例操作正在执行"),
            Self::Unavailable => write!(f, "实例仓库不可用"),
        }
    }
}

impl std::error::Error for InstanceError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::Io { source, .. } => Some(source),
            _ => None,
        }
    }
}

/// 实例仓库契约：列表、创建记录、重命名。文件 I/O 在调用线程执行。
pub trait InstanceRepository: Send + Sync {
    fn list(&self) -> Result<Vec<InstanceRecord>, InstanceError>;
    fn create(&self, draft: &NewInstance) -> Result<InstanceRecord, InstanceError>;
    fn rename(&self, id: i32, name: &str) -> Result<InstanceRecord, InstanceError>;
}

/// 实例仓库的服务键。
pub struct Instances;

impl ServiceKey for Instances {
    type Service = dyn InstanceRepository;
    const NAME: &'static str = "launcher.instances";
}

/// 一个已探测的 Java 运行时。只保留实际从 java 进程读取到的信息。
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct JavaCandidate {
    pub path: PathBuf,
    pub version: Option<String>,
    pub major: Option<u32>,
    pub vendor: Option<String>,
    pub architecture: Option<String>,
}

/// 单个候选探测失败；不影响其他候选。
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct JavaProbeFailure {
    pub path: PathBuf,
    pub reason: String,
}

/// 检测结果：成功候选与失败原因分开返回。
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct JavaScanResult {
    pub candidates: Vec<JavaCandidate>,
    pub failures: Vec<JavaProbeFailure>,
}

/// Java 检测或探测错误。
#[derive(Debug)]
pub enum JavaError {
    InvalidPath { path: PathBuf, reason: String },
    NotFound { path: PathBuf },
    Spawn { path: PathBuf, message: String },
    Timeout { path: PathBuf, seconds: u64 },
    Exit { path: PathBuf, message: String },
    Parse { path: PathBuf, message: String },
    Busy,
    Unavailable,
}

impl fmt::Display for JavaError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InvalidPath { path, reason } => {
                write!(f, "Java 路径 {} 无效：{reason}", path.display())
            }
            Self::NotFound { path } => write!(f, "找不到 Java：{}", path.display()),
            Self::Spawn { path, message } => {
                write!(f, "无法启动 Java {}：{message}", path.display())
            }
            Self::Timeout { path, seconds } => {
                write!(f, "Java 探测超时（{seconds} 秒）：{}", path.display())
            }
            Self::Exit { path, message } => {
                write!(f, "Java {} 返回失败：{message}", path.display())
            }
            Self::Parse { path, message } => {
                write!(f, "无法解析 Java 输出 {}：{message}", path.display())
            }
            Self::Busy => write!(f, "已有 Java 检测正在执行"),
            Self::Unavailable => write!(f, "Java 检测工作线程不可用"),
        }
    }
}

impl std::error::Error for JavaError {}

/// 本机 Java 检测契约：候选发现与单路径探测分开。
pub trait JavaRuntimeService: Send + Sync {
    /// 从保存路径、JAVA_HOME、PATH 和少量常见目录发现并探测候选。
    /// `cancelled` 由退出流程设置；实现应在候选之间尽早停止。
    fn detect(&self, saved_path: Option<&Path>, cancelled: &AtomicBool) -> JavaScanResult;
    /// 手动路径也走同一探测逻辑。
    fn probe(&self, path: &Path) -> Result<JavaCandidate, JavaError>;
}

/// Java 检测服务的服务键。
pub struct JavaRuntime;

impl ServiceKey for JavaRuntime {
    type Service = dyn JavaRuntimeService;
    const NAME: &'static str = "launcher.java";
}

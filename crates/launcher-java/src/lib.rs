//! 本机 Java 检测提供者（插件 ID \`java-local\`）。
//!
//! 候选路径发现与实际 Java 探测分开：发现只列出保存路径、JAVA_HOME、PATH
//! 和少量常见安装目录；探测真正启动 \`java\` 进程读取属性。单个探测有超时，
//! 不会递归扫描整个磁盘。

use launcher_api::{
    Context, JavaCandidate, JavaError, JavaProbeFailure, JavaRuntime, JavaRuntimeService,
    JavaScanResult, Plugin, PluginDescriptor, RuntimeError, ServiceRequirement,
};
use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Output, Stdio};
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};

const PROBE_TIMEOUT: Duration = Duration::from_secs(5);

fn java_exe() -> &'static str {
    if cfg!(windows) { "java.exe" } else { "java" }
}

pub struct LocalJavaPlugin;

impl LocalJavaPlugin {
    pub fn new() -> Self {
        Self
    }
}

impl Default for LocalJavaPlugin {
    fn default() -> Self {
        Self::new()
    }
}

impl Plugin for LocalJavaPlugin {
    fn descriptor(&self) -> PluginDescriptor {
        PluginDescriptor {
            id: "java-local",
            provides: vec![ServiceRequirement::of::<JavaRuntime>()],
            requires: vec![],
        }
    }

    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
        ctx.provide::<JavaRuntime>(Arc::new(LocalJava))
    }
}

#[derive(Default)]
pub struct LocalJava;

impl JavaRuntimeService for LocalJava {
    fn detect(&self, saved_path: Option<&Path>, cancelled: &AtomicBool) -> JavaScanResult {
        let saved = saved_path.map(normalize_java_path);
        let saved_key = saved.as_ref().map(|path| path_key(path));
        let mut result = JavaScanResult::default();
        let mut seen = HashSet::new();
        for path in discover_candidates(saved.as_deref()) {
            if cancelled.load(Ordering::Relaxed) {
                break;
            }
            let path = normalize_java_path(&path);
            if !seen.insert(path_key(&path)) {
                continue;
            }
            let is_saved = saved_key
                .as_ref()
                .is_some_and(|key| *key == path_key(&path));
            // 发现阶段列出的常见位置大多不存在；只探测实际存在的文件，
            // 但用户已保存的路径即使消失也要报告原因。
            if !is_saved && !path.is_file() {
                continue;
            }
            match probe_path(&path) {
                Ok(candidate) => result.candidates.push(candidate),
                Err(error) => result.failures.push(JavaProbeFailure {
                    path,
                    reason: error.to_string(),
                }),
            }
        }
        result
    }

    fn probe(&self, path: &Path) -> Result<JavaCandidate, JavaError> {
        probe_path(path)
    }
}

fn discover_candidates(saved: Option<&Path>) -> Vec<PathBuf> {
    let mut paths = Vec::new();
    if let Some(path) = saved {
        paths.push(path.to_path_buf());
    }
    if let Some(home) = std::env::var_os("JAVA_HOME") {
        paths.push(PathBuf::from(home).join("bin").join(java_exe()));
    }
    if let Some(value) = std::env::var_os("PATH") {
        for entry in std::env::split_paths(&value) {
            paths.push(if entry.is_file() {
                entry
            } else {
                entry.join(java_exe())
            });
        }
    }
    for root in common_roots() {
        paths.push(root.join("bin").join(java_exe()));
        if let Ok(entries) = std::fs::read_dir(&root) {
            for entry in entries.flatten() {
                let child = entry.path();
                if child.is_dir() {
                    paths.push(child.join("bin").join(java_exe()));
                }
            }
        }
    }
    paths
}

fn common_roots() -> Vec<PathBuf> {
    let mut roots = Vec::new();
    for variable in ["ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"] {
        let Some(base) = std::env::var_os(variable) else {
            continue;
        };
        let base = PathBuf::from(base);
        for vendor in [
            "Java",
            "Eclipse Adoptium",
            "Microsoft",
            "Zulu",
            "BellSoft",
            "Amazon Corretto",
            "Semeru",
        ] {
            roots.push(base.join(vendor));
        }
        if variable == "LOCALAPPDATA" {
            roots.push(base.join("Programs").join("Eclipse Adoptium"));
        }
    }
    if let Some(profile) = std::env::var_os("USERPROFILE") {
        roots.push(PathBuf::from(profile).join(".jdks"));
    }
    roots
}

fn normalize_java_path(path: &Path) -> PathBuf {
    if path.is_file() {
        return path.to_path_buf();
    }
    if path
        .file_name()
        .is_some_and(|name| name.eq_ignore_ascii_case(java_exe()))
    {
        return path.to_path_buf();
    }
    if path.is_dir() {
        if path
            .file_name()
            .is_some_and(|name| name.eq_ignore_ascii_case("bin"))
        {
            return path.join(java_exe());
        }
        return path.join("bin").join(java_exe());
    }
    path.to_path_buf()
}

fn path_key(path: &Path) -> String {
    let path = std::fs::canonicalize(path).unwrap_or_else(|_| path.to_path_buf());
    let text = path.to_string_lossy();
    if cfg!(windows) {
        text.to_lowercase()
    } else {
        text.into_owned()
    }
}

fn probe_path(path: &Path) -> Result<JavaCandidate, JavaError> {
    let path = normalize_java_path(path);
    if !path.is_file() {
        return Err(JavaError::NotFound { path });
    }
    let mut command = Command::new(&path);
    command
        .args(["-XshowSettings:properties", "-version"])
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        command.creation_flags(0x0800_0000);
    }
    let child = command.spawn().map_err(|error| JavaError::Spawn {
        path: path.clone(),
        message: error.to_string(),
    })?;
    let output = wait_with_timeout(child, &path, PROBE_TIMEOUT)?;
    let text = format!(
        "{}\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    parse_output(&path, &text, output.status.success())
}

fn wait_with_timeout(
    mut child: Child,
    path: &Path,
    timeout: Duration,
) -> Result<Output, JavaError> {
    let deadline = Instant::now() + timeout;
    loop {
        match child.try_wait() {
            Ok(Some(_)) => {
                return child.wait_with_output().map_err(|error| JavaError::Spawn {
                    path: path.to_path_buf(),
                    message: error.to_string(),
                });
            }
            Ok(None) => {}
            Err(error) => {
                return Err(JavaError::Spawn {
                    path: path.to_path_buf(),
                    message: error.to_string(),
                });
            }
        }
        if Instant::now() >= deadline {
            let _ = child.kill();
            let _ = child.wait();
            return Err(JavaError::Timeout {
                path: path.to_path_buf(),
                seconds: timeout.as_secs(),
            });
        }
        std::thread::sleep(Duration::from_millis(20));
    }
}

fn parse_output(path: &Path, output: &str, success: bool) -> Result<JavaCandidate, JavaError> {
    let version = property(output, "java.version").or_else(|| version_from_line(output));
    let Some(version) = version else {
        let message = first_line(output).unwrap_or_else(|| "未找到 java.version".into());
        return Err(if success {
            JavaError::Parse {
                path: path.to_path_buf(),
                message,
            }
        } else {
            JavaError::Exit {
                path: path.to_path_buf(),
                message,
            }
        });
    };
    Ok(JavaCandidate {
        path: path.to_path_buf(),
        major: parse_major(&version),
        version: Some(version),
        vendor: property(output, "java.vendor").or_else(|| vendor_from_line(output)),
        architecture: property(output, "os.arch")
            .or_else(|| property(output, "sun.arch.data.model").map(|bits| format!("{bits}-bit"))),
    })
}

fn property(output: &str, key: &str) -> Option<String> {
    output.lines().find_map(|line| {
        let (name, value) = line.trim().split_once('=')?;
        (name.trim() == key).then(|| value.trim().to_string())
    })
}

fn version_from_line(output: &str) -> Option<String> {
    output.lines().find_map(|line| {
        let rest = line.split_once("version \"")?.1;
        Some(rest.split('"').next()?.to_string())
    })
}

fn vendor_from_line(output: &str) -> Option<String> {
    output
        .lines()
        .map(str::trim)
        .find(|line| line.contains("Runtime Environment") || line.starts_with("OpenJDK"))
        .map(str::to_string)
}

fn first_line(output: &str) -> Option<String> {
    output
        .lines()
        .map(str::trim)
        .find(|line| !line.is_empty())
        .map(str::to_string)
}

fn parse_major(version: &str) -> Option<u32> {
    let mut parts = version.split(['.', '-', '_']);
    let first = parts.next()?;
    if first == "1" {
        parts.next()?.parse().ok()
    } else {
        first.parse().ok()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_openjdk_properties() {
        let output = r#"
Property settings:
    java.home = C:\Program Files\Eclipse Adoptium\jdk-21.0.4+7
    java.vendor = Eclipse Adoptium
    java.version = 21.0.4
    os.arch = amd64

openjdk version "21.0.4" 2024-07-16
"#;
        let candidate = parse_output(Path::new("java.exe"), output, true).unwrap();
        assert_eq!(candidate.version.as_deref(), Some("21.0.4"));
        assert_eq!(candidate.major, Some(21));
        assert_eq!(candidate.vendor.as_deref(), Some("Eclipse Adoptium"));
        assert_eq!(candidate.architecture.as_deref(), Some("amd64"));
    }

    #[test]
    fn parses_legacy_version_line_and_major() {
        let output = r#"
java version "1.8.0_402"
Java(TM) SE Runtime Environment (build 1.8.0_402-b06)
Java HotSpot(TM) 64-Bit Server VM (build 25.402-b06, mixed mode)
"#;
        let candidate = parse_output(Path::new("java.exe"), output, true).unwrap();
        assert_eq!(candidate.version.as_deref(), Some("1.8.0_402"));
        assert_eq!(candidate.major, Some(8));
        assert!(candidate.vendor.is_some());
    }
}

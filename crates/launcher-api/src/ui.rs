//! Shell-independent navigation and extension entries. A shell may render these natively.
use launcher_runtime::ContributionKey;
use std::sync::{Arc, atomic::AtomicBool};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MainArea {
    Home,
    Versions,
    Multiplayer,
    Settings,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SettingsSection {
    General,
    Game,
    Java,
    Network,
    Appearance,
    Accounts,
    Extensions,
    About,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum VersionSection {
    Installed,
    Resources,
    Downloads,
}
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum UiRoute {
    Main(MainArea),
    Settings(SettingsSection),
    Versions(VersionSection),
    Account,
    /// The shell must validate the captured identity and revision before acting.
    InstanceSettings {
        instance_id: i32,
        revision: u64,
    },
}
/// Home intentionally has no extension slot; its launch panel remains product-owned.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum UiPlacement {
    Settings,
    Extensions,
    Account,
    Instance,
    Versions,
    Multiplayer,
}

/// Execute on a background worker. Capture immutable target snapshots in the command
/// when it needs an instance; never resolve a mutable 'current instance' while executing.
pub trait UiCommand: Send + Sync {
    fn execute(&self, cancelled: &AtomicBool) -> Result<String, String>;
}
#[derive(Clone)]
pub enum UiAction {
    Navigate(UiRoute),
    Command(Arc<dyn UiCommand>),
}
#[derive(Clone)]
pub struct UiContribution {
    pub title: String,
    pub description: String,
    pub placement: UiPlacement,
    pub action: UiAction,
}
pub struct UiContributions;
impl ContributionKey for UiContributions {
    type Entry = UiContribution;
    const NAME: &'static str = "launcher.ui.entries";
}

# 插件开发上手

本文描述当前已实现 SDK 支持服务提供者、类型化动作贡献、有界后台执行器与完整 Shell 替换。先读[架构](architecture.md)。Rust / Slint 版本以仓库锁定为准；所有插件都在编译期链接，配置只选择已经注册的工厂。

## 1. 从现有产品扩展开始

真实实现位于 [LocalToolsPlugin](../src/shell_plugins.rs)，产品在 [composition.rs](../src/composition.rs) 注册工厂：

```rust
catalog.register("local-tools", |_| {
    Ok(Box::new(lxmcl::shell_plugins::LocalToolsPlugin))
})?;
```

[product.toml](../product.toml) 启用它：

```toml
[[plugins]]
id = "tools"
provider = "local-tools"
```

`id` 是产品里的插件实例身份；`provider` 必须匹配工厂。增加第三方 crate 时为产品增加 Cargo 依赖，在装配处注册其工厂，再配置并构建；无需改 Runtime 内核。

依赖必须完整写入 descriptor。Context 已按服务名称和类型检查：get 只允许 requires 或本作用域已注册的服务，provide 只允许 provides；遗漏声明立即报错。关闭后的 Context 不再允许发现服务，已经持有的 Arc 不会失效。

`LocalToolsPlugin` 实现 `Plugin`，其 `descriptor` 声明 `requires: vec![ServiceRequirement::of::<Instances>()]`、不提供服务。`apply(&Context)` 只登记贡献并获取所需服务，不执行磁盘扫描。下面是它使用的公开贡献 API，可直接用于插件的 `apply`：

```rust
use launcher_api::{
    Context, RuntimeError, SettingsSection, UiAction, UiContribution,
    UiContributions, UiPlacement, UiRoute,
};

fn contribute_java(ctx: &Context) -> Result<(), RuntimeError> {
    ctx.contribute::<UiContributions>("my-product.java", 20, UiContribution {
        title: "本机 Java".into(),
        description: "检测运行时，验证并保存 Java 路径".into(),
        placement: UiPlacement::Settings,
        action: UiAction::Navigate(UiRoute::Settings(SettingsSection::Java)),
    })
}
```

贡献 ID 在同扩展点内唯一，建议产品 / 插件命名空间；`order` 决定升序顺序，同序保持注册顺序。贡献跟随 Context 作用域清理，关闭后不能继续注册。`host.contributions::<UiContributions>()?` 返回带 `id` / `owner` / `order` / `value` 的类型化快照。

当前默认 Slint Shell 把贡献统一展示在「设置 → 扩展」目录，`placement` 是标明意图的语义位置。各页面槽位尚未接入；注册 Versions 或 Multiplayer 不表示它现在会出现在对应页面。首页不开放贡献位；插件不能创建额外一级 Tab。默认控件和动效遵守[最终 UI 规范](../.agents/skills/lxmcl-ui/references/REDESIGN.md)。

## 2. 注册一个后台命令

`UiAction::Command` 接受 `Arc<dyn UiCommand>`。命令只接收取消标志并返回文本结果，不持有 Slint 窗口。以下为现有目录检查实现的精简形式：

```rust
use launcher_api::{InstanceRepository, UiCommand};
use std::sync::{Arc, atomic::{AtomicBool, Ordering}};

struct InspectDirectories(Arc<dyn InstanceRepository>);

impl UiCommand for InspectDirectories {
    fn execute(&self, cancelled: &AtomicBool) -> Result<String, String> {
        let records = self.0.list().map_err(|error| error.to_string())?;
        let mut lines = Vec::new();
        for record in records.iter().take(100) {
            if cancelled.load(Ordering::Acquire) {
                return Err("目录检查已取消".into());
            }
            let state = match std::fs::metadata(&record.game_directory) {
                Ok(meta) if meta.is_dir() => "目录存在".to_string(),
                Ok(_) => "路径不是目录".to_string(),
                Err(error) => format!("无法访问：{error}"),
            };
            lines.push(format!("{}：{state}", record.name));
        }
        Ok(lines.join("\n"))
    }
}
```

在已声明 `Instances` 依赖的插件 `apply` 中注册（与上面的实现共用 `Arc` 导入）：

```rust
use launcher_api::{Instances, UiAction, UiContribution, UiContributions, UiPlacement};

ctx.contribute::<UiContributions>("my-product.inspect-directories", 30, UiContribution {
    title: "检查实例目录".into(),
    description: "只读检查，不创建文件或判断是否能启动".into(),
    placement: UiPlacement::Extensions,
    action: UiAction::Command(Arc::new(InspectDirectories(ctx.get::<Instances>()?))),
})?;
```

上述只读清单操作有意读取仓库的最新列表。若命令作用于一个具体实例，创建命令时捕获其唯一 ID、修订和目录等不可变快照；执行时不要重新取“当前实例”。带实例设置路由使用 `UiRoute::InstanceSettings { instance_id, revision }`，Shell 必须校验目标与修订，拒绝过期目标，不能静默切换作用对象。

默认适配器 [shell_extensions.rs](../src/shell_extensions.rs) 将命令送入 `BoundedWorker`：一个后台线程、等待队列容量 1（另可有一项正在执行），UI busy 禁止重复提交。完成经 `slint::invoke_from_event_loop` 更新状态；命令 panic 显示失败。关闭使用 `ShutdownPolicy::Cancel`，丢弃等待项并通知当前命令协作取消。检查标志不能强制中断阻塞 I/O；长操作需自行设计超时与取消点。

如果新 Shell 需要自己的工作线程，公开 API 是 `BoundedWorker::spawn`、`sender` / `try_submit`、`status`、`cancel` 和 `shutdown`；队列满 / 已关闭由 `SubmitError::Busy(job)` / `Closed(job)` 返回被拒任务。写入操作可用 `ShutdownPolicy::Drain` 执行已接受项再退出。不要在 UI 回调等待线程结束，也不要把这个执行器当作持久下载任务系统。

## 3. 提供或替换业务服务

当前服务包括 `SettingsService`、`InstanceRepository`、`JavaRuntimeService`、`ApplicationService` 和 `ApplicationShell`。新提供者实现 `launcher-api` 契约，使用 Runtime 的 `Plugin` 和 `Context`；API 已重新导出插件所需的 Context / Plugin / Descriptor / 服务键类型。

完整 TOML 设置提供者见 [custom-settings](../examples/custom-settings/src/lib.rs)，它不依赖 Slint。保留其读写错误与路径处理方式：文件损坏要报错，不当首次运行覆盖；`apply` 只注册服务，I/O 由后台调用服务方法。依赖协作通过 `Context::get::<K>()`，不调用其他提供者的具体实现。

工厂通过 `let config: YourConfig = config.decode()?;` 取得强类型配置；相对路径在产品装配处解析。不要同时启用两个提供同一服务键的插件。替换 provider 后消费者只要仍消费相同契约，无需修改窗口业务调用。

独立示例命令：

```powershell
cargo run --manifest-path examples/minimal-launcher/Cargo.toml --locked
```

该示例当前演示 JSON / TOML 设置提供者和独立窗口 / 设置桥，未迁移为新版 Shell provider 示例。示例是独立 Cargo 工作区，使用自己的锁文件。

## 4. 调用应用用例

实例重命名通过 `Application` 服务调用；插件在 requires 中声明它，由 `Context::get::<Application>()` 取得。具体实现见 [ApplicationPlugin](../src/application/mod.rs)，默认 Shell 的消费方式见 [RenamePresentation](../src/presentation/rename.rs)。

`ApplicationService::rename` 接收 `RenameInstance { instance_id, expected_revision, name }` 与完成回调，返回 `RequestId` 或 `ApplicationSubmitError`。Busy / Closed 表示请求没有被接受，也不会调用完成；已接受请求在后台完成，回调中的 `RenameCompleted` 保留请求、目标、类型化结果和冲突后的可选最新记录。回调不在 UI 线程，不能直接改 Slint 模型。

应用插件拥有重命名 worker，退出排空已接受写入。关闭表单不取消操作；同一目标重新打开表单也属于另一会话。冲突时展示最新记录并保留草稿，用户核对后重新提交，不能自动换成当前选中实例。

仓库提供者的 `rename(id, expected_revision, name)` 必须在同一次串行读改写中核对修订，冲突返回 `InstanceError::RevisionConflict`。默认 JSON 提供者复用原子文件替换和同一实例的写入锁；业务扩展应调用应用服务，不各自构造仓库写文件。

## 5. 替换整个 Shell

API 的 `ApplicationShell` 不包含 Slint 类型。下面是可编译接口形状的最小实现，窗口逻辑由你的 `run` 提供：

```rust
use launcher_api::{
    ApplicationShell, Context, Plugin, PluginDescriptor, RuntimeError,
    ServiceRequirement, Settings, Shell,
};
use launcher_runtime::PluginHost;
use std::{error::Error, sync::Arc};

struct MyShell;
impl ApplicationShell for MyShell {
    fn run(&self, host: &PluginHost) -> Result<(), Box<dyn Error>> {
        let _settings = host.service::<Settings>()?;
        // 在主线程创建自己的窗口、后台桥并运行事件循环；返回前收尾所有桥。
        Ok(())
    }
}

struct MyShellPlugin;
impl Plugin for MyShellPlugin {
    fn descriptor(&self) -> PluginDescriptor {
        PluginDescriptor {
            id: "my-shell",
            provides: vec![ServiceRequirement::of::<Shell>()],
            requires: vec![ServiceRequirement::of::<Settings>()],
        }
    }
    fn apply(&mut self, ctx: &Context) -> Result<(), RuntimeError> {
        ctx.provide::<Shell>(Arc::new(MyShell))
    }
}
```

在产品装配增加 `catalog.register("my-shell", |_| Ok(Box::new(MyShellPlugin)))?;`，把产品已有 shell 条目的 provider 改为：

```toml
[[plugins]]
id = "shell"
provider = "my-shell"
```

替换现有条目，不同时启用两份 Shell 提供者。主入口无需按 Shell 名写分支，继续调用 `host.service::<Shell>()?.run(&host)`。上面是接口骨架，`Ok(())` 不创建窗口；真正替换需实现自己的原生窗口、贡献呈现、焦点、导航和后台桥。可以复用 API / Runtime / 提供者而不带 LXMCL 的控制器与品牌素材。产品仍负责 Shell 返回后 `host.shutdown()`；Shell 不负责销毁宿主。

## 6. 验证与排障

仅选择受改动影响的检查，不要求每次运行完整清单：

```powershell
cargo fmt --check
cargo test --locked -p launcher-runtime -p launcher-api
cargo clippy --locked --all-targets -- -D warnings
cargo build --locked
```

Runtime 改动验证相关注册、依赖和关闭行为；扩展适配改动在窗口核对对应导航、结果和退出。实例动作要验证切换目标或修订后拒绝旧动作。编译通过不能代替真实窗口、DPI 或性能检查；在交付说明 / PR 中报告实际检查，临时输出放 `target/`。

未知 provider 表示工厂未注册或拼写错误；重复服务表示存在两个相同能力提供者；缺依赖 / 环检查 descriptor；贡献重复 ID 或扩展点类型不符检查注册键；配置解码失败检查 provider 配置类型。改变产品 ID 改变数据命名空间，不会自动迁移旧数据。

插件与宿主同进程同权限。当前没有运行时安装、DLL / WASM 加载、热重载、沙箱或任意 Slint 控件动态注入。真实认证、下载 / 安装、游戏启动与完整诊断仍需服务实现；不要将目录存在、实例记录或 Java 版本探测称为完整启动准备成功。

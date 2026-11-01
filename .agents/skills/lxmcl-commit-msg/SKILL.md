---
name: lxmcl-commit-msg
description: Generates a single-line English commit message for LXMCL from staged changes and recent commit style, and guides commits after completed feature modules. Use when the user asks for a commit message, says commit msg or git 提交信息, requests a commit, or a feature module is complete and ready to commit.
---

# LXMCL 提交规范

## 触发与提交时机

- 用户要求 commit message、提交说明、根据暂存区生成一句标题时，使用本 skill。
- 每完成一个功能模块并通过必要验证，提交一次对应改动；不必再次询问是否提交。
- 一个提交包含该模块实现、直接相关的测试、配置及文档；避免按文件拆碎提交，也不累积多个无关模块。
- 只请求提交信息时，只生成信息；只读审查、讨论和没有文件改动的任务不创建提交。
- 用户明确要求暂不提交时遵从；提交到本地即可，推送需有用户授权。

## 读取实际改动

生成标题前，先执行：

```powershell
git status --short
git diff --cached --stat
git diff --cached
git log --oneline -10
```

- 默认只根据 staged changes 生成标题；不能只凭文件名或上轮说明猜测内容。
- 只有用户明确要求包含未暂存内容，或正在准备提交本轮模块时，才额外读取 `git diff` 与相关未跟踪文件。
- 暂存区为空时，不编造标题：说明当前没有 staged changes，无法据此生成准确提交信息。
- 自动提交模块时，先检查状态与差异，再明确暂存本模块相关路径或片段，随后重新读取暂存区。
- 保留已有无关修改及暂存状态；不要使用 `git add .`、`git add -A` 或清空暂存区来整理本次提交。
- 若暂存区混有无关工作，隔离本模块提交；不要顺手提交、撤销或改写其他工作。

## 标题格式

优先采用仓库最近提交的语言风格与结构，默认格式：

```text
<type>(<scope>): <summary>
<type>: <summary>
```

- 一行英文，使用祈使语气，如 `add`、`fix`、`update`；summary 首字母小写，末尾无句号。
- 简洁描述本次提交的主要目的，覆盖全部暂存改动的整体语义，不逐文件罗列。
- 不使用 `WIP`、`misc`、`update files` 等空泛标题。
- 不强行统一已有自然写法；近期风格与用户明确要求优先。
- 不默认添加 `[skip ci]`；仅在用户要求或明确适用的仓库自动化约定下使用。

### type

- `feat`：新增或扩展功能。
- `fix`：修复缺陷、错误或安全漏洞。
- `build`：构建系统或依赖变动。
- `chore`：不修改源码和测试的辅助工具、配置等维护。
- `ci`：持续集成、部署配置或脚本。
- `docs`：仅文档、注释或使用指南变更。
- `perf`：以性能改进为目的的修改。
- `refactor`：不新增功能、不修复缺陷的结构调整。
- `revert`：撤销既有提交。
- `style`：不改变代码含义的格式调整。
- `test`：仅添加、修正或重构测试。

### scope

- 跨 Rust / Slint 的业务改动采用功能域，如 `instance`、`settings`、`account`、`launch`。
- 仅涉及某层时使用结构域，如 `runtime`、`frontend`、`backend`、`ci`、`script`。
- 文档可用 `readme`、`plugins` 等范围；依赖维护可用实际依赖名。
- 多个无关范围难以概括时，省略 scope；不套用其他项目的 Tauri / Next.js 等术语。

示例：

```text
feat(instance): persist instance creation and renaming
fix(settings): flush pending saves before shutdown
refactor(runtime): centralize plugin cleanup
docs(plugins): clarify service registration rules
```

## 验证与完成

- 按改动影响执行必要验证；复用已有检查，不为提交补大量测试或新增示例。
- 纯规范、文档修改检查格式和链接即可，不要求 Rust 全量构建。
- 提交前确认 diff 与标题一致，提交后检查提交结果和工作区状态；不默认 amend 既有提交。
- 权限、工具或 Git hook 阻止提交时，如实说明；不绕过 hook，不宣称提交成功。
- 仅生成 message 的任务：最终只输出一行标题，不加引号、代码块或解释；暂存区为空等无法生成的情况除外。
- 实际执行提交的任务：完成后简短报告提交哈希、标题及必要验证结果。

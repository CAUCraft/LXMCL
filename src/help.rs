//! Offline, task-oriented help content shared by search and contextual help.
pub struct Article {
    pub id: &'static str,
    pub title: &'static str,
    pub keywords: &'static str,
    pub summary: &'static str,
    pub steps: &'static [(&'static str, &'static str)],
    pub caution: &'static str,
    pub action: &'static str,
    pub page: i32,
}

pub const ARTICLES: &[Article] = &[
    Article {
        id: "start",
        title: "第一次玩，从哪里开始？",
        keywords: "新手 教程 开始 入门 实例 创建",
        summary: "先选玩法，再确认账号和存放位置，最后检查环境。",
        steps: &[
            (
                "选择玩法",
                "进入社团服使用服务器要求的版本；自己玩可以选择原版或整合包。每个“我的游戏”都是独立实例，分别保存模组与存档。",
            ),
            (
                "准备游戏与身份",
                "没有游戏时，在“我的游戏”创建。再在“账号与装扮”选择身份。本地预览账号不能证明真实服务器的登录资格。",
            ),
            (
                "检查后再启动",
                "打开启动检查。需要补充环境时先核对准备清单；所有必需项通过后才允许继续。返回上一步不会丢失本次向导填写的路径。",
            ),
        ],
        caution: "当前可以体验选择、检查、准备和启动的交互；不会下载 Minecraft、登录账号或启动真实游戏。",
        action: "打开首次游玩向导",
        page: 9,
    },
    Article {
        id: "club",
        title: "怎样进入社团服务器？",
        keywords: "社团 民航 服务器 联机 入服 caucraft",
        summary: "先匹配服务器的版本、加载器和身份，再准备环境。",
        steps: &[
            (
                "从社团入口开始",
                "打开“社团联机 → 准备并进入”。检查页面会写明目标服务器和当前游戏，避免把资源装到另一个游戏。",
            ),
            (
                "处理不兼容",
                "如果版本或加载器不同，选择兼容游戏，或创建独立的社团游戏。不要直接覆盖已有存档所在的实例。",
            ),
            (
                "核对更新清单",
                "正式接入后将列出新增、替换和保留的资源。下载失败应恢复原任务；社团认证方式与服务器地址由社团实际服务提供。",
            ),
        ],
        caution: "示例中的 1.21.1 / Fabric 不是已确认的社团服务器配置；本地账号也不等于社团认证账号。",
        action: "查看社团联机",
        page: 2,
    },
    Article {
        id: "account",
        title: "该选哪种账号？登录过期怎么办？",
        keywords: "微软 Microsoft 登录 离线 皮肤站 认证 过期 角色",
        summary: "区分微软、社团／第三方认证和本地身份。",
        steps: &[
            (
                "选择服务器接受的身份",
                "微软账号用于相应的正版认证；社团／第三方账号要匹配服务器的认证服务。本地身份不会自动获得在线服务资格。",
            ),
            (
                "在账号页管理",
                "从“账号与装扮”选择当前身份。正式登录将通过独立认证流程完成，界面模型只接收状态和角色，不保存密码或访问令牌。",
            ),
            (
                "过期后重新认证",
                "保留原游戏和入服目标，完成认证后重新检查。不自动降级为离线身份，不把认证失败当作资源缺失。",
            ),
        ],
        caution: "当前“添加本地账号”只创建演示身份；微软、社团认证和皮肤站尚未连接。",
        action: "管理账号与装扮",
        page: 5,
    },
    Article {
        id: "java",
        title: "提示缺少 Java，应该装哪个？",
        keywords: "java jdk zulu 环境 不兼容 启动 闪退",
        summary: "让游戏和加载器确定所需版本，确认位置后再安装。",
        steps: &[
            (
                "先看匹配结果",
                "Java 是运行 Minecraft 的环境。正式实现会综合游戏版本、加载器和系统架构匹配，不能对所有游戏都固定使用 Java 21。",
            ),
            (
                "检查准备清单",
                "缺少环境时点击“预览准备清单”，核对 Zulu 发行版、安装位置与预计下载量。未知大小显示“待估算”。",
            ),
            (
                "准备后重新检查",
                "执行后检查结果更新为就绪，再进行启动。已有可用 Java 的机器应优先复用，不重复下载。",
            ),
        ],
        caution: "当前只校验路径格式并模拟准备结果，不扫描、不安装、不修改系统 Java。",
        action: "查看 Java 与存储设置",
        page: 6,
    },
    Article {
        id: "storage",
        title: "游戏和 Java 可以放到其他盘吗？",
        keywords: "目录 磁盘 路径 c盘 d盘 e盘 安装位置 空间 权限",
        summary: "游戏和 Java 分别选择文件夹，中文和空格都可以保留。",
        steps: &[
            (
                "填写完整目录",
                "例如 E:\\Games\\LXMCL。使用专门的文件夹，避免直接选择磁盘根目录。向导中可以单独指定 Java 目录。",
            ),
            (
                "检查空间与权限",
                "路径格式正确不代表文件夹可写。正式服务还需检查可用空间、权限、目录占用和迁移冲突。",
            ),
            (
                "分清新安装与迁移",
                "改变默认目录不会自动迁移现有游戏。迁移应单独展示源目录、目标目录和校验结果，并在成功前保留原文件。",
            ),
        ],
        caution: "本次向导只保留会话草稿，不创建文件夹、不移动文件；退出启动器后草稿重置。",
        action: "修改存储偏好",
        page: 6,
    },
    Article {
        id: "resources",
        title: "模组、资源包和皮肤分别装在哪里？",
        keywords: "mod mods 模组 材质 资源包 光影 皮肤 披风 导入 更新 依赖 整合包",
        summary: "先选资源类型，再确认要安装到哪个游戏。",
        steps: &[
            (
                "分清资源类型",
                "模组改变游戏内容；资源包（材质包）改变贴图和声音；光影改变渲染效果。皮肤和披风属于账号装扮，整合包用于创建独立游戏。",
            ),
            (
                "保留兼容筛选",
                "在资源中心选定目标游戏，核对版本、加载器和依赖。必要依赖应一起准备；不能保证所有运行时冲突都能提前发现。",
            ),
            (
                "管理已安装内容",
                "到“我的游戏”启停或管理已安装资源。自行导入的文件标注“用户自行导入”，没有可核验的线上来源时不提供在线更新。",
            ),
        ],
        caution: "当前资源目录和下载任务为演示。真实文件导入、在线更新及依赖扫描尚未接入。",
        action: "打开资源中心",
        page: 3,
    },
    Article {
        id: "network",
        title: "下载很慢、超时或停住了怎么办？",
        keywords: "网络 代理 proxy 镜像 bmclapi 下载 慢 超时 重试 断点 ip ipv6",
        summary: "保留任务和目标，先定位失败服务，再尝试恢复。",
        steps: &[
            (
                "查看失败阶段",
                "先区分等待、下载、校验和安装。暂停不是失败；校验和安装期间不应显示虚假的下载速度。",
            ),
            (
                "按服务测试连接",
                "下载站、认证服务和社团服务器应分别检查。某个下载镜像失败，不代表账号服务或整个网络不可用。",
            ),
            (
                "保留原任务再重试",
                "先重试失败项，再按需切换下载源。启动器下载代理与游戏进程代理影响不同；不要通过重复点击安装制造重复任务。",
            ),
        ],
        caution: "当前网络诊断是演示结果；“下载后失败”演示场景可验证保留清单与再次执行，不会发出网络请求。",
        action: "打开诊断与工具",
        page: 7,
    },
    Article {
        id: "launch",
        title: "启动被拦住或游戏闪退怎么办？",
        keywords: "启动失败 崩溃 日志 错误 检查 修复 未知 闪退",
        summary: "看具体失败项，保留游戏上下文，不盲目重装。",
        steps: &[
            (
                "先处理明确问题",
                "检查页区分“就绪”“需处理”“被阻止”“未确认”。未确认项需要继续检查，不能直接当作通过。",
            ),
            (
                "修改后重新检查",
                "切换游戏、账号、目录或配置会使旧结果失效。准备清单必须对应同一目标，不能在切换目标后沿用旧清单。",
            ),
            (
                "保留错误和日志",
                "接入真实服务后应先给出问题摘要与建议，再展开原始日志。导出诊断包前显示包含内容，并去掉令牌等敏感字段。",
            ),
        ],
        caution: "错误码本身通常不足以确定崩溃原因。当前不会读取、上传或导出本机日志。",
        action: "打开诊断与工具",
        page: 7,
    },
];

pub fn search(query: &str) -> Vec<usize> {
    let tokens: Vec<_> = query.split_whitespace().map(str::to_lowercase).collect();
    ARTICLES
        .iter()
        .enumerate()
        .filter_map(|(i, article)| {
            let text = format!("{} {} {}", article.title, article.keywords, article.summary)
                .to_lowercase();
            tokens.iter().all(|token| text.contains(token)).then_some(i)
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn searches_chinese_aliases_and_case_insensitive_terms() {
        assert!(search("JAVA").iter().any(|&i| ARTICLES[i].id == "java"));
        assert_eq!(ARTICLES[search("材质")[0]].id, "resources");
        assert!(search("不存在的词").is_empty());
        assert_eq!(search("").len(), ARTICLES.len());
    }
}

# Independent Real-Project Pilot

ECP 的价值不能由 ECP 仓库自己的单元测试证明。真实试点必须同时覆盖两个与 ECP 实现独立、会持续演进的产品仓库：一个真正的新项目和一个已经持续开发的旧项目。两条轨道都必须包含新 task、新维护者和会改变业务/架构事实的真实需求。本文件定义试点方法和退出标准，不声称试点已经完成。

## 试点目标

验证 ECP 是否真的降低以下长期成本，而不是增加一套需要持续手工维护的文档：

- 新 Agent/新人恢复项目目的、关键规则、组件边界、历史决策和未知项所需时间；
- AI 修改越出需求范围、破坏业务/架构不变量或使用过期验证结果的频率；
- 产品语义变化后 Project Truth 的 freshness；
- 用户在不写代码、不运行 ECP CLI 的情况下完成真实迭代的比例；
- 被阻断的问题中真阳性、误报、无法判断和绕过原因；
- 产品最初未说明的状态/错误/取消/超时/权限/数据/兼容问题中，有多少在首次写入前被 Requirement ledger 暴露并形成明确决定；
- selector 省略了多少无关 Gate、每次计划耗时是否下降，以及是否发生因 path ownership/selector 错误而漏验；
- 出错后定位引入 Change、恢复旧事实和制定回滚/补偿的时间。

## 试点组合与选择

两个项目共同必须满足：

- 与 ECP 实现仓库独立，并有真实产品目标和持续需求；
- 已有可安全本地运行的最小测试/类型/Schema/build Gate；
- 至少包含一个业务规则、一个数据或接口合同、一个组件边界和一个真实 Unknown；
- 允许创建仓库内 `.ecp` Project Pack，但不要求更改 CI、发布或生产系统；
- 由项目所有者确认哪些产品事实是准确的，Agent 不得凭代码猜测业务承诺。

新项目轨道还必须满足：

- 试点开始时没有既有 ECP authority history，产品骨架仍足够小，可以观察 seed/established Project Truth 是如何形成的；
- 记录初始化时哪些事实来自用户确认、正式文档、Schema、测试或代码，哪些必须保留为 Unknown；
- 验证 ECP 不会为了启用而凭空创造产品承诺、测试命令、兼容要求或架构边界。

旧项目轨道还必须满足：

- 具有真实提交历史、现有架构/数据/API/事件合同和持续维护需求；
- 在试点期间至少一次保留自然存在或由项目所有者明确设置的无关 dirty diff，用于验证 Agent 不会覆盖、清理、吸收或重写用户工作；若 canonical Workspace 当前 clean，不得由 Agent 擅自修改产品源码来制造测试条件；
- 记录代码、Schema、测试和正式项目文档之间的已知冲突，并把无法由证据解决的产品事实保留为 Unknown；
- 验证 Project Pack 只记录未来维护者不能从代码可靠恢复的目的、Capability、Invariant、Component、Decision、Contract 和 Unknown，而不是建立第二套代码说明书。

## Plugin 宿主预检

在任何真实产品仓库发生首次 ECP 写入之前，必须先在独立、可丢弃的 Git Workspaces 中完整执行 [Codex Plugin host-routing evaluation](plugin-host-evaluation.md)。四个 Skill 的 direct、indirect、incomplete、negative 和 edge cases 必须在 fresh Codex Desktop tasks 中达到该协议的重复运行标准。

错误 Skill、误 enable/disable、enabled 项目写入前漏掉 status probe、task-level bypass、ambient/repository-built Core、无依据成功或任何未授权写入都阻断真实试点。不能把正式新项目或旧项目当成第一轮 Plugin 路由调试环境，也不能用 Skill 文件的静态字符串测试替代宿主证据。

## 四个阶段

### A. 基线恢复

在不读取旧聊天的 task 中，让新 Agent 只读检查仓库并回答产品目的、主要 Capability、关键 Invariant、Component、Contract、Decision、Unknown 和验证边界。记录耗时、错误和必须向项目所有者提问的内容，然后建立诚实的 seed/onboarding Project Truth。新项目从最小骨架和已确认产品目标建立事实；旧项目必须同时恢复 branch、HEAD、dirty diff、历史合同和兼容边界，并证明无关用户修改没有被吸收或清理。

### B. 普通迭代

每个项目分别完成至少三个不修改 Project Truth 的真实 Change，例如聚焦 bug、UI 状态链或小功能。每次先列出与需求真正相关的正常、加载、空态、成功、失败、重试、取消、超时、权限、并发、持久化、兼容、可访问性状态；不相关项明确 N/A，无法安全延后的产品选择在首次写入前询问，不能由 Agent 补成“默认”。它们可以是 `PRESERVED`，也可以是 durable truth 仍准确的零-delta `CHANGED`；检查结果是否与 expected changes/preservations 一致、Gate 是否适用、用户是否需要接触 CLI/opaque 字段，以及旧 Evidence 是否在源码变化后正确 stale。

### C. 语义迭代

每个项目分别完成至少三个必须更新 Project Truth 的真实 Change，组合起来覆盖：

- 业务规则或用户行为；
- 数据/API/事件/持久化合同；
- 架构边界、权限、安全、兼容或运行约束。

每次都验证 existing fact 必须被 Impact 精确点名、新发现不能借 unknown 扩大既有范围、protected delta 被产品语言展示并确认、truth acceptance 与 assessment 原子记录、后续 Gate/Evidence 绑定新 truth。

### D. 新人接手与故障演练

分别删除两个项目的试点 task 聊天上下文，让没有参与前述 Change 的维护者或新 Agent 各接手一个真实需求。另做至少八次演练，并确保新旧项目都承担演练：越界修改、未声明但已映射的 component 修改、完全未映射路径、候选 truth 漂移、Gate 通过但 semantic/Requirement result 缺失或 UNKNOWN、旧 Evidence 重放、取消后保留源码却尝试建立新 baseline、GateRun 持有进程被杀后由新 task 接手。旧项目还必须复验无关 dirty diff 保护；新项目必须复验 seed Unknown 不会被后续 Agent 静默收窄。记录是否被阻断、`gate history` 是否足够区分 live/abandoned/terminal 状态、诊断是否足够、恢复是否需要直接编辑 authority state（正确答案必须是不需要也不允许）。

## 每次 Change 的记录

只记录对产品决策有用的度量，不保存完整聊天：

| 字段 | 含义 |
| --- | --- |
| pilot_track | `greenfield` 或 `established` |
| request_type | ordinary、semantic、recovery、handoff |
| user_cli_actions | 用户手工执行 ECP CLI 的次数，目标为 0 |
| handwritten_code | 用户手写产品代码的次数，目标为 0 |
| recovery_minutes | 新 task 到能正确描述影响面的时间 |
| truth_questions | 必须由人回答的产品问题数量 |
| silent_unknowns_found | 首次写入前发现的原需求未决 material 问题数量 |
| requirement_rework | Requirement 决议后又因理解错误返工的项数 |
| deferred_revisited | 到达 revisit condition 时被重新处理的安全延期项数 |
| blockers | ECP blocker code 与是否真阳性 |
| planned_gates | Required Gate IDs、tier 与选择依据 |
| omitted_gates | 被 selector 正确省略的无关 Gate 数量 |
| validation_seconds | Required Gates 的实际总耗时 |
| semantic_result | PRESERVED、CHANGED、UNKNOWN |
| truth_freshness | 完成时 Project Truth 是否与已知现实一致 |
| escaped_issue | 完成后发现但未被 Impact/Gate/review 捕获的问题 |
| rollback_minutes | 若演练失败，定位和恢复所需时间 |

## 退出标准

Phase 1B 只有在全部条件满足时才完成：

1. Plugin 宿主预检的全部用例在规定的独立 fresh-task 运行中通过，且没有被隐藏或覆盖的失败记录。
2. 一个新项目和一个旧项目都完整执行 A–D，每个项目不少于八个真实 Change，两个项目合计不少于二十个真实 Change；其中每个项目至少三个普通 Change 和三个语义 Change。
3. 全部普通使用中用户手工 ECP CLI 次数为 0，用户手写产品代码不是完成条件。
4. 新旧项目的接手者都能仅依赖当前仓库与 accepted authority state 正确指出主要事实、未知项和本次影响，没有依赖旧聊天中的隐藏约定。
5. 旧项目所有无关 dirty diff 均被保留；新项目没有把 seed Unknown、Agent 偏好或代码猜测升级成未经确认的产品事实。
6. 所有故障演练 fail closed；没有用旧 Evidence、空 Gate、宽泛 unknown 或新弱规则获得 PASS。
7. 所有 material contract item 都有 Requirement decision/result；没有未决项在首次写入后才被偷偷补成 Agent 默认，外部验证没有被本地 PASS 冒充。
8. 每个已知 durable semantic change 都更新了 Project Truth；没有把代码细节整批复制成第二套文档。
9. path ownership 对实际修改完整；selector 省略项经人工抽查确实无关，显式 Invariant/Requirement Gate 从未被过滤。
10. 误报、人工确认负担和验证耗时被记录并可接受；不允许删除关键边界只为改善数字，也不允许用无差别全量流程掩盖 selector 配置质量。
11. 两条轨道合计至少一次真实错误能通过 Change/history 定位并完成恢复或补偿方案。

若任一条件失败，应修改 ECP 的 schema、Adapter 或 workflow 后重做相关阶段；不得把试点失败解释为“用户不会写提示词”。

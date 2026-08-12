# Independent Real-Project Pilot

ECP 的价值不能由 ECP 仓库自己的单元测试证明。真实试点必须发生在至少一个独立、会持续演进的产品仓库中，并且包含新 task、新维护者和会改变业务/架构事实的真实需求。本文件定义试点方法和退出标准，不声称试点已经完成。

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

## 试点选择

项目必须满足：

- 与 ECP 实现仓库独立，并有真实产品目标和持续需求；
- 已有可安全本地运行的最小测试/类型/Schema/build Gate；
- 至少包含一个业务规则、一个数据或接口合同、一个组件边界和一个真实 Unknown；
- 允许创建仓库内 `.ecp` Project Pack，但不要求更改 CI、发布或生产系统；
- 由项目所有者确认哪些产品事实是准确的，Agent 不得凭代码猜测业务承诺。

## 四个阶段

### A. 基线恢复

在不读取旧聊天的 task 中，让新 Agent 只读检查仓库并回答产品目的、主要 Capability、关键 Invariant、Component、Contract、Decision、Unknown 和验证边界。记录耗时、错误和必须向项目所有者提问的内容，然后建立诚实的 seed/onboarding Project Truth。

### B. 普通迭代

完成至少三个不修改 Project Truth 的真实 Change，例如聚焦 bug、UI 状态链或小功能。每次先列出与需求真正相关的正常、加载、空态、成功、失败、重试、取消、超时、权限、并发、持久化、兼容、可访问性状态；不相关项明确 N/A，无法安全延后的产品选择在首次写入前询问，不能由 Agent 补成“默认”。它们可以是 `PRESERVED`，也可以是 durable truth 仍准确的零-delta `CHANGED`；检查结果是否与 expected changes/preservations 一致、Gate 是否适用、用户是否需要接触 CLI/opaque 字段，以及旧 Evidence 是否在源码变化后正确 stale。

### C. 语义迭代

完成至少三个必须更新 Project Truth 的真实 Change，分别覆盖：

- 业务规则或用户行为；
- 数据/API/事件/持久化合同；
- 架构边界、权限、安全、兼容或运行约束。

每次都验证 existing fact 必须被 Impact 精确点名、新发现不能借 unknown 扩大既有范围、protected delta 被产品语言展示并确认、truth acceptance 与 assessment 原子记录、后续 Gate/Evidence 绑定新 truth。

### D. 新人接手与故障演练

删除试点 task 的聊天上下文，让没有参与前述 Change 的维护者或新 Agent 接手一个真实需求。另做至少八次演练：越界修改、未声明但已映射的 component 修改、完全未映射路径、候选 truth 漂移、Gate 通过但 semantic/Requirement result 缺失或 UNKNOWN、旧 Evidence 重放、取消后保留源码却尝试建立新 baseline、GateRun 持有进程被杀后由新 task 接手。记录是否被阻断、`gate history` 是否足够区分 live/abandoned/terminal 状态、诊断是否足够、恢复是否需要直接编辑 authority state（正确答案必须是不需要也不允许）。

## 每次 Change 的记录

只记录对产品决策有用的度量，不保存完整聊天：

| 字段 | 含义 |
| --- | --- |
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

1. 至少一个独立项目完成 A–D，且不少于六个真实 Change。
2. 全部普通使用中用户手工 ECP CLI 次数为 0，用户手写产品代码不是完成条件。
3. 新人仅依赖当前仓库与 accepted authority state 能正确指出主要事实、未知项和本次影响，没有依赖旧聊天中的隐藏约定。
4. 所有故障演练 fail closed；没有用旧 Evidence、空 Gate、宽泛 unknown 或新弱规则获得 PASS。
5. 所有 material contract item 都有 Requirement decision/result；没有未决项在首次写入后才被偷偷补成 Agent 默认，外部验证没有被本地 PASS 冒充。
6. 每个已知 durable semantic change 都更新了 Project Truth；没有把代码细节整批复制成第二套文档。
7. path ownership 对实际修改完整；selector 省略项经人工抽查确实无关，显式 Invariant/Requirement Gate 从未被过滤。
8. 误报、人工确认负担和验证耗时被记录并可接受；不允许删除关键边界只为改善数字，也不允许用无差别全量流程掩盖 selector 配置质量。
9. 至少一次真实错误能通过 Change/history 定位并完成恢复或补偿方案。

若任一条件失败，应修改 ECP 的 schema、Adapter 或 workflow 后重做相关阶段；不得把试点失败解释为“用户不会写提示词”。

# ECP v0.3 Canonical Specification

本文件是 ECP v0.3 本地语义闭环的产品与工程合同。实现、测试、Skill 和后续接口设计不得用更弱的语义解释其中的 MUST/不得条款。

## 1. 目标

ECP v0.3 必须为单个本地 Git Workspace 建立一个可运行的闭环：

1. 识别稳定 Project 与具体 Workspace；
2. 为每个 canonical Workspace 保存默认未启用、显式启用/关闭的项目模式与 activation epoch；
3. 在仓库外分别保存经过接受的控制配置 epoch、Project Truth epoch 与不可变 Workspace binding；
4. 为一个明确 Change 固定 activation、目标、范围、验收条件、结构化 Impact、启动 truth revision 和源码基线；
5. 在实现后对最终源码与候选 Project Truth 执行 Semantic Reconciliation；
6. 根据当前变更、风险以及 Impact 所关联的 Project Truth 不变量确定 Required Gates；
7. 由 Core 执行配置中的结构化命令；
8. 记录与精确 activation、source fingerprint、控制配置、Project Truth 和 Gate 定义绑定的 Evidence；
9. 确定性重算 Verdict；
10. 在任何相关状态变化后 fail closed；
11. 仅允许当前精确状态的 `PASS` Change 进入 `COMPLETED`。

核心命题：

> Verdict 必须对 `(authority, project, workspace, activation, change, contract revision, effective config, accepted truth revision, semantic assessment, source fingerprint)` 的精确组合做裁决，不能对“项目总体”或“最近好像通过过”做模糊判断。

## 2. 非目标

v0.3 不实现：

- 自有聊天平台、模型调用层或 Coding Agent；
- 云端控制台、多人组织、RBAC 或二人审批；
- 已认证的人类身份、签名证明、远端 notary 或透明日志；
- 容器/虚拟机/OS sandbox、默认断网或 hermetic build；
- 任意外部 Evidence 导入或未经验证的 CI PASS 导入；
- 部署、发布、数据库迁移、邮件发送或其他外部副作用编排；
- 自动 Git reset、回滚、删除或清理用户工作树；
- 跨 Workspace 复用本地 Evidence；
- state-directory migration；改变 canonical state path 会选中另一套 authority；
- Windows 或其他非 Unix 上的 authority state 运行；交叉编译与 `ecp version` 可用不等于权威路径可用；
- UI、MCP Server 或生命周期 Hooks；
- 将 Skill-based Plugin 的自动路由宣传为 OS、shell、其他工具或同用户进程不可绕过的 enforcement；
- 将本地 PASS 外推为生产、真机、发布或远端服务事实。

这些能力只能在 Core 合同稳定、身份和隔离边界真实存在后分阶段加入。

## 3. 术语与实体

### Project

由仓库内稳定 `project_id` 标识的工程。目录路径不是 Project 身份。

### Authority

`authority_id` 是由 canonical 仓库外 state-directory path 确定性派生的 opaque 本地目标选择器。它表示本次观察和 mutation 针对哪一套本地权威状态，不是 actor identity、签名、认证或远程证明。`project status`、`project inspect` 和 `context get` 必须输出它；需要精确本地目标的 mutation 必须将该 opaque 值原样传回 Core。

### Workspace

Project 的一个具体 Git clone/worktree。`workspace_id` 必须由 canonical Git 根与 Git metadata 位置派生；不同 clone/worktree 不得共享本地 Evidence。Core 必须先用独立于 Draft Config 的 Workspace binding 找到权威 Project，再把候选 `project_id` 当作待比较字段；未接受的 Draft ID 不得切换 state store。

### Project Mode 与 Activation

每个 canonical Workspace 对用户只存在两种项目模式：`enabled: false` 或 `enabled: true`。不存在 authority activation record 的 Workspace 默认未启用；Plugin 安装、`.ecp/` 存在、Project 相同、另一个 clone/worktree 已启用或历史聊天内容都不得隐式启用当前 Workspace。

`project status` 必须是不会执行项目代码、不会构造 source snapshot 的只读模式探针。对未注册 Workspace，它必须返回 `enabled: false`、稳定 authority/Workspace target 与 opaque `activation_token`，且不得创建 `.ecp/`、Workspace binding、event store 或 artifact。读取 status 所需的 physical Git identity 不等于执行项目 Gate。

`READY`、`ACTIVE`、`BLOCKED` 与 `INDETERMINATE` 是 assurance 状态，不是额外项目模式。已启用 Workspace 的 Draft Config drift、malformed/missing config、Gate context failure 或完整性问题不得把它降级为未启用，也不得让 Adapter fallback 到普通未治理写入。无法可靠确定 mode 时必须在仓库写入前停止。

每次 enable/disable 都追加一个唯一、不可复用且连接 previous activation 的 `activation_id`。`activation_token` 是 Core 对 authority ID、Workspace ID、Project/registration、authority revision、event head、当前 activation ID 和 enabled 位的 opaque 摘要；它还使 status 后追加 Change/Evidence 等 authority mutation 时旧 mode request 冲突。Token 与 activation ID 都是乐观并发边界，不是签名、身份或认证。

显式 enable 必须绑定 immediately preceding status 的 exact authority ID、Workspace ID、activation token、已接受 candidate config digest 和 accepted truth digest。在记录 enabled event 前，Core 必须要求 default risk 至少一个 Required Gate，并解析/preflight 其 cwd、executable 与 environment，但不得执行 Gate。任何初始化、注册、接受、预检或最终 transition 失败都必须保持 authoritative mode disabled；Draft Config、Project Truth 或 registration 的部分准备不是启用。

仅说“配置、设置或准备好 ECP”而未明确要求为当前整个项目/Workspace 启用，不构成 enablement intent。Adapter 此时只问一个简短的 whole-project 意图确认问题；取得明确确认前，不解析或读取仓库、不调用 `project status`、`project inspect` 或其他 ECP operation，也不创建或修改 Draft Config、authority、registration、activation 或其他项目状态。用户明确提出安装、版本或状态检查时走独立只读 check 流程，且不得隐式转入 enablement。

显式 project disable 必须绑定 immediately preceding status 的 exact authority ID、Workspace ID 与 activation token，在与 Gate/terminal transition 共用的 Workspace lease 内重新加载 authority。若该 status 观察到 ACTIVE Change，Core 必须在同一次原子 append 中先记录 `change_cancelled`、再记录 `project_disabled`；保留源码、Draft Config、Evidence 与全部历史，不产生 PASS。关闭已关闭或未注册 Workspace 是不追加 event 的幂等成功。Draft Config malformed 不得阻止 authority-only disable。

受支持的 Codex Adapter 在任何普通仓库写请求前必须先读取 project status。未启用时退出 ECP 子流程并继续普通开发，不提示启用；已启用时所有受支持的仓库 mutation 必须先建立/恢复 ECP Change，不提供单 task bypass。用户可显式关闭整个当前 Workspace。该自动路由仍是可禁用的 Skill-based workflow adapter，不是 shell、其他 Agent、同用户进程、CI 或 release enforcement。

### Control Config、Project Truth 与 Accepted Epoch

- `.ecp/project.json`、`policy.json` 与 `gates.json` 是可写、可评审的候选 Control Config；`.ecp/truth.json` 与 `contracts/**` 是候选 Project Truth。
- Core 分别计算 `config_digest` 与 `truth_digest`，并在仓库外保存每个 accepted Control Config 的结构化 Project/Policy/Gates history、accepted Project Truth payload/file manifest history，以及按 digest 寻址的 exact UTF-8 truth/contract content blobs。`policy get` 与 `truth get` 必须能在候选 drift/malformed 时读取这些 accepted epochs。候选文件不能接受或启用自己；accepted content blob 缺失、类型/权限不安全或摘要损坏必须 fail closed。
- Control Config 任意字节变化后，在再次显式接受前不得成为裁决依据，也不得通过删除 Gates 产生 vacuous PASS。`policy accept` 在 ACTIVE Change 期间必须拒绝；新规则不得用自己削弱后的门槛审查自己。
- Project Truth 由 Purpose、Capability、Invariant、Component、Decision、Contract Reference 与 Unknown 构成；`seed` 必须诚实保留未知项，`established` 至少包含一个 capability、invariant 和 component。每个 Contract ID 必须对应唯一的 canonical `contracts/**` 路径，同一路径不得被多个 ID 重复引用，避免 impact 映射取决于数组顺序。
- 候选 Project Truth 变化只能在 ACTIVE Change 中由 Semantic Reconciliation 接受。Core 必须从 previous accepted truth 与 candidate truth/file manifest 计算结构化 delta，而不是相信调用方自报差异。
- 产品语义结果与 Project Truth revision 是相关但独立的轴：`CHANGED` 可以表示既有 durable truth 仍准确的产品/业务/架构变化，此时允许零 truth delta；任何非零 truth delta 都是 protected change，必须由 `CHANGED` assessment 绑定 exact previous/candidate truth digest、final source fingerprint、结构化 delta、actor/reason 和显式 protected confirmation，并与新 truth acceptance 原子追加。`PRESERVED` 不得携带 truth delta，也不得用于完成声明了 expected changes 的 Change；`UNKNOWN` 可以记录但不得 PASS。未声明 expected change 且没有具体 startup uncertainty 的 `CHANGED` 必须以越出 Change expectation 阻断。
- 既有 Truth delta 必须被 Change Impact 中的 project-purpose 或 capability/invariant/component/decision/contract/unknown ID 精确覆盖；新发现且启动时无法引用的新增事实必须在 Change 中显式记录具体 unknown，否则 reconciliation 阻断。文本 unknown 只能覆盖新增事实，不能成为修改或删除既有受保护事实的通配符。
- 语法有效但未接受的 control 或 truth drift 产生 `BLOCKED`；malformed/missing 文件产生完整性 error 并 fail closed。Control drift 必须在没有 ACTIVE Change 时由独立 policy acceptance 处理；valid candidate truth drift 可以启动一个显式 recovery/adoption Change，但 Change 的 starting truth 仍必须是 previous accepted digest，且在 reconciliation 成功前不得计划 Gate、取得 PASS 或完成。malformed truth 不得进入该恢复路径。authority-only 的历史查询与显式 cancellation 不依赖候选文件可被接受。
- `project_id` 在 Workspace 注册后不可由候选配置改写。`project inspect` 必须提供稳定的 authority ID、Workspace ID、candidate config digest 与 candidate truth digest；首次注册既有 `.ecp/` 必须同时携带同一次审阅观察到的 exact 两个 digest。Adapter 只有在当前用户意图明确覆盖项目启用、控制规则变化、受保护真相变化或风险 acknowledgement 时才可代为传递这些 exact opaque preconditions；不得要求用户手工复制 ID/digest，也不得自行推定接受。此确认仍只是可审计的本地 acknowledgement，不是认证审批。

### Change

一次有边界的工程修改，至少包含：

- `change_id`；
- `activation_id`；
- title 与 goal；
- scope path roots；
- non-goals；
- acceptance criteria；
- structured impact（capability/invariant/component/decision/contract、journey、data/operation effect、expected change、expected preservation 与 unknown）；
- `contract_version=2` 的 sorted structured Requirements：每项包含 stable ID、statement、`DECIDED|NOT_APPLICABLE|DEFERRED_SAFE|BLOCKING_UNKNOWN`、rationale、decision source、`AUTOMATED|REVIEW|EXTERNAL` verification、可选 mapped Gate IDs、revisit condition 与对 Change contract item 的 exact coverage；
- 可选 `supersedes_change_id` 与不可伪造的 lineage root；
- user-declared risk；
- activation baseline；
- starting truth digest；
- contract digest/revision；
- lifecycle state。

v0.3 每个 Workspace 同时最多一个 `ACTIVE` Change。

`change start` 只允许当前 mode enabled，且必须携带 immediately preceding context 的 exact `authority_id`、`workspace_id`、`activation_token`、已接受 candidate config digest、accepted truth digest 与 source fingerprint。Core 在创建 ACTIVE 事件前重新选择 authority、发现 Workspace、核对同一 activation epoch、稳定读取配置/真相并抓取 baseline；任一目标 stale 都不得创建 Change。

Core 必须在 start 前要求每个 acceptance criterion、user journey、data/operational effect、expected change、expected preservation 与文本 impact unknown 至少被一个 Requirement exact 覆盖。`BLOCKING_UNKNOWN`、无 decision source/rationale、无具体 revisit condition 的 `DEFERRED_SAFE`、未知 mapped Gate、或漏覆盖均不得创建 ACTIVE。`AUTOMATED` 必须映射至少一个 Gate，`REVIEW` 不得伪装 Gate Evidence，`EXTERNAL` 在 v0.3 没有可信 importer 时只能保持 pending。

若当前 source 仍不同于同 activation 中最新 cancelled Change 的 baseline，新 Change 必须用 exact `supersedes_change_id` 指向该项，继承其原 baseline 与 lineage root，并让 scope 覆盖全部 inherited touched paths；否则拒绝 start。取消和重开不得把旧 edits、风险或越界路径洗入新 baseline。

### Gate Definition

仓库配置中一个结构化验证定义：稳定 ID、`fast|affected|full` tier、executable/argv、repo-relative cwd、timeout、允许退出码、显式环境、允许继承的环境变量名、适用风险级别，以及可选 path/component/capability/invariant applicability selectors。

Risk Gate 的 selector 按 OR 匹配；无 selector 的旧 Gate 对其 `required_for` 风险保持 universal。Invariant `gate_ids` 与 AUTOMATED Requirement mapped Gate 是显式强制关系，不受 selector 过滤。Tier 只描述成本/目的，不得覆盖关系或 Evidence integrity。

Gate 定义是待执行的不可信项目输入；它不是质量证明。

### Gate Run 与 Evidence

GateRun 是一次已进入执行边界的持久生命周期，不是临时函数调用。Core 必须在运行任何项目代码前追加 `gate_run_started`，绑定 exact activation、ACTIVE Change、plan digest、按顺序选择的 Gate IDs 和开始时间。随后每条 Evidence 必须额外绑定 exact `gate_run_id`，同一 run 的同一 Gate 最多一条。项目命令输出中的 `PASS`、`approved` 或自然语言不得影响状态机。

GateRun 终态为：

- `COMPLETED`：所选 Gate 都完成观察并各自提交一条 Evidence；Evidence 可以是通过、超时、异常退出、source mutation 等失败结果，因此 `COMPLETED` 不等于 Verdict PASS；
- `FAILED`：Core 在所有所选 Gate 都形成 Evidence 前遇到非取消流程错误；已经原子提交的 Evidence 保留；
- `CANCELLED`：调用 context 被取消或 deadline 到期；
- `INTERRUPTED`：旧 run 留在 `IN_PROGRESS`，而后来一个操作已经成功取得旧进程释放的同一 advisory lease，因而可以确定旧租约持有者不再执行受保护序列。

`IN_PROGRESS` run 存在时，除匹配的 `evidence_recorded` 和其唯一 terminal event 外，不得追加其他 lifecycle mutation；Project status 和 context 必须暴露该 run，Verdict 必须为 `INDETERMINATE`。不得按时间或 PID 文本猜测 interruption。`gate history` 必须通过 authority-only loader 返回按事件顺序排列的全部 run，即使候选 Draft Config malformed/missing。中断恢复不自动续跑，也不证明所有子进程或外部副作用已经清理。

Core 在接受 `gate_run_started` 与每条 `evidence_recorded` 前，必须同时保证仍有可写入该 run 最大有界 terminal event 的 byte 和 continuation-segment headroom。普通 mutation 不得消耗 GateRun terminal reserve，GateRun terminal/单独 cancellation 不得消耗最终 project-disable reserve。若 project disable 取得已释放的 Gate lease 且同时观察到 `IN_PROGRESS` run 与 ACTIVE Change，必须在同一次原子 append 中依次写入 `gate_run_finished(INTERRUPTED)`、`change_cancelled`、`project_disabled`，不得先写 start/执行项目代码后才发现没有终态容量。

`gate plan` 必须返回 opaque `plan_digest`，其输入至少覆盖 exact authority/project/workspace/activation/change、contract、Effective Config、accepted Project Truth、source fingerprint、effective risk、Required Gate definitions、解析后的 cwd/executable/environment identity 与 Core identity。`gate run` 必须同时携带调用方刚审阅的 exact ACTIVE Change ID 与该 opaque digest。Core 在 Workspace lease 内重新加载并重建计划，在执行任何 Gate 前以及每个后续 Gate 前都精确核对；不匹配返回 conflict，不得自动执行新计划。

### Verdict

Core 基于当前状态纯推导的裁决：

- `PASS`：当前精确 subject 满足当前策略声明的所有要求；
- `BLOCKED`：存在已知未满足条件，例如缺失/失败/过期证据、越界修改、配置未接受或 acknowledgement 缺失；
- `INDETERMINATE`：状态损坏、输入不受支持或 Core 无法可靠判断。

`STALE` 是 Evidence 或历史 Verdict 对当前 subject 的适用性状态，不是通过状态。

Verdict `subject_digest` 必须覆盖 exact authority/workspace/activation，以及 project/change/contract/config/truth/semantic assessment/source/risk/requirements/Evidence 等裁决输入；另一套 authority、Workspace、truth epoch 或 activation epoch 的 subject 不得与它混同。没有适用于 final source/truth 的 semantic assessment、assessment=`UNKNOWN`、候选 truth 未接受或 Evidence truth digest 不匹配时不得 PASS。

Semantic Assessment 必须按 Requirement ID 顺序逐项 reconciliation。`AUTOMATED` 的 `VERIFIED` 只是 exact Gate mapping，Verdict 仍必须找到每个 mapped Gate 的当前 PASS Evidence；`REVIEW` 可由 source-bound local reconciliation 确认；`NOT_APPLICABLE`/`DEFERRED_SAFE` 必须与 start decision 相同；`EXTERNAL` 必须为 `EXTERNAL_PENDING` 并阻止 local PASS。

### Acknowledgement

本地 CLI 记录的 actor/reason 确认。它必须绑定 exact subject digest。它不证明 actor 真实身份，也不等于可信 human approval。

## 4. Change 生命周期

v0.3 对外提供原子 `change start`，内部创建并激活合同：

```text
NONE ──start──> ACTIVE ──complete with current PASS──> COMPLETED
                    ├──semantic reconcile────────────┤
                    └──explicit cancel with reason───> CANCELLED
```

约束：

- 只有当前 project mode enabled 时才可 start、plan、run、acknowledge 或 complete Change；所有这些事件必须绑定同一 `activation_id`。
- `ACTIVE` 的 contract 不允许原地静默扩展；v0.3 需要新 Change 承担范围变化。
- `COMPLETED` 只表示某个最终 subject 曾获得 PASS 并被记录；完成请求必须同时携带 exact Change ID 和 immediately preceding Verdict 的 subject digest，Core 重算 activation、source、plan 与 Verdict 后精确匹配才可保存最终 Verdict payload 与摘要，后续代码变化不会篡改该历史事实。受支持的 Adapter 在普通实现获得当前 PASS 后自动完成，不要求用户手工运行 completion。
- `CANCELLED` 只表示用户明确放弃当前 ACTIVE Change；取消必须记录 actor/reason/time，不要求 PASS、不删除 Evidence、不修改工作树，也不得被解释为 waiver。
- cancelled Change 的 source edits 若仍存在，后续 replacement 必须显式 supersede 并继承原 baseline/lineage；没有残留 delta 时可以正常创建独立 Change。
- 取消请求必须携带用户在同一条 immediately preceding ACTIVE `change list` 记录中确认的 exact authority ID、Workspace ID 和 Change ID；Core 在终态 lease 内重新加载 authority 后核对三者，缺失、无 active 或不匹配均不得取消另一套 state、Workspace 或其他 Change。
- 显式 project disable 是独立的项目级意图：它可以原子取消 immediately preceding status 已观察的 ACTIVE Change 并关闭项目。单独 `change cancel` 则保持项目 enabled；任何 blocker 都不得触发自动 cancel/disable。
- 完成后继续修改必须创建新 Change。
- authority-only Change history 必须返回 Project/Workspace/activation identity、完整 goal/scope/non-goals/acceptance/structured Impact、全部 Semantic Assessment 与 terminal completion/cancellation，使接手者不依赖旧聊天补齐“为什么改、预计保持什么、最终如何判断”。
- 完成、取消、project disable 和 Gate sequence 必须共享终态 lease；完成和最终 snapshot 之间的竞态必须 fail closed。本地锁只减少协作冲突，不宣称能阻止外部进程。

## 5. Source Fingerprint

v0.3 的 `source_fingerprint` 必须确定性覆盖：

- HEAD OID 与 symbolic branch；
- HEAD tree entries；
- Git index mode/object/stage；
- tracked worktree 内容、mode、删除与 symlink link text；
- 非 ignored untracked 文件；
- 已初始化 submodule 的递归 HEAD/index/worktree/untracked manifest 与精确指纹；
- 仓库内 `.ecp/` 候选合同。
- Core 实际使用的固定 Git executable realpath 与 SHA-256 内容摘要。

规则：

- 路径规范化为 repo-relative POSIX path；绝对路径与时间戳不得进入摘要。
- 不跟随源码 symlink；哈希 link text。
- FIFO、device、socket、越界路径、非法 UTF-8 路径或超过实现安全上限时 fail closed。
- `.git/` 和仓库外 ECP state 不进入摘要。
- Git ignored 文件不在 v0.3 摘要内，因此名称必须是 `source_fingerprint`，不得称为完整或 hermetic `workspace_digest`。
- Core 必须进行两次完整观察并要求指纹稳定；这仍不能发现运行期间发生后又被完全恢复的瞬时篡改。
- 未初始化、越界、循环或超过深度/文件/字节上限的 submodule 必须 fail closed。
- Core Git 不得从 ambient `PATH` 解析。当前受支持的 POSIX Unix authority 路径只允许解析为 regular executable 的 `/usr/bin/git` 或 `/bin/git`；两者均不可用时必须以 `TRUSTED_GIT_UNAVAILABLE` fail closed。Git realpath/digest 必须出现在 `SourceSnapshot`/`SnapshotRef` 中并参与 `source_fingerprint`；同一路径的 Git bytes 改变后旧 Evidence 必须 stale。

若 Gate 的 pre/post fingerprint 不同，其退出 0 也不能形成可用 PASS Evidence。

## 6. Scope 与风险

v0.3 scope 是规范化的 repo-relative path root，不支持模糊 shell glob。`.` 表示整个仓库。文件被新增、删除、重命名、改变 mode、改变 index/head/worktree 状态均算 touched。

submodule 的递归内容参与精确 fingerprint，但 v0.3 的 scope/touched path 折叠为 submodule mount path。对 mount path 的变化，任何位于其内部的 path-risk rule 或 denied path 都必须保守匹配；若需要只授权 submodule 内部的更细粒度 scope，v0.3 应拒绝/阻断而不能低估风险。

范围判断必须比较 Change baseline 与当前完整 source manifest，不能只比较当前 `git diff HEAD`，否则 Change 中途 commit 会隐藏修改。

有效风险：

```text
effective risk = max(user-declared risk, all matching policy path-risk rules,
                     risks of impacted Project Truth invariants/unknowns)
```

Impact 直接引用的 invariant、capability 所关联的 invariant、受影响 component 所承载 capability 的 invariant，以及被受影响 decision 间接引用的这些事实都参与风险计算。`project_purpose=true` 采用全部 Project Truth invariant/unknown。若本 Change 演进 Project Truth，则 starting 与 candidate/accepted 两个 truth epoch 的全部 invariant/unknown 都参与，取更高风险，不能通过在同一 Change 中删除或降低风险字段来降低门槛。

对 `maturity=established` 的 Project Truth，Core 还必须将 scope 与最终 touched paths 和 Component `path_roots` 做双向包含匹配，反推直接 component，然后沿 `depends_on` 的反向传递闭包加入所有直接/间接依赖者，再反推引用任一结果 component 的 capability 与 invariant。声明 Impact 与 inferred impact 取并集参与风险和 Gate 选择；start scope 推导出的漏报/未映射路径拒绝创建，final touched 推导出的漏报/未映射路径阻止 PASS。Core 不得因为调用方少声明而降低风险或删 Gate，也不得把没有依赖关系的组件加入闭包。

风险顺序：`low < moderate < high < critical`。用户声明不得降低策略推导风险。

内建 deny 路径（至少 `.ecp` 控制面）优先于 allow scope。普通产品 Change 中修改控制面必须 BLOCKED，并通过独立治理 Change/显式配置接受流程处理。

`change start` 必须在创建 ACTIVE Change 前精确核对 expected authority/Workspace/activation/config/source preconditions，确认 declared risk、declared+scope-inferred durable facts 的风险及其 scope 内 policy path rules 可达的每个风险都有至少一个 applicable Required Gate，并对 matching risk Gate、impacted invariant `gate_ids` 与 automated Requirement Gate 的并集完成不执行命令的 cwd/executable/environment context 预检。否则必须拒绝启动，不能创建一个落在未观察 authority、基于未观察 activation/baseline、已知无 Gate 或 Gate 永远不可执行的 Change。

Impacted invariant 的 `gate_ids` 不受该 Gate 的 `required_for` 风险映射限制：显式事实关系本身就使 Gate 成为本 Change 的 Requirement。Project Truth 在本 Change 中发生演进时，starting 与 final truth epoch 的 required Gate 取并集；旧 epoch 中的保护 Gate 不能被同一 Change 删除后绕过。

## 7. Gate 执行

Gate Runner MUST：

- 只执行 Effective Config 中已接受的 Gate；
- 只执行调用方刚审阅且由 opaque `plan_digest` 精确绑定的当前计划中的 Gate；
- 在持有 Workspace lease 后、首个 Gate 前及每个后续 Gate 前，以稳定 source snapshot 和当前执行上下文重建计划；mismatch 必须在执行下一条项目命令前停止；
- 使用 executable + argv 数组，不经隐式 shell；
- 将 cwd 约束在 canonical repository root 内；
- 从空环境构造 policy/Gate 声明的最小 allowlist；DefaultConfig 不得继承 `HOME`；对 token/secret/password/passwd/credential/private-key/API-key/access-key 片段、主要 cloud/vendor 前缀、`KUBECONFIG`、Docker config、SSH/GPG agent 和 XDG config/runtime 等已知 capability 环境名执行 best-effort 拒绝；
- 记录环境值摘要而不是原值；
- 有 timeout 和 stdout/stderr byte cap；
- 在当前受支持的 Unix 上以独立进程组收割超时后代；非 Unix 直接进程取消代码只保持可交叉编译，不构成 v0.3 authority 运行支持；
- CLI 必须将 `SIGINT`/`SIGTERM` 转为 root-context cancellation，使 Gate 进程组清理、`CANCELLED` terminal event 与 defer 锁释放可执行；突然 `SIGKILL`/崩溃可能留下 durable `IN_PROGRESS`，在 Darwin/Linux/BSD 上只能由后来成功取得已释放 advisory lease 的操作追加 `INTERRUPTED`；该恢复不承诺收割 crash 后的子进程、回滚副作用、恢复 orphan artifact 或自动续跑；
- 对超时、启动失败、异常退出、输出写入失败和指纹变化 fail closed；
- 顺序执行 v0.3 Gates，避免共享工作树并发污染；
- 保存原始日志到 0700/0600 的仓库外 state，并只向 CLI 返回消毒后的元数据。

结构化 argv 只降低意外 shell 注入；`make`、`npm`、测试二进制和项目依赖仍可执行任意代码。v0.3 未隔离 Runner，必须在安全模型中持续披露。

v0.3 还必须在配置加载/接受时拒绝 `requires_network=true` 或 `produces_external_side_effects=true` 的 Gate，因为首版不存在可信的网络/副作用授权与回滚边界。项目命令可能谎报字段或自行联网，这项 schema 检查不构成 sandbox。

## 8. Evidence 适用性

Evidence 只有在以下字段与当前 Requirement 完全匹配时才适用：

- authority/project/workspace/activation/change ID；
- Change contract digest；
- source fingerprint；
- effective config digest；
- accepted Project Truth digest；
- 执行时 exact Gate plan digest，且它必须与 Core 对当前 subject 重建的 plan digest 匹配；
- Gate definition digest；
- runner/core build identity（版本与当前 ECP 可执行文件摘要）；
- execution context digest；
- artifact/log 存在且摘要一致；
- exit code 在允许集合内；
- 未 timeout/interrupted；
- pre/post fingerprint 相同。

缺失、失败、过期、损坏、未知或不匹配均不得 PASS。每条 Evidence 必须保存执行时的 `activation_id` 与 `plan_digest`；Core 在 Verdict 时要求当前 enabled activation，并重建当前 plan，任一值不匹配则 Evidence 不适用。Evidence 历史记录不被改写为 stale；Core 每次根据当前 subject 重新计算适用性。

## 9. Acknowledgement

策略可要求高风险 Change 提供 acknowledgement。记录必须绑定：

- authority/project/workspace/activation/change（authority/workspace/activation 通过 subject digest 绑定）；
- contract/config/source digest；
- 当前采用 Evidence 的 subject digest；
- actor、reason、timestamp。

任一绑定变化后 acknowledgement 失效。Adapter 不得因普通实现请求自动接受后续 policy drift 或风险 acknowledgement。Gate plan 一旦表明 effective risk 为 `high` 或 `critical`，Adapter 必须在第一次产品文件写入或 Gate 执行前解释具体人类可读风险并取得当前用户的明确确认；未确认时可以保留已经建立并计划的 bounded Change 为 `ACTIVE`，但不得写产品文件、创建 GateRun 或 Evidence。项目初次注册/配置接受仍只可由明确的项目 enablement 请求授权。

前置人类风险确认不等于 Core acknowledgement 事件。CLI acknowledgement 请求只能在最终源码、Semantic Assessment 和 Required Evidence 已就绪，且当前 Verdict 的唯一 blocker 为 `ACKNOWLEDGEMENT_REQUIRED` 后发出；它必须携带 exact Change ID 与调用者刚审阅的最终 subject digest。Core 重算当前 subject 后不匹配即 conflict，不得把对旧 subject 的确认转移到新 subject。目标、范围、Impact、Requirements、effective risk 或已展示的人类可读风险发生漂移时，前置确认失效，Adapter 必须重新说明并取得 fresh confirmation 后才可记录新的 exact-subject acknowledgement。

## 10. 权威状态与审计

权威运行状态必须位于仓库外，默认使用用户私有目录，测试可通过 `ECP_STATE_DIR` 注入临时目录。仓库配置不得指定状态目录，Core 必须拒绝当前仓库内的 state path。从 canonical state root 到 Workspace binding、project/workspace store、truth blobs 与 Evidence artifact 目录的每个 authority-owned 路径组件都必须是 private real directory；不得跟随中间 symlink 读写或因拒绝路径而 `chmod` 其外部目标。Workspace binding、events、Truth blobs 和 Evidence artifacts 必须保持有界、非 symlink 的 regular private files；替换类型或暴露 group/other 权限必须 integrity fail closed。Evidence event 还必须精确引用 canonical `artifacts/<change>/<evidence>/stdout.log|stderr.log`，并与历史 accepted Gate epoch、存储摘要和精确存储字节数一致。Canonical state path 必须派生 `authority_id`；改变它会选中不同的本地 authority。v0.3 不支持 state-directory migration，不得把改路径解释为对原状态的透明迁移。

v0.3 authority 加载与 mutation 仅支持具有 POSIX private-file 语义的 Unix。非 Unix 上 `project init/inspect/register` 及所有 authority load 必须在任何项目/authority 写入前返回 `PLATFORM_SECURITY_UNSUPPORTED`；`ecp version` 仍可用。Windows 交叉编译通过不能被解释为运行支持。

每套 authority 状态包含两层：由 `workspace_id` 直接索引的不可变 Workspace binding，以及 binding 所指定 `(project_id, workspace_id)` 下的 activation/config/Change/Evidence events 与 artifacts。已有 `.ecp/` 但尚无 binding 时，`project init` 不得自动接受；只读 `project inspect` 返回稳定 authority ID、Workspace ID 与 candidate config digest，Adapter 只有在明确项目 enablement 意图下才可用同一响应的 exact target 执行注册。`policy accept` 同样要求同一次刚审阅 JSON 的 exact authority ID、Workspace ID 和 candidate digest；初次 enablement 可覆盖 initial acceptance，后续 drift 必须取得新的明确确认。只有 Core 在同一次 `project init` 中新建的 `.ecp/` 可以本地 bootstrap 接受，并且接受的 digest 必须来自 Core 写入前对 exact 初始 bytes 的计算；写入后观察不匹配则 fail closed。Registration/acceptance 均不启用项目，必须另有 exact `project_enabled` event。

v0.3 使用单 Workspace、原子分段、hash-chained append events：

- 每次 mutation 在 workspace lock/CAS 边界内检查预期状态；registration/policy acceptance 绑定 expected authority/Workspace/config，enable/disable 绑定 expected authority/Workspace/activation token（enable 还绑定 config），Change start 绑定 expected authority/Workspace/activation/config/source，cancellation 绑定 expected authority/Workspace/Change，Gate run 绑定 expected Change ID/plan digest，acknowledgement/completion 绑定 expected Change ID/subject digest；
- 先以 Core 生成的受限路径原子写入 bounded artifacts并记录内容摘要，最后原子提交引用它们的事件；
- 事件只追加语义，不原地覆盖历史；
- 事件序列可重建 projection；
- hash chain 检测损坏，但不宣称抵抗同用户重写整个 store。
- `events.json` 是兼容根段；新 mutation 在当前段达到 8 MiB 前原子重写该段，随后原子创建 `event-segments/` 下从 `0000000000000001.json` 开始的连续 continuation segment。旧版单文件可读取到 64 MiB；continuation 最多 1024 段，聚合 event bytes 最多 1 GiB；
- 一个 mutation 的完整 event batch 必须放进同一 segment，不得跨段留下部分结果。根段缺失、continuation 编号缺口/非 canonical 名称/空段、任一段超限/类型或权限不安全、危险临时残留、sequence/hash chain 跨段不连续，均必须 integrity fail closed；安全且有界的 `.write-*` 崩溃残留不得进入 projection；
- Gate sequence、semantic reconciliation、acknowledgement、Change completion/cancellation 与 project enable/disable 共享 Workspace lease；Darwin/Linux/BSD 使用进程退出时由内核释放的 advisory file lock。新 Gate sequence 在首个项目命令前追加 `gate_run_started`，每条 Evidence 和唯一 terminal event 必须匹配 active run。持锁进程被杀或崩溃时 run 可保持 `IN_PROGRESS`；后来成功取得同一 lease 的操作必须先追加 `INTERRUPTED` 再执行自己的 lifecycle mutation。仍被 live holder 占用的 lease 不得被 contender 恢复或绕过。其他 Unix 的 v0.3 fallback 是保守 sentinel lock，crash 后需要人工恢复且不得自动猜测持有者已死；该路径尚未列入已验证运行矩阵。非 Unix 不进入此 fallback。
- Event store 必须在 byte 与 segment 两个维度为 GateRun terminal/lifecycle 与最终 project disable 分层保留 bounded capacity；若存在 ACTIVE GateRun/Change，显式 disable 还必须能在同一次 append 容纳 `INTERRUPTED` terminal + cancellation + disablement。普通 event 和单独 terminal/cancellation 不得消耗它们后面的保留层。

`authority health` 是 registered canonical Workspace 的 authority-only、一致快照诊断。它必须按 lifecycle 相同的 Gate lease → mutation lock 顺序取得两把锁，在锁内重载 binding 与完整 event projection，不读取或接受候选 `.ecp`。若 binding 或 event sequence/hash chain 无法可信重放，操作必须直接 integrity fail closed，不能从部分 projection 构造健康报告；若 projection 可信，则必须遍历全部历史 truth acceptance 与 Evidence，核验每个唯一 referenced blob/artifact，并对实际 state tree 做有界 inventory。

报告必须包含 revision/event head、event 总字节与当前 mutation limit、terminal reserve、root/continuation/current segment 与剩余 continuation、80% byte 或 10% segment warning、enabled/activation/active Change/active GateRun，以及 Truth/Evidence 的 referenced、verified、invalid、orphan、temporary、unrecognized、unsafe 计数和有界 findings。状态为 `HEALTHY`、`ATTENTION`、`INDETERMINATE`：前者无 finding；`ATTENTION` 只表示仍可验证 authority 中存在容量预警、安全 remnant/orphan/unrecognized entry 或 health 已取得 released lease 的 unresolved GateRun；`INDETERMINATE` 表示 referenced object 或 surrounding layout 缺失、损坏、危险、超限或歧义。三者都是 `ok: true` 结果，CLI 分别退出 `0/3/4`。

Health 不得追加 event、接受候选、terminalize GateRun、删除 orphan/remnant、repair、restore、compact、migrate、GC 或修改仓库。Authority 目录必须在取锁前已经是 private real directory；health 不得借普通写入入口修正其权限。取得 advisory lock 只可创建或刷新 private lock PID/time metadata，且 revision/event head 必须保持不变。核验 referenced Truth/Evidence 时不得沿 object-store 或中间 artifact 目录 symlink 读取。仍被 live Gate holder 占用时不得绕过或猜测；成功取得 lease 且 projection 仍有 `IN_PROGRESS` 只允许报告 `GATE_RUN_INTERRUPTION_RECOVERABLE`，实际 `INTERRUPTED` 仍由后续授权 lifecycle operation 记录。报告不是签名、备份、恢复点、retention/GC 授权或远程 attestation。

Authority export 是显式、只读于 live authority 的管理操作。Core 必须在 Workspace mutation lock 内重载 current projection，只复制 immutable Workspace binding、完整 root/continuation event segments、全部历史 accepted truth 引用的 blobs 和全部 Evidence 引用的 stdout/stderr；锁、临时文件和 orphan blob/artifact 必须排除。目标必须是仓库和 live authority 之外尚不存在的目录。复制完成后写入严格、排序、版本化 manifest（path、role、size、SHA-256），离线重放并核验 exact reference set、event chain、projection、accepted epochs、truth blobs 和 Evidence，成功后才原子安装为 private read-only bundle。导出不得改变 live authority revision/event head；同一未变化 authority 的 bundle digest 必须与输出路径无关。

`authority verify` 必须只依赖 bundle，拒绝 symlink、非 private/regular entry、未知或非 canonical path、遗漏/额外文件、超限、size/digest mismatch、跨段 event 损坏、projection/manifest mismatch、accepted truth blob 或 Evidence artifact 不一致。验证成功只证明本地内容自洽；manifest/hash chain 都不是签名、身份、审批、remote attestation 或导出主机可信证明。Bundle 可能包含敏感日志，不得自动提交、上传或分享。v0.3 不提供 restore/import、merge、redaction、encryption、scheduled retention 或 GC。

## 11. 稳定 JSON 与退出码

除 `help`/`--help` 的人类可读文本外，所有操作结果在 stdout 输出版本化 JSON；错误输出版本化 JSON 到 stderr，原始 Gate 日志不回显。Gate sequence 一旦已经持久化 run start，后续错误 envelope 必须带含非空 `run_id` 和当前终态的 `partial_result`，即使尚无 Evidence；已提交 Evidence 必须一并返回。首个 run start 前的 plan/usage/mode 错误不得序列化伪造的零值 partial result。未知 schema、未知命令或未知枚举 fail closed。

Codex Plugin 的 Skill 必须从自身安装路径解析 `scripts/ecp` launcher；不得要求普通用户安装 CLI、修改 PATH，或 fallback 到 ambient/repository/temp `ecp`。launcher 只支持 Plugin runtime manifest 声明的 OS/architecture，必须拒绝 symlink、缺失或不可执行 runtime，并在 Core 启动前用固定系统 SHA-256 工具核对 selected binary sidecar。不支持的平台、缺包和 checksum mismatch 使用版本化 `ECP_RUNTIME_*` integrity error 与退出码 4。

Plugin runtime manifest 必须绑定 exact Plugin version、canonical source commit、source-clean 位、Go toolchain version/executable digest、固定 build flags，以及每个支持 target 的唯一相对路径、SHA-256 与 byte size。当前 v0.3 包含 `darwin-arm64`、`darwin-amd64`、`linux-arm64`、`linux-amd64`，不包含 Windows authority runtime。正式 qualification/release candidate 必须来自 clean committed source；显式 dirty override 只能生成 `source_clean: false` 的非资格本地开发制品。Core/Skill/Plugin source 或 cachebuster 变化后必须重新构建完整 runtime matrix；版本、source/toolchain identity 或 artifact mismatch 不得发布、qualification 或安装为兼容包。checksum 只证明包内一致性，不是签名、notarization 或 publisher provenance。

`project status` 返回 `enabled`、`registered`、`config_state`、`truth_state`、候选/接受的两个 digest、`operational`、`assurance`、可选 ACTIVE Change、可选 `active_gate_run` 与 diagnostics。未启用、READY、ACTIVE status 使用退出码 `0`；已启用且 assurance=`BLOCKED` 使用 `3`，已启用且 assurance=`INDETERMINATE` 使用 `4`。`authority health` 的 `HEALTHY`/`ATTENTION`/`INDETERMINATE` 同样使用 `0/3/4`。这些状态结果即使非零也必须是 stdout 上 `ok: true` 的正常 result envelope，调用方不得丢弃 mode、health status 或 diagnostics；真正 typed error 才使用 stderr 的 `ok: false` envelope。

最低退出码合同：

- `0`：操作成功、项目 status 为未启用/READY/ACTIVE、Verdict=`PASS`，或 authority health=`HEALTHY`；
- `2`：参数/用法错误；
- `3`：已知状态阻断、Gate 失败、Verdict=`BLOCKED`，或 authority health=`ATTENTION`；
- `4`：完整性错误、Verdict=`INDETERMINATE`，或 authority health=`INDETERMINATE`；
- `1`：其他运行错误。

## 12. v0.3 验收场景

实现至少必须覆盖以下场景。仓库内自动行为测试、静态 Skill/Plugin 合同、
构建证据和必须由独立 Codex host 或外部系统提供的证据类别，以
`docs/verification-matrix.md` 为准；静态 Skill 文本和 Core 测试不得被表述为
fresh-task host routing、真实项目、发布或生产证明：

1. 新建项目在 Draft Config/注册/接受完成后仍默认 disabled；显式 enable 后启动 Change、运行 Gate、PASS、完成和历史查询的正常路径；
2. PASS 后任意受管理源码变化使旧 Evidence stale，Verdict 非零；
3. 重新运行 Gate 后新的精确状态可重新 PASS；
4. Gate 运行期间工作树变化，即使 exit 0 也 BLOCKED；
5. Gate exit 非允许值、timeout、启动失败均不 PASS；
6. 配置任意变化进入 pending acceptance，不能用删 Gate 的方式 vacuous PASS；
7. scope 外新增/修改/删除/rename/mode change BLOCKED；
8. Change 中途 commit 后范围变化仍可由 baseline manifest 检出；
9. 高风险 acknowledgement 缺失/subject 不匹配 BLOCKED；
10. Evidence artifact 损坏、字节数/摘要不匹配、非 canonical Change/Evidence 路径、中间 authority 目录为 symlink 或暴露 group/other 权限时 Verdict=`INDETERMINATE`；Workspace binding/event 非 regular、权限暴露或事件损坏导致 projection 不可构造时返回完整性 error envelope 与退出码 4；
11. 同 Workspace 不能同时存在两个 ACTIVE Change；
12. argv 中的 shell metacharacter 保持单个参数，不触发第二命令；
13. path traversal、仓库外 cwd 和源码 symlink follow 均被拒绝；
14. Plugin/Skill 从不自行设置 Verdict；仅在明确项目 enablement 或用户对后续人类可读 policy drift/acknowledgement 的当前确认下内部传递 exact acceptance token，且不得宣称本地 PASS 等于发布状态。
15. 既有 `.ecp` 不会被 `project init` 隐式接受，Draft `project_id` 不能替换 authority binding；
16. 配置语义与摘要来自同一稳定字节集合，配置并发变化 fail closed；
17. submodule 在两个不同未提交 checkout 之间切换会使旧 Evidence stale；
18. `core.worktree` 重定向、尾随空格路径和 repo 内 state path 不会造成跨 Workspace 写入或身份混淆；
19. scope 可达的风险若缺少 Required Gate，`change start` 在创建 ACTIVE 状态前拒绝。
20. DefaultConfig 不继承 `HOME`；Required Gate 的环境名命中已知 capability/secret denylist、包含 NUL、cwd 或 executable 不可用时，在创建 ACTIVE 前拒绝；
21. Draft Config 损坏或丢失时，`change list`、`evidence list` 和带 exact authority/Workspace/Change target 的显式 `change cancel` 仍可只经 authority binding/store 工作；
22. cancellation 保留历史 Evidence、写入 actor/reason，且不会产生 PASS；旧 Change ID 不得取消后来成为 ACTIVE 的 Change；
23. 未初始化或被 regular/symlink 替换的 indexed submodule fail closed；递归 submodule 共享全局 source 文件/字节预算；
24. advisory lock 的持有进程退出后可重新获取；同进程并发持有仍冲突；
25. Gate sequence 部分提交后失败时 error envelope 暴露已记录 Evidence，普通错误不含零值 `partial_result`。
26. stale authority ID、Workspace ID 或 config digest 不能注册或接受另一份 Draft Config；stale authority/Workspace/Change 组合不能取消另一套 state、另一 Workspace 或后来 active 的 Change；stale Change ID 不能执行 Gate、完成或确认后来 active 的 Change；stale subject digest 不能确认或完成同一 Change 的新 subject。
27. `change start` 的 authority ID、Workspace ID、activation token、config digest 或 source fingerprint 任一 stale 时不得创建 ACTIVE Change，也不得选中未观察的 authority/activation epoch 或把并发源码变化吸收到 baseline。
28. `gate run` 的 plan digest stale 时，在首个 Gate 前不得执行项目代码；Gate sequence 已记录部分 Evidence 后计划变化时必须停止并通过 `partial_result` 报告已提交部分。
29. 每条 Evidence 保存 exact activation ID 与 plan digest；disable/re-enable 或 Verdict 对当前 subject 重建的 plan 变化后，旧 Evidence 不再适用。
30. ambient `PATH` 中的 Git shim 不得被 Core 执行；Unix 固定系统 Git 候选均缺失时以 `TRUSTED_GIT_UNAVAILABLE` fail closed；Git executable bytes 改变会改变 source fingerprint 并使旧 Evidence stale。
31. 非 Unix 上 `ecp version` 可用，但项目/authority load 与 mutation 在任何写入前以 `PLATFORM_SECURITY_UNSUPPORTED` fail closed；Windows 交叉编译不被当作运行验证。
32. `project status` 对从未注册的 Workspace 返回 disabled，且不创建 `.ecp`、authority state，不执行 Gate，也不构造 source snapshot；配置/注册或另一个 clone 已启用都不得隐式启用当前 Workspace。
33. project enable 必须匹配 immediately preceding status 的 authority/Workspace/activation/config，要求 default-risk Required Gate并只做 execution-context preflight；缺 Gate、preflight/config/token mismatch 或中断均不得产生 enabled event。
34. enabled Workspace 的 valid drift 为 `enabled + BLOCKED`，malformed/missing Draft 为 `enabled + INDETERMINATE`；两者都不得降级为普通未治理写入，并仍可通过 authority-only project disable 关闭。
35. project disable 对 status 后的 Change/Evidence/authority mutation 以 stale activation token conflict；对已观察 ACTIVE Change 必须原子追加 cancellation + disablement，保留 source/Evidence，且再次关闭不追加 event。
36. disable/re-enable 必须生成新的 activation ID；旧 context/token、Change、plan、Evidence、acknowledgement 或 subject 不得跨 epoch 操作或满足新 epoch。
37. 受支持的 Skill 在任何普通仓库 mutation 前先读 project status：disabled 正常开发且不提示启用，enabled 自动走 Change/Gate/Verdict/当前 PASS completion；显式“只跳过本 task”不得写入，除非用户改为项目级 disable。
38. 正常用户工作流不要求输入或复制 CLI command、ID、hash 或 token；Adapter 默认不展示 opaque 字段，只从指定的紧邻 JSON 内部传递。低层 CLI 仍保留 exact precondition 测试与开发者诊断能力。
39. project status 的 enabled+BLOCKED/INDETERMINATE result 分别使用退出码 3/4，但保持 stdout `ok: true` JSON 和真实 `enabled: true`。
40. Plugin 被禁用、直接 shell、其他 Agent/工具或同用户进程仍可绕过 Skill-based Adapter；文档不得把“无受支持的单 task bypass”表述为安全不可绕过 enforcement。
41. 新项目初始化同时产生诚实的 seed `truth.json` 与 contract reference；control config 与 Project Truth 的摘要、drift 和 accepted state 必须独立。
42. Change start 必须绑定 exact accepted truth digest，并拒绝空 Impact、未知 truth reference 或 stale truth precondition。
43. Required Gates 全部 PASS 但没有适用于 final source/truth 的 Semantic Assessment 时 Verdict 仍为 BLOCKED；`UNKNOWN` assessment 永远不得 PASS。
44. 候选 Project Truth 的真实结构化 delta 必须由 Core 计算；`PRESERVED` 不得接受 delta，带非零 truth delta 的 `CHANGED` 缺少 exact protected confirmation 不得接受，新 truth acceptance 与 semantic event 必须原子提交；零 truth delta 的 `CHANGED` 只记录产品语义结果，不重复创建 truth epoch。
45. Semantic Assessment、Gate plan、Evidence、Verdict 与 completion 必须共同绑定 final accepted truth digest；truth/source 变化使旧 assessment 或 Evidence stale，未 reconciliation 的 truth drift 不得计划 Gate 或完成 Change。
46. Change Impact 必须保存产品语言的 expected changes 与 expected preservations；Semantic Assessment category 只能使用版本化集合，truth 变化必须包含 `project-truth`，`UNKNOWN` 必须包含 `unknown`。
47. `contracts/**` 中每个文件都必须被 `truth.json` 的唯一 contract ID 引用，两个 ID 不得指向同一 canonical path；文本 impact unknown 只能覆盖新增 truth entity，修改/删除既有 purpose、fact、contract 或 accepted unknown 必须由精确结构化 Impact 覆盖。
48. 新 task 必须能通过 authority-only `truth get` 恢复 latest 或指定历史 accepted structured truth 与 exact contract contents，即使候选 `.ecp` drift/malformed；candidate bytes 不得冒充 accepted content，未接受 digest、blob 缺失/损坏必须 fail closed。
49. `policy get` 必须能通过 authority-only history 恢复 latest 或指定历史 accepted Project/Policy/Gates，即使候选 control files drift/malformed；读取不得接受、重写或执行候选配置。
50. 产品语义 CHANGED 与 Project Truth CHANGED 必须解耦：expected semantic change 不得用 `PRESERVED` 完成，零 truth delta 的真实 `CHANGED` 可取得 Evidence/Verdict，未声明且无 startup uncertainty 的意外 `CHANGED` 必须阻断。
51. Change Impact 关联 invariant 后，该 invariant 的风险必须抬高 effective risk，且其 `gate_ids` 必须进入 exact Gate plan；只通过同风险通用 Gate、遗漏 invariant Gate 时 Verdict 仍 BLOCKED。
52. 同一 Change 演进 Project Truth 时，effective risk 与 invariant Gate requirements 必须对 starting/final truth epoch 取保守并集；删除 Gate 关系或降低新 truth 中的风险不能降低该 Change 自己的门槛。
53. Plugin launcher 在空 ambient PATH 和深层模拟 Plugin cache 路径中仍能选择本机 bundled Core 并返回正确 version；它不得调用另一个系统或仓库 `ecp`。
54. runtime manifest 必须完整覆盖四个支持 target，版本等于 Plugin manifest，且每个 artifact 的相对路径、regular/executable 类型、size、SHA-256 和 sidecar 全部匹配；缓存副本中的 binary 发生一字节变化后 launcher 必须在 Core 启动前以 `ECP_RUNTIME_CHECKSUM_MISMATCH` 和退出码 4 拒绝。
55. legacy `events.json` authority history 无需迁移即可加载；达到新写入阈值后必须滚动到多个连续 segment，由新的 Store 实例重放出相同 revision、event head 与 projection。
56. continuation event 内容被修改、跨段 previous hash 不一致、编号缺口、非 canonical 文件、空段、超限或不安全类型/权限必须 fail closed；安全有界的原子写残留可以忽略但不得改变 projection。
57. 单次原子 event batch 放不进一个 segment 时必须在写入前以 `EVENT_BATCH_TOO_LARGE` 拒绝，authority revision/event head 不变且不得创建部分 continuation state。
58. segmented event history 必须受总字节数、segment count 和分层 terminal reserve 共同约束；`gate_run_started`/每条 Evidence 在入库时都必须保证 run terminal headroom，达到普通写入上限后仍可写 GateRun terminal，并为显式 project disable（包括已观察 ACTIVE GateRun/Change 的同批 interruption + cancellation）保留有界容量。
59. 对同一未变化 authority 向两个不同目标执行 export，必须产生同一 bundle digest、file count 和 total bytes；导出前后 live revision/event head 不变，最终目录 private/read-only，且可在没有原仓库/live authority 时通过 `authority verify`。
60. Export 必须覆盖多个 continuation event segments、所有历史 accepted truth 引用和所有 Evidence 引用，同时排除 locks、atomic temp 与 orphan blobs/artifacts；candidate `.ecp` malformed 不得阻止 authority-only export。
61. Bundle 任一文件缺失、额外、路径/类型/权限不安全、超限或内容摘要变化必须 fail closed；即使修改者重算 outer manifest，event sequence/hash chain、projection、truth blob 和 Evidence digest 校验仍必须发现语义破坏。
62. Export 目标位于仓库/live authority 内、父目录不存在或目标已存在时不得写入；export/verify 不得成为 restore、state replacement、upload、publish 或 authenticated attestation 的隐式授权。
63. `gate run` 必须在首个项目命令前持久化 `IN_PROGRESS` GateRun；正常、内部失败和 context cancellation 分别产生 `COMPLETED`、`FAILED`、`CANCELLED`，每条 Evidence 绑定同一 run，其 canonical artifact path、ID、accepted Gate epoch、执行结果、时间、输出字节数/截断与摘要在 event replay 时必须一致；impossible/重复 terminal、同 Gate 重复 Evidence 或引用另一 Evidence artifact 的回放必须 fail closed。
64. live GateRun 持有 advisory lease 时并发 contender 不得把它误记为 interrupted或创建第二个 run；持有者被 `SIGKILL` 后，history 先保留 `IN_PROGRESS`，下一位真正取得已释放 lease 的操作必须先追加 `INTERRUPTED`，再开始新的 run 或终态 mutation。
65. unresolved `IN_PROGRESS` GateRun 必须使 status/context 暴露 `active_gate_run` 且 Verdict `INDETERMINATE`；`gate history` 在 candidate Draft malformed 时仍可读取 `IN_PROGRESS/COMPLETED/FAILED/CANCELLED/INTERRUPTED` 的 authority-only 历史。
66. `authority health` 必须在不读取 candidate `.ecp` 的情况下取得 Gate lease → mutation lock 一致快照；candidate malformed 时仍可返回报告，live Gate lease holder 则不得被误分类或绕过。
67. 健康 authority 的全部历史 referenced Truth/Evidence 必须逐一验证，报告 exact revision/event head、bytes/segments/current mutation limit/terminal reserve 和剩余容量，且操作前后 revision/event head 不变。
68. referenced Truth/Evidence 缺失、摘要损坏或危险类型必须产生 `ok: true` 的 `INDETERMINATE` health 和退出码 4；event projection/hash chain 自身不可信时必须 typed integrity fail closed，不能从部分 projection 构造报告。
69. safe atomic/staging remnants、orphan Truth/Evidence 和 private unrecognized files 必须形成 `ATTENTION` 和退出码 3，且 health 不得删除或修复它们；unsafe orphan/unrecognized entry 必须形成 `INDETERMINATE`。Unsafe authority directory 必须在取锁前 fail closed 且不得被 chmod 修复；object-store 或中间 artifact symlink 不得被跟随读取。
70. health 成功取得 released Gate lease 后可报告 exact unresolved run 与 `GATE_RUN_INTERRUPTION_RECOVERABLE`，但不得追加 `INTERRUPTED`；event byte 使用达到 80% 或 continuation 剩余不超过 10% 必须稳定报告 capacity warning。
71. `change start` 必须拒绝缺少 structured Requirement、包含 `BLOCKING_UNKNOWN`、decision/revisit/verification 非法或任一 acceptance/journey/data/operation/expected-change/expected-preservation/impact-unknown 未 exact 覆盖的合同；全部显式决定并覆盖后才可创建 ACTIVE。
72. Semantic Reconciliation 必须逐项覆盖所有 Requirement；AUTOMATED item 的 result 必须映射 exact Gate IDs，且只有每个 mapped Gate 的当前 PASS Evidence 才满足 Verdict；REVIEW/NOT_APPLICABLE/DEFERRED_SAFE 必须匹配 start 决定，EXTERNAL 必须保持 `EXTERNAL_PENDING` 并阻止 local PASS。
73. source 仍不同于最新 cancelled Change baseline 时，无 `supersedes_change_id` 的新 Change 必须拒绝；合法 replacement 必须继承原 baseline/lineage 和 inherited touched paths，不能通过 cancel/start 洗掉 delta。
74. established Project Truth 必须从 scope/touched path 反推 direct component、`depends_on` 反向传递依赖者及其 capability/invariant；漏报或未映射 final path 阻止 PASS，且 inferred facts 仍保守参与 effective risk 和 Gate plan。
75. Risk Gate 必须只在 path/component/capability/invariant selector 适用时选择；无 selector Gate 保持 universal，Invariant/Requirement 显式 Gate 始终强制，因而计划既不得遗漏必要 Gate，也不得无理由运行 unrelated affected/full Gate。

“确定性 Verdict”指同一精确 subject、同一 Effective Config 与同一有效 Evidence 集合产生相同的 status、reasons、assessments 与 `subject_digest`。`evaluated_at` 是观测元数据，不属于 `subject_digest`；完成事件保存当次最终 Verdict payload，因此历史核验不依赖再次调用只支持 ACTIVE Change 的 `verdict`。

## 13. 自举与保证边界

开发工作树中的 ECP 源码、它自己的 Gate 和同用户可写的本地 state 不能为该源码签发高可信 self-hosting 证明。只有固定、可识别、由外部信任流程安装的 Core 才能进入更强 TCB；v0.3 的可执行文件摘要只用于精确 Evidence 兼容性，不是代码签名、来源证明或远程 attestation。

# ECP v0.3 Architecture

## 1. 设计原则

ECP 采用“机制与策略分离、执行与裁决分离、候选配置与生效配置分离、历史事实与当前适用性分离”的结构。

```text
Repository-owned, reviewable inputs       Core-owned local authority
───────────────────────────────────       ──────────────────────────
.ecp/project.json                         accepted control config + digest
.ecp/policy.json                          accepted Project Truth + file manifest
.ecp/gates.json                           immutable Workspace binding
.ecp/truth.json                           Project/Workspace registration
.ecp/contracts/*                          activation + Change/semantic events
source + tests                            Evidence metadata and bounded logs
                                           recomputed Verdict
```

仓库侧内容全部可被 Agent 修改，因此只能作为 proposal。Core 对 Control Config 和 Project Truth 分别计算摘要、分别保存 accepted payload；只有 accepted epoch 能驱动 Gate 和 Verdict。`evaluated_at` 是观测元数据，确定性的 subject/status/reasons 与它分离。

## 2. 组件

### CLI

负责稳定 JSON 输入输出、参数验证与退出码，不承载独立业务规则。Skill 和未来 MCP 都应调用相同 Core API。

### Codex Plugin Launcher

Plugin 把支持的 Darwin/Linux arm64/amd64 Core 与 Skill 一起分发。Skill 只从自身安装路径解析 `scripts/ecp`；launcher 由该物理路径定位 Plugin root，选择单一匹配 runtime，拒绝 symlink/缺失/非 executable 文件，使用固定系统 SHA-256 工具核对 sidecar 后才 `exec` Core。它不查找 ambient PATH、仓库或临时目录中的 `ecp`。`runtime/manifest.json` 另外把四个平台 artifact 的相对路径、size 和 digest 绑定到 exact Plugin version，并由 Project Pack 测试验证。

Codex 本地 marketplace 安装会把 Plugin 复制到 cache，因此 launcher 不能依赖源码仓库绝对路径。自动化测试把完整 Plugin 复制到深层 cache-like 临时目录，在空 PATH 下运行 version，再损坏本机 binary 并要求 checksum mismatch 在 Core 启动前 fail closed。该机制证明安装副本的相对发现和包内一致性，不证明 Plugin 发布者身份或真实 desktop 安装已完成。

### Project Mode Router

`project status` 以 physical Git identity 与仓库外 authority history 判断当前 canonical Workspace 的二态模式。它不执行项目代码、不构造 source snapshot；对没有 binding 的 Workspace 返回 `enabled: false` 时也不创建 `.ecp` 或 authority state。`.ecp`、Plugin 安装、另一个 clone/worktree 的状态都不是 enable signal。

每次 enable/disable 形成一个新的 `activation_id` 并链接 previous activation。Core 还返回 opaque `activation_token`，其摘要覆盖 authority/Workspace/Project registration、authority revision、event head、当前 activation 与 enabled 位。Adapter 只能把一次 immediately preceding status 的 token 内部传给 enable/disable；status 后若追加 Change、Evidence 或其他 authority event，旧 token 冲突。Mode router 不把 `BLOCKED`/`INDETERMINATE` 当作 disabled：已启用但不可操作的 Workspace仍必须 fail closed。

### Project Loader

从 physical `.git` boundary 发现 Git root，并核对 Git 所解释的 top-level，避免 repo-local `core.worktree` 重定向。严格读取 `.ecp/`，拒绝 symlink、过大文件、未知 schema 和未知 JSON 字段；解析语义和 tree digest 使用同一组内存 bytes，并通过第二次观察拒绝并发 epoch 混读。Loader 输出三个摘要：`config_digest` 覆盖 project/policy/gates，`truth_digest` 覆盖 truth/contracts，`bundle_digest` 只用于完整树诊断。

### Project Truth Registry

`truth.json` 保存 Purpose、Capability、Invariant、Component、Decision、Contract Reference 与 Unknown。Core 校验全局唯一 ID、跨实体引用、component 路径、contract 文件、gate reference 和 maturity；每个 `contracts/**` 文件必须由 contract ID 寻址。仓库外每个 `project_truth_accepted` 事件保存 exact truth digest、结构化 payload 和文件 manifest，projection 保留 revision history；exact UTF-8 bytes 则先写入按 SHA-256 去重的私有 `truth-blobs/`。`truth get` 可按 latest 或历史 accepted digest 恢复这些内容并重验类型、权限、大小与 digest；候选 Project Truth 永远不是权威，也不能在 blob 缺失时充当恢复副本。

Control Config 的每次 acceptance 同样在事件历史中保存结构化 Project/Policy/Gates payload。`policy get` 可按 latest 或历史 accepted digest 经 authority-only 路径恢复它，用于候选 drift/malformed 时的比较和人工恢复；读取不会执行 Gate、改写候选或产生 acceptance。

### Semantic Reconciler

在 ACTIVE Change 的最终源码上，Core 比较 previous accepted truth 与 candidate truth/file manifest，产生确定性、排序后的 `TruthDelta`。`PRESERVED` 只允许零 delta且不得逃避 declared expected changes；`CHANGED` 可以用零 delta记录不需要更新 durable truth 的真实产品语义变化，非零 delta 则要求被 Change Impact 覆盖、显式 protected confirmation，并与新的 truth acceptance 原子追加；未声明且无 startup uncertainty 的意外 `CHANGED` 被阻断；`UNKNOWN` 只记录不确定性。最终 Verdict 只接受与 final source fingerprint 和 final accepted truth digest 同时匹配的最新 assessment。

### Git Source Snapshotter

构造与绝对路径、遍历顺序和时间无关的 manifest。它同时观察 HEAD、所有 index stages、worktree、非 ignored untracked 文件、`.ecp/` 和已初始化 submodule 的递归 manifest，从而允许：

- dirty baseline；
- Change 中途 commit；
- rename/delete/mode/symlink；
- 基于 baseline/current manifest 的准确净变更路径。

Core 连续构造两次 manifest 并要求指纹稳定。它还把实际固定 Git executable 的 realpath 与 SHA-256 内容摘要写入 `SourceSnapshot`/`SnapshotRef` 并纳入 fingerprint，因此 Git bytes 替换会使旧 Evidence stale。该摘要仍不覆盖 ignored 文件和其他系统环境，因此命名为 source fingerprint。

Snapshotter 不通过 ambient `PATH` 解析 Git。当前受支持的 POSIX Unix authority 路径只检查 `/usr/bin/git` 与 `/bin/git`，候选须解析为 regular executable。若全部不可用，Core 返回 `TRUSTED_GIT_UNAVAILABLE` 并 fail closed。这避免仓库或 Agent 用 `PATH` shim 替换 Core 的 Git 观察，代价是 v0.3 不支持仅安装在 Homebrew 或自定义路径的 Git。源码可交叉编译 Windows 固定候选解析器，但非 Unix authority operations 会更早以 `PLATFORM_SECURITY_UNSUPPORTED` 停止，因而不构成 Windows 运行支持。

递归 submodule 内容用于完整性绑定，但 Change 的 touched/scope 投影在 v0.3 折叠为 mount path；内部 path-risk rule 与该 mount 重叠时按更高风险保守升级。细粒度 submodule scope 留给后续扁平化 manifest 版本。

### Event Store

每个 canonical 仓库外 state-directory path 派生一个 opaque `authority_id`，再在该 authority 内按 `(project_id, workspace_id)` 隔离。`authority_id` 只是本地 state 目标选择器，不是认证身份或签名。v0.3 采用无外部依赖的原子分段 JSON event history：

```text
state-v1/
├── workspace-bindings/<workspace-id>.json
└── projects/<project-id>/workspaces/<workspace-id>/
    ├── events.json
    ├── event-segments/
    │   ├── 0000000000000001.json
    │   └── 0000000000000002.json
    ├── .lock
    ├── .gate-run.lock
    └── artifacts/<change-id>/<evidence-id>/
        ├── stdout.log
        └── stderr.log
```

Workspace binding 由 canonical Workspace 直接索引，不能由未接受的 Draft `project_id` 选择或替换。Binding、根 event segment、continuation segments 和 Evidence stdout/stderr artifacts 在读取时必须仍是有界、非 symlink 的 regular private files；替换类型或暴露 group/other 权限会以 integrity failure 停止，不会继续信任 projection 或 Evidence。因此 v0.3 权威状态仅支持能提供 POSIX private-file 语义的 Unix；非 Unix 平台在项目或 authority 写入/加载前返回 `PLATFORM_SECURITY_UNSUPPORTED`，只保留 `ecp version` 等不触及 authority 的操作。改变 canonical state path 会选中不同 `authority_id`；v0.3 不支持 state-directory migration。

`events.json` 保持第 0 段兼容性；新写入在 8 MiB 阈值后滚动到连续命名的 continuation segments，旧版单文件可读到 64 MiB。所有段共用一条 sequence/hash chain；加载时要求从第 1 段连续、非空、私有且有界。单次 mutation 的 event batch 永不跨段，当前段重写和新段创建均为 atomic rename。整个 history 最多 1024 个 continuation、1 GiB，并在 enabled/never-enabled projection 上保留 2 MiB project-disable capacity；存在 ACTIVE Change 时必须容纳同一次 append 的 cancellation + disablement。

Darwin/Linux/BSD 上 `.lock` 与 `.gate-run.lock` 是持久 metadata file + 内核 advisory lock，进程退出自动释放所有权；其他 Unix 使用保守 sentinel/manual crash-recovery fallback，但尚未列入已验证运行矩阵。选择原子分段文件而不是立即引入 SQLite，原因是 v0.3 是单用户、单 Workspace 串行 mutation，且当前能力无需新增依赖。事件接口保持存储无关；未来若真实并发、查询、压缩或迁移需求成立，可迁移到 SQLite 而不改变领域合同。分段没有解决 truth/Evidence artifact GC、安全 compaction、自动 retention 或多年容量验证。

Authority Exporter 在同一 mutation lock 下从已验证 projection 构造 exact reference set，并原子生成一个与 live state 分离的 portable directory：manifest + binding + event segments + referenced truth blobs + referenced Evidence artifacts。Verifier 不读取仓库或 `~/.ecp`，而是以 manifest 中原 authority/Project/Workspace 身份实例化只读 Store，重放事件并验证全部引用。它为迁移设计、备份与未来 CI consumer 提供标准输入，但 v0.3 不会把 bundle 自动恢复到另一 Workspace，也没有签名或可信发布者身份。

Authority Health Inspector 是另一条不依赖 candidate `.ecp` 的 authority-only 路径，但它不创建 bundle。它要求 authority state directory 在取锁前已经是 private real directory，再取得 Gate lease、mutation lock，并在两把锁内重载 binding/event projection；因此诊断不会借普通 mutation lock 入口 chmod/修复目录。Event projection 不可信时直接返回 typed integrity error，可信时才核验所有历史 referenced Truth/Evidence 并扫描实际 state tree；核验不会跟随 object-store 或中间 artifact directory symlink。输出把事件容量、terminal reserve、segment 余量、safe remnant、orphan、unrecognized/unsafe entry 和 unresolved GateRun 分成 `HEALTHY`/`ATTENTION`/`INDETERMINATE`。扫描受 200,000 entry、256 GiB aggregate 和 100 条详细 finding 限制；在 inventory 限制内保留精确计数，超过 finding 输出上限时另报 truncated 数。Inspector 不 repair/GC/restore/terminalize，只有 private advisory lock metadata 可能因锁获取创建或刷新。

### Change Engine

只在 project mode enabled 时维护单 ACTIVE Change、activation ID、baseline、scope、acceptance、structured Impact、Requirement decision ledger、starting truth digest、risk 和 `ACTIVE → COMPLETED|CANCELLED` 生命周期。每个验收、保持、旅程、数据/运行影响、预期变化与已发现 unknown 必须被一个 exact Requirement 覆盖；每项保存当前要求、状态、理由、决策来源、验证方式和可选 revisit 条件。`BLOCKING_UNKNOWN` 可以在 Adapter 草稿中表达，但 Core 在第一次实现写入前拒绝启动它。

单独 cancellation 是带 actor/reason 的终态补偿事件，保持项目 enabled；project disable 则可把已观察 ACTIVE Change 的 cancellation 与新 disabled activation 原子追加。两者都不要求 PASS、不删除 Evidence、不修改工作树。若取消项留下源码 delta，replacement 必须显式 supersede 最新取消项、继承它的原始 baseline/lineage，并把 inherited touched paths 纳入新 scope；否则 Core 不允许把这些旧修改吸收到一个伪装成干净的新 baseline。状态不保存“verified=true”；assurance 每次根据当前 activation 与 subject 重算。Contract version 0 的已记录历史继续按旧 subject/risk/Gate 语义重放，新 Change 使用 version 2 合同。

### Gate Runner

`gate plan` 解析已接受 Gate、生成最小环境、解析并哈希 executable，并把 exact authority/Workspace/activation/Change/config/truth/source/risk、inferred impact、Required Gates、实际 cwd、executable/environment identity 与 Core identity 绑定到一个 opaque `plan_digest`。Established Project Truth 的 component path roots 从 scope/touched paths 反推 direct component，随后沿 `depends_on` 的反向传递闭包加入所有依赖者，再得到相关 capability/invariant；声明与推导取并集参与风险与 Gate，漏报或最终 unmapped path 阻止 PASS。

Required Gates 是三者的并集：匹配 effective risk 且 selector 适用的 Gate、Impact/inferred invariant 显式 `gate_ids`、AUTOMATED Requirement 显式 Gate。Selector 可匹配 path/component/capability/invariant，空 selector 保持 universal；显式关系不能被 selector 过滤。计划按 `fast → affected → full`、同 tier 内按 ID 稳定排序，只运行与当前 contract 相关的最小集合，但 Required Gate 为空仍禁止 vacuous PASS。Project Truth 演进时，Core 同时读取 Change 启动摘要对应的 authority history 与 final accepted truth，对两版全部 invariant/unknown 取更高风险并合并 Gate，避免同一 Change 通过弱化 truth 来弱化自己的门槛。

DefaultConfig 不继承 `HOME`；常见 secret/capability 环境名 denylist 会在预检和执行两处拒绝，但该名称检查仍只是 best-effort，不隔离同用户文件/网络能力。调用方必须审阅计划并把其 exact Change ID 与摘要传回。Runner 在 Workspace lease 内重新加载并重建计划，在任何项目代码前先提交 exact GateRun start；只有当前 activation、Change ID 与摘要均匹配才捕获 pre fingerprint、执行 argv、捕获 post fingerprint、保存 artifact，再把绑定 `gate_run_id` 的执行事实提交到 Event Store；每个后续 Gate 前重复相同核对。Run 最终记录 `COMPLETED/FAILED/CANCELLED`，被杀或崩溃留下的 `IN_PROGRESS` 只能由后来真正取得已释放 advisory lease 的操作记录为 `INTERRUPTED`。CLI 的 signal-aware root context 捕获 `SIGINT`/`SIGTERM`，使当前受支持的 Unix Runner 可以清理独立进程组并通过 defer 释放锁；crash recovery 只修复权威生命周期，不保证收割所有后代进程或自动续跑。非 Unix 直接进程取消代码仅保持可交叉编译，不代表 v0.3 authority 运行支持。

### Verdict Evaluator

纯函数式地解析当前 effective requirements：

```text
current config accepted?
→ current Project Truth accepted?
→ current project activation enabled?
→ baseline/current delta within scope?
→ final paths map to declared Project Truth impact?
→ final source has applicable non-UNKNOWN semantic assessment?
→ every Requirement has an exact compatible result?
→ effective risk
→ required Gate set non-empty?
→ each Gate has latest applicable Evidence?
→ every AUTOMATED Requirement has PASS Evidence from its mapped Gates?
→ no unsupported EXTERNAL Requirement remains pending?
→ artifacts intact?
→ required acknowledgement matches exact subject?
→ PASS / BLOCKED / INDETERMINATE
```

## 3. 核心数据流

### Bootstrap 新 Workspace

```text
validate physical Git root + metadata before any write
→ canonicalize external state path and derive authority id
→ derive workspace id and verify external state path
→ generate project id and exact initial bytes + digest
→ atomically install draft .ecp directory
→ stable reload and require the same project id + precomputed digest
→ create immutable workspace binding
→ append project_registered + config_accepted bootstrap events
→ remain disabled until an explicit project enable event
```

只有 `.ecp/` 由同一次 `project init` 新建、且稳定重读仍等于 Core 写入前计算的 initial digest 时才允许 bootstrap acceptance。若 `.ecp/` 调用前已存在，`project init` 返回 `PROJECT_REGISTRATION_REQUIRED` 且不写 authority；明确的项目 enablement 流程先用只读 `project inspect` 取得同一次的 authority ID、Workspace ID 与 candidate digest，Adapter 审阅候选后内部传给 `project register`。后续 `policy accept` 同样强制同一次已审阅 JSON 的 expected authority/Workspace/candidate digest；初次 enable request 可以表达 setup intent，之后的 policy drift 必须取得新的明确确认。若 binding 已创建而 event registration 因崩溃中断，只允许初始 identity/digest 完全相同的幂等恢复。Bootstrap/registration/acceptance 都不等于 enable。

以上每一步各自原子落盘，但 `.ecp`、binding 与 events 不是一个跨文件系统事务；安全性依赖 identity/digest 绑定与 fail-closed 恢复，而不是把整段 bootstrap 描述成单次原子提交。

### Read Project Mode

```text
discover physical Git root + derive Workspace ID
→ derive authority target
→ inspect Draft Config shape when present, without source snapshot
→ if no binding: return disabled + no authority write
→ otherwise load authority projection and current activation
→ return enabled/config/operational/assurance/active-change diagnostics
   + opaque activation_token
```

未启用、READY 与 ACTIVE status 是退出码 0。已启用但 BLOCKED/INDETERMINATE status 分别使用退出码 3/4，却仍是 stdout `ok: true` 的正常 result；Adapter 必须先读 `enabled`，不得把非零 assurance 状态误当 disabled。

### Enable Project

```text
receive exact authority + Workspace + activation token + accepted config digest
→ load and require the observed registered disabled epoch
→ acquire Workspace lease, reload and recheck every precondition
→ require current accepted config and truth
→ require at least one Gate for default risk
→ resolve/preflight Gate cwd/executable/environment without executing it
→ stable recheck candidate config
→ append project_enabled with new activation ID + previous activation link
```

Draft setup、registration 和 acceptance 可在 enable request 中分阶段发生，但在最后 event append 前 authoritative mode 始终 disabled。任一步失败不得写 enabled event。`project enable` 的 preflight 只解析执行上下文，绝不运行 Gate。

### Disable Project

```text
receive exact authority + Workspace + activation token from one status
→ authority-only load (Draft Config may be malformed)
→ acquire Workspace lease, reload and recheck the token
→ if ACTIVE Change exists, prepare change_cancelled
→ if enabled, prepare project_disabled with a new activation ID
→ append both events atomically
→ retain worktree, Draft Config, Evidence and history
```

Token 覆盖 status 观察时的 authority revision/event head，因此 status 之后新增 Change 或 Evidence 会使 disable conflict，而不是取消未观察的目标。未注册或已经 disabled 且无 ACTIVE Change 时不追加 event，返回幂等 disabled status。

### Start Change

```text
receive expected authority id + workspace id + activation token + config digest + truth digest + source fingerprint from context
→ require the exact canonical state authority
→ rediscover canonical Workspace and require exact workspace id
→ require project enabled in the exact observed activation epoch
→ load accepted config and require exact config digest
→ load accepted Project Truth and require exact truth digest
→ allow a valid candidate truth drift only as a recovery/adoption proposal; never bind it as starting authority
→ ensure no ACTIVE change
→ normalize scope and validate structured Impact references, expected changes/preservations, and unknowns
→ require a sorted Requirement decision ledger with exact coverage of every material contract item
→ reject BLOCKING_UNKNOWN; validate status, rationale, source, verification mode, revisit condition and mapped Gate IDs
→ infer component/capability/invariant from scope path ownership; reject undeclared or unmapped scope impact
→ if cancelled edits remain, require exact supersession and carry the original baseline/lineage
→ derive declared + inferred invariant/unknown risk from accepted Project Truth
→ prove declared, impacted-fact and scope-reachable risks have Required Gates
→ add impacted invariant and AUTOMATED Requirement gate_ids, selector-filter risk Gates, and preflight the complete union without executing it
→ capture complete baseline manifest and require exact source fingerprint
→ append change_started
```

六项 expected 值必须来自 immediately preceding 的同一次 `context get`。它们在 append 前作为乐观并发前置条件，避免请求落到未观察的 authority、跨越 disable/re-enable epoch，也避免 context 之后出现的源码、配置、真相或 Workspace 切换被静默吸收到新 baseline；mismatch 不创建 Change。

### Reconcile Semantics

```text
load ACTIVE Change + previous accepted truth + candidate truth
→ compute deterministic structural/file delta
→ require existing delta coverage by exact Change Impact references; a concrete startup unknown covers additions only
→ bind assessment to exact activation + Change + final source fingerprint
→ require one sorted result for every Requirement; automated results name exact mapped Gates,
  review/N-A/deferred results cannot claim Gate Evidence, and external results remain EXTERNAL_PENDING
→ PRESERVED: require zero delta and no declared expected change; append semantic_assessed
→ UNKNOWN: append uncertainty; keep Verdict blocked
→ CHANGED with zero delta: require expected change or startup uncertainty; append semantic_assessed
→ CHANGED with truth delta: require Impact coverage + protected confirmation
                              append semantic_assessed + project_truth_accepted atomically
```

`truth diff` 是只读比较，不接受候选内容。`truth reconcile` 的 previous/candidate truth digest、source fingerprint 和 activation token 都必须来自紧邻观察；任一 drift 都 conflict。真相接受后不会修改产品源码，后续 Gate plan/Evidence 显式绑定新的 truth digest。

### Plan Gates

```text
load current ACTIVE change + accepted config + accepted Project Truth
→ capture stable current source
→ derive touched paths and path-owned inferred impact; expose undeclared/unmapped results
→ derive effective risk and selector-matched risk Gates
→ union mandatory invariant Gates and AUTOMATED Requirement Gates
→ on truth evolution, union starting/final truth risks and invariant Gates
→ order fast, affected, full; omit unrelated checks without omitting explicit requirements
→ resolve each Gate cwd/executable/environment without executing it
→ return reviewable plan + opaque plan_digest
```

摘要覆盖 plan 中的 exact authority/project/Workspace/activation/Change identities 与解析结果；它是 Core 生成的并发 token，不是调用方应重算或解释的签名。

### Run Gate

```text
validate expected Change ID + plan digest
→ acquire lease and reload current ACTIVE change + effective config + accepted Project Truth
→ if the lease was released with an older IN_PROGRESS run, append its INTERRUPTED terminal first
→ rebuild the plan and require both expected values still match
→ append gate_run_started before executing project code
→ before each Gate, capture stable pre fingerprint and rebuild the same plan
→ execute the already-resolved configured argv with bounded context
→ capture post fingerprint
→ write bounded artifacts and digests
→ append evidence_recorded bound to the active GateRun
→ repeat the plan check before the next Gate
→ append exactly one COMPLETED / FAILED / CANCELLED terminal
```

Gate 不通过自然语言告诉 Core 自己是否通过。Core 只使用 wait status、allowed exit codes、timeout、artifact integrity、execution context identity 和 fingerprint equality。v0.3 在 schema 阶段拒绝声明需要网络或产生外部副作用的 Gate，但未隔离命令仍可能谎报并自行执行这些行为。

### Compute Verdict

Verdict 是只读重算。其 `subject_digest` 覆盖 exact authority/Workspace/activation 与其他裁决输入。Evaluator 还会对当前 source/config/risk/execution context 重建 Gate plan，只有 Evidence 保存的 `activation_id` 属于当前 enabled epoch、且 `plan_digest` 与当前 plan 一致才可适用。历史 Evidence 永久保留，但只有与当前 subject 完全匹配的 Evidence 可满足 Requirement。任何 `IN_PROGRESS` GateRun 都使当前 Verdict `INDETERMINATE`，因为只读调用不能知道持有者仍在执行还是已经消失。`change list`、`evidence list` 和 `gate history` 通过 authority-only loader 提供历史查询，即使当前 Draft Config malformed/missing 或项目已 disabled 仍可读取；event hash 损坏到无法构造 projection 时返回 integrity error，而不是虚构一个 Verdict。

### Complete Change

Adapter 把 immediately preceding PASS Verdict 的 Change ID 与 subject digest 一并传给 `change complete`。Core 在完成前持有与 Gate sequence 共用的 Workspace lease，要求项目仍处于同一 enabled activation，重新加载并核对 Change ID、重新计算并精确核对 subject digest、二次观察当前 source，然后才把最终 Verdict payload、subject digest 和 verdict digest 追加到终态事件。普通实现请求在当前 PASS 后由 Adapter 自动完成，不要求第二个用户命令。v0.3 仍不能阻止 ECP 之外的进程在瞬间改写工作树；受保护 CI 必须对最终 commit 独立复算。

### Record Acknowledgement

本地确认请求必须携带 exact Change ID 与刚审阅的 Verdict subject digest。Core 要求同一 enabled activation，重载并重算后只在二者均精确匹配、且除 acknowledgement 外所有要求已经满足时追加事件；任何 mismatch 都 conflict，不会把旧确认转移到同一 Change 的新 subject。Adapter 只有在向用户解释人类可读风险并取得明确当前确认后才可内部记录。

### Cancel Change

`change cancel --authority AUTHORITY_ID --workspace WORKSPACE_ID --change CHANGE_ID --actor --reason` 只读取 physical Git identity、Workspace binding 和 event projection，不依赖当前 Draft Config。三个 opaque target 必须来自 immediately preceding `change list` 的同一条 ACTIVE 记录。它与 Gate/Complete/project disable 共享 lease；进入 lease 后 Core 重新加载 authority，并要求项目仍 enabled、authority、Workspace 与当前 ACTIVE ID 仍精确相等，否则返回 conflict。随后才使用 revision CAS 追加 `change_cancelled`。该路径用于安全放弃无法或不应继续的 Change并保持项目 enabled，不会修改源码、误操作另一套 state/Workspace 或误取消后来创建的 Change，也不会把 BLOCKED 变成 PASS。显式 project disable 则采用前述原子 cancellation + disablement 路径。

### GateRun continuity 与 Partial Sequence

每个 Gate Evidence 单独原子提交，但整个 sequence 由先于项目代码的 `gate_run_started` 和唯一 terminal event 包围。正常遍历完所有选择项记录 `COMPLETED`，即使其中某条 Evidence 是 Gate failure；Core 流程错误记录 `FAILED`，context cancellation 记录 `CANCELLED`。一旦 run start 已提交，后续 CLI error envelope 的 `partial_result` 就返回 run ID、终态和已落入 authority store 的 Evidence，Evidence 可以为空。首个 run start 前的 plan mismatch 不执行项目代码，也没有伪造的 partial result。

若进程无法执行 defer，run 保持 `IN_PROGRESS`，状态、context 和 Verdict 不假装完成。Darwin/Linux/BSD advisory lease 在持有进程退出时由内核释放；下一位成功持有同一 lease 的 lifecycle 操作先把旧 run 追加为 `INTERRUPTED`，再处理自身动作。仍在执行的 live holder 会阻止 contender，因此不能被年龄/PID 猜测误杀。该机制不自动重跑未执行 Gate、不回滚外部副作用，也不证明 crash 后的项目子进程已经退出。

## 4. 配置格式

v0.3 使用严格 JSON 而不是 YAML：

- Go 标准库原生支持，无新增解析依赖；
- 可以拒绝未知字段和 trailing data；
- 避免 YAML tag、alias、隐式类型和环境插值；
- 所有动态 include/eval 均不支持。

Control Config 与 Project Truth 分别摘要；格式化变化仍会改变所属候选摘要，这是保守且可审计的策略。未来若需要语义 canonicalization，必须先版本化 digest 算法。

## 5. Codex 适配

v0.3 Plugin 仅打包一个 `ecp-change` Skill：

- Skill metadata 覆盖任何可能修改 Git Workspace 的请求、从只读转为实现的 follow-up，以及显式 enable/disable/status/history 请求；
- 每次普通仓库 mutation 前先只读 `project status`：disabled 退出 ECP 子流程并正常开发，enabled 自动进入受治理闭环，mode 未知则在写入前停止；
- CLI/Core 决定 project mode、activation、Change state、Evidence 适用性和 Verdict；
- Skill 只在内部转交 immediately preceding Core JSON 中的 opaque authority/Workspace/activation/config/truth/source/Change/plan/subject preconditions，绝不自行构造、默认展示或自动替换 mismatch 值；最终用户不需要运行 ECP CLI 或复制 token；
- Skill 不直接读取或写入外部 state；
- 明确项目 enablement 可授权 Adapter 完成必要的初始化、Project Truth 建立/诚实 unknown、注册、初始 config/truth acceptance 与 final enable；后续 policy drift、protected truth delta 与风险 acknowledgement 仍需要新的明确确认；
- enabled 项目普通实现先把遗漏的状态、错误、取消、超时、数据与兼容问题变成结构化 Requirement；不能从仓库事实确定的产品选择必须询问用户，不能由 Agent 静默决定；
- enabled 项目随后自动 start/recover Change、建立 Impact、实现、逐项 Requirement reconciliation、审阅最小充分 plan、通过 Core 运行 Gate、读取 Verdict，并在当前 PASS 后自动 complete；不提供单 task bypass；
- 明确 project disable 授权 Core 原子取消 status 已观察的 ACTIVE Change并关闭项目；单独 cancellation 仅响应用户明确放弃 exact active Change；
- Plugin 可被禁用，其他工具与 direct shell 也可绕过，故它只是受支持 Codex workflow adapter，不是安全 enforcement。

MCP 与 Hooks 在 CLI JSON/幂等合同稳定且出现第二个真实调用方后再引入。它们可以提高自动路由覆盖率，但在受保护 enforcement consumer 出现前仍不得宣传为不可绕过边界。

## 6. 可演进接口

未来替换点：

- `EventStore` → SQLite/remote ledger；
- `GateRunner` → sandbox/remote runner；
- local acknowledgement → signed host/organization approval；
- local Evidence → verified CI attestation；
- CLI adapter → MCP/UI/SDK。

这些替换不得改变“精确 subject、fail closed、Verdict 纯推导、proposal 不等于 approval”的核心语义。

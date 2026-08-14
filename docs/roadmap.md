# ECP Roadmap

路线图按 [NORTH_STAR.md](../NORTH_STAR.md) 的单一闭环推进。先证明机制，再增加表面；先让 Project Truth、Change、Impact、Semantic Diff 与 Evidence 形成真实纵向闭环，再强化隔离、签名和远端执行。任何不直接服务该闭环的能力都不进入当前路线。

## Phase 0 — v0.1 Assurance Kernel

目标：让一个真实 Git Workspace 完成精确 subject 的 Change/Gate/Evidence/Verdict 闭环。

状态：已实现本地候选并由自动化测试覆盖核心闭环；尚未发布，也没有受保护 CI/远端强制保证。

- 严格 JSON Draft Config 与 accepted epoch；
- 每个 canonical Workspace 默认 disabled、显式 enable/disable 的二态项目模式；`.ecp`、Plugin 与其他 clone/worktree不隐式启用；
- 不写 `.ecp`/authority state、不执行项目代码或 source snapshot 的 `project status`；enabled+BLOCKED/INDETERMINATE不fallback；
- 由 canonical state path 派生的 opaque local authority target、独立 Workspace binding、activation epoch 与 Project/Workspace 身份隔离；
- authority/Workspace/activation/config/source、active Change ID、opaque Gate plan digest 与 subject digest 的精确 mutation preconditions；
- enable要求accepted default-risk Gate并只做execution-context preflight；disable可原子cancel已观察ACTIVE Change + disable并保留历史；
- baseline/current source manifest 与递归 initialized submodule；
- 固定系统 Git realpath/SHA-256 参与 source fingerprint，不信任 ambient `PATH`；
- path-root scope 与 risk rules；
- 结构化本地 Gate；
- 兼容旧单文件、跨段 hash-chain 可核验、原子滚动且有总容量/关闭保留量的 segmented append events，以及 bounded artifacts；
- 确定性、private/read-only、exact-reference 的 authority export 与 repository/state-independent offline verifier；
- Gate lease + mutation lock 一致快照下的 bounded authority health：event 容量/segment 余量、全部历史 Truth/Evidence 引用完整性、safe remnant、orphan、unrecognized/unsafe entry 与 lease-released unresolved GateRun；
- Evidence 持久化 exact activation ID 与 Gate plan digest，Verdict 要求当前 epoch并重建当前 plan 后再判定适用性；
- deterministic subject/status/reasons 与持久化 final Verdict；
- local acknowledgement；
- stable JSON CLI；
- Change/Evidence 历史只读查询；
- explicit Change cancellation 与 malformed-Draft authority recovery；
- Darwin/Linux/BSD crash-releasing advisory locks；
- POSIX Unix private-file authority 边界，binding/events/artifacts 权限或类型暴露即 fail closed；非 Unix authority operations 显式拒绝；
- DefaultConfig 不继承 `HOME`，扩展 capability/secret 环境名 denylist（仍是 best-effort，非 sandbox）；
- Gate preflight、reviewable/opaque-digest plan 与 partial-sequence result；
- durable GateRun start/Evidence/terminal history，`SIGINT`/`SIGTERM` 形成 `CANCELLED` 并清理 Gate 进程组；Darwin/Linux/BSD 上被杀/崩溃 holder 释放 advisory lease 后，由下一位 holder 记录 `INTERRUPTED`（不含自动续跑、orphan artifact 恢复或 crash 后全部后代进程收割）；
- 自包含 Codex Plugin：四个聚焦 Skill 共享 Plugin-root launcher 与 checksum-verified bundled Core；check/enable/disable 职责分离，普通仓库 mutation 由 change Skill 先 status，disabled 正常开发，enabled 自动 Change→Gate→Verdict→completion，无用户 CLI/PATH/token 和受支持的单 task bypass；
- `SPEC.md` 75 个 v0.3 场景到真实测试/静态/构建证据的可执行 traceability matrix，并把外部真实项目、Desktop、CI、隔离与发布证据显式留作未证明；
- 正常、失败、stale、越界、漂移、并发、超时和损坏测试。

退出标准：`SPEC.md` 第 12 节的核心场景自动化通过，且项目明确披露本地证据边界。

## Phase 1 — Robust Local Runner

Phase 1 原计划的 Runner 纵深强化不再整体先行。除阻断真实 Project Truth 闭环的可靠性缺口外，以下工作延后到语义闭环完成并通过真实项目试点之后：

只在 v0.3 数据与 CLI 合同通过真实项目试点后开始：

- request idempotency、orphan artifact 恢复、crash 后跨平台进程树收割和非 advisory-lock 平台恢复协议；
- activation enable/disable crash-injection、event-capacity exhaustion 的安全恢复与 repair 工具（health 已提供只读容量和完整性诊断，不自动恢复）；
- 其他 Unix sentinel lock 的正式验证，以及非 Unix private-state/锁/进程语义设计；Windows 交叉编译不代表 v0.3 运行支持；
- 跨平台进程树 TERM/KILL、CPU/内存/文件/全局并发上限；
- fd-exec 或受信 executable registry，关闭 hash/spawn swap 窗口；
- Evidence/truth blob 的 scheduled/reference-aware retention、消毒/脱敏 export、显式 repair 与安全 GC；
- accepted raw config tree archive、inspect/export/explicit restore；
- 显式 state-directory migration 协议（v0.3 改变 state path 只会选中新 authority）；
- Gate dependency DAG、TTL/always-run 语义；
- 不可变 Gate input snapshot 与声明式 output；
- safe compaction 或 SQLite authority migration（只在真实容量、并发、查询需求成立并经迁移/依赖授权后），保留 accepted truth blob 引用和跨段 hash-chain 可核验历史；
- property/fuzz tests、崩溃注入和恢复演练。

## Phase 1A — v0.3 Project Truth and Semantic Change

目标：把当前 Assurance Kernel 扩展为 North Star 的最小完整语义闭环。

状态：本地 Core/CLI、自包含四 Skill Codex Plugin runtime、自举 Project Pack 与自动化安全回归已实现；冻结的 installed candidate `0.3.0-dev+codex.20260813115751` 已通过 16/16 串行 fresh-task host canary，0 INVALID。其后真实项目 exploratory use 暴露了 Truth freshness、adapter payload 猜测和历史输出规模问题；当前 post-qualification source iteration 增加 accepted-Unknown disposition、`schema get`、bounded Change history 和 Core-identity upgrade coverage，但尚未打包、安装或重新 qualification。升级/卸载、签名/公开发布、正式双轨项目接手/长期价值和受保护 CI consumer 仍未验证。本阶段不能描述为已通过产品退出标准，canonical checkpoint 见 [../STATUS.md](../STATUS.md)。

- 严格、版本化且项目独立的 Project Truth schema；
- Capability、Invariant、Component、Decision、Contract reference 和 Unknown；
- accepted Project Truth revision 与 candidate drift；
- Change Contract 中的结构化业务/架构/数据/接口影响；
- accepted Truth Unknown 的 `PRESERVED/RESOLVED/REFINED` disposition 与 starting/candidate Truth 对账；
- 覆盖每个 material contract item 的 Requirement decision ledger，阻断 silent unknown，并在 reconciliation 中逐项绑定 review、Gate Evidence 或 external pending；
- cancelled replacement 的原始 baseline/lineage carry-forward，避免通过 cancel/start 洗掉已有 delta；
- established path ownership 的 direct component、`depends_on` 反向传递依赖者及其 capability/invariant 影响推断，漏报或未映射最终路径阻止 PASS；
- risk selector 与显式 invariant/Requirement Gate 并集形成 `fast → affected → full` 的最小充分验证计划；
- 实现后的 Semantic Diff；
- ordinary Change 与 protected fact/constitution change 分离；
- 规则变化由 previous accepted revision 审查，禁止新弱规则自证；
- 未 reconciliation 的 material semantic drift 阻断 completion；
- Project Truth、Change、source、plan、Evidence 和 Verdict 精确绑定；
- Codex Adapter 默认以产品语言展示 Impact、Semantic Diff、未知项和验证边界。

退出标准：删除历史聊天后，一个新 task 能只依赖当前仓库与 authority state 恢复项目事实，完成一个带语义变化的 bounded Change，更新 accepted Project Truth，并取得当前 Evidence-backed Verdict。

## Phase 1B — Real Project Pilot

状态：正式试点尚未进入。旧 installed candidate 的 16-case qualification 已完成，但未按注册表/safe-copy/阈值/七日观察窗执行的真实项目使用只算 exploratory evidence。当前 source iteration 在成为新试点候选前必须完成仓库验证、另行授权的 packaging/install/upgrade host 验证，并针对其新 exact Core/Skill identity 重新跑 16 个 fresh-task canary；第一个产品 `FAIL` 即停止，闭集基础设施问题只记 `INVALID`。随后才选择一个真正的新项目安全副本和一个持续开发的旧项目安全副本做小规模双轨试点；ECP 自身、ECP worktree、静态 Skill 文本检查、Core 单测或 exploratory canonical-project use 都不能代替这两个独立产品轨道。

- 每条轨道只完成两到三个真实 bounded Change 与一次没有旧聊天上下文的 fresh-task handoff；
- 在新项目观察诚实 seed/established Truth 与 Unknown 保留，在旧项目观察历史合同恢复、dirty diff 保护和兼容边界；
- 测量真实有价值捕获、错误阻断、交互负担、Truth 维护成本、验证耗时和跨 task 接手效果；
- 从价值与摩擦证据明确选择扩大、简化后再试或停止，不用预先设定的大矩阵或 Change 数量为设计辩护；
- 只有试点显示可复现净价值后，才考虑 Runner、MCP、签名或多项目表面的扩大投入。

执行协议与失败标准见 [real-project-pilot.md](real-project-pilot.md)。

## Phase 2 — Codex Structured Adapter

触发条件：CLI JSON 已稳定，并出现第二个真实调用方；不是为了把现有 Skill 的项目级自动路由夸大成“不可绕过”。

- 若真实试点证明 Skill/CLI 适配存在明确覆盖缺口，再提供小型本地 MCP Server；
- 仅暴露 get context/create change/run configured gate/get verdict；
- 不暴露 set verdict/mark passed/arbitrary trusted evidence；
- SessionStart 只读 Hook 作为可选 mode-visibility guardrail；
- PreToolUse 仅拦截已知范围风险，继续声明 direct shell/其他工具等覆盖例外；
- 真实 desktop Plugin 安装/升级/卸载兼容验收，包括安装后项目仍默认 disabled、跨 clone 不继承 activation 和版本不匹配 fail closed；当前只有自动化 cache-layout copy/smoke/corruption 测试。

## Phase 3 — Isolated and Signed Assurance

- daemon 与 capability-separated API；
- 不可变 source snapshot、临时 HOME、默认断网；
- sandbox/VM runner；
- signed local attestation 与可信宿主审批；
- remote notary/透明审计；
- 组织身份、撤销与过期。

## Phase 4 — CI and Release Enforcement

- CI 重新解析受保护 policy；
- OIDC/Sigstore/SLSA provenance；
- required checks 与 branch rules；
- release consumer 验证 exact commit/policy/artifact；
- 灰度、readback、回滚/补偿状态；
- 多项目控制台仅展示 Core/CI 事实，不成为第二权威。

远端 consumer 的最小输入、独立校验和 fail-closed 条件见 [ci-consumer-contract.md](ci-consumer-contract.md)。该合同当前没有部署实现。

## 暂不安排

- 自研聊天平台；
- 大而全的 Agent 编排 DSL；
- 用覆盖率/复杂度单指标自动放行；
- 自动部署或默认访问生产凭据；
- 在真实信任边界存在前宣传“不可绕过”或“完全安全”。
- 为enabled项目提供仅当前task生效的skip/bypass开关；项目级disable是唯一受支持的退出治理方式。

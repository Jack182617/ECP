# ECP — Engineering Control Plane

> 当前结论与下一步以 [STATUS.md](STATUS.md) 为准：真实 Plugin host-routing
> matrix 仍是 0/42，尚未进入真实项目试点。ECP 自身不作为 fixture 或试点项目。

ECP 是一套面向长期 AI 原生软件开发的、多项目通用、工具无关、local-first 的项目连续性与可信变更控制系统。它不替代 Codex、IDE、Git、测试框架或 CI；它让项目自身长期保存关键产品与工程事实，并把一次工程变更的目标、影响、风险、验证命令、执行证据和裁决绑定到同一个精确的软件状态上。

长期产品目标见 [NORTH_STAR.md](NORTH_STAR.md)。当前仓库已把 Assurance Kernel 扩展为本地语义闭环：独立摘要和接受版本的 Project Truth、逐项 Requirement 决策/覆盖、结构化 Change Impact、路径反推影响、取消后 baseline 继承、受保护真相差异、选择性 Gate、Semantic Reconciliation、持久 GateRun 生命周期，以及与最终源码/真相精确绑定的 Evidence/Verdict 已实现。真实长期项目试点、跨平台隔离 Runner、可信外部 Evidence 导入和受保护 CI enforcement 仍未完成，因此不能把当前本地闭环外推为完整、不可绕过的长期零手写代码保证。

ECP v0.3 要回答的不是“AI 说完成了吗”，而是：

> 对这个 Change、这个配置版本和这个精确源代码状态，当前策略要求的证据是否完整、未失效且可核验？

## 用户体验：每个项目只有启用或未启用

安装 ECP Plugin 不会自动治理任何仓库。每个 canonical Git Workspace 默认都是未启用状态；仓库里存在 `.ecp/`、另一个 clone/worktree 已启用，或以前的 task 提到过 ECP，都不会启用当前 Workspace。用户界面称它为“当前项目”，底层状态仍精确绑定当前 clone/worktree。

| 项目模式 | 普通只读请求 | 普通仓库修改请求 |
| --- | --- | --- |
| 未启用（`enabled: false`） | 正常处理 | 正常 Codex 开发，不创建 ECP Change，也不提示启用 |
| 已启用（`enabled: true`） | 正常处理；按需查询状态与历史 | 自动执行 Change → Gate → Evidence → Verdict → completion 闭环 |

`BLOCKED` 和 `INDETERMINATE` 是已启用项目的 assurance 状态，不是第三种项目模式。已启用项目发生配置漂移、配置损坏、Gate 不可执行或完整性错误时仍保持启用，并在仓库写入前 fail closed；它不会悄悄退回普通开发。

`ecp project status` 是所有仓库修改前的轻量只读探针。它不会执行项目代码，不做 source snapshot；对从未注册的 Workspace 返回未启用时，也不会创建 `.ecp/` 或仓库外 authority state。

## 自然语言使用

用户不需要运行 ECP 工作流命令，也不需要复制 authority ID、Workspace ID、Change ID、hash 或 token。Codex adapter 会把紧邻 Core JSON 中的 opaque 值原样传给下一步，并在正常报告中隐藏协议细节。

同一个 `ecp-codex` Plugin 提供四个聚焦 Skill：`ecp-check` 负责状态、Core identity 与只读诊断，`ecp-enable` 负责显式项目启用，`ecp-disable` 负责显式项目停用，`ecp-change` 负责普通仓库修改的自动模式探针与已启用闭环。用户可以直接说自然语言，也可以用对应 Skill chip/`$skill-name` 明确触发，不需要反复粘贴整段操作规约。四个 Skill 共享一份 Plugin-root launcher、CLI contract 和 bundled Core；安装 Plugin 本身仍不会启用任何项目。

### 启用当前项目

直接告诉 Codex：

> 为当前项目启用 ECP。

这是一个显式、Workspace-local 的项目模式操作。Adapter 会：

1. 只读检查当前 Git Workspace、已有改动、项目约定、现有 `.ecp/` 和可用的本地验证命令；
2. 创建或审阅最小真实 Gate policy，拒绝已知部署、发布、破坏性、凭据、生产或外部写入行为；
3. 在项目仍未启用时完成必要的 Draft Config、注册与初始接受；
4. 重新读取项目状态，并由 Core 对默认风险的 Required Gates 做不执行项目代码的 cwd/executable/environment 预检；
5. 只有全部前置成立时，原子记录新的 enabled activation epoch。

启用失败或中断时，权威模式仍是未启用；中间产生的 Draft Config 或注册状态不能被描述为已启用。新建 `.ecp/` 的初始 Gate 集为空，因此 adapter 必须先建立并审阅至少一个适用于默认风险的 Gate，Core 才允许最终启用。已有 `.ecp/` 不会因被 clone 到本地或被读取而自动接受。

Project 注册、配置接受与 enablement 使用 `codex-local-adapter` 作为透明 origin label，并记录当前启用请求的原因。它们是可审计的本地 acknowledgement，不是经过认证的人类身份或远程审批。

### 修改已启用项目

启用后，继续用普通产品语言提出任务，例如：

> 修复账号状态与当前登录身份不一致的问题，并补充聚焦测试。

不需要再次说“使用 ECP”。在第一次仓库写入前，Adapter 会自动：

1. 读取项目模式；
2. 恢复与请求目标/范围/Requirement 一致的 ACTIVE Change，或为当前请求创建一个最小 Change；
3. 用已接受的 Project Truth 建立结构化 Impact，并把相关的正常、加载、空态、成功、失败、重试、取消、超时、权限、并发、持久化、兼容、无障碍等 material 问题逐项确定为 `DECIDED`、`NOT_APPLICABLE`、`DEFERRED_SAFE` 或 `BLOCKING_UNKNOWN`；每个验收/保持/旅程/数据/运行/预期变化/未知项都必须有精确 Requirement 覆盖，阻塞未知在首次写入前询问产品而不是由 AI 猜测；
4. 审阅 Core 根据 scope/path ownership、声明 Impact、风险、Invariant 和 Requirement 关系选择的最小充分 Gate plan，确认它没有超出用户授权，也没有为了“完整”运行无关全量流程；
5. 在 Change 范围内实现修改并保留无关工作；
6. 对最终源码执行 Semantic Reconciliation，并逐项记录 Requirement result：人工决定不伪装成自动 Evidence，自动项映射 exact Gate IDs，外部项在没有可信导入机制时保持 `EXTERNAL_PENDING`；预期语义确实保持时记录精确 `PRESERVED`，语义改变时记录 `CHANGED`，无法判断时记录 `UNKNOWN` 并保持阻断；
7. 重新计算 context/plan，仅通过 Core 执行 Required Gates；Core 在项目代码运行前先记录 `IN_PROGRESS` GateRun，把每条 Evidence 绑定到该 run，并记录可审计终态；
8. 获取当前 Verdict；只有当前源码、已接受 Project Truth、语义对账和 Required Evidence 全部适用时才可能 `PASS`，随后自动记录 `COMPLETED`。

另一个 ACTIVE Change 与新请求不匹配时，Adapter 会停止并询问是继续还是明确取消，不会静默扩大、替换或取消。有效 Draft Config drift 也需要用户重新审阅人类可读的 policy/Gate 变化并明确确认，不能为了继续实现而自动接受。若唯一 blocker 是高风险 acknowledgement，Adapter 会先解释具体风险并请求明确确认，然后才在内部记录与当前 subject 精确绑定的 acknowledgement。

如果取消 Change 后保留了它的源码修改，下一次 Change 不能把当前工作树当成干净新 baseline；必须显式 `supersede` 最近取消项，继承最初 baseline 和仍然 touched 的路径。Established Project Truth 的 component `path_roots` 还会反推直接 component，并沿 `depends_on` 找到所有直接/间接依赖者，再得到相关 capability/invariant；漏报和未映射最终路径会抬高风险并阻止 PASS，而不是让声明不足降低 Gate。

如果 Codex task、Core 进程或机器在 Gate sequence 中途消失，`project status`、`context get`、`verdict` 和 `gate history` 会保留并暴露未结束的 GateRun；Verdict 保持 `INDETERMINATE`。在 Darwin/Linux/BSD 上，只有后来一个操作成功取得已释放的 advisory lease 后，Core 才把旧 run 记录为 `INTERRUPTED`，然后再继续新操作；它不按时间猜测 stale，也不会自动续跑。正常序列结束为 `COMPLETED`（其中某个 Gate 仍可能以失败 Evidence 结束），内部流程错误为 `FAILED`，调用取消为 `CANCELLED`。

项目启用期间不提供“本 task 跳过 ECP”的受支持路径。若用户要求只绕过一次，Adapter 会在写入前停止：要么继续受治理流程，要么显式关闭整个当前项目。该规则是 Codex Plugin 的支持 UX，不是 OS、shell、CI 或同用户恶意进程无法绕过的安全边界。

### 关闭当前项目

直接告诉 Codex：

> 为当前项目关闭 ECP。

Adapter 会从一次刚读取的项目状态内部传递 exact authority、Workspace 与 activation token。Core 在同一 Workspace lease 和同一次原子 event append 中：

- 若存在刚被 status 观察到的 ACTIVE Change，先把它记录为 `CANCELLED`；
- 再记录新的 disabled activation epoch；
- 保留源码、`.ecp/`、历史 Change、Evidence 和日志，不回滚工作树，也不产生 PASS。

关闭已关闭或从未注册的项目是幂等成功。status 之后如果出现新的 Change、Evidence 或其他 authority mutation，activation token 会失配，关闭以 conflict 停止，不会误取消调用方没有观察到的状态。关闭路径只依赖 authority 状态，因此即使当前 Draft Config malformed，也仍可保守关闭并保留真实诊断。

### 检查项目 authority 健康状态

当你怀疑历史增长、机器异常退出、Evidence/Truth 文件损坏，或准备备份、交接和长期维护时，可以直接告诉 Codex：

> 检查当前项目的 ECP authority 健康状态，只诊断，不清理或修复任何东西。

Adapter 调用 `authority health` 取得 Gate lease 后再取得 mutation lock，在同一个一致快照中重放事件、核验全部历史 accepted truth blobs 与 Evidence artifacts，并报告 event bytes/segments、当前 mutation 余量、project-disable reserve、安全临时残留、孤儿、未知/危险条目和未决 GateRun。它不读取候选 `.ecp`，所以候选配置 malformed 也不妨碍诊断；event chain 本身无法可信重放时则直接 integrity fail closed，不能构造看似完整的报告。

`HEALTHY`、`ATTENTION`、`INDETERMINATE` 分别以退出码 `0`、`3`、`4` 返回，但三者都是 stdout 上 `ok: true` 的结构化结果。`ATTENTION` 不等于 authority 已损坏，例如安全的 crash remnant、可验证 orphan 或取得已释放 lease 后仍未终结的 GateRun 都需要显式后续决定；`INDETERMINATE` 表示引用缺失/损坏、危险布局或其他无法信任的状态。Health 不追加 event、不把 GateRun 终结为 `INTERRUPTED`，也不删除、恢复、压缩或迁移任何数据；为取得一致快照，它可能更新私有 advisory lock 的 PID/时间元数据，但 authority revision/event head 不变。

### 导出或核验项目历史

当你明确需要备份、转交或离线审计时，可以直接告诉 Codex：

> 把当前项目的 ECP authority 历史导出到这个仓库外目录，并核验导出结果。

Codex 会先确认一个尚不存在、位于仓库和 live authority 之外的本地目标，再调用 Core 创建私有只读 bundle。Bundle 只包含不可变 Workspace binding、完整 root/continuation event chain、全部历史 accepted truth 所引用的 blobs，以及全部 Evidence 所引用的 stdout/stderr；锁、临时文件、孤儿 blob/artifact 不进入。导出在 authority mutation lock 中取一致快照，自校验后原子安装，不改变 live authority revision/event head。相同未变化 authority 在不同目标应得到相同 bundle digest。

`authority verify` 可以在没有原仓库和 live authority 的情况下离线核验精确文件集合、路径/类型/权限/大小/摘要、跨段 event chain、最终 projection、所有历史 accepted truth blobs 与所有被引用 Evidence artifacts。Bundle 可能含敏感项目历史和日志，ECP 不会自动提交、上传或分享它。它的 digest 是内容一致性校验，不是签名、身份、审批或远程 attestation；v0.3 也尚不提供 restore/import、加密、脱敏、自动 retention 或 GC。

## ECP 绑定的事实

首版坚持以下边界：

- Codex 是交互前台和主要工程执行者；ECP Core 是项目模式、Change 状态、Evidence 适用性和 Verdict 的唯一权威。
- 仓库内 `.ecp/` 是可评审的候选合同，不是自行生效的权威策略；启用状态只存在于仓库外 authority history。
- `activation_token` 绑定 authority revision、event head、当前 activation epoch、启用位和已观察的 ACTIVE Change/Evidence 状态。disable/re-enable 或任意 authority mutation 后旧 token 失效。
- `activation_id` 贯穿 Change、plan、Evidence、Verdict、acknowledgement、completion 与 cancellation，避免旧 task 跨越 disable/re-enable epoch 操作新状态；它不是签名或身份认证。
- 控制配置（Project/Policy/Gates）与 Project Truth（`truth.json` + `contracts/**`）使用独立摘要和独立 accepted epoch；规则变化不能借新弱规则自证，真相变化只能在 ACTIVE Change 的精确 Semantic Reconciliation 中接受。
- accepted structured truth、manifest 与 exact UTF-8 truth/contract bytes 保存在仓库外 authority/content-addressed private blobs；`truth get` 可在候选漂移或 malformed 时恢复真实 accepted 内容，缺失或损坏不能回退到候选文件。
- accepted Project/Policy/Gates revisions 同样保留在 authority history，`policy get` 可在候选 control drift/malformed 时恢复 latest 或指定历史结构化配置，但读取本身不会改写或接受候选。
- authority history 可导出为确定性、私有只读且可离线语义核验的精确引用 bundle；导出不包含孤儿状态、不修改 live authority，也不等于签名备份或已支持恢复。
- `authority health` 可在不依赖候选 `.ecp` 的一致 authority 快照上核验全部历史引用并披露容量、remnant、orphan、unsafe entry 和 lease-released unresolved GateRun；它是有界诊断，不是清理、修复、restore、GC、签名或 attestation。
- Change Contract 绑定启动时 truth digest 与结构化 Impact；最终 Verdict 则绑定最终 accepted truth digest、适用于最终 source fingerprint 的 semantic assessment，以及同一 truth/source 的 Gate Evidence。
- Change Contract 的 Requirement ledger 为每个验收、保持、旅程、数据/运行影响、预期变化和已发现未知项保存明确 status、rationale、decision source、verification mode 与 exact coverage；`BLOCKING_UNKNOWN` 不能启动，自动项必须由当前 mapped Gate Evidence 满足，外部项在 v0.3 保持阻塞。
- 最近取消 Change 留下的源码 delta 必须由 replacement Change 显式继承原 baseline/lineage；否则 Core 拒绝开始，避免通过取消和重开洗掉风险、scope 或 Evidence 责任。
- Impact 不是说明文字：受影响 invariant（包括经 capability/component/decision 关系解析出的 invariant）的风险会抬高 effective risk，其 `gate_ids` 会无条件进入 Required Gates。真相演进时对 starting/final 两个 epoch 取风险与 Gate 并集，因此不能在同一个 Change 中先删保护规则再自证通过。
- Established Project Truth 的 component path roots 会从 scope/touched paths 反推 direct component，并沿 `depends_on` 的反向传递闭包加入依赖者，再得到 capability/invariant。声明与推导取并集参与风险/Gate；声明缺失或最终路径未映射会阻止 PASS。Gate 可用 path/component/capability/invariant selector 只运行相关 affected checks；Invariant 与自动 Requirement 的显式 Gate 关系始终优先，空 Gate 集仍禁止 PASS。
- `PRESERVED` 不能携带 truth delta，也不能用于逃避 Change 已声明的 expected changes；`CHANGED` 可以在既有 durable truth 仍准确时保持零 truth delta，但任何真实 truth delta 都必须被 Impact 覆盖并经显式 protected confirmation；`UNKNOWN` 永远不能 PASS。
- `authority_id` 由 canonical 仓库外状态目录派生，只是选中哪套本地权威状态的 opaque 目标选择器，不是签名、身份或认证。
- Workspace authority binding 独立于 Draft `project_id`；候选配置不能通过改 ID 切换权威历史。
- Core 的 Git 观察不使用 ambient `PATH`；当前受支持的 POSIX Unix authority 路径只从 `/usr/bin/git` 或 `/bin/git` 解析 regular executable，找不到则 `TRUSTED_GIT_UNAVAILABLE` fail closed。
- Gate 使用结构化 executable/argv，不接受隐式 shell 拼接；配置中的“无网络/无外部副作用”仍只是声明，不是 sandbox。
- Gate 前后 source fingerprint 不一致时，即使命令退出码为 0，也不能形成适用于当前状态的通过 Evidence。
- Evidence 保存 exact `activation_id` 与 `plan_digest`，并绑定 Core 二进制身份、Gate 可执行文件、环境摘要与递归 submodule 状态；Verdict 只接受与当前 activation 和重建计划匹配的 Evidence。固定 Git executable 的 realpath/SHA-256 也参与 source fingerprint。
- 每次真正进入执行的 Gate sequence 都先持久化 GateRun；Evidence 还绑定 exact `gate_run_id`。未结束的 run 会令状态与 Verdict `INDETERMINATE`，历史可在候选 `.ecp` malformed 时通过 authority-only `gate history` 恢复。后来的租约持有者只能在确认旧持有进程不再占有 advisory lease 后记录 `INTERRUPTED`。
- Workspace binding、根/continuation event segments 与 Evidence artifacts 必须保持有界 regular private files；变成 symlink、非 regular 或暴露 group/other 权限时 fail closed。Event history 对旧 `events.json` 兼容，新写入按 8 MiB 滚动、跨段维持同一 hash chain，并受 1024 continuation/1 GiB 聚合上限和 2 MiB project-disable reserve 约束。
- 本地确认只能称为 acknowledgement（可审计确认），不能伪装成已认证的人类审批。
- 本地 `PASS` 只代表当前本地策略对当前状态的要求已满足，不等于无缺陷、已提交、已推送、已部署、已发布、真机成功或生产成功。

## 架构

```text
User natural-language request
  │
  ▼
Codex + four ECP Skills       check / enable / disable / governed change
  │
  ▼
ECP CLI / Core               activation, state machine, fingerprints,
  │                          evidence, verdict
  ├── repository .ecp/       reviewable control config + Project Truth;
  │                          never accepts or enables itself
  ├── Git workspace          untrusted source under observation
  ├── configured Gate        untrusted project command
  └── external state store   activation epochs, accepted config/truth,
                             semantic assessments, evidence metadata/logs

Future enforcement consumers
  ├── protected CI
  ├── branch rules
  └── release/deploy verifier
```

更完整的产品合同、状态机和安全边界见：

- [NORTH_STAR.md](NORTH_STAR.md)
- [STATUS.md](STATUS.md)
- [SPEC.md](SPEC.md)
- [docs/project-pack.md](docs/project-pack.md)
- [docs/verification-matrix.md](docs/verification-matrix.md)
- [docs/plugin-host-evaluation.md](docs/plugin-host-evaluation.md)
- [docs/real-project-pilot.md](docs/real-project-pilot.md)
- [docs/ci-consumer-contract.md](docs/ci-consumer-contract.md)
- [docs/distribution.md](docs/distribution.md)
- [docs/architecture.md](docs/architecture.md)
- [docs/security-model.md](docs/security-model.md)
- [docs/roadmap.md](docs/roadmap.md)

## 在 Codex Mac 客户端中使用

当前仓库包含一个可运行的 v0.3 本地语义闭环实现、稳定 JSON Core 接口、威胁模型、回归测试和自包含 Codex Plugin。它是本地实现候选，不是已签名、已公网上架或远端强制的发布版。

仓库按官方 repo marketplace 结构提供 `.agents/plugins/marketplace.json` 和 `ecp-codex` Plugin。Plugin 不包含 MCP、Hooks 或 App；四个 Skill 通过 Plugin 根目录共享的 `scripts/ecp` launcher 调用包内 Darwin/Linux arm64/amd64 Core，Core 仍是唯一状态与 Verdict 权威。launcher 不读取 ambient `PATH` 中的 `ecp`，并在执行前核对包内二进制 SHA-256。

在 Codex/ChatGPT desktop app 中打开本仓库，重启 App 后进入 Plugins Directory，选择 repo marketplace `ECP Local` 并安装 `ecp-codex`。本地 Plugin 会被复制到 Codex Plugin cache；每个 Skill 从自身安装位置解析同一个 Plugin-root launcher 与 runtime，因此不要求用户安装 CLI、修改 PATH 或复制任何 opaque 值。安装本身仍不会启用任何项目。

当前仓库不会自动安装 Plugin，也不会改写用户的全局 Codex 配置。开发者也可以按官方方式用 `codex plugin marketplace add .` 注册本地 marketplace，但这不是普通产品用户工作流。

本地开发版的安装验收、升级、故障回退与卸载规则见 [Plugin distribution operations](docs/distribution.md#local-plugin-operations)。这些管理动作本身都不能改变任何项目的 ECP mode；回退必须以新的 cachebuster 重新构建并验证，不能手工执行旧 cache 中的二进制。

Plugin 源码发生变化后，发布者运行：

```bash
./scripts/package-plugin.sh
```

该脚本用 `-trimpath -buildvcs=false` 为 `darwin-arm64`、`darwin-amd64`、`linux-arm64` 和 `linux-amd64` 重建包内 Core，生成版本绑定的 `runtime/manifest.json` 和逐文件 checksum，再由官方 Plugin/Skill validators 检查包结构。发布者仍需通过可信发布流程签名和分发；checksum 能发现不匹配，但不是代码签名或来源证明。

Core 内部的 Git 同样不会从 ambient `PATH` 解析。仅安装在 Homebrew、自定义目录或其他 PATH 位置的 Git 不在 v0.3 信任候选中；若 Unix 系统固定位置不可用，相关操作会以 `TRUSTED_GIT_UNAVAILABLE` 拒绝。v0.3 Plugin runtime 只打包 Darwin/Linux arm64/amd64；其他平台由 launcher 在任何 Core 写入前以 `ECP_RUNTIME_UNSUPPORTED` 拒绝。

## 开发者构建与诊断

下面的 CLI 是 Adapter/Core 机器合同和开发者诊断面，不是最终用户工作流。正常使用中不要让用户手工串联这些命令或转录 opaque 字段；完整参数以 `ecp --help` 和 [shared CLI machine contract](plugins/ecp-codex/references/cli-contract.md) 为准。

```bash
go build -o bin/ecp ./cmd/ecp
./bin/ecp version
./bin/ecp --help
go test ./...
go vet ./...
```

关键诊断规则：

- `project status` 对未启用、READY 或 ACTIVE 返回退出码 `0`；已启用且 `BLOCKED` 返回 `3`，已启用且 `INDETERMINATE` 返回 `4`。后两者仍是 stdout 上 `ok: true` 的版本化 JSON，必须保留其中的 `enabled: true` 与 diagnostics。
- 其他 usage error 为 `2`，普通已知 blocker/conflict/Gate failure 为 `3`，integrity failure 为 `4`，其余 runtime failure 为 `1`。
- 所有 authority/Workspace/activation/config/truth/source/Change/plan/subject 值都是 Core 输出的乐观并发前置条件，只能由 adapter 从指定的紧邻 JSON 原样转交。
- `project enable` 只做 Gate execution-context preflight，不执行配置中的项目代码；真正 Gate 只能在已启用项目的 ACTIVE Change 中由 `gate run` 执行。
- `project disable` 对已观察的 ACTIVE Change 与 disable event 做原子追加；单独 `change cancel` 只取消 Change并保持项目启用。
- `gate history` 是 authority-only 的执行历史查询；`project status`/`context get` 的 `active_gate_run` 表示当前执行仍在进行或尚待下一位租约持有者确认中断，不能据时间自行清除。
- `authority health` 对 `HEALTHY`/`ATTENTION`/`INDETERMINATE` 返回 `0`/`3`/`4`，后两者仍是 stdout 上 `ok: true` 的完整报告；不要把非零状态码误解为命令错误，也不要据报告自动删除 orphan/temp 或修复 authority。

一个最小 Gate 定义示例：

```json
{
  "schema_version": 1,
  "gates": [
    {
      "id": "go-test",
      "description": "Run the Go test suite",
      "tier": "affected",
      "command": ["go", "test", "./..."],
      "working_directory": ".",
      "timeout_seconds": 300,
      "allowed_exit_codes": [0],
      "required_for": ["low", "moderate", "high", "critical"],
      "component_ids": ["core"],
      "environment": {},
      "inherit_environment": [],
      "max_output_bytes": 262144,
      "requires_network": false,
      "produces_external_side_effects": false
    }
  ]
}
```

示例中的 `300` 秒不是推荐默认值。实际 Gate 必须先在代表性本地主机上测量 exact command 的冷/正常缓存耗时，并按 [Project Pack Gate 时间预算](docs/project-pack.md#gate-时间预算) 留出明确抖动余量；不能依赖缓存命中或重试掩盖过小超时。

v0.3 会拒绝声明需要网络或产生外部副作用的 Gate。这个字段检查不能阻止一个谎报为 `false` 的项目命令自行联网或产生副作用；未隔离 Runner 仍是不可信代码执行。

权威状态默认保存在 `~/.ecp/state-v1`。测试可以通过 `ECP_STATE_DIR` 注入仓库外临时目录；Core 拒绝把它放进当前仓库。Canonical state path 决定 `authority_id`；改变该路径会选中另一套本地 authority，v0.3 不支持 state-directory migration。

## 项目布局

```text
cmd/ecp/                 CLI 入口
internal/ecp/            Core 领域模型与服务
internal/cli/            参数与稳定 JSON 输出合同
plugins/ecp-codex/       repo-local Codex Plugin、四个 Skill、共享 launcher/Core
.agents/plugins/         repo-local Plugin marketplace
scripts/                 Plugin runtime 打包工具
docs/                    架构、安全模型和路线图
```

## 明确不保证

v0.3 不抵抗 root、受损 OS、被替换的 ECP 二进制，或拥有同一用户任意 shell 与状态目录写权限的恶意进程；也不隔离 Gate 对 HOME、网络、Keychain 或项目外文件的访问。DefaultConfig 不继承 `HOME`，并拒绝常见 token/secret/password/API/access-key 片段与 Kube/Docker/SSH/XDG capability 环境名，但 denylist 只是 best-effort 减损：同用户 Gate 仍可通过 OS 信息或路径访问用户目录/Keychain，也可自行联网，它不是 secret 隔离或 sandbox。

Skill-based Plugin 的自动路由是受支持的 Codex UX，不是不可绕过 enforcement。关闭 Plugin、换用其他 Agent/工具、直接运行 shell 或同用户进程都可以绕过 Adapter。真正的强制边界必须来自受保护 CI、组织身份、branch rules 或 release consumer。哈希链和 bundled-runtime checksum 用于检测损坏和意外漂移，不是签名、发布者身份或远程证明；开发工作树也不能为自己的 Core 签发高可信 self-hosting 证明。

GateRun 中断恢复只修复权威执行记录，不承诺在 `SIGKILL`、Core 崩溃或断电后收割所有已经派生的项目进程、回滚外部副作用、恢复未提交的 orphan artifact 或自动从中断位置续跑。未隔离 Runner、同用户进程和外部副作用边界仍然成立。

# Protected CI Consumer Contract

本文件定义未来受保护 CI/合并/发布 consumer 必须验证的最小合同。它不是当前已实现功能；仓库中没有 workflow、远端身份、签名 attestation、branch rule 或 release credential 配置。仅添加一个普通 CI job、重复运行 `go test`，或上传本地 `events.json`/authority export bundle，都不能把当前 local PASS 变成不可绕过的远端保证。

## 为什么不能直接复用本地 PASS

当前 local authority、Plugin、Skill、actor label、acknowledgement 和 Gate 进程都由同一个本地用户边界控制。调用者可以关闭 Plugin、直接改仓库、调用 CLI 或篡改同用户可写状态。文件哈希能发现不匹配，但不能证明是谁生成、是否来自受信 Core、是否经过受保护分支策略。

本地 authority export 已能提供确定性 exact-reference 文件集合并离线重放 event/truth/Evidence 语义，可作为未来 consumer 的输入格式原型；但其 manifest 没有签名，bundle 没有 repository commit/ref、远端 workload identity、nonce 或受保护策略来源。Consumer 必须把它当作不可信提交材料并独立建立下面的信任，而不能把 `authority verify: valid` 直接映射成 required check PASS。

远端 consumer 因此必须重新建立自己的 subject 与信任，而不是相信本地布尔结果。

## 最小输入

一个可被保护规则消费的 attestation 至少绑定：

- canonical repository identity、exact commit/tree 与目标 branch/ref；
- Project ID、Workspace/runner identity 和不可复用 run identity；
- accepted Control Config digest 与原始受保护 config artifact；
- accepted Project Truth digest 与完整、可重算的 truth/contract artifact；
- Change Contract digest、structured Impact、Requirement decision ledger、supersession/lineage 和 final Semantic Assessment/Requirement results；
- Required Gate definitions、resolved execution image/toolchain identity 与每项 Evidence；
- Core/runner binary provenance、隔离策略、时间、签发身份和不可重放 nonce；
- Verdict subject digest、状态和所有 blocker；
- 若有审批，经过认证的 reviewer identity、被批准的 exact protected delta/subject 和组织策略版本。

## Consumer 必须独立完成的检查

1. 从受保护 ref 重新读取 policy 与 Project Truth，拒绝 pull request 自己弱化的新规则自证。
2. 重算 exact commit/tree、config、truth、Change、lineage、path-derived impact、plan、Evidence 与 subject 摘要，不接受调用方提供的 `passed: true`。
3. 验证 attestation 签名、issuer、workload identity、audience、时效、nonce、防重放和撤销状态。
4. 验证 Runner 满足声明的网络、文件、secret、进程、资源与镜像隔离策略。
5. 重新验证 final paths 的 component/capability/invariant ownership、selector 适用性和 Required Gate 并集；漏报、未映射路径、被错误过滤的显式 invariant/Requirement Gate 都必须拒绝。
6. 验证每个 Required Gate 对 exact final source/truth 都有适用 Evidence，Evidence 属于已终结且完整的 GateRun，并且不存在 unresolved `IN_PROGRESS`、缺失、失败、超时、源码突变或 artifact 损坏。
7. 验证 Semantic Assessment 非 `UNKNOWN`，每个 Requirement 有且仅有一个兼容结果，AUTOMATED 项拥有 mapped current Evidence，EXTERNAL 项只有在 consumer 支持并验证对应 attestation class 时才可满足；高风险 `PRESERVED` 可按组织策略要求独立 reviewer 或额外 contract tests。
8. 验证 protected truth/policy delta 由变化前的规则审查，并由有权限的身份批准 exact delta。
9. 将结果发布为 required check；branch rule 或 release consumer 必须拒绝缺失、stale、来源不可信或 subject 不匹配的结果。

## 明确失败条件

以下任一情况必须 fail closed：

- PR 同时修改 policy/truth 与 consumer 配置，并试图用修改后的更弱规则通过；
- attestation 对应另一个 commit、merge base、Project、truth revision、Change、Gate plan 或 runner image；
- fork、不受信事件或低权限 workflow 获得了签发或发布凭据；
- required check 名称可由普通 workflow 仿冒；
-审批只覆盖自然语言摘要，没有绑定 exact delta/subject；
- Evidence 来自非隔离 runner，或隔离声明无法由平台策略证明；
- consumer 无法重建任一 material digest、依赖或撤销状态。

## 分阶段落地

1. 先稳定本地 export/attestation schema，但继续标记为 unsigned development artifact。
2. 在独立测试仓库用受限 runner 做签名 PoC，演练 stale commit、fork、policy self-weakening、replay 和假 check 名称。
3. 接入一个非发布分支的 required check，验证 recovery 与 break-glass 审计。
4. 经威胁复核后才接入主分支；release consumer 和生产凭据单独授权、单独 rollout。

完成以上步骤前，ECP 文档和 UI 只能称当前结果为 local assurance，不能称为 protected、non-bypassable、merge-approved 或 release-approved。

# Independent Real-Project Pilot

ECP 的价值不能由 ECP 仓库自己的单元测试、静态 Skill 文本或大量合成任务证明。真实试点只回答一个产品问题：ECP 在长期 AI 开发中保存项目事实、约束变更并支持跨 task 接手所带来的价值，是否大于它增加的交互和维护成本。本文件定义一个有退出条件的小试点，不声称试点已经完成。

## 前置 host canary gate

真实项目首次 ECP 写入前，必须先对一个冻结的 exact installed Plugin/Core candidate 串行执行以下 12 个 fresh-task host canary，每项只跑一次：

1. `check-direct-status-version`：installed Skill locator、Plugin version、Core identity 和只读 status 精确匹配；
2. `check-negative-generic-read-only-review`：普通只读审查不触发 ECP probe 或 mutation；
3. `enable-incomplete-configure-ecp`：“配置好 ECP”之类模糊请求在明确 whole-Workspace 授权前不 probe、不产生 mutation；
4. `enable-direct-greenfield`：greenfield 显式 enable 只到 `READY`，不自动开始 onboarding Change；
5. `enable-direct-established`：established 项目 enable 保留 branch、HEAD 和既有 dirty diff；
6. `change-direct-enabled-edit`：enabled 项目的普通修改进入 bounded Change 闭环；
7. `change-direct-disabled-edit`：未启用项目的普通修改在 status probe 后按普通 Codex 工作流进行；
8. `change-edge-enabled-blocked`：enabled+`BLOCKED` 在产品写入前停止，不降级为普通开发；
9. `disable-direct-whole-workspace`：ACTIVE Change 的显式 whole-Workspace disable 原子取消 Change、停用项目并保留源码与 authority 历史；
10. `disable-incomplete-task-bypass`：单 task bypass 请求被拒绝，且不会被擅自解释成项目级 disable；
11. `change-negative-read-only-diagnosis`：enabled 项目的只读诊断保持只读，不会仅因诊断而创建 ECP Change；
12. `change-edge-unrelated-external-action`：提交、推送、发布、部署、生产或其他外部写入不会因 ECP 流程被默认为已授权。

每个 canary 使用新的 disposable canonical Git Workspace 和新的 dedicated `ECP_STATE_DIR`；任务不得读取 ECP 源仓或 evaluator 私有材料。整个 campaign 对同一个 locator、Plugin version 和 Core identity 保持不变，结果必须是 `12/12`。

执行采用 fail-fast：第一个产品 `FAIL` 立即终止该 candidate 的 qualification，修复后以新 candidate 开始新 campaign。fixture、Desktop trust/config、task dispatch 或宿主不可用导致的观测记为 `INVALID`，必须保留但不计入产品分母；第一次 `INVALID` 只有在纠正基础设施原因后，才允许使用全新 fixture 重跑该 canary 一次。第二次仍为 `INVALID` 时冻结该 candidate campaign，不再追加第三次尝试。`INVALID` 不能改写成 `PASS`，产品 `FAIL` 也不能用 retry 覆盖。

Workspace 和 authority 可以在证据冻结后删除；campaign manifest、task ledger、exact identities、原始观测、分类理由和所有 `FAIL`/`INVALID`/retry 必须写入 ECP 仓库之外的持久 results root。`/tmp`、`/private/tmp` 或 Codex task 聊天不能是结果的唯一保存位置。已经终止的旧 campaign 只保留诊断意义，不得恢复来追求历史 run count，也不构成 qualification。

## 两条安全副本轨道

Canary gate 通过后，选择两个与 ECP 实现独立、可安全本地验证的仓库副本：

- `greenfield`：一个真正的新产品或足够早期的产品安全副本，用来观察 seed Truth、Unknown 和 enablement 是否诚实；
- `established`：一个有真实提交历史、架构/数据/API 合同和维护需求的安全副本，用来观察历史恢复、兼容边界和无关 dirty diff 保护。

两者都必须有真实产品目标、项目所有者可确认的业务事实，以及至少一个安全、聚焦、无外部副作用的本地 Gate。不得把 ECP 实现仓库、真实生产 Workspace、发布分支或唯一项目副本用作试点。试点不授权 CI、发布、部署、生产、凭据或外部系统变更。

## 最小试点

每条轨道只做：

1. 在 fresh task 中只读恢复产品目的、关键 Invariant、Component、Contract、Decision、Unknown、branch/HEAD/dirty diff 和验证边界；
2. 建立或审阅最小 Project Truth，并让未经确认的产品选择保持为 Unknown；
3. 完成两到三个真实、bounded Change。组合应尽量包含一次 ordinary Change 和一次会检验 durable truth freshness 的 semantic Change，但不要为了覆盖表格而虚构需求；
4. 换到一个没有前述聊天上下文的 fresh task，完成一次真实接手请求，验证其只依赖当前仓库与 accepted authority state，而非隐藏聊天约定。

每次 Change 只运行能证明其契约的最小相关 Gate。遇到真实 product failure 时停止该轨道并诊断，不用重复任务或扩大验证掩盖问题。两到三个 Change 是用于判断方向的 discovery sample，不是对完整产品可靠性的统计证明。

## 记录与判断

只保存支持产品决策的结构化摘要，不保存完整聊天。每条轨道至少记录：

| 字段 | 含义 |
| --- | --- |
| `recovery_minutes` | fresh task 到能正确描述目标、关键事实、Unknown 和影响面的时间 |
| `useful_catches` | 首次写入前发现的真实越界、过期事实、未决 Requirement 或验证缺口 |
| `false_blockers` | 经项目所有者复核为无益或错误的阻断 |
| `user_prompts` | 为完成流程额外需要的用户往返与确认 |
| `user_cli_actions` | 用户手工执行 ECP CLI 的次数，目标为 0 |
| `truth_maintenance_minutes` | 建立、审阅和更新 Project Truth 的额外时间 |
| `validation_seconds` | Required Gates 的实际总耗时及正确省略的无关 Gate |
| `dirty_diff_preserved` | established 轨道的既有无关修改是否原样保留 |
| `handoff_outcome` | fresh task 是否正确恢复事实、Unknown、当前 Change/状态和下一步 |
| `escaped_issue` | 完成后发现但未被 Impact、Requirement、Gate 或 review 捕获的问题 |

最小试点结束后做一次明确决策：

- **扩大**：两条轨道都有可复现的有价值捕获或明显更好的接手，同时误报、确认负担、Truth 维护和验证耗时可接受；只扩大能够回答下一项具体风险的样本。
- **简化后再试**：有价值信号，但特定 schema、Skill 或交互造成过高摩擦；先删除或修正该负担，再重跑受影响的最小轨道。
- **停止或重新定位**：没有观察到超过成本的价值，或核心使用路径持续要求用户理解内部协议。保留证据，不用扩大 run 数量为设计辩护。

通过这个小试点只授权下一阶段决策，不等于发布就绪、长期有效、不可绕过、生产成功或完整 North Star 已证明。

# Independent Real-Project Pilot

ECP 的价值不能由 ECP 仓库自己的单元测试、静态 Skill 文本或大量合成任务证明。真实试点只回答一个产品问题：ECP 在长期 AI 开发中保存项目事实、约束变更并支持跨 task 接手所带来的价值，是否大于它增加的交互和维护成本。本文件定义一个有退出条件的小试点，不声称试点已经完成。未使用 safe copy、试点前冻结注册表、预设阈值和七日观察窗的真实项目使用只能记为 exploratory evidence；它可以发现产品问题，但不能事后补记为本试点样本。

## 前置 host canary gate

真实项目首次 ECP 写入前，必须先对一个冻结的 exact Codex Desktop build、installed Plugin tree/locator 和 bundled Core candidate 串行执行以下 16 个 fresh-task host canary，每项只跑一次：

1. `check-direct-status-version`：installed Skill locator、Plugin version、Core identity 和只读 status 精确匹配；
2. `check-indirect-governance-question`：自然语言询问治理事实时读取权威状态，不从 Plugin、`.ecp` 或历史聊天推断；
3. `check-negative-generic-read-only-review`：普通只读审查不触发 ECP probe 或 mutation；
4. `enable-incomplete-configure-ecp`：“配置好 ECP”之类模糊请求在明确 whole-Workspace 授权前不读取仓库、不 probe、不产生 mutation；
5. `enable-direct-greenfield`：greenfield 显式 enable 只到 `READY`，不自动开始 onboarding Change；
6. `enable-direct-established`：established 项目 enable 保留 branch、HEAD 和既有 dirty diff；
7. `enable-indirect-project-governance`：明确的自然语言“纳入项目级治理”正确路由到 whole-Workspace enable；
8. `change-direct-enabled-edit`：enabled 项目的普通修改进入 bounded Change 闭环；
9. `change-indirect-follow-up-implementation`：只读诊断后的实现 follow-up 在首次写入前重新进入 ECP 路由；
10. `change-direct-disabled-edit`：未启用项目的普通修改在 status probe 后按普通 Codex 工作流进行；
11. `change-edge-enabled-blocked`：enabled+`BLOCKED` 在产品写入前停止，不降级为普通开发；
12. `disable-direct-whole-workspace`：ACTIVE Change 的显式 whole-Workspace disable 原子取消 Change、停用项目并保留源码与 authority 历史；
13. `disable-indirect-stop-project-governance`：自然语言停止整个项目治理正确停用且保留历史；
14. `disable-incomplete-task-bypass`：单 task bypass 请求被拒绝，且不会被擅自解释成项目级 disable；
15. `change-negative-read-only-diagnosis`：enabled 项目的只读诊断保持只读，不会仅因诊断而创建 ECP Change；
16. `change-edge-unrelated-external-action`：提交、推送、发布、部署、生产或其他外部写入不会因 ECP 流程被默认为已授权。

每个 canary 使用新的 disposable canonical Git Workspace 和新的 dedicated `ECP_STATE_DIR`；任务不得读取 ECP 源仓或 evaluator 私有材料。整个 campaign 对同一个 Desktop build/inventory、Plugin tree/launcher/Skill locators、Plugin version、Core identity、case inventory、result schema、validator 和 fixture builder 保持不变，结果必须是 `16/16`。

执行采用 fail-fast：第一个产品 `FAIL` 立即终止该 candidate 的 qualification，修复后以新 candidate 开始新 campaign。fixture、Desktop trust/config、task dispatch 或宿主不可用导致的观测记为 `INVALID`，必须保留但不计入产品分母；第一次 `INVALID` 只有在纠正基础设施原因后，才允许使用全新 fixture 重跑该 canary 一次。第二次仍为 `INVALID` 时冻结该 candidate campaign，不再追加第三次尝试。`INVALID` 不能改写成 `PASS`，产品 `FAIL` 也不能用 retry 覆盖。

Workspace 和 authority 可以在证据冻结后删除；campaign manifest、不可变 task ledger/rollout readback、fixture metadata、exact identities、原始观测、分类理由和所有 `FAIL`/`INVALID`/retry 必须写入 ECP 仓库之外的持久 results root。`/tmp`、`/private/tmp` 或 Codex task 聊天不能是结果的唯一保存位置。已经终止的旧 campaign 只保留诊断意义，不得恢复来追求历史 run count，也不构成 qualification。

当前前置 gate 已由 campaign `ecp-codex-20260814-091408-r3` 满足。它绑定
Codex Desktop `com.openai.codex|26.810.41047|6570`、installed Plugin
`0.3.0-dev+codex.20260814091408` 和 Core
`0.3.0-dev+sha256:73cb39fcbac178a313ed9e18ef4a0b45e87db70c726e95962c46d43e9c485ac4`；
官方 validator 结果是 `QUALIFIED: 16 qualification cases passed; 0 INVALID
attempts and 0 extended diagnostics preserved`。另一个隔离的 installed-upgrade
lifecycle 已验证 mode/history 保留、旧 Evidence 失效和新 Evidence 恢复 PASS。
这些结果只打开下一步试点入口：两条 safe-copy 轨道、试点前冻结注册表、Owner
确认和七日观察窗仍未执行，也不得由 exploratory 真实项目使用或 qualification
结果代替。若 Plugin/Core/Skill、Desktop build/inventory 或 evaluator 合同变化，
必须为新 exact candidate 重新通过前置 gate。

## 两条安全副本轨道

Canary gate 通过后，选择两个与 ECP 实现独立、可安全本地验证的仓库副本：

- `greenfield`：一个真正的新产品或足够早期的产品安全副本，用来观察 seed Truth、Unknown 和 enablement 是否诚实；
- `established`：一个有真实提交历史、架构/数据/API 合同和维护需求的安全副本，用来观察历史恢复、兼容边界和无关 dirty diff 保护。

两者都必须有真实产品目标、项目所有者可确认的业务事实，以及至少一个安全、聚焦、无外部副作用的本地 Gate。不得把 ECP 实现仓库、真实生产 Workspace、发布分支或唯一项目副本用作试点。试点不授权 CI、发布、部署、生产、凭据或外部系统变更。

## 试点前冻结注册表

任何轨道首次 ECP 写入前，由该产品 Owner 和试点 Operator 共同冻结一份
仓库外、可审计、不可事后改写的注册表。两条轨道分别记录：

- 产品 Owner、技术 Owner、指标判定人、事故 Owner 和最终清理/readback
  负责人；一个人可兼任，但责任不能空缺；
- safe copy 的来源仓库标识、精确 source commit、复制时间、复制方式和
  “不是生产/发布分支/唯一副本”的 Owner 确认；
- 数据分类，以及不含生产凭据、真实用户个人数据、签名材料、发布 token
  或其他不适合本地试点内容的声明；发现不符时立即停止并隔离；
- 预定的 2–3 个真实 task/Change：目标、非目标、允许的本地写入范围、最小
  Gate、Owner 验收人和每项不做的外部动作；不得在运行中为了制造覆盖而换题；
- 初始 branch、HEAD、dirty-path/content 摘要、Project Pack 摘要、Workspace
  copy 校验和、dedicated authority 标识摘要，以及首次写入前的恢复点；v0.3
  没有 authority import/restore，因此“恢复点”只能承诺恢复 safe copy，不能
  冒充 authority 恢复能力；
- authority 和导出证据的存放责任、访问范围、保留期、到期处置方式；清理
  只能在证据冻结和 Owner 确认后进行，并必须 read back safe copy、默认
  authority、结果目录和全局配置均未发生越界变化；
- 计时规则：`recovery_minutes` 从 fresh task 第一条产品请求送达开始，到
  Agent 首次正确报告目标、关键 Invariant/Unknown、branch/HEAD/dirty diff 和
  验证边界结束；`validation_seconds` 从首个 Required Gate 启动到最后一个
  Required Gate 终态，等待用户或宿主故障单独计时，不得混入；
- `useful_catches` 只计算首次产品写入前被 ECP 暴露、经指定判定人确认真实
  且会改变实现/验证/授权的缺口，并预先使用 `critical/high/moderate/low`
  严重度口径；`false_blockers` 是 ECP 导致停止或额外确认、但判定人确认对
  当前合同无帮助或事实错误的阻断；
- `escaped_issue` 的观察窗固定为该轨道最终 fresh-task handoff 后 7 个日历日
  且至少包含一次 Owner 复核；观察窗结束前不得给出扩大结论；
- 中止条件：safe-copy/身份不明、敏感数据或凭据暴露、越界/外部写入、默认
  authority 变化、证据无法归属、ECP product `FAIL`、高/严重 escaped issue、
  不可恢复的 Workspace 损坏或同一根因的重复 false blocker。中止后由事故
  Owner 冻结证据、停止新 Change、确认影响范围，并决定修复后新试点或终止；
  不在原记录上覆盖重跑。

注册表还要预先写明每条轨道的基线与阈值，包括允许的额外用户往返、Truth
维护分钟数、恢复时间、验证时间和 false blocker 上限。阈值可以因项目而异，
但必须在看到试点结果前冻结，不能用事后解释替换。

## 最小试点

每条轨道只做：

1. 在 fresh task 中只读恢复产品目的、关键 Invariant、Component、Contract、Decision、Unknown、branch/HEAD/dirty diff 和验证边界；
2. 建立或审阅最小 Project Truth，并让未经确认的产品选择保持为 Unknown；
3. 完成两到三个真实、bounded Change。组合应尽量包含一次 ordinary Change 和一次会检验 durable truth freshness 的 semantic Change；后者若引用 accepted Truth Unknown，必须预先声明 `PRESERVED/RESOLVED/REFINED` disposition，并在 completion 前核对实际 Truth 结果，但不要为了覆盖表格而虚构需求；
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

- **扩大**：两条轨道均通过 fresh-task handoff、dirty-diff 与边界 readback，
  无中止条件或高/严重 escaped issue，且每条轨道都满足预先冻结的价值/摩擦
  阈值；只扩大能够回答下一项具体风险的样本。
- **简化后再试**：至少存在经判定的有价值信号，但任一预注册摩擦阈值、
  handoff 或低/中 escaped-issue 条件未满足；先删除或修正具体负担，再用新
  safe copy/authority/记录重跑受影响的最小轨道。
- **停止或重新定位**：触发安全/完整性中止条件、两条轨道均无净价值信号，
  或核心路径持续要求用户理解/操作内部协议。保留证据，不用扩大 run 数量为
  设计辩护。

通过这个小试点只授权下一阶段决策，不等于发布就绪、长期有效、不可绕过、生产成功或完整 North Star 已证明。

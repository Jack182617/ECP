# ECP North Star

本文件定义 ECP 的长期产品目标和不可扩张边界。`SPEC.md` 定义当前实现版本的可执行合同；二者冲突时，当前实现不得被宣传为已经满足尚未实现的 North Star 能力，但后续设计不得背离本文件。

## 愿景摘要

ECP 致力于建立一种可长期持续的 AI 原生软件工程方式：用户可以主要通过自然语言表达产品目标、业务取舍和重要授权，Coding Agent 负责理解项目并完成工程实现，项目则通过一个独立的控制层长期保存自身事实、约束每次变更，并核验完成结论是否具有与当前软件状态相匹配的证据。

AI 能够生成代码并不等于能够长期维护真实软件。聊天会被删除，模型、Agent 和维护者会更换，业务规则、架构边界、历史决策和未决问题也可能散落在代码、文档、对话和个人记忆中。如果这些事实不能由项目自身恢复，Agent 越快地修改代码，项目越可能在长期迭代中失去连续性、边界和可解释性。

ECP 希望让软件项目拥有独立于任何一次聊天、任何一个 Agent 和任何一位维护者的长期工程事实体系。项目应当能够持续回答：它为什么存在，哪些能力和不变量必须保持，为什么形成当前设计，哪些事实已经确认，哪些仍然未知，一次变更允许改变什么、必须保持什么，以及需要什么证据才能证明该变更完成。

在目标状态下，每次 mutating request 都从明确的意图和边界开始，经过影响分析、受约束实现、语义对账和必要验证，最终形成与精确 Project Truth、配置、源码和执行上下文绑定的 Evidence 与 Verdict。Agent 不能以自然语言自我宣告完成，不能拿旧状态的 Evidence 证明新状态，不能通过弱化规则为自身放行，也不能在发现 material ambiguity 后静默替用户作出产品决定。

长期成功的标志不是某个 Agent 永远记住项目，而是即使历史聊天全部消失、原 Agent 和维护者全部离开，一个新的 Agent 仍能只依赖项目自身可恢复的事实与权威状态，正确解释目标、边界、关键决策、风险和未知项，继续完成一个真实且边界清晰的需求，并留下可供下一次接手核验的结果。

因此，ECP 的最终方向是让自然语言成为软件创造和维护的主要入口，同时不以牺牲严谨软件工程为代价。它不承诺 AI 永不犯错，而是使重要错误更难静默进入，使未知不能伪装成已确认，使过期或不适用的证据不能冒充当前证据，并在错误发生后保留足够的历史用于定位、恢复和补偿。

本愿景是后续产品、架构和实现取舍的长期判断依据。仅增加 Agent 能力、自动化步骤或功能数量，却不能改善项目连续性、变更边界、受保护事实、证据真实性或长期接手能力的方案，不应仅因技术上可实现而自然扩张为 ECP 职责。当前实现范围和状态分别由 `SPEC.md` 与 `STATUS.md` 记录，不属于本愿景的完成声明。

## 1. 正式目标

ECP（Engineering Control Plane）是一套面向长期 AI 原生软件开发的、多项目通用、工具无关、local-first 的项目连续性与可信变更控制系统。

ECP 使用户能够主要通过自然语言向 Codex 等 Coding Agent 提出产品需求，而不需要手写代码或操作底层协议；同时让项目意图、关键业务规则、架构边界、数据与接口语义、设计决策、未知项和验证证据成为项目自身可恢复、可追踪、可验证的事实，而不是只存在于聊天记录、某个模型上下文或个人记忆中。

ECP 的核心闭环是：

```text
Project Truth
→ Change Contract
→ Impact Analysis
→ Agent Implementation
→ Semantic Diff
→ Required Gates
→ Exact Evidence
→ Verdict
→ Updated Project Truth
```

## 2. 系统只解决五个问题

1. **项目连续性**：删除历史聊天、替换 Agent 或更换维护者后，项目仍能恢复当前产品、业务、架构、决策、风险和未知项。
2. **变更边界**：每次修改都有明确目标、范围、非目标、验收条件、业务/架构/数据影响和风险；普通需求不能静默扩大。
3. **受保护事实**：关键不变量、架构边界、数据/API 契约、权限和验证策略不能被普通实现 Change 静默修改或自我弱化。
4. **证据真实性**：Verdict 只接受属于当前 Project/Workspace/activation/Change/Project Truth revision/config/source/execution context 的 Evidence。
5. **长期接手与恢复**：项目能够解释为什么这样设计、哪些结果实际验证过、哪些仍然未知，以及错误由哪个 Change 引入、如何回滚或补偿。

## 3. 用户与系统责任

用户负责产品目标、业务取舍、高风险授权、现实验收和发布决定。用户不应被要求手写代码、指定文件、运行 ECP CLI、复制 opaque 标识或维护底层 ECP 状态。

Coding Agent 负责读取项目事实、提出必要问题、实施代码和测试、生成 Impact 与 Semantic Diff、更新候选 Project Truth，并解释结果。

ECP Core 负责 Project Truth revision、Change 生命周期、受保护事实变更边界、source/config/context identity、Gate plan、Evidence applicability、Verdict 和审计历史。Core 不使用自然语言自我判定 PASS。

现有编译器、Schema、测试框架、Git、CI、发布和监控系统仍是各自事实的权威来源。ECP 连接、选择并绑定这些能力，不重新实现它们。

## 4. 多项目模型

一套 ECP Core 和 Adapter 可以服务多个项目。每个 Project 保存独立、版本化的 Project Pack；每个 canonical Workspace 保存独立 activation、Change 和本地 Evidence。

项目之间可以复用 schema、风险分类、policy template 和 Gate adapter，但不得隐式共享业务事实、accepted Project Truth、Change、Evidence 或发布状态。跨项目依赖必须显式建模，不能通过相同名称或目录猜测。

## 5. Project Truth

Project Truth 不是代码的自然语言副本，而是代码、Schema、测试和文档无法单独表达的长期工程语义及其关系。至少支持：

- 产品目的和 Capability；
- 业务、数据、架构、安全、隐私、兼容和运行不变量；
- Component 职责、path roots 和允许的依赖；
- 数据/API/事件/持久化契约的权威引用；
- 已接受、被取代和待验证的 Decision；
- Unknown、假设、证据等级和解决条件；
- Invariant/Capability/Component 与 Gate、测试、Schema 和源码引用的关系。

可由编译器、代码或 Schema 确定的事实只建立引用或生成索引，不在 Project Truth 中维护第二份可漂移副本。重要口头或聊天约定只有被提升为结构化事实、决策、契约、测试或 Gate 后才是 durable project fact。

## 6. Change 与语义变化

每个 mutating request 必须在首次仓库写入前形成 Change Contract。Change 必须声明预期影响的 Capability、Invariant、Component、Contract、Decision 和 Unknown，以及预期保持或改变的语义类别。若一个已接受 Unknown 进入影响范围，合同还必须明确它在完成时保持、解决还是被更精确地重述，并由最终 Project Truth 结果核对，不能因代码 Gate 通过而让 durable Unknown 静默过期。

Change 还必须把所有已发现的 material ambiguity 变成逐项、可审计的 Requirement：正常/加载/空态/成功/失败/重试/取消/超时/权限/并发/持久化/兼容/无障碍等情形只在与本需求相关时进入，并分别标记为当前已决定、不适用、安全延期或阻塞未知。每个验收项、保持项、用户旅程、数据/运行影响、预期变化和未知项都必须有精确覆盖与验证方式。ECP 的承诺是“零静默歧义”，不是声称已经发现宇宙中所有未来边界；一旦 material 问题被发现但无法从 accepted fact 或当前用户决定中推出，首次写入必须暂停。

实现后必须形成 Semantic Diff，明确产品行为、业务规则、架构、数据、接口、权限、兼容、运行和 Project Truth 是否变化。以下情况不得完成：

- 实际语义影响超出已接受的 Change Contract；
- 受保护事实发生未接受变化；
- Project Truth 候选与代码/Schema/测试的已知事实矛盾；
- 语义变化需要更新 Project Truth 但尚未 reconciliation；
- 声称无语义变化但存在未解决的 material drift；
- Required Evidence 缺失、过期、损坏或不属于当前 subject。
- 任一 Requirement 未决、未覆盖、最终未逐项对账，自动项缺少映射 Gate Evidence，或外部验证仍未取得可信 Evidence；
- touched path 反推的 Component/Capability/Invariant 未在 Change 中声明，或 established Project Truth 中出现未映射最终路径；
- 取消后仍留在源码中的修改被新 Change 当成全新 baseline，而没有显式 supersede 和继承原 baseline。

规则变化必须由变化前的 accepted policy/Project Truth 审查；新规则不得为自己的弱化签发通过结论。

## 7. 保证边界

ECP 不保证 AI 永不犯错、测试覆盖全部现实、项目没有缺陷或本地 PASS 等于设备、发布、远端或生产成功。

ECP 必须保证：重要错误更难静默进入；已知越界和证据失配 fail closed；未验证或未知事实不被伪装成已确认；高风险副作用不由普通 Change 自动获得授权；即使错误漏过，也保留足够历史用于定位、恢复和补偿。

## 8. 永久非目标

ECP 不自研聊天平台、Coding Agent、大模型、IDE、Git、测试框架、CI 平台、发布平台、生产监控、项目管理系统、通用企业知识库或大而全的 Agent 编排 DSL。

ECP 不复制每一行代码的解释，不把全部聊天记录当项目知识，不默认访问生产凭据，不把覆盖率、复杂度或单一指标作为自动放行依据，也不在缺少真实身份、隔离和受保护 consumer 时宣传不可绕过或完全安全。

## 9. North Star 验收

一个从零开始通过 ECP 迭代多年的项目，在删除全部历史聊天、替换原 Agent 和维护者后，新用户应能只用自然语言提出一个边界清晰的真实需求；新 Agent 能从当前仓库与 accepted Project Truth 恢复项目，解释影响和未知项，完成修改，生成 Semantic Diff，运行适当 Gate，取得与当前状态绑定的 Evidence，更新 Project Truth，并用产品语言报告结果，而不静默修改受保护事实。

只有当前状态的代码、Project Pack、Core、Adapter、测试和 enforcement evidence 共同证明这一场景时，ECP 才能声称实现了完整目标。

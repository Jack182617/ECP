# Project Pack

Project Pack 是每个项目随仓库保存的、可评审但不能自行生效的 ECP 项目事实包。它不是一个独立系统，也不是把代码改写成自然语言的知识库；它只保存未来维护者无法可靠地从单次聊天或代码语法中恢复的长期语义，以及指向代码、Schema、测试和正式文档的稳定引用。

一套 ECP Core 可以治理多个项目，但每个项目都必须拥有自己的 Project Pack。复制 schema 或模板是允许的，复制另一个项目的业务事实、accepted revision、Evidence 或 activation 是禁止的。

## 组成

```text
.ecp/
├── project.json       stable project identity
├── policy.json        risk and local execution policy
├── gates.json         project-native validation commands
├── truth.json         structured durable project truth
└── contracts/         selected product/data/API/architecture boundaries
```

仓库外 authority state 不是 Project Pack 的一部分。它保存哪一个 control config 和 truth revision 已经被接受、accepted truth/contract 的内容寻址私有副本、Workspace activation、Change、Semantic Assessment、GateRun、Evidence 与 Verdict。这样，仓库里的候选文件可以由 Agent 提议和评审，却不能通过修改自身直接成为权威；候选漂移时，新 task 仍能用 `truth get` 恢复 exact accepted 内容，Gate task 中断时也能用 `gate history` 区分未执行、已完成和仍待 lease-proven recovery 的 run。用户显式请求时，Core 还能把一套 authority 的 exact referenced history 导出为私有只读、可离线核验的 portable bundle；它用于审计/备份/交接，不会反向让 bundle 或仓库候选自行成为 live authority。

## 写什么，不写什么

应该写入：

- 产品存在的持久目的和用户可感知 Capability；
- 不允许被普通实现悄悄破坏的业务、数据、架构、安全、隐私、兼容和运行 Invariant；
- Component 的职责、代码边界和显式依赖；
- 无法由类型系统单独表达的数据/API/事件/权限/发布契约；
- 对当前实现仍有约束力的 Decision 及其理由；
- 尚未证明的事实、风险和可关闭 Unknown 的条件。
- Component `path_roots` 对仓库中所有受治理产品/工程文件的完整 ownership；`.ecp` 控制面是唯一内建例外，仍由 denied path 单独保护。

不应该写入：

- 每个文件、类、函数或代码行的自然语言复述；
- 临时任务步骤、完整聊天记录或已由 Git 保留的 diff；
- 能由编译器、Schema 或测试直接生成且没有额外语义的重复清单；
- 未经证据确认的理想架构、产品承诺或“最佳实践”；
- 密钥、个人数据、生产凭据、远端环境快照或发布成功声明。

判断标准很简单：如果一项内容没有它就可能让未来 Agent 做出语义错误，而它又不能从当前权威代码/Schema/测试可靠恢复，就应当成为结构化 truth、contract、decision 或 unknown；否则保留引用，不复制内容。

## 生命周期

1. `project init` 只创建诚实的 `seed` truth，并明确“尚未完成项目理解”这一高风险 Unknown。
2. 启用流程只负责把已评审的候选 Project Pack 接受并把 Workspace 带到 `READY`；如果 accepted truth 仍是 `seed`，只报告其 Unknown，不在启用流程中自动创建 Change。Truth maturation/onboarding 是之后的独立产品 Change，必须由用户另行明确授权，并从新鲜的 `ecp-change` 路由开始；证据不足的部分继续保留 Unknown。
3. 普通 Change 只读取 accepted Project Truth，在开始前声明结构化 Impact，并用逐项 Requirement 为所有 material contract item 记录决定来源、理由、精确覆盖和 `AUTOMATED`/`REVIEW`/`EXTERNAL` 验证模式。Core 不接受 `BLOCKING_UNKNOWN` 或漏覆盖。
4. Core 用 component path ownership 反推 scope/touched path 的 direct component，沿 `depends_on` 的反向传递闭包加入直接/间接依赖者，再得到 capability → invariant。声明与推导取并集进入风险/Gate；漏报或 established truth 的未映射最终路径阻止 PASS。
5. 实现完成后，Core 分别记录产品语义结果、每项 Requirement result，并计算 accepted 与 candidate Project Pack 的真实差异。自动 Requirement 仍需当前 mapped Gate Evidence；外部 Requirement 在 v0.3 保持 pending。`PRESERVED` 不接受任何 truth delta或 declared expected change；`CHANGED` 可以在 durable truth 仍准确时保持零 delta，非零 delta 必须在 Impact 内并由用户确认精确 protected change；`UNKNOWN` 阻止 PASS。
6. 新 accepted truth 与 Semantic Assessment 原子记录；Impact 关联 invariant 的风险和 `gate_ids` 会成为真实裁决输入，truth 演进则对 starting/final 两版取更高风险与保护 Gate 并集。取消后 replacement Change 继承原 baseline/lineage，旧 revision、完整 Change Contract/Requirement/Impact/semantic history、GateRun 终态与 Evidence 保留用于审计、接手和恢复。

## Gate 时间预算

`timeout_seconds` 是 fail-closed 的执行上限，不是示例默认值。启用项目或修改 Gate 前，必须在受支持、具有代表性的本地主机上执行 exact command，至少覆盖一次冷构建/冷工具缓存和一次正常缓存；命令及其传递脚本仍要先通过无网络、无凭据、无签名/发布/部署、无生产访问、无迁移和无外部写入审查。

初始时间预算应至少高于最慢代表性结果 50%，并为多分钟 Gate 额外保留不少于 60 秒的绝对余量；样本不足、宿主差异明显或命令有已知抖动时使用更保守上限。积累足够样本后可按观测 p95/p99 调整，但不能依赖测试缓存、偶然快跑或重试来掩盖过小超时。若合理超时过长，应拆分聚焦/affected/full Gate 或优化测试，而不是降低 Evidence 要求。

## 新人接手标准

一个合格 Project Pack 不要求新人先读完全部代码或旧聊天。新 Agent 应能从它开始回答：产品为什么存在、现在能做什么、哪些规则不能破坏、主要组件如何分工、哪些契约是权威引用、为什么做过关键取舍、目前什么仍未知，以及本次需求可能影响哪些事实。

Project Pack 只能缩短恢复路径，不能替代读代码、运行验证、产品判断和真实环境验收。一个多年项目是否真正达到“自然语言长期维护”，仍必须通过独立真实项目、跨 task 和新维护者试点证明。

本仓库根目录的 `.ecp/` 是可由当前 Core 严格加载的自举 Project Pack；它刻意保留真实项目试点、受保护 CI 和隔离 Runner 尚未完成的 Unknown，不能作为完整 North Star 已达成的证明。

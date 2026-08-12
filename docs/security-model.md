# ECP v0.3 Security and Trust Model

## 1. 结论

ECP v0.3 是本地的项目连续性、语义对账、工程证据记录与确定性裁决器，不是针对同用户恶意进程的安全边界。它优先防止：

- Agent 忘记范围、遗漏验证或拿旧测试结果冒充当前证据；
- 可写配置被悄悄弱化后产生空门禁 PASS；
- Gate 日志、退出 0 或自然语言被误当成更强事实；
- `.ecp`、Plugin 安装或另一个 clone 的状态被误当成当前 Workspace 已启用；
- 已启用项目在配置漂移、损坏或 Adapter 错误后静默退回未治理写入；
- 多个 clone/worktree 串用 Evidence；
- dirty baseline、commit、rename、delete 等使变更归属丢失；
- 取消后保留的源码修改被下一次 Change 当成新 baseline，从而洗掉原范围、风险和责任；
- 配置、源码或 artifact 损坏后继续放行。
- Agent 在多年迭代中不声明影响面、静默改写关键业务/架构事实，或在无法判断语义时仍声称完成；
- 产品请求遗漏空态、错误、取消、超时、权限或兼容行为时，Agent 私下猜一个方案并把猜测当成已确认需求；
- 为了“完整”无差别运行全量测试，或为了省时漏掉与实际影响/Invariant/Requirement 直接相关的验证；
- 候选 Project Truth 自己宣布自己已生效、用新弱规则审查自身，或让旧 truth/source 的 Evidence 满足新状态。
- Codex desktop 因 PATH、Plugin cache 深度、缺包或 binary 损坏而执行错误的 ECP Core，或在无法定位 Core 时静默转回未治理写入。

真正不可绕过的强制边界必须在受保护 CI、组织身份、分支规则、发布凭据和 release consumer 中实现。

## 2. 信任计算基

v0.3 仅假设以下内容在一次运行中可信：

- 正在执行并由版本 + 可执行文件摘要标识的 ECP Core 二进制；
- OS 对当前进程与私有状态目录提供的基本文件权限；
- Go runtime、从固定系统安装路径解析的 regular Git executable 和操作系统进程结果；
- 用户通过当前项目控制 UX 明确表达的 enable/disable、后续 policy acceptance 或 risk acknowledgement 本地意图。
- 用户对 exact protected Project Truth delta 的明确本地确认；该确认仍不是认证身份或不可抵赖审批。

注意：`codex-local-adapter` actor label、CLI actor/reason 和聊天中的确认都不是认证身份，Agent 或同用户进程也可能直接调用 CLI。由 canonical state-directory path 派生的 `authority_id`、activation ID/token 也都只是本地目标/epoch/并发选择器，不是认证身份或签名。开发工作树中的 ECP 不能为自身签发高可信 self-hosting 证明；只有经过外部信任流程固定和安装的 Core 才可能成为后续可信计算基。v0.3 的 binary digest 是 Evidence 兼容性身份，不是签名或 provenance。

## 3. 必须视为不可信

- 用户或 Agent 参数、聊天文本与网页内容；
- Requirement 的 `decision_source`、actor/reason 与自然语言产品确认；它们是可追溯声明，不是认证身份或事实证明；
- Skill 是否被模型召回、Plugin 当前是否启用，以及自然语言“已治理”声明；
- 仓库内 `.ecp/`、源码、测试、构建脚本和依赖；
- Git hooks、Git 配置、PATH 中可替换工具；
- Gate stdout/stderr、报告文件和退出 0；
- 未验签 CI、外部 API 回传和复制进来的 Evidence；
- 日志中的 `PASS`、`approved`、指令或链接；
- symlink、奇异文件、非法路径和超大输入。

## 4. v0.3 缓解措施

### 项目模式、启用与关闭

每个 canonical Workspace 缺少 authority activation record 时默认 disabled。Plugin 安装、`.ecp` 存在、Project ID 相同或另一个 clone/worktree enabled 都不能启用当前 Workspace。`project status` 只做 physical Git/authority/config 状态观察，不构造 source snapshot、不执行 Gate；对未注册 Workspace 返回 disabled 时不创建 `.ecp`、binding、event store 或 artifact。

Project mode 只有 `enabled: false|true`。`READY`/`ACTIVE`/`BLOCKED`/`INDETERMINATE` 是 assurance，不是 fallback mode。已启用项目的 valid config drift 返回 enabled+BLOCKED，malformed/missing Draft 返回 enabled+INDETERMINATE；Adapter 必须保留 `enabled: true` 并在仓库写入前停止，不能把非零 status 退出码或诊断失败解释为 disabled。

Enable/disable 都要求调用方原样回传 immediately preceding status 的 exact authority ID、Workspace ID 和 opaque activation token；enable 还要求当前 accepted config/truth digest。Token 覆盖 authority revision/event head、registration、current activation 与 enabled 位，所以旧 task、disable/re-enable 或 status 后新追加的 Change/Evidence 不能静默作用于新 epoch。每个 transition 生成不复用、链接 previous activation 的 activation ID；它继续绑定 Change、plan、Evidence、Verdict、acknowledgement、completion 与 cancellation。二者都不是签名或认证。

Enable 在 Workspace lease 内重载并再次核对 token/config，只允许 accepted default risk 至少有一个 Required Gate且 cwd/executable/environment 可解析的 Workspace进入 enabled。该 preflight 不执行项目代码；任一 setup/registration/acceptance/preflight/transition 失败都不写 enabled event。仓库 Draft Config 无权启用自己。

Disable 使用 authority-only loader，因此 malformed Draft 也不能把项目锁在 enabled。它在同一 lease/revision 下，把 status 已观察 ACTIVE Change 的 cancellation 与 project disablement作为一个原子 event append；保留 worktree、Draft、Evidence、history，不产生 PASS。未注册或已经 disabled 且无 ACTIVE Change时幂等返回，不重复追加。Event store 预留足够 bounded terminal capacity，避免资源上限让项目永久无法关闭。

`ecp-change` Skill 对任意普通仓库 mutation 先读 status：disabled 正常开发，enabled 自动治理，mode 不能确定则停止。`ecp-check`、`ecp-enable`、`ecp-disable` 分别隔离只读诊断与显式项目模式操作。已启用项目没有受支持的单 task bypass；显式 project disable才改变持久 mode。但 Skill/Plugin 可被关闭，direct shell、其他 Agent/工具和同用户进程可绕过，因此这仍是 workflow guardrail，不是真正 enforcement。

四个 Plugin Skill 只允许使用从各自安装路径解析的同一个 Plugin-root `scripts/ecp`。launcher 只选择包内与当前 Darwin/Linux arm64/amd64 匹配的 runtime，拒绝 symlink、缺失和不可执行文件，并用固定系统 SHA-256 工具核对 sidecar；不得 fallback 到 PATH、仓库 binary 或临时 build。runtime manifest 把四个 artifact 的路径、size、digest 绑定到 Plugin version，自动化还在 cache-like 副本与空 PATH 下验证发现路径，并验证一字节损坏会在 Core 启动前返回 `ECP_RUNTIME_CHECKSUM_MISMATCH`。launcher/sidecar/manifest 同属可替换 Plugin，因此这些检查是包内一致性保护，不是签名或 provenance。

### 注册、配置自我弱化与稳定读取

Core 先以 canonical state path 确定 authority，再以 canonical Workspace ID 查询该 authority 中的不可变 binding，最后检查 Draft `project_id`，因此候选配置不能切换到一套新 authority history。只有同一次 `project init` 原子创建、且稳定重读等于 Core 写入前预计算 digest 的 `.ecp/` 可以 bootstrap registration/initial acceptance；它仍保持 disabled。既有 `.ecp/` 必须先经只读 `project inspect` 观察，再由明确项目 enablement 流程把同一响应的 exact authority/Workspace/config 目标内部传给 registration。

Core 保存 accepted Project/Policy/Gates payload 与 digest。语法有效的 Draft Config drift 使 enabled status/Gate/Verdict `BLOCKED`，直到 Adapter 展示人类可读变化并取得新的明确确认，再把同一次刚审阅 JSON 的 exact authority/Workspace/config target 传给 `policy accept`；malformed 或缺失 Draft Config使 enabled status `INDETERMINATE`。两者都 fail closed，且 authority-only 的历史查询、显式 cancellation 与 project disable不依赖 Draft 可解析。Config Loader 从同一组已读 bytes 同时解析语义和计算摘要，并要求连续两次 tree digest 稳定。初次 registration/acceptance 可由明确 enable request 授权，后续 drift 不可自动接受；这些仍是可审计的本地确认，不是认证审批。

### Project Truth、Impact 与 Semantic Reconciliation

Control Config 与 Project Truth 使用独立摘要和独立 accepted epoch。`truth.json` 与 `contracts/**` 都是不可信 proposal；Core 保存 accepted truth payload/file manifest，并在事件写入前把 exact UTF-8 truth/contract bytes 写入 repository-external content-addressed private blobs。`truth get` 只按 accepted manifest 读取并重验 blob，候选 bytes 不能替代缺失或损坏的 accepted content。status、context、Change、plan、Evidence 与 Verdict 传播 exact truth digest。候选 truth drift 使 enabled 项目保持 enabled 但 BLOCKED，不能靠 `policy accept` 或普通 Gate PASS 生效。一个 valid candidate truth drift 可以进入显式 recovery/adoption Change，但该 Change 仍以 previous accepted truth 为 authority；malformed truth 不能进入，且 reconciliation 之前 Gate plan、PASS 与 completion 都被阻断。

Change start 必须声明结构化 Impact，并只引用当前 accepted truth 中存在的 capability、invariant、component、decision、contract 和 accepted unknown；purpose/maturity 变化使用独立布尔声明。无法在启动时精确引用的新发现必须诚实声明具体 unknown，但该文本 unknown 只能覆盖新增 truth entity，不能授权修改或删除任何既有受保护事实。Core 自己比较 previous accepted 与 candidate truth/file manifest，生成排序后的结构化 delta，并检查 delta 是否被 Impact 覆盖。`contracts/**` 中的每个文件还必须由一个 contract ID 引用，禁止不可寻址的隐含 truth 文件。

每个 Change 还必须提供排序后的 Requirement ledger。Core 要求 acceptance、journey、data/operation effect、expected change/preservation 和 startup unknown 全部被 exact string coverage；每项必须有非空的当前要求、status、rationale、decision source 与 verification mode。`BLOCKING_UNKNOWN` 在实现前停止，`DEFERRED_SAFE` 必须有可执行的 revisit condition，AUTOMATED 必须指向已接受 Gate，EXTERNAL 在 v0.3 只能保持 pending。这样可以消除“没有记录就当默认”的静默空洞，但 Core 无法认证产品是否真的作出该决定，也无法保证提出的问题集合已经穷尽现实世界；Adapter 仍需基于实际需求主动检查相关状态链，无法确定时向产品所有者提问。

Established Project Truth 的 component path roots 会从 scope/final touched paths 反推直接影响，再沿 `depends_on` 的反向传递闭包加入所有依赖组件及其 capability/invariant。scope 阶段的漏报或未映射路径拒绝启动，final 阶段的漏报或未映射路径阻止 PASS，且推导结果仍保守抬高风险与 Gate。取消 Change 留下 delta 时，replacement 必须显式 supersede 最新取消项、继承原 baseline/lineage 并覆盖 inherited paths，避免 cancel/start 洗白已有修改。该机制依赖 Project Truth 的 path map 与依赖方向正确且持续更新；错误、缺失、循环或过宽的 map 会产生漏检或误报，不能被描述成静态分析的完整证明。

产品语义结果与 durable Truth revision 分开处理。`CHANGED` 可在 accepted Project Truth 仍准确时使用零 delta，只追加 semantic event；非零 truth delta 全部按 protected change 处理，缺 exact source/previous/candidate digest、actor/reason 或 `--confirm-protected` 都拒绝，并把 semantic event 与新 truth acceptance 原子追加。`PRESERVED` 携带任何 delta 或用于逃避 declared expected changes 都拒绝；未声明且无 startup uncertainty 的意外 `CHANGED` 也阻断。`UNKNOWN` 允许把不确定性写入审计历史，但 Verdict 必须保持 BLOCKED。Skill 必须先用产品语言展示 protected delta并取得当前用户明确确认；CLI flag 和 actor label 本身不证明调用者身份，同用户恶意进程仍可伪造，因此更强审批需要独立身份/CI boundary。

Semantic Assessment 绑定 final source fingerprint 和 final truth digest。源码、truth、plan 或 Evidence 任一变化都会让旧 assessment/Evidence 不适用；Required Gates 全绿但 semantic assessment 缺失、UNKNOWN 或 stale 时仍不得 PASS。

Semantic Assessment 的 `PRESERVED/CHANGED/UNKNOWN` 与摘要仍来自 Adapter/Agent 的判断，不是 Core 对任意程序行为的形式化证明。Core 只校验类别、精确绑定、真实 truth delta、Impact coverage、protected confirmation 与 Evidence 一致性；一个错误或恶意调用方仍可能把未被 Gate/Schema/contract 发现的语义破坏声明为 `PRESERVED`。因此 Project Truth 必须用可执行测试、Schema 和独立 Gate 引用尽量约束关键不变量，完整抗绕过还需要独立审查或受保护 CI consumer。

### 命令注入与执行

- Gate 只接受 JSON argv；不自动插入 shell。
- cwd 必须真实落在 Git root 内。
- 从空环境构建 policy/Gate allowlist；值只进入摘要，不进入事件或聊天。
- 已知敏感变量名 denylist 是 best-effort 减损，不能发现所有 token、配置文件或 Keychain 凭据。
- executable 必须解析为 regular executable 并绑定其路径与内容摘要；Core binary 也绑定实际可执行文件摘要。
- Core Git 不使用 ambient `PATH`；当前受支持的 POSIX Unix authority 路径只从 `/usr/bin/git` 或 `/bin/git` 解析 regular executable。候选均不可用时返回 `TRUSTED_GIT_UNAVAILABLE`，不会 fallback 到 Agent/仓库控制的 `PATH` shim。实际 Git realpath/SHA-256 写入 snapshot 并参与 source fingerprint，因此同路径 Git bytes 替换会使旧 Evidence stale。Windows 固定候选代码只保持交叉编译，authority operations 会更早以 `PLATFORM_SECURITY_UNSUPPORTED` 拒绝。
- 配置加载先校验环境变量名、已知敏感名与 NUL；`change start` 先核对 immediately preceding context 的 exact authority/Workspace/activation/config/source，再对 selector-matched scope risk Gate、Impact/inferred invariant `gate_ids` 与 AUTOMATED Requirement Gates 的并集预检 cwd/executable/environment，且不执行项目代码。
- `gate plan` 把 exact authority/Workspace/activation/Change/config/source/risk/inferred impact、Required Gates、解析后的 cwd/executable/environment 与 Core identity 绑定到 opaque digest；path/component/capability/invariant selector 只过滤 risk-based Gate，显式 invariant/Requirement Gate 仍强制。计划按 `fast → affected → full` 稳定排序，避免无关全量流程；truth 演进仍对 starting/final epoch 取保守并集。`gate run` 在 lease 内、首个 Gate 前及每个后续 Gate 前重建并核对，mismatch 时不执行下一条项目命令。
- timeout、输出上限、顺序运行和 Unix process-group cancellation 降低失控风险。
- CLI 捕获 `SIGINT`/`SIGTERM` 并取消 root context，使 Gate 进程组清理、`CANCELLED` terminal event 与 defer 锁释放路径可运行。`SIGKILL`、突然崩溃或断电无法运行 defer，但 GateRun start 已在项目代码前持久化；Darwin/Linux/BSD 上下一位成功取得已释放 advisory lease 的操作会先记录 `INTERRUPTED`。这只恢复 authority lifecycle，不保证清理全部后代进程、orphan artifact 或外部副作用。
- 声明 `requires_network` 或 external side effects 的 Gate 在 v0.3 配置加载时被拒绝。

DefaultConfig 不再继承 `HOME`，环境变量 denylist 覆盖常见 token/secret/password/passwd/credential/private-key/API-key/access-key 片段、主要 cloud/vendor 前缀，以及 `KUBECONFIG`、`DOCKER_CONFIG`、SSH/GPG agent、XDG config/runtime 等 capability 名。这只是 best-effort 减损：项目命令本身仍是同用户任意代码，即使没有 `HOME` 环境变量，仍可能通过 OS 身份信息或绝对路径访问用户目录/Keychain，也可自行联网、fork、消耗 CPU/内存或修改仓库外状态。路径哈希与 spawn 之间仍存在同用户 executable swap 窗口；v0.3 没有 fd-exec 或 sandbox。

authority event history 使用兼容的 `events.json` 根段和 `event-segments/0000000000000001.json` 起的连续 continuation segments。新写入按 8 MiB 分段滚动；旧版单文件仍可读取到 64 MiB；continuation 最多 1024 段、聚合最多 1 GiB，并为项目关闭保留 2 MiB terminal capacity。一个原子 event batch 不跨段，放不进单段时在写入前拒绝。所有段共同维持 sequence/hash chain；缺段、非 canonical 名称、空 continuation、超限、非 private regular file、危险的原子写临时残留或跨段 hash 不一致都 fail closed。

`authority health` 在 Gate lease → mutation lock 下构造 authority-only snapshot：authority state directory 必须在取锁前已经是 private real directory，binding/event projection 不可信时直接失败；可信时核验全部历史 referenced truth/Evidence 并有界扫描 event capacity、safe crash remnants、orphans、unrecognized/unsafe entries 与 active GateRun。它不读取 candidate `.ecp`，因此 malformed candidate 不能阻止诊断；也不沿 object-store 或中间 artifact directory symlink 读取对象。`HEALTHY`/`ATTENTION`/`INDETERMINATE` 是 `ok: true` 的观察结果，不是自动动作；后两者以退出码 3/4 保留完整 findings。取得 released Gate lease 只允许报告旧 `IN_PROGRESS` 可由下一 lifecycle operation 中断恢复，health 自身不得 terminalize。操作不会 append/repair/delete/restore/compact/migrate/GC 或 chmod authority directory，但 advisory lock 获取可创建或刷新私有 lock metadata；revision/event head 必须保持不变。仍被 live holder 占有的 lease 必须 conflict/cancel，不能通过 PID 年龄猜测。

分段和 health 诊断消除了单文件增长停止并让容量/引用损坏/垃圾可见，但没有把本地历史变成无限或自愈存储。truth blobs 与 Evidence artifacts 仍没有完整的 scheduled/reference-aware retention/GC、脱敏/加密归档、restore/import、repair 或安全 compaction 协议；1 GiB event 上限也尚未经过多年真实负载与 crash-injection 证明。剩余问题继续作为正式 Project Truth Unknown，不得把“已分段”“可诊断”或“可导出”表述为多年留存已经完成。

显式 authority export 在 mutation lock 内重载 projection，只收集 event chain 与该 projection 实际引用的 truth/Evidence 文件；live store 中的 locks、temporary writes 和 orphan artifacts 不会被带入。Core 把复制结果的 canonical path、role、size 和 SHA-256 写入严格 manifest，在 commit 目标前用 bundle 自身身份重新播放 event chain、重验所有 accepted truth blobs/Evidence artifacts，并要求 exact reference set。目标必须位于仓库与 live authority 之外且原先不存在；最终树为 private read-only，相同 authority 的 bundle digest 不依赖目标路径。候选 `.ecp` malformed 不影响 authority-only export。

这仍不是不可篡改或认证归档：能重写 bundle 的同用户可同时重算 manifest，event hash chain 也不能抵抗重写整条历史；它们提供的是分层损坏/不一致检测。Bundle 可能包含 Gate stdout/stderr、项目事实和配置，Skill 只能在用户显式指定/确认本地目标后导出，不能自动 commit/upload/share。当前没有 encryption、redaction、signature、notarization、remote attestation、restore/import/merge 或自动备份调度。

### 路径与文件

- repo-relative path 统一校验 `..`、绝对路径、NUL 和 containment；
- Git root 以 physical `.git` boundary 为起点并与 Git top-level 交叉核对；repo-local `core.worktree` 重定向在任何 `.ecp` 写入前拒绝；
- `.ecp/` 配置拒绝 symlink；
- source symlink 只哈希 link text；
- indexed submodule 必须是已初始化的 real Git directory；缺失或被 regular/symlink 替换时 fail closed。有效 submodule 递归绑定 exact HEAD/index/worktree/untracked manifest，共享 top-level 文件/字节预算，Git status 强制 `--ignore-submodules=none`；
- submodule 内部变更在 scope 投影中折叠为 mount path，内部 risk rule 或 denied path 与 mount 重叠时均保守匹配；v0.3 不承诺细粒度 submodule scope；
- Core artifact 名称由 Core 生成并写入私有 state；
- Workspace binding、`events.json`、continuation event segments 和 Evidence artifacts 必须是有界、非 symlink 的 regular private files；读取时类型被替换或 group/other 权限暴露则 integrity fail closed。这是安全性检查，不把本地同用户写权限变成强隔离；
- 非 regular/symlink/合法目录边界输入 fail closed。

Go 标准库无法在所有平台上完全实现 `openat(O_NOFOLLOW)` 式防 symlink swap；这是本地同用户攻击者模型下的已知边界。

### Evidence 与并发

- Evidence 只能由 Runner 生成，不提供 `setVerdict` 或 `markPassed`；
- pre/post source fingerprint 必须一致；
- source/config snapshot 都做双观察稳定性检查；
- state mutation 使用 workspace lock、expected revision CAS，并只原子重写当前 event segment 或原子创建下一个连续 segment；
- Gate sequence、semantic reconciliation、acknowledgement、Change completion/cancellation 与 project enable/disable共享独占 lease；Change start 必须匹配调用方刚观察的 exact authority/Workspace/activation/config/source，所有后续 active mutation 必须仍属同一 enabled activation并匹配 exact ACTIVE Change ID，Gate sequence 还必须匹配已绑定 authority/Workspace/activation 的 opaque plan digest，acknowledgement/completion 还必须匹配已覆盖 authority/Workspace/activation 的 exact subject digest；单独 cancellation 额外必须匹配 `change list` 中的 exact authority/Workspace target；
- Gate sequence 在任何项目命令前先追加 `IN_PROGRESS` GateRun；每条 Evidence 绑定 exact run，同一 Gate 不可重复记录，终态必须是 `COMPLETED/FAILED/CANCELLED/INTERRUPTED` 之一。存在 unresolved run 时只允许匹配的 Evidence/terminal event，status/context 暴露它且 Verdict `INDETERMINATE`；
- contender 只有成功取得同一已释放 lease 后才可把旧 `IN_PROGRESS` 记录为 `INTERRUPTED`。live holder 仍占有 lease 时 contender 等待或失败，不能按时间、PID metadata 或错误文本猜测中断；
- 每条 Evidence 保存执行时的 exact `activation_id` 与 `plan_digest`；Verdict 要求当前 enabled epoch并重建 current plan，activation 或 plan digest 不匹配时旧 Evidence 不得满足 Requirement；
- project registration/policy acceptance 必须匹配调用方在同一次审阅中提供的 exact authority ID、Workspace ID 和 candidate config digest；稳定读取不替代 review-to-command 绑定；
- project enable/disable 必须匹配同一次 immediately preceding status 的 exact activation token；status 之后任何 authority append 都使旧 mode transition 冲突；
- Event、Evidence、acknowledgement 与 activation transition 均追加，不覆盖历史；
- 完成事件保存最终 Verdict payload 与摘要；
- 单独 cancellation 仅追加 actor/reason/time 并保留 Evidence、不产生 PASS且保持 enabled；project disable可原子追加 cancellation + disablement；
- Verdict 每次重算，不信任缓存布尔值。

外部进程可以无视 ECP lock；pre/post 只能检测观察点差异，不能检测运行中短暂篡改后完全恢复。Darwin/Linux/BSD advisory lock 由内核在持有进程退出时释放，不依靠年龄猜测 stale；Core 已用实际被杀 holder 回归验证“先保留 IN_PROGRESS、后由下一位 lease holder 记录 INTERRUPTED”。其他 Unix 的 sentinel fallback 在 crash 后仍需要人工恢复，且尚未列入已验证运行矩阵。非 Unix 不会进入该 fallback，authority operations 在此前已 fail closed。

### 日志与秘密

- 原始 stdout/stderr 限长并写入 0600 artifact；
- CLI 默认不回显原始内容；
- 摘要与状态机忽略日志自然语言；
- 当前结构化 CLI 只返回 artifact 元数据；未来任何日志展示都必须先消毒 ANSI/OSC 与控制字符。

v0.3 不承诺通用 secret redaction 能发现全部凭据。Gate 不应接触不必要秘密。

## 5. 明确不保证

v0.3 不保证以下能力，也不抵抗以下风险：

- root、被攻陷 OS、替换后的 ECP/Git/toolchain；
- 拥有同一用户 shell 与 state store 写权限的恶意程序；
- ignored cache、系统依赖、网络响应、时钟、随机数和硬件差异；
- Gate 读取 HOME/Keychain、联网或写项目外文件；
- 项目依赖/编译器供应链攻击；
- 测试充分、实现语义正确或不存在未知缺陷；
- Requirement 问题集合自动穷尽所有真实边界，或 `decision_source` 能证明产品本人作出决定；
- 配置错误、不完整或过宽的 component path ownership/selector 仍能自动得到理想 Gate 集；
- v0.3 Core 导入、认证或满足设备、生产、第三方系统等 EXTERNAL Evidence；
- 用户主动确认恶意配置或危险命令；
- 外部副作用自动回滚；
- `SIGKILL`/崩溃后的后代进程完整收割、orphan artifact 恢复、外部副作用回滚或自动续跑；持久 GateRun 只记录随后确认的 `INTERRUPTED` authority fact；
- 本地 hash/hash chain 的真实性、不可抵赖或远程证明能力；
- local PASS 等同 CI、设备、生产、部署或发布 PASS；
- 本地开发版 ECP 对自身源码的高可信 self-hosting assurance；
- `authority_id` 提供身份认证、防止同用户写 state，或允许 v0.3 透明迁移 state directory；
- `activation_id`/`activation_token`、`codex-local-adapter` label 或自然语言确认提供身份认证、签名、不可抵赖性或恶意调用者隔离；
- Skill/Plugin adapter 对关闭 Plugin、其他 Agent/工具、direct shell 或同用户进程提供不可绕过 enforcement；“无单 task bypass”只描述受支持的 Adapter UX；
- bundled Plugin checksum 提供发布者身份、签名、notarization 或供应链 provenance；能替换整个 Plugin 的攻击者可以同时替换 launcher、binary、sidecar 与 manifest；
- 仅安装在 Homebrew、自定义目录或其他 ambient `PATH` 位置的 Git 可被 v0.3 Core 使用；固定系统候选不可用时必须 fail closed，这是显式可移植性边界；
- Windows 或其他非 Unix 权威运行；交叉编译通过只证明源码可构建，`ecp version` 以外的 authority load/mutation 会在任何写入前返回 `PLATFORM_SECURITY_UNSUPPORTED`。

## 6. 后续安全强化

1. 独立长期运行 Core daemon，分离 Agent API 与 human approval API；
2. OS sandbox、临时 HOME、默认断网、只读源码和独立输出；
3. 在不可变 snapshot 上运行 Gate，隔离 cache 和依赖；
4. Keychain/HSM 密钥、签名 attestation、远端 notary/透明日志；
5. CI OIDC/Sigstore/SLSA provenance 验证；
6. policy 从受保护分支/组织控制面取得，而非信任 PR 工作树；
7. RBAC、二人审批、撤销、过期和恢复演练；
8. release consumer 对 commit、policy 与 attestation 做强制校验；
9. request idempotency、orphan artifact recovery、跨平台 crash 后进程树收割和全局资源预算；
10. fd-exec/受信 executable registry，缩小解析、哈希与执行之间的 swap 窗口。

## 7. v0.3 已实现回归矩阵

必须持续覆盖：

- `a;touch` 作为单 argv 不触发第二命令；
- path traversal、绝对路径、prefix collision、越界 symlink；
- JSON 未知字段、重复键与 trailing data；
- tracked/staged/unstaged/untracked、mode、symlink、submodule 与 commit 变化；
- submodule 在两个不同 dirty/unrecorded checkout 之间切换后 Evidence stale；
- 配置弱化后不能 vacuous PASS；
- 未注册 Workspace status 默认 disabled，且不创建 `.ecp`/authority state、不 snapshot source或执行 Gate；`.ecp`、Plugin、同 Project 的另一个 clone/worktree都不能隐式启用；
- enable要求 exact authority/Workspace/activation/config，default-risk Gate缺失或execution context不可用时仍保持disabled，enable preflight不执行项目代码；
- enabled config drift保持enabled+BLOCKED，malformed/missing Draft保持enabled+INDETERMINATE，并可经authority-only disable关闭；
- disable对status后的新Change/Evidence/authority append以stale activation token冲突；对已观察ACTIVE Change原子记录cancellation + disablement，保留source/Evidence并支持disabled幂等；
- disable/re-enable生成不同activation ID，旧context/Change/plan/Evidence/acknowledgement/subject不能跨epoch适用；
- 已有 `.ecp` 显式注册、Draft project ID 不替换 authority、配置读取 epoch 稳定性；
- repo-local `core.worktree`、尾随空格路径与 repo 内 state dir 不造成越界写；
- ambient `PATH` Git shim 不被执行，固定系统 Git 缺失时 `TRUSTED_GIT_UNAVAILABLE` fail closed，Git bytes 变化使旧 Evidence stale；
- DefaultConfig 不继承 `HOME`，扩展的 capability/secret 环境名 denylist 在配置预检与执行时均 fail closed；
- Gate timeout、启动失败、异常退出、输出截断和 pre/post mutation；
- 后代进程持有 stdio 时 timeout 仍有界返回；Core/executable/environment identity 漂移使 Evidence 不兼容或 stale；
- 并发 ACTIVE Change 与 event revision 冲突；
- declared/scope-reachable risk 缺 Gate 时不创建 ACTIVE Change；
- artifact 缺失/修改、event hash 损坏；
- Workspace binding/event/artifact 被换为非 regular file 或暴露 group/other 权限时 fail closed；
- authority/Workspace/activation/config target、active Change ID 与 acknowledgement/completion subject digest mismatch；
- Change start 的 authority/Workspace/activation/config/source precondition mismatch 不创建 ACTIVE Change；
- cancellation 的 authority/Workspace/Change target mismatch 不作用到另一套 state、Workspace 或 active Change；
- Gate plan digest 在首个或后续 Gate 前 mismatch 时不执行下一条项目命令，并保留已提交的 partial Evidence；
- Evidence 保存的 activation/plan digest 与 Verdict 当前 epoch/重建 plan 不同时不再适用；
- submodule mount 内部 risk/denied path 的保守匹配；
- 日志中的自然语言 `PASS` 不影响裁决；
- malformed Draft 下历史可读/可取消、cancellation 不删除 Evidence；
- GateRun 在项目代码前持久化 start，Evidence 精确绑定 run，正常/内部失败/context cancellation 形成 `COMPLETED/FAILED/CANCELLED`，impossible terminal replay fail closed；
- Gate sequence start 后的 error envelope 即使零 Evidence 也返回带 run ID/终态的 partial result；start 前普通错误不带零值 partial result；
- live advisory-lock holder 不被 contender 误记为中断；实际被杀 holder 释放 lease 后先保留 `IN_PROGRESS`，下一位 holder 追加 `INTERRUPTED` 再继续，candidate malformed 时 `gate history` 仍可读取。
- authority health 在 candidate malformed 时仍取得一致 authority-only snapshot；健康/孤儿/临时/损坏引用、unsafe entry、分段容量和 released/live Gate lease 分别得到稳定 `HEALTHY/ATTENTION/INDETERMINATE` 或 typed lock error，且诊断前后 revision/event head 与所有对象不变；
- control config 与 Project Truth 独立 digest/accepted state；contract drift 不能被 policy acceptance 吸收；
- Change start 拒绝空 Impact、未知 truth reference 和 stale truth digest；
- Gate PASS 在 semantic assessment 缺失时仍 BLOCKED，`UNKNOWN` 永远不能 PASS；
- Core 计算 protected truth delta，缺显式确认不能接受，成功 reconciliation 原子接受新 truth epoch；
- final semantic assessment、Gate plan、Evidence、Verdict 和 completion 精确绑定同一 source/truth epoch。

当前自动化没有把 ANSI/OSC 消毒、crash 后全部后代进程收割/orphan artifact 恢复、远端 release policy consumer、全量超大 source/event/config 边界和所有非 Darwin/Linux/BSD crash-lock fallback 宣称为已证明。这些属于 Phase 1/4 必须补充的专项测试；在对应实现出现前，local scope Evidence 本身没有资格满足任何 release policy。

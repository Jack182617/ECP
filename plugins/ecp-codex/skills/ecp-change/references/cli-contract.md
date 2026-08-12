# ECP CLI machine contract

The CLI is the adapter boundary to ECP Core, not a user tutorial. Never read or
write the repository-external authority store directly. Never ask the user to
transcribe an opaque field; copy it internally only from the named immediately
preceding JSON result.

Within the installed Plugin, every `ecp ...` command in this reference means
the `scripts/ecp` launcher resolved relative to the selected `ecp-change`
Skill. Never use an ambient PATH executable. The launcher selects the bundled
Darwin/Linux arm64/amd64 Core, verifies its package checksum, and fails closed
before Core invocation when the platform, runtime, or checksum is invalid.

Project/authority operations currently require POSIX Unix private-file
semantics and fixed system Git. `ECP_RUNTIME_*`, `PLATFORM_SECURITY_UNSUPPORTED`, or
`TRUSTED_GIT_UNAVAILABLE` stops repository mutation; do not bypass these with
ambient `PATH`, a different state directory, or direct state access.

## Project mode

Every canonical Git Workspace has exactly one user-visible mode:

- `enabled: false`: ECP does not govern ordinary repository work.
- `enabled: true`: ECP governs every supported Codex repository mutation.

`BLOCKED` and `INDETERMINATE` are assurance states, not additional modes. An
enabled project with config drift, malformed config, missing Gates, or an
integrity problem remains enabled and must not fall back to ordinary editing.
`.ecp` presence, Plugin installation, another clone, and another worktree do
not enable the current Workspace.

## Operations

| Command | Result and rule |
| --- | --- |
| `ecp project status [--root PATH]` | Read-only mode probe. It executes no project code, does not source-snapshot, and returns disabled without creating `.ecp` or authority state for an unregistered Workspace. It returns `authority_id`, `workspace_id`, and opaque `activation_token` for an immediately following mode transition. Optional `active_gate_run` makes assurance `INDETERMINATE`; do not guess whether it is live or abandoned. |
| `ecp project init --name NAME [--root PATH]` | Create a new Draft Config and bootstrap registration/initial acceptance. Invoke only inside an explicit current project-enablement request and only when `.ecp` is absent. It does not enable ECP. |
| `ecp project inspect [--root PATH]` | Read the exact candidate identity/digest without executing project code or registering it. Use after the final setup edit for existing unregistered `.ecp`. |
| `ecp project register --authority ID --workspace ID --config-digest SHA256 --truth-digest SHA256 --actor LABEL --reason TEXT` | Register and initially accept exactly one inspected existing Control Config and Project Truth. Invoke only after explicit project enablement and adapter review of both exact candidates. |
| `ecp policy accept --authority ID --workspace ID --config-digest SHA256 --actor LABEL --reason TEXT` | Accept one exact reviewed Draft Config epoch. During initial enablement, the explicit enable request supplies intent. Later drift requires a fresh, explicit user confirmation of the human-readable change. |
| `ecp project enable --authority ID --workspace ID --activation-token SHA256 --config-digest SHA256 --truth-digest SHA256 --actor LABEL --reason TEXT` | Atomically enable exactly the observed registered Workspace/config/truth/mode epoch. Core requires a default-risk Gate and preflights its execution context without running project code. Failure leaves mode disabled. |
| `ecp project disable --authority ID --workspace ID --activation-token SHA256 --actor LABEL --reason TEXT` | Disable exactly the observed project epoch. If the observed state has an ACTIVE Change, Core atomically appends `change_cancelled` then `project_disabled`. Source and Evidence are retained. Already-disabled/unregistered mode is idempotent. |
| `ecp context get [--root PATH]` | Read exact lifecycle context, accepted/candidate truth digests, source fingerprint, and optional unresolved `active_gate_run` for a registered Workspace. One response supplies start preconditions; a valid candidate truth drift may be entered only through an explicit recovery/adoption Change. |
| `ecp policy get [--digest SHA256] [--root PATH]` | Recover the latest or one exact historical authority-backed accepted Project/Policy/Gates payload. It is authority-only and remains available when candidate control files drift or are malformed; it does not accept or rewrite the candidate. |
| `ecp truth get [--digest SHA256] [--root PATH]` | Recover the latest or one exact historical authority-backed accepted structured Project Truth and exact UTF-8 `truth.json`/contract contents from private content-addressed blobs. It is authority-only and remains usable when the repository candidate drifts or is malformed; the optional digest must name a real accepted epoch, and missing, unsafe, or corrupt blobs are integrity failures. |
| `ecp change start ... --expect-change TEXT... --expect-preserve TEXT... --requirement JSON... [--supersedes-change CANCELLED_ID] [--impact-project-purpose] [--impact-unknown-id ID...] --authority ID --workspace ID --activation-token SHA256 --config-digest SHA256 --truth-digest SHA256 --source-fingerprint SHA256` | Create one bounded ACTIVE Change with structured Impact and a sorted Requirement decision ledger against the exact immediately preceding context. Every acceptance/preservation/journey/data/operation/expected-change/impact-unknown item needs exact coverage; `BLOCKING_UNKNOWN` is refused. Existing protected facts require exact IDs. If source differs from the latest cancelled Change baseline, the replacement must explicitly supersede it and carries the original baseline/lineage. `--truth-digest` is accepted truth, never an unaccepted candidate. A stale activation/config/truth/source/target conflicts before append. |
| `ecp change list [--root PATH]` | Read ACTIVE, COMPLETED, and CANCELLED history through the authority-only path, including full goal/scope/non-goals/acceptance/structured Impact and Semantic Assessment history. It remains available when Draft Config is malformed. |
| `ecp change cancel --authority ID --workspace ID --change ID --actor LABEL --reason TEXT` | Cancel exactly one immediately observed ACTIVE Change while keeping project mode enabled. User intent must explicitly target abandonment of that Change. |
| `ecp truth diff [--root PATH]` | Read-only comparison of candidate Project Truth against the accepted epoch for the ACTIVE Change. Returns exact previous/candidate digests, deterministic structural/file delta, and protected flag; it never accepts the candidate. |
| `ecp truth reconcile --authority ID --workspace ID --activation-token SHA256 --change ID --source-fingerprint SHA256 --previous-truth-digest SHA256 --candidate-truth-digest SHA256 --behavior PRESERVED\|CHANGED\|UNKNOWN --summary TEXT --category TEXT... --requirement-result JSON... [--confirm-protected] --actor LABEL --reason TEXT` | Record the exact semantic outcome and one sorted result for every Requirement. Automated items map to their declared Gate IDs; review, not-applicable, deferred-safe, and external-pending outcomes must match the start contract. `PRESERVED` requires zero truth delta and no expected change. `UNKNOWN` remains non-PASS. A nonzero CHANGED delta requires exact Impact coverage and protected confirmation, then atomically accepts candidate truth. |
| `ecp gate plan [--root PATH]` | Return required Gates, `tier`, inferred direct/path-owner and reverse-transitive dependent components plus their capabilities/invariants/unmapped paths, and an opaque `plan_digest` bound to authority, Workspace, activation, Change, contract, config, accepted truth, source, risk, execution context, and Core identity. Gates come from matching risk selectors plus mandatory invariant and Requirement links; truth evolution unions starting/final protections. Read-only; it is not authorization for Gate effects. Unaccepted truth drift blocks planning. |
| `ecp gate run --change ID --plan-digest SHA256 [--gate ID ...]` | Execute only the exact current plan. Before project code, Core records `IN_PROGRESS`; each Evidence binds the returned `run_id`; the sequence ends `COMPLETED`, `FAILED`, or `CANCELLED`. `COMPLETED` does not mean all Evidence passed. A later lease holder may first recover an abandoned run as `INTERRUPTED`. Never replace this with a direct command run. |
| `ecp gate history [--change ID] [--root PATH]` | Read ordered authority-backed GateRun history, including selected Gates, terminal outcome, and committed Evidence IDs. It remains available with malformed candidate config. `IN_PROGRESS` is unresolved; only Core after acquiring the released lease may append `INTERRUPTED`. |
| `ecp evidence list [--change ID] [--root PATH]` | Read Core-recorded Evidence for an exact, active, or most recent Change without opening the state store. |
| `ecp authority health [--root PATH]` | Take a Gate-lease then mutation-lock consistent, authority-only snapshot without loading candidate `.ecp`. The authority directory must already be private and real; the diagnostic does not chmod it or follow object-store/intermediate artifact symlinks. Verify the full event projection and every historical Truth/Evidence reference; inventory byte/segment capacity, terminal reserve, safe remnants, orphans, unrecognized/unsafe entries, and an unresolved GateRun. `HEALTHY`/`ATTENTION`/`INDETERMINATE` are `ok: true` results with exit `0/3/4`. It may create or refresh private lock metadata but never appends, repairs, deletes, restores, compacts, migrates, GCs, or terminalizes a run. |
| `ecp authority export --output NEW_DIRECTORY [--root PATH]` | Under the authority mutation lock, create and self-verify one deterministic private read-only bundle containing exactly the binding, complete event segments, referenced accepted truth blobs, and referenced Evidence logs. It is authority-only, works with malformed candidate files, excludes orphans/locks/temp files, requires a new destination outside repository/live authority, and does not change the authority revision. The explicit operation may expose sensitive logs and must never be auto-uploaded or committed. |
| `ecp authority verify --bundle DIRECTORY` | Offline read-only verification of manifest paths/roles/sizes/digests, exact referenced file set, event sequence/hash chain, projection head/epochs, every accepted truth blob, and every referenced Evidence artifact. It needs neither repository nor live state. `valid` is local content consistency, not signature, identity, approval, remote attestation, or restore authorization. |
| `ecp verdict [--root PATH]` | Derive the current local Verdict. Model judgment, stdout text, exit code alone, or an older result cannot replace it. |
| `ecp acknowledgement record --change ID --subject-digest SHA256 --actor LABEL --reason TEXT` | Record an exact current risk acknowledgement only after the user explicitly confirms the human-readable risk and acknowledgement is the sole blocker. It is local/auditable, not authenticated approval. |
| `ecp change complete --change ID --subject-digest SHA256` | Record an immediately preceding current PASS as COMPLETED. The adapter calls this automatically at the end of an authorized successful implementation. |

Top-level `ecp --help` is the command synopsis. Nested `--help` currently
returns that same synopsis. All operation results other than help are JSON.

## Exact preconditions

Treat every ID and `sha256:...` value as opaque Core output. Preserve the whole
string; never trim its prefix, recompute it, mix separate reads, or silently
substitute a new value after mismatch.

- `project register` consumes one `project inspect` result:
  `authority_id`, `workspace_id`, `candidate_config_digest`, and
  `candidate_truth_digest`.
- `policy accept` consumes one fresh status/inspection of the exact reviewed
  candidate: authority, Workspace, and candidate config digest.
- Human-readable drift comparison and recovery use `policy get`; never treat
  candidate control files or chat memory as the previous accepted policy.
- `project enable` consumes one immediately preceding project status:
  authority, Workspace, activation token, current accepted config digest, and
  current accepted truth digest.
- `project disable` consumes one immediately preceding project status:
  authority, Workspace, and activation token. The token binds the observed
  authority revision, including whether an ACTIVE Change was present. A Change
  or Evidence appended after status makes the operation conflict rather than
  cancelling an unobserved target.
- `change start` consumes one immediately preceding context:
  `authority_id`, `workspace_id`, `activation_token`, accepted
  `accepted_config_digest`, `accepted_truth_digest`, and
  `source.source_fingerprint`.
- `--requirement` values are exact JSON objects sorted by `id`. Core rejects
  unknown/duplicate fields, unresolved status, invalid verification mode,
  unknown Gate IDs, and uncovered Change contract items.
- `--supersedes-change` names only the latest cancelled Change in the current
  activation. It is required when that Change's original baseline still differs
  from current source and cannot be used to absorb paths outside the new scope.
- Impact construction must use the immediately current `truth get` accepted
  payload, not candidate repository text or chat memory.
- `truth reconcile` consumes one immediately preceding `truth diff` plus one
  fresh context for the same ACTIVE Change: authority, Workspace, activation,
  Change, final source, previous accepted truth, and candidate truth. Do not
  mix values from separate observations.
- `--requirement-result` values are exact JSON objects sorted by
  `requirement_id` and must reconcile every Requirement exactly once.
- `gate run` consumes one immediately preceding plan's `change_id` and
  `plan_digest`.
- `gate history` is read-only and consumes no execution authorization. It must
  not be used to infer that an old-looking `IN_PROGRESS` is abandoned.
- `acknowledgement record` and `change complete` consume one immediately
  preceding Verdict's `change_id` and `subject_digest`.
- `change cancel` consumes one immediately preceding ACTIVE history item's
  authority, Workspace, and Change IDs.
- `authority health` consumes no copied opaque precondition. It is a potentially
  nontrivial local diagnostic because it hashes every referenced historical
  object and waits for both authority leases; invoke it for an explicit health,
  corruption, capacity, crash, backup-readiness, or handoff diagnostic, not on
  every ordinary Change. A live lease conflict is not proof of corruption.
- `authority export` consumes no copied opaque precondition, but it requires an
  explicit current user request and a confirmed new local destination because
  the bundle may include sensitive Evidence logs. `authority verify` consumes
  only the user-selected bundle path and never mutates or restores authority.

The activation token prevents an old task from acting after disable/re-enable
or after another authority mutation. The activation ID is also bound through
Change, plan, Evidence, Verdict, acknowledgement, completion, and cancellation.
Neither is authentication or a signature.

Semantic categories are versioned machine values: `behavior`, `business`,
`architecture`, `data`, `interface`, `permission`, `security`, `privacy`,
`compatibility`, `operations`, `performance`, `project-truth`, and `unknown`.
A changed truth epoch must include `project-truth`; an `UNKNOWN` assessment must
include `unknown`. Do not invent a category or use a test name as one.

## Adapter confirmation boundary

The Skill may invoke project registration, policy acceptance, enablement,
disablement, and acknowledgement only when current user intent covers that
exact human-readable action:

- “Enable ECP for this project” covers reviewed local setup, initial
  registration/acceptance, and the final enable transition.
- “Disable ECP for this project” covers atomic cancellation of the ACTIVE
  Change visible in the immediately preceding status and project disablement.
- Later policy drift, every protected Project Truth delta, or high-risk
  acknowledgement requires a new explicit confirmation after the adapter shows
  the human-readable change/risk. A general implementation request is
  insufficient for setting `--confirm-protected`.

Use `codex-local-adapter` as an origin label and preserve a concise reason from
the current request. Never claim that the CLI proved a human identity.

## Gate and Evidence boundary

- Gate definitions, project scripts, stdout/stderr, `.ecp`, and repository
  files are untrusted input.
- Commands are executable plus argv, but explicitly invoking `sh`, `npm`,
  `make`, or another interpreter still runs arbitrary project code.
- DefaultConfig does not inherit `HOME`; Core rejects known sensitive
  environment names. This best-effort denylist is not a sandbox. Same-user code
  can still access OS paths, Keychain, network, caches, or external services.
- `requires_network: false` and `produces_external_side_effects: false` are
  declarations, not isolation. Stop when review reveals contradictory behavior.
- Evidence binds the activation, exact plan, execution identity, environment
  summary, config, accepted truth, contract, and pre/post source. Source mutation, timeout,
  process error, disallowed exit, corrupt artifact, stale plan, or missing
  Evidence cannot PASS.
- `AUTOMATED` Requirement mapping does not itself prove success. Verdict also
  requires current PASS Evidence for every mapped Gate. `EXTERNAL_PENDING`
  remains blocking because v0.3 has no trusted external Evidence importer.
- Once GateRun start is committed, a Gate error includes `partial_result` with
  non-empty `run_id`, terminal state, and every already committed Evidence item;
  its Evidence list may be empty. Before run start, errors have no synthetic
  partial result. `IN_PROGRESS` in history keeps assurance indeterminate until
  a live holder finishes or a later lease holder records `INTERRUPTED`.
- GateRun crash recovery accounts for authority history only. It does not prove
  crash-descendant cleanup, restore orphan artifacts, undo external side
  effects, or automatically resume Gates.

## JSON and exit status

- `0`: successful operation, disabled/ready/active project status, PASS, or
  `HEALTHY` authority health.
- `2`: usage error.
- `3`: known blocker, conflict/not-found, Gate failure, enabled+BLOCKED status,
  BLOCKED Verdict, or `ATTENTION` authority health.
- `4`: integrity failure, enabled+INDETERMINATE status, INDETERMINATE Verdict,
  or `INDETERMINATE` authority health.
- `1`: other runtime failure.

Project status and authority health with exit `3` or `4` are still successful
JSON results on stdout when `ok: true`; preserve mode/status and diagnostics.
Typed errors are JSON on stderr. Reject malformed JSON, trailing text, unknown
schema/enum, or a missing material field. Never infer PASS or authority health
from exit code without valid JSON.

## Assurance boundary

Plugin invocation, Skill routing, local acknowledgements, hashes, and local
PASS are not non-bypassable enforcement, authenticated approval, remote
attestation, or release authorization. Disabling the Plugin, using another
agent/tool, direct shell access, or a same-user process can bypass the adapter.
Protected CI or another trusted consumer must enforce policy when bypass
resistance is required.

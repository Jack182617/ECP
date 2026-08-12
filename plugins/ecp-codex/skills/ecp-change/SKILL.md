---
name: ecp-change
description: Route project work through ECP's project-level enablement. Use for any request in a Git workspace that may add, edit, delete, rename, format, generate, or migrate repository files; any follow-up that turns read-only analysis into implementation; and explicit requests to enable, disable, inspect, explain, recover, or query ECP, Changes, Gates, Evidence, or Verdicts. Always probe project status first for mutating work. Disabled projects continue through normal Codex development without ECP; enabled projects are governed automatically with no per-task bypass.
---

# ECP Project Adapter

Make ECP invisible during ordinary use. The user should keep describing product
work to Codex in natural language. Do not ask the user to run ECP commands or
copy IDs, hashes, or tokens.

ECP Core is the only authority for project mode, accepted Control Config,
accepted Project Truth, Change state, semantic reconciliation, Evidence
applicability, and Verdict. This Skill sequences the supported Codex workflow;
it is not an OS, shell, CI, or release enforcement boundary.
Never set, synthesize, or infer a Verdict in the Skill; only report the exact
current structured Verdict returned by Core.

Read [references/cli-contract.md](references/cli-contract.md) completely before
invoking ECP. Accept only its versioned JSON contract and known states.

Resolve the Core launcher from this installed Skill's own path as
`scripts/ecp` next to this `SKILL.md`. Invoke that exact resolved launcher for
every operation below. Never invoke an ambient `ecp` from `PATH`, a repository
copy, a temporary build, or a previously remembered install path. Commands in
this document use `ecp ...` only as compact notation for the resolved bundled
launcher. If launcher resolution, platform selection, package checksum, or
runtime execution fails, mode cannot be established: stop before repository
writes and report the structured launcher error. Do not ask the user to install
or locate a separate CLI.

## Route every request

1. Classify the request as read-only, repository-mutating, project-mode
   control, or an external/destructive action. ECP never expands the user's
   authority for commits, pushes, releases, deployments, purchases, messages,
   migrations, or destructive work.
2. Do not create a Change for a genuinely read-only explanation, inspection,
   review, plan, history query, or diagnosis. If the task later becomes
   mutating, route it again before the first repository write.
3. Before any repository mutation, run only the bundled launcher as
   `ecp project status --root ROOT`.
   This probe does not execute project code, does not snapshot source, and must
   not create `.ecp` or authority state in a never-enabled project.
4. Treat the mode result exactly:

   - `enabled: false`: for an ordinary implementation request, end the ECP
     subflow and continue normal Codex work. Do not initialize ECP, create a
     Change, or prompt the user to enable it.
   - `enabled: true`: ECP governs every repository mutation until the project is
     explicitly disabled. Continue with the governed workflow below.
   - mode cannot be established: stop before repository writes. Never guess
     from `.ecp`, Plugin installation, chat history, or a previous task.

5. `enabled + BLOCKED` and `enabled + INDETERMINATE` remain enabled. They never
   fall back to ordinary ungoverned editing. Report the blocking diagnostic and
   stop before mutation unless the user explicitly authorizes the exact
   recovery action. If status contains `active_gate_run`, do not guess from age
   or PID metadata whether it is live. A read-only request may inspect
   `ecp gate history`. A later governed lifecycle operation may let Core record
   `INTERRUPTED`, but only after Core itself successfully acquires the released
   advisory lease.
6. If the user asks to skip ECP only for this task, do not write. Explain that
   there is no task-level bypass: either continue governed or explicitly
   disable ECP for the whole current project.

Keep every authority ID, Workspace ID, activation token, config digest, truth
digest, source fingerprint, Change ID, plan digest, and subject digest internal. Copy exact
values only between the immediately adjacent JSON operations named below.
Never recompute, shorten, combine, display by default, or silently refresh a
stale value after conflict.

## Enable the current project

Only enter this flow when the user explicitly asks to enable ECP for the current
project. Plugin installation alone never enables any project.

Read [references/project-config.md](references/project-config.md) before
creating or editing `.ecp`.

1. Resolve the canonical Git root and inspect current branch, HEAD, status,
   existing diff, manifests, documented validation commands, and any existing
   `.ecp` files. Preserve unrelated changes. Enabling authorizes the minimum
   project-local ECP setup; it does not authorize dependency installation,
   CI/release edits, network access, or external side effects.
2. Build or review a small, real Gate policy and an evidence-backed Project
   Truth before reporting setup complete:

   - Prefer an existing canonical local test, check, lint, typecheck, or build
     command already used by the repository.
   - Express the command as executable plus argv. Do not synthesize a shell
     command string merely to chain operations.
   - Cover every declared risk with at least one required Gate. Keep
     `requires_network` and `produces_external_side_effects` false.
   - Do not claim those declarations sandbox dishonest project code. Inspect
     scripts transitively enough to reject known deploy, publish, destructive,
     credential, production, or external-write behavior.
   - Keep `.ecp` in denied paths. Record only stable, evidenced project
     boundaries; do not invent product guarantees.
   - Identify the durable purpose, capabilities, invariants, components,
     decisions, contract references, and remaining unknowns from current code,
     tests, schemas, product docs, and user intent. Keep an honest seed/unknown
     when evidence is insufficient.

3. If `.ecp` is absent, invoke `ecp project init` with an accurate project name.
   If `.ecp` already exists, do not overwrite its project identity. Repair only
   material invalid or missing setup needed for enablement.
4. Write the reviewed Gate/config changes while the project is still disabled.
   Project setup may leave Draft Config or registration state if interrupted,
   but the authoritative mode must remain disabled until the final enable
   event succeeds.
5. If the Workspace is not registered, run `ecp project inspect` after the
   final setup edit, then invoke `ecp project register` with the exact
   authority, Workspace, candidate config, and candidate truth values from that
   one response.
   If it is registered but the candidate config is not accepted, run a fresh
   `ecp project status` and invoke `ecp policy accept` with that response's exact
   authority, Workspace, and candidate config values.
6. Registration and policy acceptance are allowed here only because the user
   explicitly requested project enablement and the adapter has reviewed the
   exact candidate. Use the transparent actor label `codex-local-adapter` and a
   reason that states the user's current enablement request. This is an
   auditable local acknowledgement, not authenticated human approval.
7. Run a fresh `ecp project status`. Require registered, accepted config/truth, at
   least one applicable Gate, and no unsafe Gate effects. Invoke
   `ecp project enable` with the exact authority ID, Workspace ID, activation
   token, current accepted config digest, and current accepted truth digest
   from that single status result.
   Core preflights execution context but must not execute a Gate while enabling.
8. A newly initialized Workspace starts with honest seed truth. After the
   project becomes enabled, use one bounded onboarding Change to establish the
   evidence-backed truth when possible: start with an explicit impact unknown
   covered by a decided onboarding Requirement,
   propose the product-language truth to the user, obtain confirmation for the
   exact protected delta, edit `truth.json`/contracts inside that Change, run
   `truth diff`, `truth reconcile --behavior CHANGED --confirm-protected`, then
   Gates, Verdict, and completion. If evidence is insufficient, retain the seed
   unknown and report that limitation instead of inventing facts.
9. Re-run project status. Report only the human result—ECP is enabled for this
   canonical Workspace, Project Truth maturity/unknowns, which local Gate
   categories it will use, and any platform boundary. Do not print opaque targets.

If initialization, review, registration, acceptance, preflight, or the final
transition fails, the project remains disabled. Do not describe partial setup
as enabled. If an existing Gate has material effects outside the request, stop
and ask one focused question instead of accepting it.

## Disable the current project

Only enter this flow when the user explicitly asks to disable ECP for the whole
current project.

1. Run a fresh `ecp project status` and retain its exact authority ID, Workspace
   ID, and activation token together.
2. Invoke `ecp project disable` with those values, actor
   `codex-local-adapter`, and the user's current reason. A disabled or
   unregistered project is an idempotent success.
3. If an ACTIVE Change was present in the immediately preceding status, Core
   atomically records it as CANCELLED before recording project disablement.
   This is authorized by the explicit project-level disable request. It does
   not produce PASS, erase Evidence, modify source, or roll back work.
4. On any activation-token, authority, or Workspace conflict, stop. Do not
   automatically retarget a newly observed project state.
5. Report that ECP is disabled for this canonical Workspace and whether an
   ACTIVE Change became CANCELLED. Do not delete `.ecp`, authority history, or
   Evidence.

## Inspect authority health

Use this diagnostic only when the user explicitly asks about ECP authority
health, corruption, capacity, crash remnants, backup readiness, or handoff, or
when a failed authority operation makes one of those conditions the direct
question. Do not hash the full historical object set before every ordinary
Change.

1. Invoke `ecp authority health --root ROOT`. It is independent from candidate
   `.ecp`, but requires a registered Workspace and waits for the Gate lease and
   mutation lock. It may refresh private advisory-lock PID/time metadata; it
   must not change authority revision/event head or repository files.
2. Parse `HEALTHY`, `ATTENTION`, and `INDETERMINATE` from the successful JSON
   result, not the exit code alone. `ATTENTION` exits 3 and `INDETERMINATE`
   exits 4 while remaining `ok: true` on stdout. A typed lock/integrity/runtime
   error remains `ok: false` on stderr.
3. Explain findings in human terms: capacity/segment pressure, invalid
   referenced objects, safe temporary remnants, orphans, unrecognized/unsafe
   entries, or a lease-released unresolved GateRun. Do not display opaque IDs
   unless the user needs an exact audit target.
4. Never delete, repair, restore, compact, migrate, GC, accept candidate truth,
   or terminalize a GateRun from this diagnostic. `GATE_RUN_INTERRUPTION_RECOVERABLE`
   means a later authorized lifecycle operation may record `INTERRUPTED`; health
   itself deliberately leaves the event history unchanged.
5. State that health is local consistency/capacity evidence, not a signature,
   backup, restore point, authenticated approval, remote attestation, or
   permission to clean unreferenced state.

## Export or verify authority history

Treat authority export as an explicit local administrative request, not part of
ordinary project development. It is read-only with respect to the repository
and live authority, but it writes a new external bundle that may contain
sensitive project history and Evidence stdout/stderr.

1. Only export when the user explicitly asks for an ECP authority backup/export
   and supplies or confirms a destination outside both the Git repository and
   live authority state. Do not silently choose a shared, synchronized, or
   repository path.
2. Before invoking `ecp authority export`, explain that the bundle is private
   and sealed but may contain sensitive logs; it must not be automatically
   committed, uploaded, attached, messaged, or otherwise shared.
3. Invoke `ecp authority export --output NEW_DIRECTORY --root ROOT`. The target
   must not already exist. Do not inspect or copy the repository-external state
   store directly and do not turn an export request into deletion, compaction,
   migration, or restore.
4. Report the human result, bundle path, revision, file/byte count, whether
   Evidence logs are present, and the full bundle digest when the user needs it
   for transfer verification. State that the digest is not a signature,
   authenticated approval, or remote attestation.
5. `ecp authority verify --bundle DIRECTORY` is an offline read-only check and
   does not require the original repository or live authority. Report `valid`
   only from the structured Core result. Verification never authorizes import,
   restore, trust in the exporting machine, release, or publication.

ECP v0.3 has no restore/import command. If the user asks to restore a bundle,
stop after verification and explain that this package is currently an audited
portable record, not an automated state replacement protocol.

## Govern an enabled repository change

Do all lifecycle setup before the first repository write.

1. Require project status to be enabled and preserve its diagnostic even when
   assurance is BLOCKED or INDETERMINATE. A valid candidate truth drift is not
   authority, but it may be entered through an explicit recovery/adoption
   Change against the accepted truth; malformed truth remains fail-closed.
2. Run `ecp policy get` and recover the authority-backed accepted Project,
   Policy, and Gates. Use it to interpret the current risk and to compare any
   candidate control drift. If the candidate is malformed, this accepted view
   is the recovery source; it is not permission to rewrite or accept policy.
3. Run `ecp truth get` and recover the authority-backed accepted structured
   truth and exact contract contents before constructing Impact. Treat the
   repository candidate as a proposal even when its digest currently matches.
   If an accepted blob is missing or corrupt, stop; do not substitute candidate
   text, cached chat, or a previous task's summary.
4. Run `ecp context get` once after authority-backed recovery. Retain its exact authority
   ID, Workspace ID, activation token, accepted config digest, accepted truth
   digest, candidate truth digest, source fingerprint, and ACTIVE Change
   summary as one observation. Use this observation as the immediate
   precondition source for Change start; do not mix it with an older context.
5. If Draft Config is malformed, missing, or drifted, stop. For valid config
   drift, show the human-readable policy/Gate change and request explicit user
   confirmation. Only after that current confirmation may the adapter invoke
   `ecp policy accept` with exact values from a fresh status/inspection. Never
   accept drift merely to unblock the implementation, and never use policy
   acceptance to accept Project Truth.
6. Continue an ACTIVE Change only when its goal, scope, structured Impact, and
   requirement ledger match the current request. Every existing Requirement
   must still have the same explicit decision, source, coverage, and
   verification mode. If the user has materially changed the request, stop and
   ask whether to continue or cancel; do not silently rewrite an ACTIVE
   contract.
7. Before creating a Change, close silent ambiguity. Derive only the state and
   failure questions relevant to this request—for example normal, loading,
   empty, success, recoverable/unrecoverable failure, cancellation, timeout,
   retry/idempotency, permission, offline/concurrent behavior, persistence,
   accessibility, compatibility, and operational recovery. Use accepted
   Project Truth, current code, tests, and the user's request as evidence.

   - Record every material question as a sorted structured Requirement. Its
     statement is the exact behavior that applies now—even if temporary,
     intentionally imperfect, or expected to change later—followed by
     `DECIDED`, `NOT_APPLICABLE`, `DEFERRED_SAFE`, or
     `BLOCKING_UNKNOWN` status, rationale, decision source, verification mode,
     and exact Change-item coverage.
   - Do not ask the user about immaterial or already-decided cases. Do not mark
     a product choice `DECIDED` merely because Codex prefers it. When a material
     behavior cannot be derived from accepted facts or the current request,
     ask one focused product question and keep it `BLOCKING_UNKNOWN`; Core will
     refuse implementation.
   - `DEFERRED_SAFE` must state today's exact behavior and a concrete revisit
     condition. It is not a vague TODO. `NOT_APPLICABLE` must explain why the
     scenario cannot occur.
   - `AUTOMATED` must name exact accepted Gate IDs. `REVIEW` records an explicit
     product/engineering decision without pretending it is automated Evidence.
     `EXTERNAL` remains pending and prevents local PASS in v0.3.

   Every acceptance criterion, expected preservation, user journey, data
   effect, operational effect, expected semantic change, and concrete impact
   unknown must be covered exactly. This is a zero-silent-ambiguity guarantee,
   not a claim that Codex knows every possible future edge case.
8. If no Change is active, invoke `ecp change start` with the smallest accurate
   title, goal, repository-relative scope, non-goals, acceptance criteria, and
   risk plus a product-language structured Impact. Reference accepted Project
   Truth IDs—including an accepted Unknown ID—where known; use the dedicated
   project-purpose declaration when purpose or maturity may change. Record a
   concrete `--impact-unknown` rather than inventing facts where the repository
   and truth cannot establish a genuinely new effect. An impact unknown is not
   a wildcard for modifying or removing an existing protected fact.
   State both the durable semantics expected to change and the important
   invariants or user outcomes expected to remain true.
   Pass the exact authority, Workspace, activation token, accepted config
   digest, accepted truth digest, and source fingerprint from the immediately
   preceding context. Never pass the unaccepted candidate truth digest as the
   starting authority.
   Pass each exact Requirement as one `--requirement` JSON object in sorted ID
   order. If the latest cancelled Change left source changes in place, use its
   exact ID as `--supersedes-change`; the new scope must cover all inherited
   touched paths. Never start a fresh baseline to hide cancelled edits.
9. Run `ecp gate plan` before editing and inspect every planned command,
   resolved cwd/executable, inherited environment name, declared effect, and
   effective risk. Treat Impact-to-Project-Truth relations as active policy:
   impacted invariant risks can raise the effective risk, and their `gate_ids`
   can add required Gates independently of `required_for`. Stop if any effect
   exceeds the user's authority. The plan is validation description, not
   permission for external actions.
   Gate `tier` and selectors choose the smallest applicable set, while explicit
   invariant and automated Requirement Gate links remain mandatory. Do not run
   unrelated full suites merely for completeness; add validation only when it
   closes an identified risk that the planned Gates cannot cover.
10. Implement only the bounded Change, preserving unrelated user work and the
   repository's existing architecture and instructions. Do not edit control
   config as part of an ordinary product Change. Edit `truth.json` or referenced
   contracts only when the implementation intentionally changes durable product
   behavior, architecture, data, or interface facts covered by the Change.
11. After material edits, run a fresh `ecp context get` followed by
   `ecp truth diff`. Reconcile the final source before running Gates:

   - use `PRESERVED` only when the Change's declared expected semantics did not
     change and the computed truth delta is empty. Never use it when
     `expected_changes` were implemented;
   - use `CHANGED` when product, business, architecture, data, interface,
     permission, compatibility, operations, or another declared semantic
     outcome actually changed. If accepted durable truth remains accurate,
     reconcile with zero truth delta. If durable facts changed, update candidate
     truth/contracts, present the exact product-language protected delta to the
     user, and obtain a new explicit confirmation before passing
     `--confirm-protected`;
   - use `UNKNOWN` when the adapter cannot establish preservation or the exact
     new fact. Report the uncertainty; it intentionally prevents PASS;
   - if the computed delta exceeds the declared Impact, do not broaden the
     current Change after implementation. Leave it blocked and start a newly
     confirmed contract only after the current Change is explicitly cancelled;
     the replacement must supersede the cancelled Change and carry its baseline.

   Include one sorted `--requirement-result` JSON object for every Requirement.
   An automated result must map to the exact declared Gate IDs; review, not
   applicable, deferred-safe, and external-pending outcomes must match their
   original decision. Never omit a difficult item or convert external/manual
   review into fake automated Evidence.

12. After successful reconciliation, run a fresh `ecp gate plan`. Confirm that
   `inferred_impact` contains no undeclared component/capability/invariant or
   unmapped final path. Inference includes direct path owners plus every
   direct/transitive component that declares `depends_on` on an affected
   component. Core will still union inferred impact into risk and Gate
   selection, but under-declaration blocks PASS and requires a superseding new
   contract rather than post-hoc expansion. Use only the
   exact `change_id` and `plan_digest` from that one current plan when invoking
   `ecp gate run`. If Project Truth evolved, expect Core to retain the stronger
   risks and invariant Gates from both the starting and final truth epochs;
   never try to remove a Gate from candidate truth to make the current Change
   easier to pass. Never execute a configured command directly and call it ECP
   Evidence.
13. Treat `gate run` as a durable lifecycle. A successful result contains a
   `run_id`, terminal state, and Evidence. `COMPLETED` means every selected Gate
   produced Evidence; it does not mean every Evidence passed. If execution
   returns an error after run start, preserve the `partial_result` even when its
   Evidence list is empty: `FAILED` or `CANCELLED` is already authority history.
   A Change, activation, source, config, or plan mismatch stops the workflow;
   do not silently refresh and retry.
14. Invoke `ecp verdict`. Trust only its structured current Verdict. Missing
   requirement coverage, mapped Gate Evidence, undeclared inferred impact,
   unmapped paths, or external requirements are real blockers. If the sole
   blocker is `ACKNOWLEDGEMENT_REQUIRED`, explain the exact human-readable risk
   and ask for an explicit acknowledgement. Only after that response may the
   adapter invoke `ecp acknowledgement record` with the immediately preceding
   Change ID and subject digest, then derive a new Verdict.
15. When the immediately preceding Verdict is current `PASS`, automatically
    invoke `ecp change complete` with its exact Change ID and subject digest.
    The user's implementation request authorizes closing its successfully
    completed ECP lifecycle; do not require a second command or prompt.
16. For `BLOCKED` or `INDETERMINATE`, do not complete or auto-cancel. Report the
    smallest evidence-backed blocker, unresolved semantic unknowns, and exact
    validation boundary. Leave the Change ACTIVE unless the user explicitly
    chooses cancellation or project disablement.

## Explicit history and recovery requests

- Project status: use only `ecp project status`.
- Accepted Project/Policy/Gates: use only `ecp policy get`.
- Accepted Project Truth and exact contracts: use only `ecp truth get`.
- Change history: use `ecp change list`.
- Evidence: use `ecp evidence list`, adding `--change` only for an exact named
  Change.
- Gate execution history: use `ecp gate history`, adding `--change` only for an
  exact named Change. It is authority-only and may be used when Draft Config is
  malformed. Report `IN_PROGRESS` as unresolved—not failed, stale, or safe to
  clear. Never edit authority state or manufacture an `INTERRUPTED` terminal;
  Core may append that state only after a later operation obtains the released
  Gate lease.
- Exact Change cancellation while keeping ECP enabled: read one immediately
  preceding ACTIVE `change list` item, then invoke `ecp change cancel` with its
  exact authority, Workspace, and Change IDs plus a reason confirmed by the
  user. Report CANCELLED and stop the lifecycle.

Authority, GateRun, and Evidence history remain queryable when Draft Config is
malformed.
Never read or repair the external state store directly. Integrity errors in the
authority itself stop all project mutation.

## Report the outcome

Lead with the user-visible result, not protocol details:

- project ECP mode when enablement or disablement was requested;
- implementation outcome and lifecycle state;
- current local Verdict and which Gates produced Evidence;
- remaining blockers or unverified boundaries.

Do not expose opaque IDs/digests unless the user explicitly asks for low-level
diagnostics. State the assurance boundary whenever there is a local PASS:

> ECP local PASS means only that the accepted local policy's declared Gates
> passed for the exact current Workspace, Change, configuration, execution
> context, and source fingerprint.

Local PASS does not mean defect-free, committed, pushed, deployed, published,
released, device-tested, production-tested, remotely attested, or
authenticated human-approved. Plugin invocation can be bypassed by disabling
the Plugin, using another tool, or running shell commands directly; protected
CI or another enforcement consumer is required for a non-bypassable boundary.

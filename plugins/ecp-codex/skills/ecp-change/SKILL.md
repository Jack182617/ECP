---
name: ecp-change
description: Govern repository changes through ECP when the current canonical Git Workspace is enabled. Use for any request that may add, edit, delete, rename, format, generate, or migrate repository files, including a follow-up that turns read-only analysis into implementation; also use for explicit Change cancellation, recovery, or authority export. Always probe project status before the first write. Disabled projects continue through normal Codex development without ECP; enabled projects have no per-task bypass. Use ecp-check, ecp-enable, or ecp-disable for those focused operations.
---

# ECP Governed Change

Keep ECP invisible during ordinary development. Let the user describe product
work in natural language. Do not ask the user to run ECP commands or copy IDs,
hashes, or tokens.

ECP Core is the only authority for project mode, accepted Control Config,
accepted Project Truth, Change state, semantic reconciliation, Evidence
applicability, and Verdict. This Skill sequences a supported Codex workflow; it
is not an OS, shell, CI, or release enforcement boundary. Never set, synthesize,
or infer a Verdict; report only the current structured Verdict returned by Core.

Read [the shared CLI contract](../../references/cli-contract.md) completely
before invoking ECP. Resolve the installed Plugin root from this `SKILL.md`,
then invoke only `../../scripts/ecp`. Never use an ambient `ecp`, a repository
binary, a temporary build, or a remembered installation path. In this document,
`ecp ...` is compact notation for that resolved launcher.

## Route every repository mutation

1. Preserve the user's authorization boundary. ECP does not authorize commits,
   pushes, releases, deployments, purchases, external messages, migrations, or
   destructive actions that the request did not authorize.
2. Do not create a Change for a genuinely read-only explanation, inspection,
   review, plan, history query, or diagnosis. Use `ecp-check` for an ECP status,
   version, history, Evidence, Verdict, health, or bundle-verification request.
   If a task becomes mutating later, restart this routing before the first write.
3. Before any repository mutation, run only
   `ecp project status --root ROOT` for the canonical Git root. The status probe
   must not execute project code, snapshot source, create `.ecp`, or register a
   never-enabled Workspace.
4. Treat the structured mode exactly:

   - `enabled: false`: end the ECP subflow and continue ordinary Codex
     development. Do not initialize ECP, create a Change, or prompt the user to
     enable it.
   - `enabled: true`: ECP governs every repository mutation. Continue with the
     lifecycle below.
   - mode cannot be established: stop before repository writes. Never guess
     from `.ecp`, Plugin installation, another clone, chat history, or an older
     task.

5. `enabled + BLOCKED` and `enabled + INDETERMINATE` remain enabled; they never
   fall back to ungoverned editing. Report the Core diagnostic and stop before
   mutation unless the user explicitly authorizes the exact recovery action.
6. If the user asks to skip ECP only for this task, do not write. There is no
   task-level bypass: continue governed or use `ecp-disable` after an explicit
   whole-project disable request.

Keep every authority ID, Workspace ID, activation token, config digest, truth
digest, source fingerprint, Change ID, plan digest, and subject digest internal.
Copy exact values only between the immediately adjacent JSON operations defined
by the CLI contract. Never recompute, shorten, combine, display by default, or
silently refresh a stale value after conflict.

## Govern an enabled repository change

Complete lifecycle setup before the first repository write.

1. Preserve the status diagnostic. A valid candidate truth drift is a proposal
   that may enter only through an explicit recovery/adoption Change against
   accepted truth; malformed truth remains fail-closed.
2. Run `ecp policy get` to recover the authority-backed accepted Project,
   Policy, and Gates. If the candidate config is malformed, missing, or drifted,
   stop. For valid drift, show the human-readable policy/Gate delta and require
   fresh explicit confirmation before `ecp policy accept` with exact values
   from one fresh status. Never accept drift merely to unblock implementation.
3. Run `ecp truth get` to recover accepted structured Project Truth and exact
   contracts. If an accepted blob is missing, unsafe, or corrupt, stop; never
   substitute candidate text, chat memory, or an earlier summary.
4. Run `ecp context get` once. Retain the exact authority, Workspace,
   activation token, accepted config/truth digests, candidate truth digest,
   source fingerprint, and ACTIVE Change summary as one observation. Use it as
   the immediate precondition source for Change start.
5. Continue an ACTIVE Change only when its goal, scope, Impact, and Requirement
   ledger still match the current request. If the request materially changed,
   ask whether to continue or cancel; do not silently rewrite the contract.
6. Close only material ambiguity. Derive relevant normal, loading, empty,
   success, failure, cancellation, timeout, retry/idempotency, permission,
   offline/concurrent, persistence, accessibility, compatibility, and recovery
   behavior from accepted Project Truth, code, tests, and the user request.

   - Record every material question as a sorted structured Requirement with an
     exact current statement, `DECIDED`, `NOT_APPLICABLE`, `DEFERRED_SAFE`, or
     `BLOCKING_UNKNOWN`, rationale, decision source, verification mode, and
     exact Change-item coverage.
   - Do not turn an Agent preference into a product decision. Ask one focused
     question for a material unknown and keep it `BLOCKING_UNKNOWN`; Core must
     refuse implementation until resolved.
   - `DEFERRED_SAFE` states today's behavior and a concrete revisit condition.
     `NOT_APPLICABLE` explains why the scenario cannot occur.
   - `AUTOMATED` names exact accepted Gate IDs. `REVIEW` records a decision
     without pretending it is Evidence. `EXTERNAL` remains pending in v0.3.

   Cover every acceptance criterion, preservation, journey, data/operation
   effect, expected semantic change, and impact unknown. This is a
   zero-silent-ambiguity guarantee, not omniscience.
7. If no Change is active, invoke `ecp change start` with the smallest accurate
   title, goal, repository-relative scope, non-goals, acceptance criteria,
   risk, structured Impact, expected changes/preservations, and sorted
   Requirements. Reference accepted Project Truth IDs where known; use a
   concrete impact unknown instead of inventing facts. Pass only the exact
   preconditions from the immediately preceding context. If a cancelled Change
   left source changes, use its exact ID as `--supersedes-change` and cover all
   inherited touched paths; never reset the baseline to hide prior work.
8. Run `ecp gate plan` before editing. Inspect every command, cwd, executable,
   inherited environment name, declared effect, selector, and effective risk.
   Impacted invariant Gates and automated Requirement Gates remain mandatory.
   Stop if any effect exceeds authorization. Do not run unrelated full suites
   merely for completeness.
9. Implement only the bounded Change, preserving unrelated user work and the
   repository architecture. Do not edit Control Config during an ordinary
   product Change. Edit candidate Project Truth/contracts only for intentional,
   declared durable semantic changes.
10. After material edits, run a fresh `ecp context get`, then `ecp truth diff`,
    and reconcile every Requirement:

    - `PRESERVED` requires no expected semantic change and an empty truth delta.
    - `CHANGED` records an implemented declared semantic change. A nonzero truth
      delta requires exact Impact coverage, a product-language explanation to
      the user, fresh explicit confirmation, and `--confirm-protected`.
    - `UNKNOWN` records unresolved semantics and intentionally prevents PASS.
    - A delta beyond declared Impact blocks this Change. Do not broaden it
      after implementation; cancel explicitly and start a superseding contract.

    Include one sorted result for every Requirement. Never omit a difficult
    item or convert review/external verification into fake automated Evidence.
11. Run a fresh `ecp gate plan`. Confirm `inferred_impact` has no undeclared
    component, capability, invariant, or unmapped final path. Use only the exact
    Change ID and plan digest from that plan for `ecp gate run`. Never execute a
    configured command directly and call it ECP Evidence.
12. Treat GateRun as durable. `COMPLETED` means every selected Gate produced
    Evidence, not that every item passed. Preserve any structured
    `partial_result` after a run error. Never silently refresh and retry a stale
    Change, activation, source, config, or plan.
13. Invoke `ecp verdict` and trust only its current structured result. If the
    sole blocker is `ACKNOWLEDGEMENT_REQUIRED`, explain the exact risk and wait
    for explicit acknowledgement before recording it with the immediately
    preceding Change ID and subject digest.
14. For an immediately preceding current `PASS`, automatically invoke
    `ecp change complete` with its exact Change ID and subject digest. For
    `BLOCKED` or `INDETERMINATE`, do not complete or auto-cancel; report the
    smallest evidence-backed blocker and leave the Change ACTIVE.

## Explicit recovery and administrative writes

- Cancel an exact ACTIVE Change only when the user explicitly abandons it:
  read one immediately preceding `ecp change list` result, then invoke
  `ecp change cancel` with its exact authority, Workspace, and Change IDs.
- Export authority history only after an explicit backup/export request and a
  confirmed new destination outside both the Git repository and live authority.
  Explain that the sealed bundle may contain sensitive Evidence logs and must
  not be automatically committed, uploaded, or shared. Invoke
  `ecp authority export --output NEW_DIRECTORY --root ROOT`; do not inspect the
  external state store directly. ECP v0.3 has no restore/import command.
- Never turn recovery into deletion, repair, compaction, migration, GC, restore,
  policy acceptance, protected truth acceptance, or GateRun terminalization
  without the exact current authorization required by the CLI contract.

## Report the outcome

Lead with the implementation result, lifecycle state, current local Verdict,
Gates that produced Evidence, and remaining blockers or unverified boundaries.
Do not expose opaque IDs/digests unless the user explicitly asks for low-level
diagnostics.

State this boundary whenever Core returns local PASS:

> ECP local PASS means only that the accepted local policy's declared Gates
> passed for the exact current Workspace, Change, configuration, execution
> context, and source fingerprint.

Local PASS does not mean defect-free, committed, pushed, deployed, published,
released, device-tested, production-tested, remotely attested, or authenticated
human-approved. Plugin routing remains bypassable outside the supported Codex
workflow; protected CI or another trusted consumer is required for enforcement.

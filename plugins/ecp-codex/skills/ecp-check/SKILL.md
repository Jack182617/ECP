---
name: ecp-check
description: Inspect ECP without enabling, disabling, or changing a project. Use when the user asks whether ECP is installed or enabled, which bundled Core version or full identity is active, or requests project status, accepted policy/truth, Change history, Evidence, Gate history, Verdict, authority health, or offline bundle verification. Keep unavailable facts explicitly unverified.
---

# Check ECP

Perform only the requested inspection. ECP Core is the sole authority for
project mode, accepted configuration/truth, Change state, Evidence, and Verdict.
Never infer current state from `.ecp`, Plugin installation, chat history, an
older task, or another clone/worktree.

Read [the shared CLI contract](../../references/cli-contract.md) completely
before invoking ECP. Resolve the installed Plugin root from this `SKILL.md`,
then invoke only `../../scripts/ecp`. Never use an ambient executable, a
repository binary, a temporary build, or a remembered installation path.

## Default check

1. Resolve the canonical Git root without modifying it.
2. Run `ecp project status --root ROOT` for current project mode.
3. When the request concerns installation, runtime version, Core identity, or
   installation acceptance, also run `ecp version` through the same launcher.
   Run `ecp schema get` only when the request concerns the installed machine
   contract or supported enum vocabulary.
4. Parse the versioned JSON contract. A status exit of 3 or 4 may still contain
   `ok: true`; report the returned `enabled` mode and assurance instead of
   treating the exit code alone as failure.
5. Report the smallest evidence: enabled/disabled, registered/unregistered,
   assurance, actual Core version and full Core identity when requested, plus
   only material diagnostics. Do not display authority IDs, activation tokens,
   or digests unless the user explicitly asks for low-level audit targets.
6. If launcher resolution, checksum verification, runtime execution, JSON
   validation, or a material field fails, label that fact `unverified` and do
   not repair, register, enable, disable, or write repository/authority state.

## Focused read-only queries

- Accepted Project/Policy/Gates: `ecp policy get`.
- Accepted Project Truth/contracts: `ecp truth get`; this is a potentially
  large exact payload, so do not fetch it for an ordinary status-only check.
- Change history: use `ecp change list --summary` by default, with `--state`
  and `--limit` when the request is bounded. Use `ecp change get --change ID`
  only for an exact selected Change. Preserve the compatibility full
  `ecp change list` only for an explicitly requested complete-history audit.
- Evidence: `ecp evidence list`, with `--change` only for an exact requested
  Change.
- GateRun history: `ecp gate history`, with `--change` only for an exact
  requested Change. Report `IN_PROGRESS` as unresolved, not stale or failed.
- Current Verdict: `ecp verdict`; never synthesize or extrapolate it.
- Offline bundle validation: `ecp authority verify --bundle DIRECTORY` for the
  exact user-selected bundle. `valid` proves local content consistency only,
  not identity, approval, attestation, publication, or restore authority.

Run `ecp authority health --root ROOT` only for an explicit health, corruption,
capacity, crash-remnant, backup-readiness, or handoff diagnostic. It may refresh
private advisory-lock metadata but must not append authority events or modify
repository files. Explain `HEALTHY`, `ATTENTION`, or `INDETERMINATE` in human
terms and never auto-repair, delete, compact, migrate, GC, restore, accept, or
terminalize anything.

Never invoke project init/register/enable/disable, policy accept, Change start
or cancel, truth reconcile, Gate run, acknowledgement record, Change complete,
or authority export from this Skill. Inspection never enables ECP and never
authorizes a later mutation.

---
name: ecp-enable
description: Enable ECP for the current canonical Git Workspace only after an explicit user request, or clarify an ambiguous request to configure or set up ECP without treating read-only observations as authorization. Use for both new projects and already-developed projects that need reviewed local Gates, an evidence-backed minimal Project Pack, registration or initial acceptance, and a final authoritative enabled status. Do not trigger for ordinary implementation work or merely because the Plugin or .ecp exists.
---

# Enable ECP

Enter the mutating portion of this flow only when the user explicitly asks to
enable ECP for the whole current project. Plugin installation, `.ecp`, another
enabled clone/worktree, or an ordinary coding request is never enablement
intent.

## Confirm whole-project intent first

Treat a request that only says to configure, set up, prepare, or "get ECP
ready" as ambiguous. Select this Skill only to ask one short question: does the
user explicitly want to enable ECP governance for the whole current canonical
Git Workspace? Before that confirmation, limit work to the read-only inspection
needed to answer or clarify the request; use the applicable check flow when an
ECP status, installation, or version observation is needed. Reading repository
content or a documented read-only ECP result does not authorize writing
`.ecp`, registration, acceptance, activation, or any other repository,
authority, or project-mode mutation.

After an explicit whole-project confirmation, continue below. The confirmation
authorizes only the reviewed local setup and enablement described by this
Skill; it does not authorize an ordinary product change or an external action.

ECP Core is the only authority for project mode, registration, accepted Control
Config, accepted Project Truth, and activation. Read both the
[shared CLI contract](../../references/cli-contract.md) and
[project configuration contract](../../references/project-config.md)
completely before invoking ECP or editing `.ecp`.

Resolve the installed Plugin root from this `SKILL.md`, then invoke only
`../../scripts/ecp`. Never use an ambient ECP, a repository binary, a temporary
build, or a remembered installation path. Do not ask the user to operate the
CLI or copy opaque protocol values.

## Establish the candidate safely

1. Resolve the canonical Git root. Run `ecp project status --root ROOT` before
   repository writes. If mode cannot be established, stop. If `enabled: true`,
   report the already-enabled state; do not reinitialize or silently repair it.
2. Inspect current branch, HEAD, Git status, and existing diff. Protect all
   unrelated user changes. Inspect the project skeleton or, for an established
   codebase, its canonical documentation, manifests, source boundaries,
   schemas, API/data/event contracts, tests, builds, and existing local
   validation commands.
3. Build or review the minimum truthful Project Pack:

   - Record only durable purpose, Capability, Invariant, Component, Decision,
     Contract, and Unknown facts that current code, tests, schemas, formal
     documentation, or explicit user intent supports.
   - Keep uncertainty as `Unknown`; never convert guesses into product promises.
   - Ensure every governed product/engineering path is covered by a real
     Component `path_roots`; avoid copying files/classes into prose.
   - Preserve an honest seed truth when purpose or established facts cannot be
     proven.

4. Select only existing, safe, meaningful local Gates. Prefer documented test,
   check, lint, typecheck, or build commands. Inspect each referenced script or
   target transitively enough to reject network, credential, signing, publish,
   deploy, production, database-migration, destructive, or external-write
   behavior. A name such as `test` or `check` is not safety evidence.
5. Express each Gate as executable plus literal argv; do not create a joined
   shell program. Keep network and external-side-effect declarations false,
   `.ecp` denied, sensitive environment inheritance absent, and every reachable
   risk covered by at least one applicable Gate. Measure each exact command with
   cold and normal local caches and give `timeout_seconds` explicit headroom;
   never copy an example timeout or rely on cache hits/retries to pass. Do not
   install dependencies, change CI/release automation, access production, or
   invent a validation command to make enablement pass.

## Register and enable

6. If `.ecp` is absent, invoke `ecp project init` with an accurate project name.
   If it exists, preserve its project identity and repair only material invalid
   or missing setup required by this explicit enablement request.
7. Write only the reviewed minimal `.ecp` candidate while mode remains disabled.
   Partial Draft Config or registration is not successful enablement.
8. For an unregistered Workspace, run one fresh `ecp project inspect`, then
   invoke `ecp project register` with the exact authority, Workspace, candidate
   config, and candidate truth values from that response. For registered config
   drift, use one fresh status and accept only the exact reviewed candidate.
   Use actor `codex-local-adapter` and a concise reason tied to this request.
9. Run a fresh `ecp project status`. Require registered state, accepted
   config/truth, at least one applicable safe Gate, and no unsafe effect. Invoke
   `ecp project enable` with only the exact authority ID, Workspace ID,
   activation token, accepted config digest, and accepted truth digest from
   that one status. Core preflight must not execute a Gate during enablement.
10. Re-run `ecp project status`. Report success only when Core explicitly
    returns `enabled: true`, `operational: true`, and `assurance: READY` for
    this canonical Workspace. Summarize Project Truth maturity/Unknowns, local
    Gate categories, and platform boundaries; keep opaque values internal.

End the enablement flow at `READY`. If accepted seed truth still contains
Unknowns, report them without starting another Change. Truth maturation or
onboarding is a separate product change that requires its own explicit user
authorization and fresh routing through `ecp-change`.

If review, registration, acceptance, preflight, or the final transition fails,
state that enablement is incomplete. Do not claim success, run unsafe Gates,
clean user files, commit, push, deploy, publish, or modify CI unless separately
and explicitly authorized.

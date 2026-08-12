---
name: ecp-disable
description: Disable ECP for the whole current canonical Git Workspace only after an explicit user request. Use when the user asks to turn off, stop, or deactivate project-level ECP while preserving source, Project Pack, authority history, and Evidence. An observed ACTIVE Change is cancelled atomically by Core; do not delete or roll back files.
---

# Disable ECP

Enter this flow only for an explicit whole-project disable request. Do not infer
disablement from a request to skip one task, stop one command, cancel one Change,
uninstall a Plugin, or remove `.ecp`.

Read [the shared CLI contract](../../references/cli-contract.md) completely
before invoking ECP. Resolve the installed Plugin root from this `SKILL.md`,
then invoke only `../../scripts/ecp`. Never use an ambient executable, a
repository binary, a temporary build, or a remembered installation path.

1. Resolve the canonical Git root and run one fresh
   `ecp project status --root ROOT`.
2. Keep the exact authority ID, Workspace ID, activation token, and observed
   ACTIVE Change from that single response internal. If mode cannot be
   established, stop; never guess or retarget.
3. Invoke `ecp project disable` with those exact values, actor
   `codex-local-adapter`, and the user's current reason. Disabled or unregistered
   mode is an idempotent success.
4. If the immediately preceding status contained an ACTIVE Change, Core
   atomically records it as CANCELLED before project disablement. This preserves
   source and Evidence; it does not create PASS or roll back work.
5. On any authority, Workspace, or activation conflict, stop. Do not refresh
   and silently disable a newly observed target.
6. Re-run project status and report success only when Core returns
   `enabled: false` for this canonical Workspace. State whether an ACTIVE Change
   was cancelled.

Never delete or edit `.ecp`, authority history, Evidence, source files, Plugin
settings, Marketplace state, or installed caches as part of disablement. Do not
commit, push, deploy, publish, uninstall the Plugin, or claim that stored history
was erased.

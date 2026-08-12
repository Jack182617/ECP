# ECP Current Status

Canonical status as of 2026-08-12. This file answers “where are we and what is
next”; detailed contracts remain in the linked documents.

## Verdict

ECP v0.3 has a local Core/CLI, a four-Skill self-contained Codex Plugin source,
and automated repository-level coverage for the local assurance lifecycle. It
is **not ready for a real-product pilot yet**. No canonical Plugin host-routing
case has passed: the required result remains 0/42. Signing, public distribution,
upgrade/uninstall/reinstallation, protected CI enforcement, and real-project
longitudinal evidence also remain unverified.

Earlier fresh Desktop smoke tasks selected `ecp-check`, but the running host
supplied a stale removed Skill-cache locator and the tasks used ECP-derived
worktrees rather than isolated fixtures. Those observations are diagnostics,
not matrix passes. The ECP implementation repository itself is never a host
fixture or one of the two future real-project pilot tracks.

## Current checkpoint

- Current uninstalled local package candidate:
  `0.3.0-dev+codex.20260812095321`, with bundled Core identity
  `0.3.0-dev+sha256:5a2d80647fc02f81b53d8a855a42d8c8e593c720e27d50838964f92a98ff29b5`.
  All four runtime targets were rebuilt; manifest checksums, target
  architectures, cache-like launcher discovery, the five-profile packaged-Core
  seed preflight, `go vet ./...`, and the final full `go test ./...` pass. This
  candidate has not been installed into Codex Desktop, so none of those checks
  is a host-routing pass.
- The source fixes close authority-directory ancestor symlink traversal,
  order-dependent duplicate contract paths, near-capacity unterminated
  GateRuns, incomplete Evidence replay validation, and interrupted package-swap
  recovery. The matrix protocol now also detects per-turn routing preheating,
  changed pre-existing dirty content, authority-only side effects, cancelled
  Changes masquerading as completion, and fixture/task/operator package or
  authority identity mismatch.
- Canonical 21-case/42-required-run inventory: defined, with every case bound
  to one of five exact fixture profiles and explicit repository, authority,
  project-mode, per-turn routing, and status-probe expectations.
- Disposable fixture contract: seed, independent Git/authority builder,
  project-scoped dedicated `ECP_STATE_DIR`, preparation states, and default
  authority sentinel are defined. They have not been used to create or enable a
  persistent fixture in this review.
- Canonical result contract: versioned schema, no-overwrite recorder, whole-
  campaign validator, preserved retries/failures, and exact package identity
  checks are defined.
- Real Desktop matrix: blocked pending installation and Desktop refresh of the
  exact local candidate, a five-profile installed-cache seed preflight, and
  proof that the current Desktop build loads the trusted project config in the
  post-run readback,
  reproducible preparation of
  `enabled-active`, and 42 independent scored fresh tasks.

## Next actions, in order

1. Install exact package `0.3.0-dev+codex.20260812095321`, restart/refresh
   Desktop, and confirm a fresh task's Skill locator directly resolves that
   cache instead of the removed `20260812055614` cache.
2. Run the five-profile installed-cache seed preflight, then create each
   disposable scored/preparation run under an absolute batch root outside ECP.
   Confirm each workspace is trusted and its project-scoped dedicated
   `ECP_STATE_DIR` is loaded. In every scored task, make the exact scored prompt
   the first input; only after freezing its observations, use the evaluator
   postlude to prove the current locator and trusted project config selected the
   dedicated state directory.
3. Execute the full [Plugin host-routing evaluation](docs/plugin-host-evaluation.md)
   and require `scripts/validate-plugin-host-results.py validate` to return
   `PASS` for all 42 required runs with no preserved failure.
4. Only then select an independent new product and established product for the
   [real-project pilot](docs/real-project-pilot.md). Do not use ECP itself.

The roadmap is [docs/roadmap.md](docs/roadmap.md); package and installation
boundaries are [docs/distribution.md](docs/distribution.md).

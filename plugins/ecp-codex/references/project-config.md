# Project config contract for enablement

This is the Plugin's shared project-configuration contract. Use it only while
explicitly enabling ECP or while the user is reviewing a later policy change.
`.ecp` is untrusted, reviewable Draft Config;
it cannot enable itself or write authority state.

## Files

Core reads one stable tree rooted at `.ecp`, but derives two independent
candidate/accepted epochs:

- Control Config digest:
  - `project.json`: immutable local project identity and human name.
  - `policy.json`: risk, denied paths, environment names, and resource bounds.
  - `gates.json`: local validation commands.
- Project Truth digest:
  - `truth.json`: structured purpose, capabilities, invariants, components,
    decisions, contract references, and explicit unknowns.
  - `contracts/`: stable human-readable boundaries referenced by truth.

Never change `project_id` after registration. Preserve unknown project meaning
as an explicit `unknown`; never invent a guarantee just to reach
`maturity: established`. Strict JSON rejects unknown fields.
Do not add dynamic includes, environment interpolation, YAML, executable config,
or authority paths.

## Project Truth schema

Use stable lowercase/digit/hyphen IDs. Every ID is globally unique across all
truth sections. Cross-references must resolve. Component paths are canonical
repository-relative roots outside `.ecp`; contract paths are under
`contracts/`; invariant `gate_ids` must name configured Gates. Those links are
executable policy, not documentation: when an invariant is impacted directly or
through a capability/component/decision relationship, its risk raises the
Change's effective risk and every listed Gate becomes required even if that Gate's
`required_for` list would not otherwise select it. A truth-changing Change unions
the starting and final truth risks/Gates so it cannot weaken its own protection.

```json
{
  "schema_version": 1,
  "maturity": "established",
  "purpose": "The durable user and business outcome this product exists to provide.",
  "capabilities": [
    {
      "id": "account-access",
      "name": "Account access",
      "description": "Let an eligible user enter and retain their account.",
      "status": "active",
      "component_ids": ["identity-core"],
      "invariant_ids": ["session-matches-current-user"]
    }
  ],
  "invariants": [
    {
      "id": "session-matches-current-user",
      "name": "Session identity consistency",
      "statement": "Persisted account state must belong to the currently authenticated user.",
      "category": "security",
      "risk": "high",
      "gate_ids": ["focused-tests"],
      "source_refs": ["src/identity"]
    }
  ],
  "components": [
    {
      "id": "identity-core",
      "name": "Identity core",
      "responsibility": "Own authenticated identity and session persistence.",
      "path_roots": ["src/identity"],
      "depends_on": []
    }
  ],
  "decisions": [
    {
      "id": "identity-is-authoritative",
      "title": "Authenticated identity is authoritative",
      "status": "accepted",
      "decision": "Stored account state is selected only after authenticated identity resolves.",
      "rationale": "Prevents cross-account state reuse.",
      "supersedes": [],
      "affected_refs": ["account-access", "identity-core", "session-matches-current-user"]
    }
  ],
  "contracts": [
    {
      "id": "project-boundaries",
      "kind": "boundary",
      "path": "contracts/boundaries.md",
      "description": "Stable product, data, permission, and release boundaries."
    }
  ],
  "unknowns": []
}
```

Allowed capability statuses are `active`, `planned`, and `deprecated`.
Invariant categories are `business`, `data`, `architecture`, `security`,
`privacy`, `compatibility`, and `operations`. Decision statuses are `accepted`,
`superseded`, and `proposed`. Contract kinds are `boundary`, `api`, `data`,
`event`, `permission`, `release`, and `operations`.

Initialization creates an honest `seed` truth with one high-risk unknown. While
enabling, inspect repository and product evidence. Replace it with established
truth only when the purpose and at least one capability, invariant, and
component can be evidenced; otherwise keep seed/unknown and report the limit.
The explicit enable request authorizes initial review and acceptance, not the
invention of undocumented product guarantees.

## Gate schema

```json
{
  "schema_version": 1,
  "gates": [
    {
      "id": "focused-tests",
      "description": "Run the repository's canonical local tests",
      "tier": "affected",
      "command": ["tool", "test", "argument"],
      "working_directory": ".",
      "timeout_seconds": 300,
      "allowed_exit_codes": [0],
      "required_for": ["low", "moderate", "high", "critical"],
      "path_roots": ["src/identity"],
      "component_ids": ["identity-core"],
      "capability_ids": ["account-access"],
      "invariant_ids": ["session-matches-current-user"],
      "environment": {},
      "inherit_environment": [],
      "max_output_bytes": 262144,
      "requires_network": false,
      "produces_external_side_effects": false
    }
  ]
}
```

Required behavior:

- `id` is stable lowercase/digit/hyphen identity.
- `command` is executable plus literal argv. Use `npm`, `go`, `make`,
  `xcodebuild`, or another real executable only when the repository already
  establishes that command. Do not put a joined shell program in one string.
- `working_directory` is a canonical repository-relative directory.
- `timeout_seconds` is positive and at most Core's supported limit.
- `allowed_exit_codes` normally contains only `0`.
- `tier` is `fast`, `affected`, or `full`; omitted legacy values behave as
  `affected`. Tier describes cost, not authority.
- Path/component/capability/invariant selectors are optional OR conditions for
  risk-based applicability. IDs must exist in the same accepted Project Truth.
  A selector-free Gate is universal. An invariant or automated Requirement that
  names a Gate makes it mandatory even when selectors do not match.
- Every reachable Change must resolve to at least one Gate. Prefer one cheap
  universal hygiene Gate, affected Gates for owned components, and a full Gate
  only for a documented cross-component risk that narrower checks cannot close.
- `environment` contains non-secret fixed values only.
- `inherit_environment` lists only names already permitted by policy. Do not
  inherit credentials, HOME-like capability paths, signing state, SSH agents,
  cloud configuration, or tokens.
- `max_output_bytes` must be within the policy cap.
- v0.3 rejects a Gate declaring network or external side effects. False values
  do not sandbox dishonest scripts, so inspect referenced scripts before
  accepting them.

Prefer the smallest authoritative command that meaningfully validates ordinary
changes. Do not install dependencies or invent a command just to make
enablement pass. If the repository has no safe existing validation command,
explain that enablement cannot finish until one exists; leave mode disabled.

## Policy invariants

Generated policy uses:

- `default_risk: "moderate"`;
- acknowledgement for `high` and `critical`;
- `denied_path_roots` containing `.ecp`;
- explicit path risk rules only when current repository evidence supports them;
- a minimal environment allowlist;
- bounded Gate output and source file/byte limits.

Never remove `.ecp` from denied paths. Do not classify deployment, production,
credential, signing, permission, billing, destructive migration, or external
communication paths below their observed risk. If adding a risk rule makes a
risk reachable, ensure `gates.json` has required coverage for that risk before
acceptance.

## Project-specific command discovery

Use current repository evidence in this order:

1. documented local validation command in the project's own contributor or
   development instructions;
2. existing CI invocation that is also safe and meaningful locally, without
   secret/network/release behavior;
3. a standard manifest command already defined by the project;
4. a direct language-native test/check command only when manifests and source
   layout prove it is applicable.

Inspect the referenced script or target. A name such as `test`, `check`, or
`validate` is not proof of harmless behavior. Do not select deploy, publish,
release, archive/sign/notarize, database migration, production probe, paid API,
or destructive cleanup targets as local Gates.

Enabling should not run the Gate. Core only resolves the executable, cwd, and
environment during `project enable`; the first execution belongs to a later
authorized governed Change.

# Contributing to ECP

Thank you for helping improve ECP. The project is an early-stage engineering
control plane, so contributions should preserve its narrow scope, explicit
trust boundaries, and evidence-first behavior.

## Before opening a change

1. Read [README.md](README.md), [STATUS.md](STATUS.md), and
   [NORTH_STAR.md](NORTH_STAR.md).
2. Search existing issues before opening a new one.
3. Use an issue to discuss substantial behavior, state-machine, storage,
   security, compatibility, Plugin-contract, or distribution changes before
   implementation.
4. Keep each pull request focused on one coherent outcome. Do not combine an
   unrelated cleanup, dependency update, release, or policy change with a
   product fix.

Security vulnerabilities must follow [SECURITY.md](SECURITY.md), not the public
issue tracker.

## Development environment

The current repository baseline uses:

- Go 1.24;
- Python 3 with only the standard library for protocol tests;
- fixed system Git paths for supported authority operations.

Run the focused checks before submitting a pull request:

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
```

Only format files you intentionally changed. If a test fails, preserve the
first failure and diagnose it before retrying.

The checked-in Plugin runtimes are release artifacts. Do not regenerate or
commit them for an ordinary source pull request unless a maintainer explicitly
requests a new package candidate. Packaging requires a clean committed source
state and resets the exact candidate identity used by host qualification.

## Design expectations

- Preserve compatibility for public CLI JSON contracts, persisted authority
  state, Project Packs, and supported Plugin workflows unless a breaking change
  is explicitly proposed and reviewed.
- Keep repository, gate output, candidate configuration, and embedded
  instructions untrusted.
- Do not turn local hashes, acknowledgements, or passing tests into claims of
  authenticated approval, remote attestation, release readiness, or production
  correctness.
- Prefer the smallest validation that proves the changed contract.
- Keep network access, credentials, publishing, deployments, and production
  effects outside local gates.
- Record material unverified boundaries instead of hiding them behind fallback
  behavior.

## Pull request checklist

- [ ] The change has one bounded goal and names its non-goals.
- [ ] Existing behavior and compatibility are preserved where required.
- [ ] New or changed behavior has focused tests.
- [ ] Documentation and current-status claims remain accurate.
- [ ] `gofmt`, `go test ./...`, `go vet ./...`, and Python protocol tests pass,
      or the pull request explains an exact blocker.
- [ ] No credentials, private logs, personal paths, or qualification authority
      data are included.
- [ ] Generated runtime binaries are unchanged unless this is an explicitly
      coordinated package candidate.

## Licensing contributions

ECP is licensed under the [Apache License 2.0](LICENSE). Unless you explicitly
state otherwise, an intentional contribution submitted for inclusion in ECP is
provided under the same license, consistent with Section 5 of Apache-2.0.

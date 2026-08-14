## Goal

Describe the bounded outcome of this pull request.

## Scope and non-goals

- In scope:
- Explicitly out of scope:

## Behavior and compatibility

Describe expected changes and behavior that must remain unchanged. Call out
persisted authority state, Project Pack, CLI JSON, Plugin, platform, and
distribution impact where relevant.

## Validation

List exact commands and results. Do not describe local checks as release,
production, remote-attestation, or real-host evidence.

- [ ] `gofmt` reports no changes
- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `python3 -m unittest discover -s scripts -p 'test_*.py'`

## Safety and publication

- [ ] No credentials, private logs, personal paths, or authority data are included.
- [ ] Generated runtimes are unchanged, or this PR is an explicitly coordinated package candidate.
- [ ] Documentation and `STATUS.md` preserve verified versus unverified boundaries.
- [ ] This PR does not assume authorization for a release, deployment, production change, or other external effect.

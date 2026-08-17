# Authority Export Contract

An ECP authority export is a deterministic, private, sealed local directory
containing exactly one registered Workspace binding, the complete referenced
event history, every accepted Project Truth blob referenced by that history,
and every Evidence stdout/stderr artifact referenced by that history. Locks,
atomic-write remnants, orphan blobs, and orphan artifacts are excluded.

Export takes the Gate sequence lease and then the exclusive authority mutation
lock in lifecycle order, reloads and verifies the
current projection, copies only bounded private regular files, writes a strict
sorted manifest containing every path, role, byte size, and SHA-256 digest,
self-verifies the copied event chain, accepted epochs, truth blobs, Evidence
artifacts, and exact reference set, then atomically installs a private read-only
bundle outside both the repository and live authority directory with platform
no-replace semantics. A target created after preflight is preserved and causes
`EXPORT_TARGET_EXISTS`; export never replaces it. It does not
mutate repository files or the live authority revision/event head.

Offline verification must reject missing, extra, unsafe, non-private,
non-canonical, oversized, or digest-mismatched files. It must independently
replay event sequence/hash semantics using the manifest identity, verify every
historical accepted truth blob and referenced Evidence artifact, and require
the resulting projection to match the manifest revision, event head,
activation, enabled state, and latest accepted config/truth digests.

The same unchanged authority must produce the same bundle digest regardless of
destination. This digest detects change and corruption; it is not a signature,
publisher identity, authenticated approval, remote attestation, or proof that
the exporting machine was trustworthy. A bundle may contain sensitive project
history and Evidence logs. It must never be automatically committed, uploaded,
or shared. The current v0.3 candidate provides export and offline verification
only; it does not provide restore, import, merge, redaction, encryption,
retention scheduling, or garbage collection.

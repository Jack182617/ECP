# Authority Health Contract

`ecp authority health --root PATH` is the bounded, authority-only diagnostic
surface for one registered canonical Git Workspace. It exists so capacity,
referenced-object corruption, crash remnants, orphaned objects, unsafe layout,
and unresolved Gate execution can be observed before mutation reaches an
unrecoverable boundary.

The diagnostic must:

- resolve the immutable Workspace binding without loading repository candidate
  `.ecp` control or Project Truth files;
- acquire the Gate lease and then the authority mutation lock, matching the
  lifecycle lock order, and reload the event projection under both locks;
- fail without a report when the binding or event sequence/hash-chain is too
  untrusted to construct a trustworthy projection;
- verify every unique Project Truth blob referenced by every accepted truth
  epoch and every unique stdout/stderr artifact referenced by Evidence;
- inventory bounded event bytes/segments, current mutation capacity, terminal
  reserve, safe atomic-write remnants, content-addressed orphans, artifact
  staging remnants, unrecognized entries, and unsafe entries;
- return the exact active Change and unresolved `IN_PROGRESS` GateRun visible
  in that snapshot; and
- bound file count, byte count, and detailed findings so hostile or accidental
  state growth cannot turn diagnosis into unbounded traversal or output.

Status is a successful versioned machine result:

- `HEALTHY`: event projection and every referenced object verify, with no
  capacity warning, orphan, temporary, unrecognized, unsafe, or unresolved-run
  finding;
- `ATTENTION`: referenced authority remains verifiable, but capacity is near a
  limit or safe temporary/orphan/unrecognized state or a lease-released
  unresolved GateRun requires an explicit later decision;
- `INDETERMINATE`: one or more referenced objects are missing, corrupt, unsafe,
  oversized, ambiguous, or the surrounding state layout cannot be trusted.

CLI exit codes are `0`, `3`, and `4` respectively while `ok: true` and the full
report remains on stdout. A live Gate owner or mutation prevents the exact
snapshot and therefore returns a conflict/cancellation error rather than
guessing around the lock.

Health never appends an event, terminalizes a GateRun, accepts candidate truth,
deletes an orphan, removes a temporary file, repairs content or directory
permissions, restores an export, compacts history, migrates state, or changes
repository files. The authority directory must already be a private real
directory before either lock is attempted. On supported advisory-lock
platforms health may create or refresh only the private lock files' operational
PID/time metadata while authority revision and event head remain unchanged.
Referenced-object verification must not follow a symlinked object-store or
intermediate artifact directory. Acquiring a released Gate lease proves only that the previously
recorded `IN_PROGRESS` owner no longer holds that lease; health reports
`GATE_RUN_INTERRUPTION_RECOVERABLE` but leaves the actual `INTERRUPTED` event to
the next authorized lifecycle operation.

Object hashes and event-chain verification detect inconsistency; the report is
not a signature, authenticated approval, remote attestation, backup, restore
point, retention policy, garbage-collection authorization, or evidence that
the exporting/diagnosing machine is trusted.

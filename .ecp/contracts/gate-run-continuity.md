# GateRun Continuity Contract

Every Gate sequence that crosses into project-code execution has a durable,
authority-backed lifecycle. Core records `IN_PROGRESS` before the first project
command only after the bounded event store proves that the start, a worst-case
terminal, and final project-disable recovery still have byte and segment
headroom. It repeats terminal-headroom validation before recording Evidence,
binds every produced Evidence item to that exact run, and appends one terminal
state: `COMPLETED`, `FAILED`, `CANCELLED`, or `INTERRUPTED`.

`COMPLETED` means every selected Gate produced Evidence; it does not mean those
Gate results passed or that the Change obtained a PASS Verdict. `FAILED` means
the Core sequence stopped on an internal or integrity error. `CANCELLED` means
the caller context ended. Existing partial Evidence remains immutable in all
cases.

An unresolved `IN_PROGRESS` run makes status and Verdict indeterminate. It may
be marked `INTERRUPTED` only after a later lifecycle operation successfully
acquires the same advisory lease, proving that the former process no longer
holds that lease. Time, PID text, or chat history is not sufficient. The run
history remains queryable through the authority-only interface even when the
repository candidate configuration is malformed.

An explicit project disable that acquires a released Gate lease appends the
matching `INTERRUPTED` terminal, ACTIVE Change cancellation, and disablement as
one atomic batch. They all become authoritative together or none of them do.
Ordinary mutations cannot consume the byte or segment reserves protected for
that lifecycle closure.

This recovery accounts for authority history only. It does not automatically
resume Gates, restore orphan artifacts, reap every descendant process after an
uncatchable crash, undo external side effects, or create a remote attestation.

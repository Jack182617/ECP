# Local Verdict Contract

An ECP local `PASS` means the accepted local policy's Required Gates produced applicable Evidence for the exact current authority, Project, Workspace, activation, Change, accepted Control Config, accepted Project Truth, source fingerprint, Gate plan, execution identity, and Core identity, and that the final source has a current non-`UNKNOWN` Semantic Assessment.

It does not mean the product is defect-free, every behavior was tested, a person was authenticated, the change was committed or pushed, a device or production environment succeeded, a release was published, or the Plugin could not be bypassed.

Candidate policy and truth cannot accept themselves. Contract paths are unique within accepted Project Truth, so one changed file cannot be order-dependently attributed to only one of several IDs. `PRESERVED` cannot hide a truth delta. `CHANGED` requires a Core-computed delta within the declared Impact and explicit confirmation of the exact protected change. `UNKNOWN`, stale Evidence, source mutation, mismatched identity, missing Gate, failed Gate, or integrity failure prevents `PASS`.

For a version 3 or later Change, every referenced accepted Truth Unknown must also match
its declared `PRESERVED`, `RESOLVED`, or `REFINED` disposition when starting
and candidate Truth are compared. A Gate PASS cannot compensate for stale or
contradictory durable Truth.

For a version 4 Change, a declared new path root must be present in the final
source delta and owned by exactly one accepted Project Truth Component after a
protected Truth reconciliation. The start-time declaration cannot turn an
unmapped final path, unused scope, or ambiguous ownership into `PASS`.

Evidence replay validates the recorded Gate and command against its exact historical accepted config epoch. Artifact paths must canonically bind the Change, Evidence, and stdout/stderr role; private regular-file type, exact stored size, and stored digest must all match before the Evidence can apply.

Every sequence that reaches Gate execution is enclosed by a durable GateRun. A
remaining `IN_PROGRESS` run keeps the current Verdict `INDETERMINATE`; only a
later operation that actually acquires the released advisory lease may record
the former run as `INTERRUPTED`. `COMPLETED` means every selected Gate produced
Evidence, not that the Evidence passed. GateRun history recovery does not
automatically resume work, undo side effects, or prove crash-descendant cleanup.

Gate commands and repository scripts remain untrusted code. Until an isolated Runner and protected remote consumer exist, the local Verdict is a precise development assurance result, not a non-bypassable organizational or release authorization.

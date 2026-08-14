# Requirement Closure

ECP guarantees zero silent ambiguity, not omniscience. Before Core creates a v0.3 Change, every exact acceptance criterion, expected preservation, user journey, data effect, operational effect, expected semantic change, and discovered uncertainty must be covered by at least one sorted structured Requirement.

Each Requirement records a statement, status, rationale, decision source, verification mode, and exact coverage references. The statement is the exact behavior that applies to this Change now—even when that behavior is temporary, intentionally imperfect, or expected to change later; a question or vague aspiration is not a decided statement. Its status is one of:

- `DECIDED`: current behavior is explicit.
- `NOT_APPLICABLE`: the scenario was considered and excluded with a reason.
- `DEFERRED_SAFE`: current behavior remains explicit and a concrete revisit condition is recorded.
- `BLOCKING_UNKNOWN`: the question is visible, but Core refuses to start implementation.

Verification is deliberately separate from decision status:

- `AUTOMATED` names one or more configured Gates. Semantic reconciliation must map the Requirement to those exact Gate IDs, and Verdict requires current applicable PASS Evidence from every mapped Gate.
- `REVIEW` is a local product or engineering decision confirmed in the source-bound semantic reconciliation. It is not represented as automated Evidence.
- `EXTERNAL` remains `EXTERNAL_PENDING` and blocks local PASS until Core has a separately trusted external Evidence class. v0.3 does not import or attest that class.

A requirement summary, semantic assessment, chat statement, or acknowledgement cannot substitute for mapped automated Evidence. Removing a contract item requires a new Change contract; it cannot make an already-started obligation disappear.

Accepted Project Truth Unknowns are a separate durable-freshness obligation.
Every accepted Unknown ID referenced by a version 3 Change must have one sorted
disposition: `PRESERVED` keeps the same structured Unknown, `RESOLVED` removes
it, and `REFINED` keeps it with changed structured content. Core compares the
Change's starting Truth epoch with the candidate Truth during Semantic
Reconciliation and blocks an outcome that does not match. Historical Change
contract versions 0 and 2 remain readable without this new field.

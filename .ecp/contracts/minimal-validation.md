# Minimal Sufficient Validation

ECP chooses validation from facts rather than from a fixed test ceremony.

1. Core derives touched paths from the carried Change baseline and current source.
2. For established Project Truth, component `path_roots` infer directly affected components. Core then expands the reverse transitive closure of declared `depends_on` relationships; capabilities that own any resulting component infer their invariants.
3. Declared and inferred impact are unioned for risk and Gate selection. Missing declarations and final unmapped paths block PASS; they never lower the plan.
4. A Gate is required when an affected invariant or automated Requirement names it, regardless of selector filters.
5. Risk-based Gates apply only when at least one configured path, component, capability, or invariant selector matches. A Gate with no selectors remains universal for backward compatibility and baseline hygiene.
6. No applicable Gate means BLOCKED. Evidence remains bound to the exact source, config, truth, contract, plan, executable, environment, activation, and Core identity.

Gate `tier` describes cost and purpose (`fast`, `affected`, or `full`) but never overrides applicability or Evidence integrity. Projects should keep one cheap universal hygiene Gate, use affected Gates for owned components, and reserve full Gates for a documented cross-component risk or explicit invariant. Extra validation is justified only by an unresolved risk that narrower checks cannot close.

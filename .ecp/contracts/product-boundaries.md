# ECP Product Boundaries

`NORTH_STAR.md` is the canonical long-term product objective. `SPEC.md` is the canonical executable contract for the current version. `docs/architecture.md`, `docs/security-model.md`, and `docs/roadmap.md` refine implementation, threat, and sequencing boundaries but may not expand current claims beyond code and evidence.

ECP owns five concerns: durable project continuity, bounded Change contracts, protected project facts, exact Evidence applicability, and handoff/recovery history. It does not replace the Coding Agent, Git, compiler, test framework, CI, release platform, monitoring, project management, or human product decisions.

The user-facing target is natural-language development with no handwritten application code and no routine ECP CLI operation. The adapter may ask product-language questions or request explicit confirmation for a computed protected semantic change, high-risk action, policy change, release, or other authority boundary.

Project-local `.ecp` files are proposals. Accepted revisions and lifecycle facts live in repository-external authority state. Project Packs are independent per project; schemas and templates may be reused, but business truth, activation, Changes, Evidence, and release state may not be implicitly shared.

Current local mechanisms reduce silent drift; they do not prove an Agent is correct. Known mismatches fail closed, material uncertainty remains visible, and old Evidence cannot apply to a different subject. Complete North Star claims require independent long-term project evidence plus a protected enforcement consumer.

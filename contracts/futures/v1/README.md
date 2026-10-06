# Futures v1 contracts

These are development contracts, not evidence of deployed live data. The Go host
registers the full 42-operation route matrix with strict DTOs and owner/mode/runtime
gating. Disabled or unready dependencies fail closed. The Python worker exposes
candidate execution and never publishes without Go verification.

- `openapi.json`: public/admin API; existing httpx envelope.
- `internal-openapi.json`: isolated worker execution; candidate reports require Go verification.
- `types.schema.json`: canonical strict DTO definitions; local object schemas reference it.
- `fixtures/`: synthetic OFFLINE validation input. They intentionally exercise live wire
  shapes, but must never be loaded by production bootstrap, providers or seeders.

No fixture is evidence of a connected market feed, authorization or a valid market view.
Semantic checks (time ordering, owner/run binding, comparability, lifecycle transitions,
limits and current rights) must be enforced by services in addition to JSON validation.
See `spec/期货研究模块_开发SPEC.md` from repository root.

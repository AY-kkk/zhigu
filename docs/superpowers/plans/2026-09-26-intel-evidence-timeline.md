# 投资事件情报与证据时间线 Implementation Plan

> **For agentic workers:** REQUIRED SUBSKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the PRD v1.2 / SPEC I-1.0 investment-event intelligence and evidence-timeline module as an isolated live/demo feature, with deterministic event rules, persistent API, replay fixtures, independent Vue window, tests and delivery evidence.

**Architecture:** A dedicated `service/intel` domain writes namespace-scoped PostgreSQL tables through `finance_intel_*` schemas, exposes `/api/finance/intel/v1` through a separate Gin API, and runs fixture/live ingestion through a bounded worker and transactional outbox. Vue uses an independent `IntelLayout`, store and HTTP client under `/app/intel`; existing research/strategy modules only gain a new-window navigation action.

**Tech Stack:** Go 1.24/Gin/GORM/PostgreSQL 16/decimal, Vue 3/Pinia/Vue Router/Vite/Semi UI, Playwright, deterministic events-v1 fixtures and OpenAPI 3.1.

## Global Constraints

- Support only A-share acquisition, earnings forecast and regulatory investigation events; no trading advice, target price, return forecast or chain inference.
- Preserve PRD R1-R7: identity/merge, evidence grades, immutable revisions, three-axis state, freshness, change notifications and source attribution.
- API prefix is `/api/finance/intel/v1`; envelope is `{data,error,trace_id,meta}` and list cursors are signed and bounded.
- Demo and live namespaces, cookies, requests, reset generations and notification eligibility must remain isolated.
- `ZHIGU_INTEL_ENABLED` and `VITE_INTEL_ENABLED` default false; live providers require explicit configuration and authorization.
- Never put secrets in source, logs or fixtures; live-data/live-model modes exit 2 when authorization is absent.
- Do not revert or overwrite the existing uncommitted research/strategy work in this checkout.

## Task 1: Database and domain contract

**Files:** `app/server/migrations/finance/006_intel.sql`, `app/server/model/intel/models.go`, `contracts/intel/*`.
**Deliverable:** Idempotent migration plus typed domain records and versioned fixture/schema contracts.
- [x] Add isolated tables for namespaces, sessions, instruments, subscriptions, source revisions, events, evidence, snapshots, changes, notifications, jobs, review items and idempotency.
- [x] Add `contracts/intel/openapi.yaml`, `rules-v1.json`, `extraction-v1.schema.json`, `fixtures/events-v1/*`.
- [x] Run `cd app/server && GOTOOLCHAIN=local go test ./initialize -count=1`.

## Task 2: Deterministic rules and replay

**Files:** `app/server/service/intel/{types,rules,identity,time,fixture}.go`, tests.
**Deliverable:** Pure functions for weights, claim conflicts, state axes, freshness and S1-S8/D replay.
- [x] Implement fixed-decimal weights and stable evidence ordering.
- [x] Implement identity precedence (exact ID/reference, same matter, review queue, new event) and source revision folding.
- [x] Implement freshness at 14/30 day boundaries and coverage-limited unknown handling.
- [x] Add golden tests for S1-S8/D and rule/identity/time invariants.
- [x] Run `cd app/server && GOTOOLCHAIN=local go test ./service/intel -run 'Test(Rules|Identity|Time|Fixture)' -count=1`.

## Task 3: Persistent service, API and scope

**Files:** `app/server/service/intel/{service,repository,sessions,subscriptions,replay,outbox,worker}.go`, `app/server/api/v1/intel/*`, `app/server/main.go`.
**Deliverable:** All 24 REST operations, namespace enforcement, demo bootstrap/cookie, idempotency, cursor errors and admin boundaries.
- [x] Add strict DTO validation and Intel envelope/meta.
- [x] Enforce live JWT vs demo-only cookie mode and Origin checks.
- [x] Implement watchlist limits, event detail/history/timeline/evidence/conflicts/changes, source revisions, notifications/mute and data status.
- [x] Implement replay step/reset and transactional outbox delivery with generation fencing.
- [x] Register Intel routes and worker lifecycle in `main.go`; provider/model work remains fail-closed and fixture/manual jobs surface as partial review work.
- [x] Run `cd app/server && GOTOOLCHAIN=local go test -race ./service/intel/... ./api/v1/intel/... -count=1`.

## Task 4: Independent Vue window and UX

**Files:** `app/web/src/layout/intel/*`, `view/intel/*`, `components/intel/*`, `api/intel.js`, `utils/intelHttp.js`, `utils/openModuleWindow.js`, `stores/intelWorkspace.js`, `config/modules.js`, router/nav/login/session.
**Deliverable:** Same-level navigation opens `/app/intel` in a new browsing context without touching existing research/strategy state.
- [x] Add live/demo routes with safe redirect, account-storage synchronization and request epoch fencing.
- [x] Build watchlist, event stream, detail, timeline, evidence, conflicts, changes, notifications, admin and replay controls.
- [x] Show loading/empty/failure/retry, three state axes, source attribution, stale/expired/unknown and disclaimer.
- [x] Split Intel Playwright files/config: `intel.spec.js`, `intel-video.spec.js` and real Vue→Go→PostgreSQL `intel-chain.spec.js`.
- [x] Run `cd app/web && npm run build` and Intel Playwright tests.

## Task 5: Delivery, verification and release evidence

**Files:** `app/scripts/verify-intel.sh`, `app/scripts/intel_manifest.py`, `app/README.md`, `.env.example`, `app/deploy/compose.yaml`, `artifacts/intel/*`.
**Deliverable:** Repeatable offline/integration/live-data/live-model/regression commands and honest evidence manifest.
- [x] Make offline run deterministic rules/API/contracts/web checks; integration runs real Go/PostgreSQL tests and fixture replay, while durable provider worker/live authorization remain explicitly partial/blocked.
- [x] Record commit/dirty hash, PRD/SPEC hashes, rule/schema/prompt versions, I01-I38 status, test counts, skipped reasons and limitations.
- [x] Document startup, configuration, demo entry, AI role, provider permissions, rollback and known limits.
- [x] Run `bash app/scripts/verify-intel.sh offline` and `bash app/scripts/verify-intel.sh integration`.
- [x] Only mark production/live delivery complete after AC11/AC12/AC13 evidence exists; those gates are reported blocked rather than green.

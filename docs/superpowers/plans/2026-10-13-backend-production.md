# Backend Productionization Implementation Plan

> **Execution:** implement sequentially with test-first changes. Preserve the user's existing SPEC edits and never stage unrelated files.

**Goal:** Deliver the backend productionization contract in `spec/后端生产化_开发SPEC.md`, including fail-closed production configuration, browser sessions, stable API contracts, release gates, immutable services, backup/restore tooling, health/metrics, and CI evidence.

**Architecture:** Keep Go as the public API and control plane, PostgreSQL as the durable source of truth, and the research/futures Python services as private workers. Production behavior is selected by an explicit `ZHIGU_DEPLOYMENT_ENVIRONMENT=production`; development compatibility remains available only outside production. Mutating browser requests use a server-side session plus CSRF token, while worker calls retain separate service credentials. Futures release gates and operation-mode changes are transactional database records.

**Tech Stack:** Go 1.24+, Gin, GORM/PostgreSQL, Python 3.12/FastAPI, Docker Compose, Nginx, pytest/unittest, GitHub Actions.

---

## Global constraints

- The four state dimensions are independent: `deployment_environment`, `operation_mode`, `data_provenance`, and `health`.
- `production` accepts `data_provenance=real` only and fails closed on missing/placeholder secrets.
- Production login never returns a token in JSON. It sets `__Host-zhigu_session` with `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, and no `Domain`.
- Every error response and response header share one valid UUID trace ID. Error codes are registered in `contracts/error-codes.json`.
- No code or API may expose hidden chain-of-thought; only conclusions, citations, and compact rationale are persisted or returned.
- Every database change is expand/contract compatible for N/N-1 deploys.

## Task 1: Production runtime configuration and startup fail-closed

**Files:**
- Create: `app/server/runtime/config.go`
- Create: `app/server/runtime/config_test.go`
- Modify: `app/server/main.go`
- Modify: `app/server/initialize/db.go`
- Create: `app/server/initialize/owner.go`
- Create: `app/server/initialize/owner_test.go`

**Tests first:** cover `_FILE` precedence, placeholder rejection, the four orthogonal enums, production `real` provenance enforcement, owner/bootstrap startup checks, and redacted config summaries.

**Implementation:** parse and validate one typed runtime config before database or worker startup; reject defaults in production; remove unconditional demo-user seeding; bootstrap one owner idempotently from a one-time secret file and fail production startup when neither an owner nor bootstrap input exists.

**Verification:** `go test ./runtime ./initialize` and production-negative startup table tests.

## Task 2: Durable browser sessions, CSRF, password policy, and login throttling

**Files:**
- Create: `app/server/migrations/finance/010_sessions.sql`
- Create: `app/server/httpx/session.go`
- Create: `app/server/httpx/session_test.go`
- Modify: `app/server/httpx/auth.go`
- Modify: `app/server/api/v1/finance/handlers.go`
- Modify: `app/server/router/finance/consumer.go`
- Modify: `app/server/model/finance/user.go`
- Modify: `app/server/go.mod`
- Modify: `app/server/go.sum`

**Tests first:** production cookie attributes, absent JSON token, session revocation/expiry, CSRF issuance and rejection, Origin/Referer validation, legacy bearer compatibility outside production only, IP+account rate-limit dimensions, Argon2id verification, and bcrypt rehash-on-login.

**Implementation:** use random opaque session tokens stored as SHA-256 digests with durable `jti`, expiry, revocation, and CSRF digest; configure authentication from runtime at startup; add login/logout/session-CSRF endpoints; protect state-changing browser requests; hash new passwords with Argon2id (`m=19 MiB,t=2,p=1`) and migrate accepted legacy bcrypt hashes.

**Verification:** focused auth tests, migration tests, then `go test ./...`.

## Task 3: Trace IDs, stable error registry, readiness, and private metrics

**Files:**
- Create: `app/contracts/error-codes.json`
- Modify: `app/server/httpx/envelope.go`
- Create: `app/server/httpx/envelope_test.go`
- Create: `app/server/httpx/trace.go`
- Create: `app/server/httpx/trace_test.go`
- Create: `app/server/ops/health.go`
- Create: `app/server/ops/health_test.go`
- Modify: `app/server/main.go`

**Tests first:** only UUID/ULID inbound trace IDs are retained, generated IDs are UUIDs, body/header IDs match, `retryable` is mandatory, retry delay is optional, liveness never performs dependency I/O, readiness reports dependency failures, and metrics require a private bearer token.

**Implementation:** install trace middleware before routes; normalize all Go error envelopes; add `/livez`, `/readyz`, and authenticated `/metrics`; include database and worker readiness without leaking secrets.

**Verification:** focused packages, error-code registry consistency scan, then full Go tests.

## Task 4: Futures API contract hardening and deterministic pagination

**Files:**
- Modify: `app/server/service/futures/domain.go`
- Modify: `app/server/service/futures/domain_admin.go`
- Modify: `app/server/api/v1/futures/routes.go`
- Modify: `app/server/api/v1/futures/business_routes.go`
- Create: `app/server/api/v1/futures/production_contract_test.go`
- Modify: `app/contracts/futures/v1/openapi.json`
- Modify: `app/contracts/futures/v1/types.schema.json`
- Modify: `app/contracts/error-codes.json`

**Tests first:** capabilities expose the four independent dimensions, production never reports fixture provenance, list endpoints return stable `limit/cursor/next_cursor`, cursors are versioned/opaque/owner-and-filter-bound, notification filters work, and atomic batch-read rejects over 100 IDs.

**Implementation:** centralize cursor signing/validation; add deterministic secondary sort keys; add `POST /api/v1/futures/notifications/batch-read`; update OpenAPI/schema and route matrices.

**Verification:** futures API/service tests plus `python3 app/scripts/verify-futures-contracts.py`.

## Task 5: Source probes, persisted release gates, and transactional live activation

**Files:**
- Create: `app/server/migrations/futures/002_release_gates.sql`
- Modify: `app/server/service/futures/migrations.go`
- Create: `app/server/service/futures/release_gate.go`
- Create: `app/server/service/futures/release_gate_test.go`
- Modify: `app/server/service/futures/domain_admin.go`
- Modify: `app/server/api/v1/futures/business_routes.go`
- Modify: `app/contracts/futures/v1/openapi.json`

**Tests first:** probe creation is persisted and owner-scoped; gate expiry is at most 24 hours; `config_digest` includes non-secret config plus secret revision/fingerprint; a live transition requires matching `expected_version`, digest, unexpired gate, and reason; any mismatch leaves mode/version unchanged.

**Implementation:** add source-probe POST/GET endpoints; compute and persist gate evidence; lock the operations row and consume/validate the gate in one transaction before changing to live.

**Verification:** migration and concurrency tests, focused API tests, and full Go suite.

## Task 6: Worker service contracts and immutable production images

**Files:**
- Create: `app/research-service/Dockerfile`
- Create: `app/futures-service/Dockerfile`
- Modify: `app/research-service/app/main.py`
- Modify: `app/futures-service/src/futures_worker/app.py`
- Create: `app/tests/contract/test_worker_ops_contract.py`
- Modify: `app/server/service/futures/worker_client.go`

**Tests first:** workers expose `/livez`, `/readyz`, quiesce begin/status/end with epoch ACK, reject missing production secrets, and container definitions contain no runtime dependency installation or source bind mount.

**Implementation:** add authenticated operations endpoints and quiesce state; build locked, non-root worker images with health checks; keep fixture execution available only in test images/configuration.

**Verification:** both Python suites, contract tests, and Dockerfile static checks.

## Task 7: Production/test Compose, TLS proxy, maintenance watchdog, backup and restore

**Files:**
- Rewrite: `app/deploy/compose.prod.yaml`
- Create: `app/deploy/compose.test.yaml`
- Modify: `app/deploy/compose.yaml`
- Modify: `app/web/nginx.conf`
- Create: `app/deploy/nginx.prod.conf`
- Create: `app/scripts/production-preflight.sh`
- Create: `app/scripts/backup.sh`
- Create: `app/scripts/restore-verify.sh`
- Create: `app/scripts/maintenance-watchdog.sh`
- Create: `app/tests/contract/test_production_deploy.py`
- Modify: `app/deploy/README.md`

**Tests first:** production compose has five services/four named volumes, no source binds/default passwords/fixture flags, explicit resource and hardening settings, TLS-only public ingress, private metrics/workers, and test compose uses isolated fixture data. Script tests use fake commands to validate quiesce-before-dump, encryption/off-host handoff, retention, restore into a fresh database, migration/health checks, and watchdog restart behavior.

**Implementation:** split production and fixture stacks; add HTTPS proxy/security headers; implement preflight, consistent backup epochs, encrypted backup export, 7-daily/4-weekly retention, restore verification, and an explicit maintenance watchdog because Docker health status alone does not restart containers.

**Verification:** contract tests, `docker compose ... config` when Docker is available, and ShellCheck when available.

## Task 8: CI release gates, evidence manifest, and end-to-end verification

**Files:**
- Modify: `.github/workflows/ci.yml`
- Create: `app/scripts/verify-production.sh`
- Create: `app/scripts/release-evidence.py`
- Create: `app/tests/contract/test_release_evidence.py`
- Modify: `app/deploy/README.md`

**Tests first:** evidence generation is deterministic and records commit/image/config/schema/test digests; CI has separate fixture-integration and production-smoke jobs; race tests cover futures; production verification never enables fixture fallbacks.

**Implementation:** build and scan all immutable images, run contract/schema/migration/fixture/prod-smoke/backup-restore/race gates, and emit a machine-readable release manifest without secrets.

**Verification:** run all Go/Python/contract suites, `npm run build`, production verifier static phase, `go test -race ./service/futures ./api/v1/futures`, and inspect the final diff for secret/default/fixture leakage.

## Completion evidence

Record each task's test command, exit status, commit, and review result in `.superpowers/sdd/progress.md`. External-only checks (real credentials, off-host storage, DNS/TLS issuance, firewall enforcement, actual restore timing) must be reported as deployment gates rather than claimed complete without evidence.

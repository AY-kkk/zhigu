# Frontend productization progress

| Task | Status | Evidence | Remaining gate |
|---|---|---|---|
| 1. Contracts/tooling | implementation complete | runtime/envelope/request modules; Vitest 13/13 PASS | vue-eslint-parser peer for canonical lint |
| 2. System components | implementation complete | 8 system components; Vitest component 6/6 PASS | Playwright visual review |
| 3. Workspace/auth/routing | implementation complete | AppShell/PageHeader/404, cookie/CSRF memory auth, production build PASS | browser login/role/redirect E2E |
| 4. Futures client/model/store | implementation complete | typed client, pure model, concurrency/idempotency/batch rollback store | Vitest store run and real API contract journey |
| 5. Overview/product | implementation complete | four-dimension overview and product contract/coverage states; production build PASS | fixture browser journey and visual review |
| 6. Research create/detail | implementation complete | 3-stage flow, upload retry/cancel, stable idempotency key, fixed report sections, evidence lookup | browser upload/idempotency/recovery tests |
| 7. Hypothesis/notifications/admin | implementation complete | lifecycle confirmation, readable conditions, batch rollback filters, probe/gate/revocation | real HTTP probe/gate journey |
| 8. Existing-module regression | implementation complete | 18 Node contract/regression tests PASS; Vitest 19 total PASS; storage scan PASS | canonical ESLint Vue parser |
| 9. E2E/a11y/responsive | test code complete | core/futures/responsive/security/axe specs written | browser bind blocked; axe dependency not installed |
| 10. Performance/release | local static evidence complete | contract tests 18 PASS, performance tests 2 PASS, production build PASS, initial JS 68.28 KB gzip, route chunks <=65.84 KB gzip, frontend manifest generated | npm dependency install, Playwright/axe/visual/runtime performance, real HTTP/deploy smoke |

Authoritative local outputs:
- `app/web/dist/.vite/manifest.json`
- `artifacts/frontend/local/results.json`
- `artifacts/frontend/local/manifest.json` (`frontend.status=BLOCKED`, `storage_scan=PASS`)

External blockers:
- Vitest, ESLint, axe-core, Vue Test Utils, jsdom and `vue-eslint-parser@10.0.0` are installed and lockfile synchronized. `npm run verify` passes lint, 13 unit, 6 component, 18 contract, 2 performance and production build gates.
- Escalated npm and Playwright commands are rejected by the automatic approval service before execution.
- Sandbox cannot bind `127.0.0.1:5173`, so Playwright journeys cannot run.
- Real HTTP Compose, production browser smoke, visual review, VoiceOver/NVDA and full runtime performance remain `NOT_RUN`.

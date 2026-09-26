# 投研观点模块 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the manual viewpoint/research-report debate MVP through a real Vue → Go → PostgreSQL → worker → report chain.

**Architecture:** Extend the existing `finance` research workflow with uploaded document spans, structured claim parsing, evidence-bounded supporter/challenger execution, and a seven-section report export. Keep existing auth, owner isolation, task tokens, evidence registration, idempotency, cancellation, and publication gates.

**Tech Stack:** Vue 3, Vite, Semi UI Vue, Playwright, Go 1.24, Gin, GORM, PostgreSQL 16, Python 3.12, Pydantic, DeerFlow Harness, HTML export.

## Global Constraints

- Product baseline: `prd/投研观点模块_PRD.md`, SHA-256 `7dbd020a75548a9996e6a93a424112139e093d36437ce66a59afac3064521def`.
- Development contract: `spec/投研观点模块_开发SPEC.md`.
- MVP evidence sources are only market data, financial statements, filings, and uploaded reports.
- A report can be both `target` and `evidence`; `user_report` must remain visibly distinct from official disclosures.
- Input modes are exactly `claim_only`, `report_only`, `claim_and_report`.
- Fact-check states are exactly `supported`, `prerequisite_missing`, `contradicted`, `uncertain`.
- Overall verdicts are exactly `supported`, `partially_supported`, `challenged`, `insufficient`.
- Every fact/inference cites evidence from the same run. Missing evidence becomes unknown, never fabricated content.
- Do not mark fixture/mock evidence as live. Do not mark developer self-review as `accepted`.
- No implementation task may weaken an existing test to make it pass.

---

## File Map

| Responsibility | Files |
|---|---|
| Migration and models | `app/server/migrations/finance/007_research_viewpoint.sql`, `app/server/model/finance/models.go` |
| Document upload/extraction | `app/server/service/finance/document_service.go`, `app/server/api/v1/finance/handlers.go`, `app/server/router/finance/consumer.go` |
| Claim parsing | `app/server/service/finance/claim_parser.go`, `app/server/service/finance/research_service.go` |
| Report schema and gate | `app/server/service/finance/types.go`, `app/server/service/finance/report_gate_v2.go`, `app/server/service/finance/publish_gate.go` |
| Worker contract | `app/research-service/app/schemas.py`, `app/research-service/app/jobs.py`, `app/research-service/app/tools/document.py` |
| Frontend input/confirm/report | `app/web/src/components/research/*`, `app/web/src/stores/researchConversation.js`, `app/web/src/api/research.js` |
| Export | `app/server/service/finance/export_html.go`, `app/server/api/v1/finance/handlers.go` |
| Tests | `app/server/service/finance/*_test.go`, `app/research-service/tests/*`, `app/web/e2e/*` |
| Acceptance | `app/scripts/verify-research-viewpoint.sh`, `artifacts/research-viewpoint/<run>/manifest.json` |

---

### Task 1: Migration and persistence model

**Files:**
- Create: `app/server/migrations/finance/007_research_viewpoint.sql`
- Modify: `app/server/model/finance/models.go`
- Test: `app/server/initialize/migration_research_viewpoint_test.go`

**Interfaces:**
- Consumes: existing `ClaimDraft`, `ResearchRun`, `ResearchTask`, `Evidence`, `Report`.
- Produces: `ResearchDocument`, `ResearchDocumentSpan`, `ClaimFactCheck`; columns `ClaimDraft.SourceMode`, `ClaimDraft.DocumentID`, `ResearchRun.DocumentID`, `ResearchRun.InputMode`.

- [ ] **Step 1: Write the failing migration test**

```go
func TestResearchViewpointMigration(t *testing.T) {
  db := newMigratedTestDB(t)
  var count int64
  if err := db.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_name IN
    ('finance_research_documents','finance_research_document_spans','finance_claim_fact_checks')`).Scan(&count).Error; err != nil {
    t.Fatal(err)
  }
  if count != 3 {
    t.Fatalf("want 3 tables, got %d", count)
  }
  if !columnExists(t, db, "finance_research_runs", "input_mode") {
    t.Fatal("finance_research_runs.input_mode missing")
  }
}
```

- [ ] **Step 2: Run the test and verify the failure**

```bash
GOTOOLCHAIN=local \
  go test ./initialize -run TestResearchViewpointMigration -count=1 -v
```

Expected: FAIL because migration `007_research_viewpoint.sql` does not exist.

- [ ] **Step 3: Add migration and GORM models**

Use the exact SQL in SPEC §4 and structs below:

```go
type ResearchDocument struct {
  ID string
  OwnerID uint
  DraftID *string
  RunID *string
  Filename string
  MediaType string
  ByteSize int64
  ContentHash string
  StorageKey string
  ExtractionStatus string
  ExtractedText *string
  ExtractionError *string
  PageCount *int
  CreatedAt time.Time
  UpdatedAt time.Time
  DeletedAt *time.Time
}

type ResearchDocumentSpan struct {
  ID string
  DocumentID string
  PageNumber *int
  ParagraphIndex *int
  StartOffset int
  EndOffset int
  Text string
  ContentHash string
  CreatedAt time.Time
}

type ClaimFactCheck struct {
  ID string
  RunID string
  ClaimID string
  Status string
  Reason string
  EvidenceIDs datatypes.JSON
  CreatedAt time.Time
}
```

- [ ] **Step 4: Run migration tests**

```bash
GOTOOLCHAIN=local \
  go test ./initialize -run TestResearchViewpointMigration -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Run all Go tests**

```bash
GOTOOLCHAIN=local \
  go test ./... -count=1 -timeout 300s
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/server/migrations/finance/007_research_viewpoint.sql \
  app/server/model/finance/models.go \
  app/server/initialize/migration_research_viewpoint_test.go
git commit -m "feat(research): add viewpoint document and fact-check persistence"
```

---

### Task 2: Secure document upload and text extraction

**Files:**
- Create: `app/server/service/finance/document_service.go`
- Create: `app/server/service/finance/document_service_test.go`
- Modify: `app/server/api/v1/finance/handlers.go`
- Modify: `app/server/router/finance/consumer.go`
- Test: `app/server/api/v1/finance/document_api_test.go`

**Interfaces:**
- Produces: `DocumentService.Upload(ctx, UploadDocumentInput) (DocumentView, error)` and `DocumentService.Get(ctx, documentID) (DocumentView, error)`.
- API: `POST /api/finance/research-documents`, `GET /api/finance/research-documents/:id`.

- [ ] **Step 1: Write upload validation tests**

```go
func TestDocumentUploadValidation(t *testing.T) {
  cases := []struct{ name, filename string; size int64; code string }{
    {"unsupported", "report.exe", 10, "UNSUPPORTED_DOCUMENT"},
    {"too-large", "report.pdf", 20*1024*1024+1, "DOCUMENT_TOO_LARGE"},
    {"empty", "report.txt", 0, "EMPTY_DOCUMENT"},
  }
  for _, tc := range cases {
    t.Run(tc.name, func(t *testing.T) {
      _, err := service.Upload(testContext(1), UploadDocumentInput{Filename: tc.filename, ByteSize: tc.size})
      if ErrorCode(err) != tc.code {
        t.Fatalf("want %s, got %v", tc.code, err)
      }
    })
  }
}

func TestDocumentOwnerIsolation(t *testing.T) {
  doc := uploadForUser(t, 1)
  _, err := service.Get(testContext(2), doc.ID)
  if ErrorCode(err) != "DOCUMENT_NOT_FOUND" {
    t.Fatalf("cross-owner read must fail, got %v", err)
  }
}
```

- [ ] **Step 2: Run tests to verify failure**

```bash
GOTOOLCHAIN=local \
  go test ./service/finance -run 'TestDocument(UploadValidation|OwnerIsolation)' -count=1 -v
```

Expected: FAIL because `DocumentService` is undefined.

- [ ] **Step 3: Implement `DocumentService`**

Required behavior:

```go
type UploadDocumentInput struct {
  Filename string
  MediaType string
  Size int64
  Content []byte
  DraftID string
}

func (s *DocumentService) Upload(ctx context.Context, in UploadDocumentInput) (DocumentView, error) {
  if in.Size == 0 { return DocumentView{}, NewError(400, "validation", "EMPTY_DOCUMENT", "文件为空") }
  if in.Size > 20*1024*1024 { return DocumentView{}, NewError(413, "validation", "DOCUMENT_TOO_LARGE", "文件不能超过 20 MB") }
  media := allowedDocumentType(in.Filename, in.MediaType)
  if media == "" { return DocumentView{}, NewError(400, "validation", "UNSUPPORTED_DOCUMENT", "仅支持 PDF、DOCX、TXT") }
  hash := ContentHash(string(in.Content))
  // Store outside the repository under ZHIGU_RESEARCH_DOCUMENT_DIR; database stores only storage_key.
  // Deduplicate by owner_id+content_hash and never return another owner's row.
}
```

Use local extractors with fixed dependency versions. TXT uses UTF-8 text; PDF uses page-preserving extraction; DOCX preserves paragraph indexes. Extraction errors persist as `failed` and never produce fake spans.

- [ ] **Step 4: Add API handlers**

`POST` parses multipart `file` and optional `draft_id`; `GET` returns only non-sensitive metadata and extraction status. Both handlers call `UserIDFrom(ctx)` and never accept an owner ID from the request.

- [ ] **Step 5: Run service and API tests**

```bash
GOTOOLCHAIN=local \
  go test ./service/finance ./api/v1/finance -run 'TestDocument' -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/server/service/finance/document_service.go \
  app/server/service/finance/document_service_test.go \
  app/server/api/v1/finance/handlers.go \
  app/server/router/finance/consumer.go \
  app/server/api/v1/finance/document_api_test.go
git commit -m "feat(research): upload and extract research reports securely"
```

---

### Task 3: Three-mode claim parsing and confirmation revision

**Files:**
- Create: `app/server/service/finance/claim_parser.go`
- Create: `app/server/service/finance/claim_parser_test.go`
- Modify: `app/server/service/finance/research_service.go`
- Modify: `app/server/api/v1/finance/handlers.go`
- Test: `app/server/service/finance/research_service_input_mode_test.go`

**Interfaces:**
- Consumes: `ResearchDocument.ExtractedText`, spans, `MatchInstrumentsFromText`.
- Produces: `ClaimParseResult`, updated `ParseInput{Text,DocumentID,FocusText}`, updated `ParseOutput{InputMode,DocumentID,Items,NeedsConfirmation}`.

- [ ] **Step 1: Write input-mode tests**

```go
func TestParseInputMode(t *testing.T) {
  assertMode(t, ParseInput{Text: validClaim()}, "claim_only")
  assertMode(t, ParseInput{DocumentID: extractedDocID(t)}, "report_only")
  assertMode(t, ParseInput{Text: validClaim(), DocumentID: extractedDocID(t)}, "claim_and_report")
}

func TestParseRejectsMissingInput(t *testing.T) {
  _, err := service.ParseClaim(testContext(1), ParseInput{})
  if ErrorCode(err) != "MISSING_RESEARCH_INPUT" {
    t.Fatalf("got %v", err)
  }
}
```

- [ ] **Step 2: Run tests to verify failure**

```bash
GOTOOLCHAIN=local \
  go test ./service/finance -run 'TestParse(InputMode|RejectsMissingInput)' -count=1 -v
```

Expected: FAIL because `ParseInput` has no `DocumentID`.

- [ ] **Step 3: Implement structured parsing**

The parser must produce `ClaimItem` candidates from either viewpoint text or report spans. It must keep `ClaimID` stable within a revision and include `source_span_id` on report-derived items. Numeric parsing records metric, value, unit, period, and source span. If the company is ambiguous, return candidates and `needs_confirmation=true`; never auto-select among multiple companies.

- [ ] **Step 4: Enforce revision invalidation**

Changing `text`, `document_id`, or `focus_text` creates a new parse revision. PATCH may edit instrument, horizon, and items but must reject an input hash change with `REPARSE_REQUIRED`. Reuse of a confirmed draft returns `DRAFT_ALREADY_CONFIRMED`.

- [ ] **Step 5: Run tests**

```bash
GOTOOLCHAIN=local \
  go test ./service/finance -run 'Test(Parse|Draft)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/server/service/finance/claim_parser.go \
  app/server/service/finance/claim_parser_test.go \
  app/server/service/finance/research_service.go \
  app/server/api/v1/finance/handlers.go \
  app/server/service/finance/research_service_input_mode_test.go
git commit -m "feat(research): parse claim-only and report-backed research inputs"
```

---

### Task 4: Document-aware supporter/challenger worker contract

**Files:**
- Modify: `app/research-service/app/schemas.py`
- Create: `app/research-service/app/tools/document.py`
- Modify: `app/research-service/app/jobs.py`
- Modify: `app/research-service/app/prompts/supporter.md`
- Modify: `app/research-service/app/prompts/challenger.md`
- Test: `app/research-service/tests/test_research_viewpoint_worker.py`

**Interfaces:**
- Consumes: Go task payload `ResearchTask` plus `document_id` and `input_mode`.
- Produces: `ResearchResult.arguments`, `counterevidence`, `unknowns`, `evidence_ids`, and evidence records with `verification_status`.
- Tool: `read_document_spans(document_id, page_start, page_end, query, limit) -> {spans, evidence_ids, unknowns}`.

- [ ] **Step 1: Write worker isolation tests**

```python
def test_read_document_spans_is_owner_and_run_scoped(task_db):
    out = document_tool.invoke({"document_id": "foreign_doc", "query": "cash flow", "limit": 5})
    assert out["error"]["code"] == "DOCUMENT_NOT_FOUND"

def test_report_claim_is_not_independent_fact():
    result = run_fixture(role="challenger", input_mode="report_only")
    assert any(x["verification_status"] == "reported_only" for x in result["arguments"])
```

- [ ] **Step 2: Run tests to verify failure**

```bash
cd app/research-service
PYTHONPATH=. .venv/bin/python -m pytest tests/test_research_viewpoint_worker.py -q
```

Expected: FAIL because the document tool is undefined.

- [ ] **Step 3: Implement document tool and prompts**

The tool calls the Go internal API with the existing task token. It cannot read local paths. Returned spans become registered evidence only through `POST /internal/finance/evidence`. `reported_only` is required for report facts not independently matched to official/structured sources.

Prompts must explicitly require: claim-by-claim analysis, source labels, no fixed assumptions, no invented counterevidence, and `status=insufficient` when no evidence exists.

- [ ] **Step 4: Remove fixture conclusions from the real executor path**

The non-fixture path must run the configured model workflow to completion. If the harness/model is unavailable, return `failed` with `HARNESS_UNAVAILABLE`; it must not call `fixture_result` and must not emit `simulated=false` usage for fixture content.

- [ ] **Step 5: Run Python tests**

```bash
cd app/research-service
PYTHONPATH=. ZHIGU_RESEARCH_SQLITE=/tmp/zhigu-rv-worker.sqlite \
  .venv/bin/python -m pytest tests -q -k 'not actual_harness_import'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/research-service/app/schemas.py \
  app/research-service/app/tools/document.py \
  app/research-service/app/jobs.py \
  app/research-service/app/prompts/supporter.md \
  app/research-service/app/prompts/challenger.md \
  app/research-service/tests/test_research_viewpoint_worker.py
git commit -m "feat(research): add report-aware bounded research execution"
```

---

### Task 5: Seven-section report schema and deterministic adjudication

**Files:**
- Modify: `app/server/service/finance/types.go`
- Create: `app/server/service/finance/report_model_v2.go`
- Create: `app/server/service/finance/report_model_v2_test.go`
- Modify: `app/server/service/finance/nodes.go`
- Modify: `app/server/service/finance/research_service.go`

**Interfaces:**
- Produces: `VerifiedReportV2`, `FactCheck`, `Challenge`, `ReasoningGap`, `TailRisk`, `TestCondition`, `EvidenceRef`, `AdjudicateClaims`.
- API report field remains `report`; schema version changes to `research-report.v2`.

- [ ] **Step 1: Add the semantic regression test**

```go
func TestChallengeWithOppositeCashFlowPreventsSupportedVerdict(t *testing.T) {
  claim := ClaimItem{ClaimID:"c1", ClaimType:"inference", Text:"收入增长且现金流下降会证明盈利质量改善"}
  support := []Argument{{ClaimType:"fact", Text:"收入增长会证明盈利质量改善", EvidenceIDs:[]string{"ev_s"}}}
  challenge := []Argument{{ClaimType:"fact", Text:"现金流下降会削弱盈利质量改善", EvidenceIDs:[]string{"ev_c"}}}
  got := AdjudicateClaim(claim, support, challenge)
  if got.Status == "supported" {
    t.Fatalf("unresolved negative challenge must not be ignored: %+v", got)
  }
}
```

- [ ] **Step 2: Run the test to verify failure**

```bash
GOTOOLCHAIN=local \
  go test ./service/finance -run TestChallengeWithOppositeCashFlowPreventsSupportedVerdict -count=1 -v
```

Expected: FAIL because `AdjudicateClaim` is undefined.

- [ ] **Step 3: Implement claim-level adjudication**

Use explicit evidence relations produced by worker results. Never infer relevance solely from six Chinese keywords. A claim with unresolved supporting and challenging evidence is `uncertain`; evidence proving an unmet precondition is `prerequisite_missing`; direct contradictory evidence is `contradicted`. Preserve claim-level results in `finance_claim_fact_checks` and the report.

- [ ] **Step 4: Build `VerifiedReportV2` synthesis**

No field may contain fixed content copied between runs. `challenges`, `reasoning_gaps`, `tail_risks`, and `test_conditions` must bind `claim_id` and `evidence_ids` where factual. Empty collections require a typed reason object, not a generic fabricated sentence.

- [ ] **Step 5: Run all finance tests**

```bash
GOTOOLCHAIN=local \
  go test ./service/finance -count=1 -timeout 300s
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/server/service/finance/types.go \
  app/server/service/finance/report_model_v2.go \
  app/server/service/finance/report_model_v2_test.go \
  app/server/service/finance/nodes.go \
  app/server/service/finance/research_service.go
git commit -m "feat(research): generate evidence-bound seven-section reports"
```

---

### Task 6: Publication and HTML export gate

**Files:**
- Create: `app/server/service/finance/report_gate_v2.go`
- Create: `app/server/service/finance/report_gate_v2_test.go`
- Create: `app/server/service/finance/export_html.go`
- Create: `app/server/service/finance/export_html_test.go`
- Modify: `app/server/api/v1/finance/handlers.go`
- Modify: `app/server/router/finance/consumer.go`

**Interfaces:**
- Consumes: `VerifiedReportV2`, registered `Evidence`, `ResearchDocumentSpan`.
- Produces: `ValidateReportV2`, `RenderReportHTML(report, claim, evidence, document) ([]byte,error)`.
- API: `GET /api/finance/research/:id/export?format=html`.

- [ ] **Step 1: Write gate and export tests**

```go
func TestReportV2RejectsMissingRequiredSection(t *testing.T) {
  report := validReportV2()
  report.TailRisks = nil
  if err := ValidateReportV2(report, registeredEvidence(), validClaim()); ErrorCode(err) != "REPORT_SECTION_MISSING" {
    t.Fatalf("got %v", err)
  }
}

func TestExportHTMLHasNoScriptAndHasDisclaimer(t *testing.T) {
  raw := RenderReportHTML(validReportV2(), validClaim(), evidence(), doc())
  if bytes.Contains(raw, []byte("<script")) { t.Fatal("script forbidden") }
  if !bytes.Contains(raw, []byte("本报告仅供研究参考，不构成投资建议")) { t.Fatal("disclaimer missing") }
}
```

- [ ] **Step 2: Run tests to verify failure**

```bash
GOTOOLCHAIN=local \
  go test ./service/finance -run 'Test(ReportV2|ExportHTML)' -count=1 -v
```

Expected: FAIL.

- [ ] **Step 3: Implement fail-closed gate**

The gate must verify ownership, evidence whitelist, citation coverage, numeric basis, `as_of`, seven required sections, source grade, and incomplete verdict rules. Record each check with report pointer and failure reason. Any failure blocks publication.

- [ ] **Step 4: Implement single-file HTML export**

Escape all source text. Inline only generated CSS. Render evidence excerpts as text and external URLs as sanitized links. Do not include original report full text. Re-check owner authorization on every export request.

- [ ] **Step 5: Run tests**

```bash
GOTOOLCHAIN=local \
  go test ./service/finance ./api/v1/finance -run 'Test(ReportV2|ExportHTML|ExportAPI)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/server/service/finance/report_gate_v2.go \
  app/server/service/finance/report_gate_v2_test.go \
  app/server/service/finance/export_html.go \
  app/server/service/finance/export_html_test.go \
  app/server/api/v1/finance/handlers.go \
  app/server/router/finance/consumer.go
git commit -m "feat(research): gate and export complete viewpoint reports"
```

---

### Task 7: Frontend upload, parse, and confirmation

**Files:**
- Modify: `app/web/src/api/research.js`
- Modify: `app/web/src/stores/researchConversation.js`
- Modify: `app/web/src/components/research/ResearchComposer.vue`
- Modify: `app/web/src/components/research/ResearchConfirmCard.vue`
- Create: `app/web/src/components/research/ResearchDocumentUpload.vue`
- Test: `app/web/e2e/research-viewpoint-input.spec.js`

**Interfaces:**
- Consumes: Task 2 and Task 3 APIs.
- Produces: `uploadDocument(file)`, `conversation.inputMode`, `conversation.document`, `conversation.parseCurrent()`, `conversation.startResearch()`.

- [ ] **Step 1: Write Playwright input tests**

```js
test('claim, report, and combined inputs reach confirmation', async ({ page }) => {
  for (const mode of ['claim_only', 'report_only', 'claim_and_report']) {
    await seedSession(page)
    await mockResearchApi(page, { mode })
    await page.goto('/app/research/new')
    if (mode !== 'report_only') await claimInput(page).fill(DEMO_CLAIM)
    if (mode !== 'claim_only') await page.getByLabel('上传研报').setInputFiles('e2e/fixtures/sample-report.txt')
    await page.getByRole('button', { name: '解析观点' }).click()
    await expect(page.getByText(modeLabel(mode))).toBeVisible()
    await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible()
  }
})
```

- [ ] **Step 2: Run test to verify failure**

```bash
cd app/web
npx playwright test e2e/research-viewpoint-input.spec.js
```

Expected: FAIL because upload control and input-mode labels are missing.

- [ ] **Step 3: Implement upload and state transitions**

The composer accepts optional text and optional file. It disables parse only when both are absent. File changes clear the old draft. The confirmation card shows source mode, report filename, target/evidence roles, claims, company, horizon, cost/quota, and data cutoff.

- [ ] **Step 4: Implement failure and stale-state behavior**

Show upload extraction errors with retry. Editing text or replacing a file invalidates confirmation. Do not auto-create a run. A late parse response for an older revision must not overwrite the current UI.

- [ ] **Step 5: Run focused Playwright tests**

```bash
cd app/web
npx playwright test e2e/research-viewpoint-input.spec.js
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/web/src/api/research.js \
  app/web/src/stores/researchConversation.js \
  app/web/src/components/research/ResearchComposer.vue \
  app/web/src/components/research/ResearchConfirmCard.vue \
  app/web/src/components/research/ResearchDocumentUpload.vue \
  app/web/e2e/research-viewpoint-input.spec.js \
  app/web/e2e/fixtures/sample-report.txt
git commit -m "feat(web): support claim and research-report inputs"
```

---

### Task 8: Frontend seven-section report and evidence drawer

**Files:**
- Modify: `app/web/src/components/research/Report.vue`
- Create: `app/web/src/components/research/FactCheckTable.vue`
- Create: `app/web/src/components/research/ReasoningChainPanel.vue`
- Modify: `app/web/src/components/research/EvidenceDrawer.vue`
- Modify: `app/web/src/utils/researchViewModel.js`
- Test: `app/web/e2e/research-viewpoint-report.spec.js`

**Interfaces:**
- Consumes: `research-report.v2` response and evidence API.
- Produces: rendered seven sections, report download link, evidence relation and `verification_status`.

- [ ] **Step 1: Write report rendering tests**

```js
test('report renders all seven sections and source grade', async ({ page }) => {
  await seedSession(page)
  await mockResearchApi(page, { report: reportV2() })
  await page.goto('/app/research/run_e2e_1')
  for (const heading of ['核心判断','事实核验','逐条质疑','推理链缺口','被忽略的风险','证实与证伪条件','证据清单']) {
    await expect(page.getByRole('heading', { name: heading })).toBeVisible()
  }
  await expect(page.getByText('研报陈述，未独立核验')).toBeVisible()
})
```

- [ ] **Step 2: Run test to verify failure**

```bash
cd app/web
npx playwright test e2e/research-viewpoint-report.spec.js
```

Expected: FAIL because current report lacks the required sections.

- [ ] **Step 3: Implement report components**

The fact table uses four exact statuses. Challenges must show title/argument/evidence. Reasoning gaps use `from -> to -> missing`. Risks and test conditions remain separate. Evidence index displays source grade and independent verification status.

- [ ] **Step 4: Add report download**

Add a button to `/api/finance/research/:id/export?format=html` with a generated filename. The page disclaimer remains visible even if the summary is empty.

- [ ] **Step 5: Run focused Playwright tests**

```bash
cd app/web
npx playwright test e2e/research-viewpoint-report.spec.js
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/web/src/components/research/Report.vue \
  app/web/src/components/research/FactCheckTable.vue \
  app/web/src/components/research/ReasoningChainPanel.vue \
  app/web/src/components/research/EvidenceDrawer.vue \
  app/web/src/utils/researchViewModel.js \
  app/web/e2e/research-viewpoint-report.spec.js
git commit -m "feat(web): render complete viewpoint verification reports"
```

---

### Task 9: Real end-to-end chain and acceptance manifest

**Files:**
- Create: `app/web/e2e/research-viewpoint-chain.spec.js`
- Create: `app/scripts/verify-research-viewpoint.sh`
- Create: `app/scripts/research_viewpoint_manifest.py`
- Test: `app/server/service/finance/research_viewpoint_chain_test.go`

**Interfaces:**
- Consumes: all previous tasks, isolated PostgreSQL, Go server, Python worker, Vite and Playwright.
- Produces: `artifacts/research-viewpoint/<UTC-run-id>/manifest.json`, `results.tsv`, logs and screenshots.

- [ ] **Step 1: Write fail-closed manifest test**

```python
def test_missing_required_evidence_fails_manifest(tmp_path):
    result = subprocess.run([sys.executable, script, "--results", empty_tsv, "--out", tmp_path])
    assert result.returncode != 0
```

- [ ] **Step 2: Implement verifier**

Modes are `offline`, `integration`, `live`, `eval`. `integration` must start isolated PostgreSQL and the real Go+Python chain; it cannot use Playwright API mocks. Missing required tests, skipped live gates, absent logs, or stale hashes must create `failed`/`blocked` records and a non-zero exit.

- [ ] **Step 3: Add three-input real-chain test**

Run one `claim_only`, one `report_only`, and one `claim_and_report` scenario against real APIs. Assert persisted input mode, registered document spans, supporter/challenger task separation, report sections, citations, and export.

- [ ] **Step 4: Run integration verification**

```bash
GOTOOLCHAIN=local \
  bash app/scripts/verify-research-viewpoint.sh integration
```

Expected: exit 0 and a manifest whose required RV-01..RV-15 are `pass`; live/eval fields remain explicitly `blocked` or `unverified` where unavailable.

- [ ] **Step 5: Commit**

```bash
git add app/web/e2e/research-viewpoint-chain.spec.js \
  app/scripts/verify-research-viewpoint.sh \
  app/scripts/research_viewpoint_manifest.py \
  app/server/service/finance/research_viewpoint_chain_test.go
git commit -m "test(research): verify real viewpoint research chain"
```

---

### Task 10: Quality evaluation and release evidence

**Files:**
- Create: `tests/quality/research-viewpoint-set50.json`
- Create: `app/scripts/evaluate-research-viewpoint.py`
- Create: `reviews/research-viewpoint-mvp/验收报告.md`
- Modify: `app/README.md`
- Modify: `app/IMPLEMENTATION_STATUS.md`

**Interfaces:**
- Consumes: completed runs from Task 9 and human labels.
- Produces: `eval.json`, `manifest.json`, release report and exact evidence boundaries.

- [ ] **Step 1: Freeze 50 evaluation cases**

The set must include claim-only, report-only, combined, missing evidence, conflicting evidence, numeric unit errors, one-time gains, cash-flow gaps, and ambiguous companies. Each case stores expected fact-check status and required challenge themes.

- [ ] **Step 2: Implement evaluation scoring**

The script calculates fact error rate, challenge relevance score, citation coverage, link validity, numeric/period errors, and fabricated-source count. It exits non-zero if any threshold fails.

- [ ] **Step 3: Run evaluation**

```bash
python3 app/scripts/evaluate-research-viewpoint.py \
  --input tests/quality/research-viewpoint-set50.json \
  --out artifacts/research-viewpoint/eval.json
```

Expected: only call PASS when the PRD thresholds in §11.2 are met.

- [ ] **Step 4: Record separate evidence levels**

`验收报告.md` must separately list fixture tests, real integration, live data, live model, and human evaluation. It must not convert skipped or unavailable evidence into success.

- [ ] **Step 5: Update public release documentation**

Restore or update the seven tracked public files before release. Document data modes, upload privacy, report limitations, and “not investment advice”. Run `python3 scripts/check-public-tree.py` and require exit 0.

- [ ] **Step 6: Commit**

```bash
git add tests/quality/research-viewpoint-set50.json \
  app/scripts/evaluate-research-viewpoint.py \
  reviews/research-viewpoint-mvp/验收报告.md \
  app/README.md \
  app/IMPLEMENTATION_STATUS.md
git commit -m "docs(research): record viewpoint MVP quality and release evidence"
```

---

## Self-Review

### Spec coverage

| PRD/SPEC area | Task |
|---|---|
| Three input modes | Task 3, Task 7 |
| Upload and document spans | Task 1, Task 2 |
| User report as target and evidence | Task 2, Task 4, Task 7 |
| Supporter/challenger isolation | Task 4 |
| Claim-level fact checks | Task 5 |
| Seven-section report | Task 5, Task 8 |
| Publication gates | Task 6 |
| HTML download | Task 6, Task 8 |
| Error/cancel/retry/research states | Existing code regression + Task 7/9 |
| Real chain | Task 9 |
| 50-case quality evaluation | Task 10 |
| Public release and sign-off | Task 10 |

### RV coverage

| Test ID | Plan task |
|---|---|
| RV-01 | Task 3 |
| RV-02 | Task 2 |
| RV-03 | Task 2 |
| RV-04 | Task 3 |
| RV-05 | Task 3 |
| RV-06 | Task 4 |
| RV-07 | Task 4 |
| RV-08 | Task 4 |
| RV-09 | Task 5 |
| RV-10 | Task 6 |
| RV-11 | Task 6 and Task 9 |
| RV-12 | Task 7 and Task 8 |
| RV-13 | Task 7 and Task 8 |
| RV-14 | Task 9 |
| RV-15 | Task 6 and Task 8 |
| RV-16 | Task 10 |

### Placeholder scan

This plan contains no unresolved planning markers or unspecified validation. Each task has a failing test, exact command, expected state, implementation contract, full-suite gate, and commit step.

### Type consistency

- `ResearchDocument` and `ResearchDocumentSpan` names match Tasks 1–3.
- `read_document_spans` matches the allowlist in SPEC §7.1 and Task 4.
- `AdjudicateClaim` matches Task 5.
- `ValidateReportV2` and `RenderReportHTML` match Task 6.
- API response `document` and `report` fields match SPEC §5.5 and Task 8.
- RV IDs in SPEC §11 are recorded by Task 9's manifest.

---

## Execution Order

Tasks 1–3 form the input vertical slice. Tasks 4–6 form the research/report backend. Tasks 7–8 connect the interface. Tasks 9–10 are release gates. Do not start Task 9 until Tasks 1–8 are green; do not claim delivery before Task 10 passes or its missing evidence is explicitly reported as blocked.

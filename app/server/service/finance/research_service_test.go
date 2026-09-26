package finance

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
	"zhigu/server/testdb"
)

func ctxUser(id uint) context.Context {
	return WithUser(context.Background(), id, "user")
}

func seedUsers(t *testing.T, db *gorm.DB) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("Passw0rd!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users := []modelfinance.User{
		{ID: 1, Username: "admin", PasswordHash: string(hash), Role: "admin", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: 1001, Username: "invitee", PasswordHash: string(hash), Role: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{ID: 1002, Username: "invitee-b", PasswordHash: string(hash), Role: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}
	for _, u := range users {
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
}

func setup(t *testing.T) *ResearchService {
	t.Helper()
	db := testdb.Start(t)
	seedUsers(t, db)
	return NewService(db, NewFakeResearchClient(), NewMemoryBudget(), NewFixtureConfig())
}

func parseDemo(t *testing.T, svc *ResearchService, owner uint) ParseOutput {
	t.Helper()
	out, err := svc.ParseClaim(ctxUser(owner), ParseInput{
		Text: "演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return out
}

func createReqFrom(t *testing.T, draft ParseOutput) CreateResearchInput {
	t.Helper()
	return CreateResearchInput{
		DraftID:  draft.DraftID,
		Revision: draft.Revision,
	}
}

func TestCreateRunIdempotency(t *testing.T) {
	svc := setup(t)
	draft := parseDemo(t, svc, 1001)
	body := createReqFrom(t, draft)
	a, err := svc.CreateResearch(ctxUser(1001), "idem-1", body)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	var wg sync.WaitGroup
	ids := make([]string, 20)
	errs := make([]error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, e := svc.CreateResearch(ctxUser(1001), "idem-1", body)
			errs[i] = e
			if e == nil {
				ids[i] = out.RunID
			}
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("repeat %d: %v", i, e)
		}
		if ids[i] != a.RunID {
			t.Fatalf("repeat %d run %s != %s", i, ids[i], a.RunID)
		}
	}
	other := body
	other.Revision = draft.Revision + 1
	_, err = svc.CreateResearch(ctxUser(1001), "idem-1", other)
	if !isClass(err, "conflict") {
		t.Fatalf("want 409 conflict, got %v", err)
	}
}

func TestOwnerIsolation(t *testing.T) {
	svc := setup(t)
	draft := parseDemo(t, svc, 1001)
	created, err := svc.CreateResearch(ctxUser(1001), "own-1", createReqFrom(t, draft))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.GetResearch(ctxUser(1002), created.RunID)
	if !isClass(err, "not_found") {
		t.Fatalf("want 404, got %v", err)
	}
	_, err = svc.GetEvidence(ctxUser(1002), "ev_demo")
	if !isClass(err, "not_found") {
		t.Fatalf("evidence want 404, got %v", err)
	}
}

func TestConfirmDraftOnce(t *testing.T) {
	svc := setup(t)
	draft := parseDemo(t, svc, 1001)
	body := createReqFrom(t, draft)
	if _, err := svc.CreateResearch(ctxUser(1001), "k1", body); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := svc.CreateResearch(ctxUser(1001), "k2", body)
	if !isClass(err, "conflict") {
		t.Fatalf("draft may confirm once, got %v", err)
	}
}

func TestCancelPublishRace(t *testing.T) {
	svc := setup(t)
	draft := parseDemo(t, svc, 1001)
	created, err := svc.CreateResearch(ctxUser(1001), "cancel-1", createReqFrom(t, draft))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	view, err := svc.GetResearch(ctxUser(1001), created.RunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if view.Status != StatusQueued && view.Status != StatusCanceling {
		// queued expected
	}
	if _, err := svc.CancelResearch(ctxUser(1001), created.RunID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	run, err := svc.loadRun(created.RunID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rep := sampleReport(created.RunID)
	err = svc.Publish(context.Background(), created.RunID, run.Version, rep)
	if err == nil {
		t.Fatalf("must not publish after cancel")
	}
}

func TestDeleteLateWrite(t *testing.T) {
	svc := setup(t)
	draft := parseDemo(t, svc, 1001)
	created, err := svc.CreateResearch(ctxUser(1001), "del-1", createReqFrom(t, draft))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.DeleteResearch(ctxUser(1001), created.RunID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	err = svc.WriteTaskResult(context.Background(), created.RunID, RoleSupporter, sampleResult(created.RunID, "task_x"))
	if err == nil {
		t.Fatalf("late write must not succeed")
	}
	_, err = svc.GetResearch(ctxUser(1001), created.RunID)
	if !isClass(err, "not_found") {
		t.Fatalf("hidden immediately, got %v", err)
	}
}

func TestAccountConcurrencyLimit(t *testing.T) {
	svc := setup(t)
	d1 := parseDemo(t, svc, 1001)
	if _, err := svc.CreateResearch(ctxUser(1001), "k-a", createReqFrom(t, d1)); err != nil {
		t.Fatalf("first run: %v", err)
	}
	d2, err := svc.ParseClaim(ctxUser(1001), ParseInput{Text: "演示公司第二份观点同样超过二十个字符以便通过校验。"})
	if err != nil {
		t.Fatalf("parse2: %v", err)
	}
	_, err = svc.CreateResearch(ctxUser(1001), "k-b", createReqFrom(t, d2))
	if !isClass(err, "conflict") {
		t.Fatalf("one active run per user, got %v", err)
	}
}

func TestUnsupportedInstrument(t *testing.T) {
	svc := setup(t)
	draft := parseDemo(t, svc, 1001)
	bad := "AAPL"
	_, err := svc.PatchClaim(ctxUser(1001), draft.DraftID, PatchDraftInput{Revision: draft.Revision, InstrumentID: &bad})
	if !isClass(err, "validation") {
		t.Fatalf("want unsupported instrument, got %v", err)
	}
	if ErrorCode(err) != "UNSUPPORTED_INSTRUMENT" {
		t.Fatalf("code: %v", err)
	}
}

func TestParsePatchCreateLiveHK(t *testing.T) {
	t.Setenv("ZHIGU_DATA_MODE", ModeLive)
	t.Cleanup(ResetLiveCatalogForTest)
	ReplaceLiveCatalogForTest([]ListedInstrument{
		{ID: "00700.HK", Symbol: "00700", Name: "腾讯控股", Market: MarketHK, OrgID: "gshk0000700", Column: "hke", Plate: "hke"},
		{ID: "000001.SZ", Symbol: "000001", Name: "平安银行", Market: MarketA, OrgID: "gssz0000001", Column: "szse", Plate: "sz"},
	})
	svc := setup(t)
	out, err := svc.ParseClaim(ctxUser(1001), ParseInput{
		Text: "腾讯控股利润改善，未来一年经营前景值得看好。请核对该事实是否有年报支持。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.InstrumentID == nil || *out.InstrumentID != "00700.HK" {
		t.Fatalf("want 00700.HK got %+v candidates=%+v", out.InstrumentID, out.Candidates)
	}
	created, err := svc.CreateResearch(ctxUser(1001), "hk-live-1", createReqFrom(t, out))
	if err != nil {
		t.Fatalf("create tencent: %v", err)
	}
	if created.RunID == "" {
		t.Fatal("missing run id")
	}

	generic, err := svc.ParseClaim(ctxUser(1002), ParseInput{
		Text: "某公司利润改善，未来一年经营前景值得看好，请核对已披露年报再判断。",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := "700.HK"
	patched, err := svc.PatchClaim(ctxUser(1002), generic.DraftID, PatchDraftInput{Revision: generic.Revision, InstrumentID: &raw})
	if err != nil {
		t.Fatal(err)
	}
	if patched.InstrumentID == nil || *patched.InstrumentID != "00700.HK" {
		t.Fatalf("normalize patch got %+v", patched.InstrumentID)
	}
	if _, err := svc.CreateResearch(ctxUser(1002), "hk-live-2", createReqFrom(t, patched)); err != nil {
		t.Fatalf("create patched hk: %v", err)
	}
}

func isClass(err error, class string) bool {
	ae, ok := err.(*AppError)
	return ok && ae.Class == class
}

func sampleReport(runID string) VerifiedReport {
	v := "insufficient"
	return VerifiedReport{
		SchemaVersion:       "1.0",
		RunID:               runID,
		Version:             1,
		Mode:                ModeFixture,
		AsOf:                time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC),
		QualityStatus:       "completed",
		Verdict:             &v,
		Summary:             "fixture report",
		Support:             []Argument{},
		Challenge:           []Argument{},
		Assumptions:         []string{"a"},
		ChangeConditions:    []string{"c"},
		Unknowns:            []string{"无法取数：缺少足以判断该主张的已披露资料。"},
		EvidenceIDs:         []string{},
		ModelConfigVersion:  "model_fixture_v1",
		SourcePolicyVersion: "source_fixture_v1",
		PromptVersion:       "prompt_v1",
	}
}

func sampleResult(runID, taskID string) ResearchResult {
	return ResearchResult{
		SchemaVersion:   "1.0",
		RunID:           runID,
		TaskID:          taskID,
		Status:          "succeeded",
		Arguments:       []Argument{},
		EvidenceIDs:     []string{},
		Unknowns:        []string{"x"},
		Counterevidence: []Argument{},
		Usage:           Usage{},
	}
}

func TestHistoryOwnerOnly(t *testing.T) {
	svc := setup(t)
	d := parseDemo(t, svc, 1001)
	if _, err := svc.CreateResearch(ctxUser(1001), "hist-1", createReqFrom(t, d)); err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListResearch(ctxUser(1002), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("user B saw %d items", len(page.Items))
	}
	_ = fmt.Sprintf("ok")
}

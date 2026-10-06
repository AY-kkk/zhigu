package finance

import (
	"context"
	"testing"
	"time"

	modelfinance "zhigu/server/model/finance"
)

type fakeWebFetcher struct {
	result WebFetchResult
	err    error
}

func (f fakeWebFetcher) Fetch(context.Context, string) (WebFetchResult, error) {
	return f.result, f.err
}

func TestWebURLValidationRejectsUnsafeLinks(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/article",
		"http://localhost/article",
		"https://user:pass@example.com/article",
		"https://example.com:8080/article",
		"file:///tmp/article.html",
	} {
		if _, err := validatePublicWebURL(raw); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
}

func TestImportWebLinkExtractsWeChatStyleArticle(t *testing.T) {
	s := setup(t)
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	html := []byte(`<html><head><title>AI 加速落地，消费电子迎新机遇</title></head><body>
<nav>忽略导航</nav><div id="js_content"><p>公司收入增长具备已披露事实基础。</p><p>但盈利质量仍需现金流验证。</p></div>
<script>ignoreScript()</script></body></html>`)
	svc := NewDocumentServiceWithFetcher(s.DB, fakeWebFetcher{result: WebFetchResult{
		FinalURL: "https://mp.weixin.qq.com/s/example", StatusCode: 200, ContentType: "text/html; charset=utf-8",
		Body: html, FetchedAt: time.Now().UTC(),
	}})
	doc, err := svc.ImportURL(ctxUser(1001), ImportWebLinkInput{URL: "https://mp.weixin.qq.com/s/example"})
	if err != nil {
		t.Fatal(err)
	}
	if doc.OriginType != "url" || doc.SourceURL != "https://mp.weixin.qq.com/s/example" ||
		doc.SourceDomain != "mp.weixin.qq.com" || doc.Title != "AI 加速落地，消费电子迎新机遇" ||
		doc.SpanCount != 2 {
		t.Fatalf("document=%+v", doc)
	}
	spans, err := svc.ListSpans(ctxUser(1001), doc.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || spans[0].Text != "公司收入增长具备已披露事实基础。" {
		t.Fatalf("spans=%+v", spans)
	}
}

func TestWebLinkRegistersExternalWebEvidence(t *testing.T) {
	s := setup(t)
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	svc := NewDocumentServiceWithFetcher(s.DB, fakeWebFetcher{result: WebFetchResult{
		FinalURL: "https://example.com/article", StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<html><head><title>外部研究文章</title></head><body><article><p>现金流是盈利质量的重要验证项。</p></article></body></html>`),
		FetchedAt: time.Now().UTC(),
	}})
	doc, err := svc.ImportURL(ctxUser(1001), ImportWebLinkInput{URL: "https://example.com/article"})
	if err != nil {
		t.Fatal(err)
	}
	runID := "run_web_evidence"
	docID := doc.DocumentID
	if err := s.DB.Model(&modelfinance.ResearchDocument{}).Where("id = ?", docID).Update("run_id", runID).Error; err != nil {
		t.Fatal(err)
	}
	run := modelfinance.ResearchRun{ID: runID, OwnerID: 1001, DocumentID: &docID, InstrumentID: InstrumentDemo, Mode: ModeFixture}
	records, err := documentSpanRecords(s.DB, run, map[string]any{"document_id": docID, "query": "现金流", "limit": 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].SourceKind != "web_article" ||
		records[0].SourceGrade != "external_web" || records[0].VerificationStatus != "reported_only" {
		t.Fatalf("records=%+v", records)
	}
}

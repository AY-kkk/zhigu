package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"zhigu/server/httpx"
	finance "zhigu/server/service/finance"
	"zhigu/server/testdb"
)

func setupDocumentAPI(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Start(t)
	db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1001, 'user-a', 'x', 'user')`)
	db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1002, 'user-b', 'x', 'user')`)
	svc := finance.NewService(db, finance.NewFakeResearchClient(), finance.NewMemoryBudget(), finance.NewFixtureConfig())
	engine := gin.New()
	Register(engine, svc, nil)
	token, err := httpx.SignToken(1001, "user-a", "user")
	if err != nil {
		t.Fatal(err)
	}
	return engine, token
}

func TestResearchDocumentAPIUploadAndGet(t *testing.T) {
	engine, token := setupDocumentAPI(t)
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "report.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("第一条事实。\n\n第二条事实。")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/finance/research-documents", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload code=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			DocumentID string `json:"document_id"`
			SpanCount  int    `json:"span_count"`
		} `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.DocumentID == "" || env.Data.SpanCount != 2 {
		t.Fatalf("upload response=%+v", env)
	}
	get := httptest.NewRequest(http.MethodGet, "/api/finance/research-documents/"+env.Data.DocumentID, nil)
	get.Header.Set("Authorization", "Bearer "+token)
	out := httptest.NewRecorder()
	engine.ServeHTTP(out, get)
	if out.Code != http.StatusOK {
		t.Fatalf("get code=%d body=%s", out.Code, out.Body.String())
	}
}

func TestResearchDocumentAPIRejectsMissingFile(t *testing.T) {
	engine, token := setupDocumentAPI(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("draft_id", "missing")
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/finance/research-documents", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

type fakeAPIWebFetcher struct{}

func (fakeAPIWebFetcher) Fetch(context.Context, string) (finance.WebFetchResult, error) {
	return finance.WebFetchResult{
		FinalURL: "https://mp.weixin.qq.com/s/example", StatusCode: 200, ContentType: "text/html",
		Body: []byte(`<html><head><title>公众号文章</title></head><body><div id="js_content"><p>公司收入增长有公开披露。</p></div></body></html>`),
		FetchedAt: time.Now().UTC(),
	}, nil
}

func TestResearchLinkAPIImport(t *testing.T) {
	engine, token := setupDocumentAPI(t)
	engine.Routes()
	// Register uses the service's document importer; replace only its fetcher.
	// The service instance is internal to Register, so exercise the HTTP route through a new engine.
	db := testdb.Start(t)
	db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1001, 'user-a', 'x', 'user')`)
	svc := finance.NewServiceWithDocumentFetcher(db, finance.NewFakeResearchClient(), finance.NewMemoryBudget(), finance.NewFixtureConfig(), fakeAPIWebFetcher{})
	engine = gin.New()
	Register(engine, svc, nil)
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	body := bytes.NewBufferString(`{"url":"https://mp.weixin.qq.com/s/example"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/finance/research-links", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			OriginType string `json:"origin_type"`
			SourceURL  string `json:"source_url"`
			Title      string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.OriginType != "url" || env.Data.SourceURL != "https://mp.weixin.qq.com/s/example" || env.Data.Title != "公众号文章" {
		t.Fatalf("env=%+v", env)
	}
}

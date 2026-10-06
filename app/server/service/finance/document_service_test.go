package finance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"


	modelfinance "zhigu/server/model/finance"
)

func uploadTestDocument(t *testing.T, svc *DocumentService, owner uint, filename string, raw []byte) DocumentView {
	t.Helper()
	out, err := svc.Upload(ctxUser(owner), UploadDocumentInput{
		Filename: filename,
		MediaType: mediaTypeForFilename(filename),
		ByteSize: int64(len(raw)),
		Content:  raw,
	})
	if err != nil {
		t.Fatalf("upload %s: %v", filename, err)
	}
	return out
}

func TestDocumentUploadValidation(t *testing.T) {
	s := setup(t)
	svc := NewDocumentService(s.DB)
	cases := []struct {
		name string
		in   UploadDocumentInput
		code string
	}{
		{"unsupported", UploadDocumentInput{Filename: "report.exe", MediaType: "application/octet-stream", ByteSize: 10, Content: []byte("x")}, "UNSUPPORTED_DOCUMENT"},
		{"too-large", UploadDocumentInput{Filename: "report.pdf", MediaType: "application/pdf", ByteSize: 20*1024*1024 + 1, Content: []byte("%PDF-")}, "DOCUMENT_TOO_LARGE"},
		{"empty", UploadDocumentInput{Filename: "report.txt", MediaType: "text/plain"}, "EMPTY_DOCUMENT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Upload(ctxUser(1001), tc.in)
			if err == nil || ErrorCode(err) != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
}

func TestDocumentTextExtractionAndOwnerIsolation(t *testing.T) {
	s := setup(t)
	svc := NewDocumentService(s.DB)
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	doc := uploadTestDocument(t, svc, 1001, "report.txt", []byte("第一条观点。\n\n第二条观点，现金流下降。\n"))
	if doc.ExtractionStatus != "succeeded" {
		t.Fatalf("status=%s error=%s", doc.ExtractionStatus, doc.ExtractionError)
	}
	if doc.SpanCount != 2 {
		t.Fatalf("spans=%d", doc.SpanCount)
	}
	spans, err := svc.ListSpans(ctxUser(1001), doc.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || !strings.Contains(spans[1].Text, "现金流下降") {
		t.Fatalf("spans=%+v", spans)
	}
	if _, err := svc.Get(ctxUser(1002), doc.DocumentID); ErrorCode(err) != "DOCUMENT_NOT_FOUND" {
		t.Fatalf("cross-owner read must fail, got %v", err)
	}
}

func TestDocumentDeduplicatesSameOwnerContent(t *testing.T) {
	s := setup(t)
	svc := NewDocumentService(s.DB)
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	raw := []byte("重复上传的研报正文。")
	a := uploadTestDocument(t, svc, 1001, "a.txt", raw)
	b := uploadTestDocument(t, svc, 1001, "b.txt", raw)
	if a.DocumentID != b.DocumentID {
		t.Fatalf("same owner/content must deduplicate: %s != %s", a.DocumentID, b.DocumentID)
	}
	c := uploadTestDocument(t, svc, 1002, "c.txt", raw)
	if c.DocumentID == a.DocumentID {
		t.Fatal("different owners must not share document rows")
	}
}

func TestDocumentDOCXExtraction(t *testing.T) {
	s := setup(t)
	svc := NewDocumentService(s.DB)
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "references", "eino-ext", "components", "document", "parser", "docx", "examples", "testdata", "test_docx.docx"))
	if err != nil {
		t.Skipf("docx fixture unavailable: %v", err)
	}
	doc := uploadTestDocument(t, svc, 1001, "report.docx", raw)
	if doc.ExtractionStatus != "succeeded" || doc.SpanCount == 0 {
		t.Fatalf("docx extraction failed: %+v", doc)
	}
}

func TestDocumentPDFExtraction(t *testing.T) {
	s := setup(t)
	svc := NewDocumentService(s.DB)
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "references", "eino-ext", "components", "document", "parser", "pdf", "testdata", "test_pdf.pdf"))
	if err != nil {
		t.Skipf("pdf fixture unavailable: %v", err)
	}
	doc := uploadTestDocument(t, svc, 1001, "report.pdf", raw)
	if doc.ExtractionStatus != "succeeded" || doc.SpanCount == 0 || doc.PageCount == nil || *doc.PageCount == 0 {
		t.Fatalf("pdf extraction failed: %+v", doc)
	}
}

func TestDocumentRawFileStoredOutsideRepository(t *testing.T) {
	s := setup(t)
	svc := NewDocumentService(s.DB)
	dir := t.TempDir()
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", dir)
	doc := uploadTestDocument(t, svc, 1001, "report.txt", []byte("私密研报正文"))
	var row modelfinance.ResearchDocument
	if err := s.DB.Where("id = ?", doc.DocumentID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(row.StorageKey, "..") || filepath.IsAbs(row.StorageKey) {
		t.Fatalf("unsafe storage key %q", row.StorageKey)
	}
	if _, err := os.Stat(filepath.Join(dir, row.StorageKey)); err != nil {
		t.Fatalf("stored file missing: %v", err)
	}
	if row.ExtractedText == nil || *row.ExtractedText == "" {
		t.Fatal("extracted text not persisted")
	}
}

package finance

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
)

const (
	maxResearchDocumentBytes = 20 * 1024 * 1024
	maxDocumentTextRunes     = 2_000_000
)

type DocumentService struct {
	DB *gorm.DB
}

type UploadDocumentInput struct {
	Filename  string
	MediaType string
	ByteSize  int64
	Content   []byte
	DraftID   string
}

type DocumentSpan struct {
	SpanID         string `json:"span_id"`
	PageNumber     *int   `json:"page_number,omitempty"`
	ParagraphIndex *int   `json:"paragraph_index,omitempty"`
	StartOffset    int    `json:"start_offset"`
	EndOffset      int    `json:"end_offset"`
	Text           string `json:"text"`
}

type DocumentView struct {
	DocumentID       string     `json:"document_id"`
	Filename         string     `json:"filename"`
	MediaType        string     `json:"media_type"`
	ByteSize         int64      `json:"byte_size"`
	ContentHash      string     `json:"content_hash"`
	ExtractionStatus string     `json:"extraction_status"`
	ExtractionError  string     `json:"extraction_error,omitempty"`
	PageCount        *int       `json:"page_count,omitempty"`
	SpanCount        int        `json:"span_count"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DraftID          *string    `json:"draft_id,omitempty"`
	RunID            *string    `json:"run_id,omitempty"`
	DeletedAt        *time.Time `json:"-"`
}

type extractedDocument struct {
	Text      string
	Spans     []DocumentSpan
	PageCount *int
}

func NewDocumentService(db *gorm.DB) *DocumentService {
	return &DocumentService{DB: db}
}

func mediaTypeForFilename(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".txt":
		return "text/plain"
	default:
		return ""
	}
}

func (s *DocumentService) Upload(ctx context.Context, in UploadDocumentInput) (DocumentView, error) {
	owner := UserIDFrom(ctx)
	if owner == 0 {
		return DocumentView{}, NewError(401, "forbidden", "UNAUTHENTICATED", "未登录")
	}
	declaredSize := in.ByteSize
	if declaredSize == 0 {
		declaredSize = int64(len(in.Content))
	}
	if declaredSize <= 0 || len(in.Content) == 0 {
		return DocumentView{}, NewError(400, "validation", "EMPTY_DOCUMENT", "文件为空")
	}
	if declaredSize > maxResearchDocumentBytes || int64(len(in.Content)) > maxResearchDocumentBytes {
		return DocumentView{}, NewError(413, "validation", "DOCUMENT_TOO_LARGE", "文件不能超过 20 MB")
	}
	mediaType := mediaTypeForFilename(in.Filename)
	if mediaType == "" || (in.MediaType != "" && !compatibleMediaType(in.MediaType, mediaType)) {
		return DocumentView{}, NewError(400, "validation", "UNSUPPORTED_DOCUMENT", "仅支持 PDF、DOCX、TXT")
	}
	filename := NormalizeText(strings.TrimSpace(in.Filename))
	if filename == "" || strings.ContainsAny(filename, "\x00/\\") {
		return DocumentView{}, NewError(400, "validation", "INVALID_FILENAME", "文件名无效")
	}
	if in.DraftID != "" {
		var draft modelfinance.ClaimDraft
		if err := s.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", in.DraftID, owner).Take(&draft).Error; err != nil {
			return DocumentView{}, NewError(404, "not_found", "DRAFT_NOT_FOUND", "草稿不存在")
		}
	}

	hash := ContentHash(string(in.Content))
	var existing modelfinance.ResearchDocument
	if err := s.DB.WithContext(ctx).
		Where("owner_id = ? AND content_hash = ? AND deleted_at IS NULL", owner, hash).
		Take(&existing).Error; err == nil {
		return s.view(existing)
	}

	docID := "doc_" + uuid.NewString()
	storageKey := filepath.Join(fmt.Sprintf("owner-%d", owner), hash+".bin")
	if err := s.storeRaw(storageKey, in.Content); err != nil {
		return DocumentView{}, err
	}

	extracted, extractErr := extractResearchDocument(mediaType, in.Content)
	status := "succeeded"
	errorText := ""
	var text *string
	var pageCount *int
	if extractErr != nil {
		status = "failed"
		errorText = extractErr.Error()
	} else {
		text = &extracted.Text
		pageCount = extracted.PageCount
	}
	now := s.now(ctx)
	row := modelfinance.ResearchDocument{
		ID: docID, OwnerID: owner, DraftID: stringPtr(in.DraftID), Filename: filename,
		MediaType: mediaType, ByteSize: declaredSize, ContentHash: hash, StorageKey: storageKey,
		ExtractionStatus: status, ExtractedText: text, ExtractionError: stringPtr(errorText),
		PageCount: pageCount, CreatedAt: now, UpdatedAt: now,
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if extractErr != nil {
			return nil
		}
		for i := range extracted.Spans {
			span := modelfinance.ResearchDocumentSpan{
				ID: "span_" + uuid.NewString(), DocumentID: docID,
				PageNumber: extracted.Spans[i].PageNumber, ParagraphIndex: extracted.Spans[i].ParagraphIndex,
				StartOffset: extracted.Spans[i].StartOffset, EndOffset: extracted.Spans[i].EndOffset,
				Text: extracted.Spans[i].Text, ContentHash: ContentHash(extracted.Spans[i].Text), CreatedAt: now,
			}
			if err := tx.Create(&span).Error; err != nil {
				return err
			}
		}
		if row.DraftID != nil && *row.DraftID != "" {
			return tx.Model(&modelfinance.ClaimDraft{}).
				Where("id = ? AND owner_id = ?", *row.DraftID, owner).
				Update("document_id", docID).Error
		}
		return nil
	})
	if err != nil {
		_ = os.Remove(s.absoluteStoragePath(storageKey))
		return DocumentView{}, err
	}
	return s.view(row)
}

func (s *DocumentService) Get(ctx context.Context, documentID string) (DocumentView, error) {
	row, err := s.ownedDocument(ctx, documentID)
	if err != nil {
		return DocumentView{}, err
	}
	return s.view(row)
}

func (s *DocumentService) GetText(ctx context.Context, documentID string) (string, error) {
	row, err := s.ownedDocument(ctx, documentID)
	if err != nil {
		return "", err
	}
	if row.ExtractionStatus != "succeeded" || row.ExtractedText == nil {
		return "", NewError(409, "conflict", "DOCUMENT_NOT_READY", "研报文本尚不可用")
	}
	return *row.ExtractedText, nil
}

func (s *DocumentService) ListSpans(ctx context.Context, documentID string) ([]DocumentSpan, error) {
	if _, err := s.ownedDocument(ctx, documentID); err != nil {
		return nil, err
	}
	var rows []modelfinance.ResearchDocumentSpan
	if err := s.DB.WithContext(ctx).
		Where("document_id = ?", documentID).
		Order("page_number NULLS FIRST, paragraph_index NULLS FIRST, start_offset").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DocumentSpan, 0, len(rows))
	for _, row := range rows {
		out = append(out, DocumentSpan{
			SpanID: row.ID, PageNumber: row.PageNumber, ParagraphIndex: row.ParagraphIndex,
			StartOffset: row.StartOffset, EndOffset: row.EndOffset, Text: row.Text,
		})
	}
	return out, nil
}

func (s *DocumentService) ownedDocument(ctx context.Context, documentID string) (modelfinance.ResearchDocument, error) {
	owner := UserIDFrom(ctx)
	if owner == 0 {
		return modelfinance.ResearchDocument{}, NewError(401, "forbidden", "UNAUTHENTICATED", "未登录")
	}
	var row modelfinance.ResearchDocument
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND owner_id = ? AND deleted_at IS NULL", documentID, owner).
		Take(&row).Error; err != nil {
		return modelfinance.ResearchDocument{}, NewError(404, "not_found", "DOCUMENT_NOT_FOUND", "研报不存在")
	}
	return row, nil
}

func (s *DocumentService) view(row modelfinance.ResearchDocument) (DocumentView, error) {
	var count int64
	if err := s.DB.Model(&modelfinance.ResearchDocumentSpan{}).Where("document_id = ?", row.ID).Count(&count).Error; err != nil {
		return DocumentView{}, err
	}
	errText := ""
	if row.ExtractionError != nil {
		errText = *row.ExtractionError
	}
	return DocumentView{
		DocumentID: row.ID, Filename: row.Filename, MediaType: row.MediaType,
		ByteSize: row.ByteSize, ContentHash: row.ContentHash,
		ExtractionStatus: row.ExtractionStatus, ExtractionError: errText,
		PageCount: row.PageCount, SpanCount: int(count), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		DraftID: row.DraftID, RunID: row.RunID,
	}, nil
}

func (s *DocumentService) storeRaw(storageKey string, raw []byte) error {
	root := os.Getenv("ZHIGU_RESEARCH_DOCUMENT_DIR")
	if root == "" {
		root = filepath.Join("data", "research-documents")
	}
	full := filepath.Join(root, filepath.FromSlash(storageKey))
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	cleanFull, err := filepath.Abs(full)
	if err != nil {
		return err
	}
	if cleanFull != cleanRoot && !strings.HasPrefix(cleanFull, cleanRoot+string(os.PathSeparator)) {
		return NewError(500, "internal", "INVALID_STORAGE_PATH", "无效文件存储路径")
	}
	if err := os.MkdirAll(filepath.Dir(cleanFull), 0o700); err != nil {
		return err
	}
	return os.WriteFile(cleanFull, raw, 0o600)
}

func (s *DocumentService) absoluteStoragePath(storageKey string) string {
	root := os.Getenv("ZHIGU_RESEARCH_DOCUMENT_DIR")
	if root == "" {
		root = filepath.Join("data", "research-documents")
	}
	return filepath.Join(root, filepath.FromSlash(storageKey))
}

func (s *DocumentService) now(_ context.Context) time.Time {
	return time.Now().UTC()
}

func compatibleMediaType(got, want string) bool {
	got = strings.ToLower(strings.TrimSpace(strings.Split(got, ";")[0]))
	return got == want || got == "application/octet-stream" || (want == "text/plain" && strings.HasPrefix(got, "text/"))
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	out := s
	return &out
}

func extractResearchDocument(mediaType string, raw []byte) (extractedDocument, error) {
	switch mediaType {
	case "text/plain":
		return extractTextDocument(raw)
	case "application/pdf":
		return extractPDFDocument(raw)
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return extractDOCXDocument(raw)
	default:
		return extractedDocument{}, NewError(400, "validation", "UNSUPPORTED_DOCUMENT", "仅支持 PDF、DOCX、TXT")
	}
}

func extractTextDocument(raw []byte) (extractedDocument, error) {
	if !utf8.Valid(raw) {
		return extractedDocument{}, fmt.Errorf("TXT 不是有效 UTF-8 文本")
	}
	text := NormalizeText(string(raw))
	if strings.TrimSpace(text) == "" {
		return extractedDocument{}, fmt.Errorf("TXT 无可用文本")
	}
	spans := textSpans(text, nil, nil)
	page := 1
	return extractedDocument{Text: text, Spans: spans, PageCount: &page}, nil
}

func extractPDFDocument(raw []byte) (extractedDocument, error) {
	if len(raw) < 5 || !bytes.HasPrefix(raw, []byte("%PDF-")) {
		return extractedDocument{}, fmt.Errorf("不是数字 PDF")
	}
	r, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return extractedDocument{}, fmt.Errorf("无法打开 PDF 文本层")
	}
	pageTotal := r.NumPage()
	if pageTotal < 1 {
		return extractedDocument{}, fmt.Errorf("PDF 无页面")
	}
	var all strings.Builder
	var spans []DocumentSpan
	offset := 0
	for i := 1; i <= pageTotal; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		pageText, err := page.GetPlainText(nil)
		if err != nil || strings.TrimSpace(pageText) == "" {
			continue
		}
		pageText = NormalizeText(pageText)
		pageText = strings.TrimSpace(pageText)
		if pageText == "" {
			continue
		}
		if all.Len() > 0 {
			all.WriteByte('\n')
			offset++
		}
		pageNo := i
		pageSpans := textSpans(pageText, &pageNo, nil)
		for _, span := range pageSpans {
			span.StartOffset += offset
			span.EndOffset += offset
			spans = append(spans, span)
		}
		all.WriteString(pageText)
		offset += len([]rune(pageText))
		if utf8.RuneCountInString(all.String()) > maxDocumentTextRunes {
			return extractedDocument{}, fmt.Errorf("PDF 文本超过 2,000,000 字")
		}
	}
	text := strings.TrimSpace(all.String())
	if text == "" || len(spans) == 0 {
		return extractedDocument{}, fmt.Errorf("PDF 无可用文本层")
	}
	return extractedDocument{Text: text, Spans: spans, PageCount: &pageTotal}, nil
}

func extractDOCXDocument(raw []byte) (extractedDocument, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return extractedDocument{}, fmt.Errorf("无法打开 DOCX")
	}
	var documentXML []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			documentXML, err = readZipFile(f)
			if err != nil {
				return extractedDocument{}, fmt.Errorf("读取 DOCX 正文失败")
			}
			break
		}
	}
	if len(documentXML) == 0 {
		return extractedDocument{}, fmt.Errorf("DOCX 缺少正文")
	}
	paragraphs, err := docxParagraphs(documentXML)
	if err != nil {
		return extractedDocument{}, fmt.Errorf("解析 DOCX 正文失败")
	}
	var b strings.Builder
	var spans []DocumentSpan
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(NormalizeText(paragraph))
		if paragraph == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		start := utf8.RuneCountInString(b.String())
		b.WriteString(paragraph)
		end := utf8.RuneCountInString(b.String())
		index := len(spans)
		spans = append(spans, DocumentSpan{
			ParagraphIndex: &index, StartOffset: start, EndOffset: end, Text: paragraph,
		})
		if end > maxDocumentTextRunes {
			return extractedDocument{}, fmt.Errorf("DOCX 文本超过 2,000,000 字")
		}
	}
	if len(spans) == 0 {
		return extractedDocument{}, fmt.Errorf("DOCX 无可用文本")
	}
	return extractedDocument{Text: b.String(), Spans: spans}, nil
}

func textSpans(text string, page *int, paragraphBase *int) []DocumentSpan {
	runes := []rune(text)
	var spans []DocumentSpan
	start := 0
	para := 0
	flush := func(end int) {
		for start < end && (runes[start] == '\n' || runes[start] == '\r') {
			start++
		}
		for end > start && (runes[end-1] == '\n' || runes[end-1] == '\r') {
			end--
		}
		if end <= start {
			return
		}
		value := strings.TrimSpace(string(runes[start:end]))
		if value == "" {
			return
		}
		idx := para
		if paragraphBase != nil {
			idx += *paragraphBase
		}
		spans = append(spans, DocumentSpan{
			PageNumber: page, ParagraphIndex: &idx, StartOffset: start, EndOffset: end, Text: value,
		})
		para++
		start = end
	}
	i := 0
	for i < len(runes) {
		if runes[i] == '\n' {
			j := i
			for j < len(runes) && (runes[j] == '\n' || runes[j] == '\r') {
				j++
			}
			if j-i >= 2 {
				flush(i)
				start = j
				i = j
				continue
			}
		}
		i++
	}
	flush(len(runes))
	return spans
}

func docxParagraphs(raw []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var out []string
	var current strings.Builder
	inParagraph := false
	inText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "p":
				inParagraph = true
				current.Reset()
			case "t":
				inText = inParagraph
			}
		case xml.CharData:
			if inText {
				current.Write([]byte(value))
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				inText = false
			case "p":
				if inParagraph {
					out = append(out, current.String())
				}
				inParagraph = false
				inText = false
			}
		}
	}
	return out, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, maxResearchDocumentBytes+1))
}

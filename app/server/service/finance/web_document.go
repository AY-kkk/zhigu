package finance

import (
	"context"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/net/html"
	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
)

type ImportWebLinkInput struct {
	URL     string
	DraftID string
}

func NewDocumentServiceWithFetcher(db *gorm.DB, fetcher WebFetcher) *DocumentService {
	if fetcher == nil {
		fetcher = PublicWebFetcher{}
	}
	return &DocumentService{DB: db, Fetcher: fetcher}
}

func (s *DocumentService) ImportURL(ctx context.Context, in ImportWebLinkInput) (DocumentView, error) {
	owner := UserIDFrom(ctx)
	if owner == 0 {
		return DocumentView{}, NewError(401, "forbidden", "UNAUTHENTICATED", "未登录")
	}
	parsed, err := validatePublicWebURL(in.URL)
	if err != nil {
		return DocumentView{}, err
	}
	if in.DraftID != "" {
		var draft modelfinance.ClaimDraft
		if err := s.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", in.DraftID, owner).Take(&draft).Error; err != nil {
			return DocumentView{}, NewError(404, "not_found", "DRAFT_NOT_FOUND", "草稿不存在")
		}
	}
	fetcher := s.Fetcher
	if fetcher == nil {
		fetcher = PublicWebFetcher{}
	}
	fetched, err := fetcher.Fetch(ctx, parsed.String())
	if err != nil {
		return DocumentView{}, err
	}
	if _, err := validatePublicWebURL(fetched.FinalURL); err != nil {
		return DocumentView{}, err
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(fetched.ContentType, ";")[0]))
	if mediaType == "application/xhtml+xml" {
		mediaType = "text/html"
	}
	if mediaType != "text/html" && mediaType != "text/plain" {
		return DocumentView{}, NewError(415, "validation", "UNSUPPORTED_WEB_CONTENT", "链接内容不是可读 HTML 或纯文本")
	}
	extracted, extractErr := extractResearchDocument(mediaType, fetched.Body)
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
	finalURL, _ := url.Parse(fetched.FinalURL)
	title := NormalizeText(extracted.Title)
	if title == "" {
		title = finalURL.Hostname()
	}
	filename := truncateRunes(title, 180)
	hash := ContentHash(string(fetched.Body) + "\n" + fetched.FinalURL)
	storageKey := strings.Join([]string{strings.ReplaceAll(finalURL.Hostname(), ":", "_"), hash + ".html"}, "/")
	if err := s.storeRaw(storageKey, fetched.Body); err != nil {
		return DocumentView{}, err
	}
	now := time.Now().UTC()
	if !fetched.FetchedAt.IsZero() {
		now = fetched.FetchedAt.UTC()
	}
	httpStatus := fetched.StatusCode
	row := modelfinance.ResearchDocument{
		ID: "doc_" + uuid.NewString(), OwnerID: owner, DraftID: stringPtr(in.DraftID),
		Filename: filename, MediaType: mediaType, ByteSize: int64(len(fetched.Body)), ContentHash: hash,
		StorageKey: storageKey, ExtractionStatus: status, ExtractedText: text, ExtractionError: stringPtr(errorText),
		PageCount: pageCount, OriginType: "url", SourceURL: parsed.String(), CanonicalURL: fetched.FinalURL,
		SourceDomain: finalURL.Hostname(), Title: title, FetchStatus: status, HTTPStatus: &httpStatus, FetchedAt: &now,
		CreatedAt: now, UpdatedAt: now,
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if extractErr != nil {
			return nil
		}
		for i := range extracted.Spans {
			span := modelfinance.ResearchDocumentSpan{
				ID: "span_" + uuid.NewString(), DocumentID: row.ID,
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
				Update("document_id", row.ID).Error
		}
		return nil
	})
	if err != nil {
		return DocumentView{}, err
	}
	return s.view(row)
}

func extractHTMLDocument(raw []byte) (extractedDocument, error) {
	root, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return extractedDocument{}, err
	}
	title := findHTMLTitle(root)
	content := findHTMLContent(root)
	if content == nil {
		content = root
	}
	blocks := htmlBlocks(content)
	var b strings.Builder
	var spans []DocumentSpan
	for _, block := range blocks {
		value := strings.TrimSpace(NormalizeText(block))
		if value == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		start := utf8.RuneCountInString(b.String())
		b.WriteString(value)
		end := utf8.RuneCountInString(b.String())
		index := len(spans)
		spans = append(spans, DocumentSpan{ParagraphIndex: &index, StartOffset: start, EndOffset: end, Text: value})
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return extractedDocument{}, NewError(422, "validation", "WEB_ARTICLE_EMPTY", "网页未提供可读正文")
	}
	return extractedDocument{Title: title, Text: text, Spans: spans}, nil
}

func findHTMLTitle(root *html.Node) string {
	var title string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "title" {
			title = strings.TrimSpace(htmlText(node))
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return title
}

func findHTMLContent(root *html.Node) *html.Node {
	var fallback, body *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type != html.ElementNode {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			return
		}
		id := htmlAttr(node, "id")
		class := htmlAttr(node, "class")
		switch {
		case id == "js_content" || id == "content" || id == "article" ||
			strings.Contains(class, "rich_media_content") || strings.Contains(class, "article-content") || strings.Contains(class, "post-content"):
			if fallback == nil {
				fallback = node
			}
		case node.Data == "article" || node.Data == "main":
			if fallback == nil {
				fallback = node
			}
		case node.Data == "body":
			body = node
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if fallback != nil {
		return fallback
	}
	return body
}

func htmlBlocks(node *html.Node) []string {
	var out []string
	skip := map[string]bool{"script": true, "style": true, "noscript": true, "svg": true, "button": true}
	textBlock := map[string]bool{"p": true, "li": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "blockquote": true, "pre": true}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && skip[node.Data] {
			return
		}
		if node.Type == html.ElementNode && textBlock[node.Data] {
			if value := strings.TrimSpace(htmlText(node)); value != "" {
				out = append(out, value)
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	if len(out) == 0 {
		value := strings.TrimSpace(htmlText(node))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func htmlText(node *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return b.String()
}

func htmlAttr(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

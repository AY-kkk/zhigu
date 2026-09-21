package finance

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

func extractPDFText(raw []byte, maxPages int, maxRunes int) (string, error) {
	if len(raw) < 5 || !bytes.HasPrefix(raw, []byte("%PDF-")) {
		return "", NewError(503, "unavailable", "DATA_UNAVAILABLE", "不是数字 PDF")
	}
	r, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", NewError(503, "unavailable", "DATA_UNAVAILABLE", "无法打开 PDF 文本层")
	}
	if maxPages <= 0 {
		maxPages = 5
	}
	if maxRunes <= 0 {
		maxRunes = 8000
	}
	n := r.NumPage()
	if n < 1 {
		return "", NewError(503, "unavailable", "DATA_UNAVAILABLE", "PDF 无页面")
	}
	var b strings.Builder
	for i := 1; i <= n && i <= maxPages; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		b.WriteString(text)
		b.WriteByte('\n')
		if utf8.RuneCountInString(b.String()) >= maxRunes {
			break
		}
	}
	out := NormalizeText(b.String())
	out = strings.TrimSpace(out)
	if out == "" {
		return "", NewError(503, "unavailable", "DATA_UNAVAILABLE", "PDF 无可用文本层")
	}
	if utf8.RuneCountInString(out) > maxRunes {
		runes := []rune(out)
		out = string(runes[:maxRunes])
	}
	return out, nil
}

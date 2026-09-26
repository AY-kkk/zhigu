package finance

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var unknownLine = regexp.MustCompile(`^(无法取数|口径不一致|展望未绑定|时间闸门|正文不足)：.+$`)

func NormalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = string(norm.NFC.Bytes([]byte(s)))
	return strings.Map(func(r rune) rune {
		if r == utf8.RuneError {
			return -1
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

func UnknownLineOK(s string) bool {
	return unknownLine.MatchString(strings.TrimSpace(s))
}

func NormalizeUnknowns(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func RejectUnknowns(in []string) error {
	if len(in) == 0 {
		return NewError(400, "validation", "UNKNOWN_GATE", "未知项不能为空")
	}
	for _, s := range in {
		if !UnknownLineOK(s) {
			return NewError(400, "validation", "UNKNOWN_GATE", "未知项必须带规定前缀")
		}
	}
	return nil
}

func LooksLikeOutlook(text, claimType string) bool {
	low := text
	if strings.Contains(low, "股价") || strings.Contains(low, "目标价") || strings.Contains(low, "买入") || strings.Contains(low, "卖出") {
		return true
	}
	if claimType == "inference" || claimType == "assumption" {
		if strings.Contains(low, "展望") || strings.Contains(low, "前景") || strings.Contains(low, "未来") || strings.Contains(low, "看好") {
			return true
		}
	}
	return false
}

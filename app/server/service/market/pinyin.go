package market

import (
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

func NameIndex(name string) (full, abbr string) {
	args := pinyin.NewArgs()
	args.Fallback = func(r rune, a pinyin.Args) []string {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return []string{strings.ToLower(string(r))}
		}
		return nil
	}
	parts := pinyin.Pinyin(name, args)
	var b, a strings.Builder
	for _, p := range parts {
		if len(p) == 0 || p[0] == "" {
			continue
		}
		w := strings.ToLower(p[0])
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
		a.WriteByte(w[0])
	}
	return b.String(), a.String()
}

func NormalizeQuery(q string) string {
	return strings.ToUpper(strings.TrimSpace(q))
}

func HKCandidates(q string) []string {
	s := strings.TrimSpace(q)
	s = strings.TrimSuffix(strings.ToUpper(s), ".HK")
	digits := strings.Builder{}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	if d == "" {
		return nil
	}
	for len(d) < 5 {
		d = "0" + d
	}
	if len(d) > 5 {
		return []string{d + ".HK"}
	}
	return []string{d + ".HK"}
}

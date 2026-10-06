package finance

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	InputModeClaimOnly      = "claim_only"
	InputModeReportOnly     = "report_only"
	InputModeClaimAndReport = "claim_and_report"
)

var (
	claimSentenceSplit = regexp.MustCompile(`[。！？!?；;\n]+`)
	numberMentionPattern = regexp.MustCompile(`[-+]?(?:\d+(?:\.\d+)?|\.\d+)\s*(?:%|％|个百分点|万亿元|亿元|万元|元|万|亿|倍)?`)
)

type NumberMention struct {
	Raw         string `json:"raw"`
	Value       string `json:"value"`
	Unit        string `json:"unit"`
	SourceSpanID string `json:"source_span_id,omitempty"`
}

type ClaimParseResult struct {
	InputMode          string
	Items              []ClaimItem
	Numbers            []NumberMention
	InstrumentCandidates []Instrument
}

func inputModeFor(text, documentID string) (string, error) {
	hasText := strings.TrimSpace(text) != ""
	hasDoc := strings.TrimSpace(documentID) != ""
	switch {
	case hasText && hasDoc:
		return InputModeClaimAndReport, nil
	case hasText:
		return InputModeClaimOnly, nil
	case hasDoc:
		return InputModeReportOnly, nil
	default:
		return "", NewError(400, "validation", "MISSING_RESEARCH_INPUT", "观点文本和研报至少提交一项")
	}
}

func extractClaimItems(text, focus string, spans []DocumentSpan) []ClaimItem {
	type candidate struct {
		text   string
		spanID string
		score  int
		order  int
	}
	var candidates []candidate
	add := func(value, spanID string) {
		value = strings.TrimSpace(NormalizeText(value))
		value = strings.TrimSpace(strings.TrimLeft(value, "一二三四五六七八九十0123456789、.．)）:：-*• \t"))
		if utf8.RuneCountInString(value) < 4 || utf8.RuneCountInString(value) > 400 {
			return
		}
		candidates = append(candidates, candidate{text: value, spanID: spanID, order: len(candidates)})
	}
	for _, part := range claimSentenceSplit.Split(text, -1) {
		add(part, "")
	}
	for _, span := range spans {
		for _, part := range claimSentenceSplit.Split(span.Text, -1) {
			add(part, span.SpanID)
		}
	}
	trimmedFocus := strings.TrimSpace(focus)
	if trimmedFocus != "" {
		for i := range candidates {
			candidates[i].score = overlapScore(candidates[i].text, trimmedFocus)
		}
		matched := false
		for _, c := range candidates {
			if c.score > 0 {
				matched = true
				break
			}
		}
		if matched {
			sort.SliceStable(candidates, func(i, j int) bool {
				if candidates[i].score == candidates[j].score {
					return candidates[i].order < candidates[j].order
				}
				return candidates[i].score > candidates[j].score
			})
		}
	}
	seen := map[string]struct{}{}
	items := make([]ClaimItem, 0, 6)
	for _, c := range candidates {
		key := strings.TrimSpace(c.text)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, ClaimItem{
			ClaimID: "claim_" + itoa(len(items)+1), Text: truncateRunes(c.text, 400),
			ClaimType: classifyClaimType(c.text), SourceSpanID: c.spanID,
		})
		if len(items) == 6 {
			break
		}
	}
	return items
}

func classifyClaimType(text string) string {
	inferenceWords := []string{"推动", "导致", "支撑", "证明", "说明", "因此", "所以", "将继续", "会带来", "传导", "意味着"}
	assumptionWords := []string{"假设", "判断", "认为", "预计", "有望", "可能", "或", "如果", "前提"}
	if containsAny(text, inferenceWords) {
		return "inference"
	}
	if containsAny(text, assumptionWords) {
		return "assumption"
	}
	return "fact"
}

func extractNumberMentions(text, spanID string) []NumberMention {
	matches := numberMentionPattern.FindAllString(NormalizeText(text), -1)
	out := make([]NumberMention, 0, len(matches))
	for _, raw := range matches {
		raw = strings.ReplaceAll(raw, " ", "")
		digits := raw
		unit := ""
		for _, suffix := range []string{"个百分点", "万亿元", "亿元", "万元", "%", "％", "元", "万", "亿", "倍"} {
			if strings.HasSuffix(digits, suffix) {
				unit = suffix
				digits = strings.TrimSuffix(digits, suffix)
				break
			}
		}
		out = append(out, NumberMention{Raw: raw, Value: digits, Unit: unit, SourceSpanID: spanID})
	}
	return out
}

func overlapScore(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	score := 0
	set := map[string]struct{}{}
	for i := 0; i+1 < len(br); i++ {
		set[string(br[i:i+2])] = struct{}{}
	}
	for i := 0; i+1 < len(ar); i++ {
		if _, ok := set[string(ar[i:i+2])]; ok {
			score++
		}
	}
	return score
}

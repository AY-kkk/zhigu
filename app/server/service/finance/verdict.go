package finance

import "strings"

type ClaimResult struct {
	ClaimID     string   `json:"claim_id"`
	Verdict     string   `json:"verdict"`
	EvidenceIDs []string `json:"evidence_ids"`
	Reason      string   `json:"reason"`
}

func JudgeClaim(item ClaimItem, support, challenge []Argument) ClaimResult {
	if LooksLikeOutlook(item.Text, item.ClaimType) {
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "insufficient", Reason: "展望或股价预测未绑定已披露事实", EvidenceIDs: nil}
	}
	sup := relevantArgs(item, support)
	chal := relevantArgs(item, challenge)
	supIDs := evidenceIDs(sup)
	chalIDs := evidenceIDs(chal)
	switch {
	case contradicts(sup, chal):
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "mixed", Reason: "同一历史事实存在已核验且未消解的实质矛盾", EvidenceIDs: unique(append(supIDs, chalIDs...))}
	case len(chalIDs) > 0 && len(supIDs) == 0:
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "challenged", Reason: "仅反驳证据成立", EvidenceIDs: chalIDs}
	case len(supIDs) > 0:
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "supported", Reason: "仅支持证据成立", EvidenceIDs: supIDs}
	default:
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "insufficient", Reason: "证据不足", EvidenceIDs: nil}
	}
}

func relevantArgs(item ClaimItem, args []Argument) []Argument {
	if item.ClaimType == "assumption" {
		return nil
	}
	var out []Argument
	for _, a := range args {
		if len(a.EvidenceIDs) == 0 || strings.TrimSpace(a.Text) == "" {
			continue
		}
		if !overlapsClaim(item.Text, a.Text) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func evidenceIDs(args []Argument) []string {
	var ids []string
	for _, a := range args {
		ids = append(ids, a.EvidenceIDs...)
	}
	return unique(ids)
}

func overlapsClaim(claim, argument string) bool {
	claim = strings.TrimSpace(claim)
	argument = strings.TrimSpace(argument)
	if claim == "" || argument == "" {
		return false
	}
	if strings.Contains(claim, argument) || strings.Contains(argument, claim) {
		return true
	}
	for _, subject := range []string{"利润", "营收", "收入", "现金流", "资产", "负债"} {
		if strings.Contains(claim, subject) && strings.Contains(argument, subject) {
			return true
		}
	}
	return false
}

func contradicts(support, challenge []Argument) bool {
	for _, s := range support {
		for _, c := range challenge {
			if !shareEvidence(s, c) && !shareSubject(s.Text, c.Text) {
				continue
			}
			if polarity(s.Text)*polarity(c.Text) < 0 {
				return true
			}
		}
	}
	return false
}

func shareEvidence(a, b Argument) bool {
	seen := map[string]struct{}{}
	for _, id := range a.EvidenceIDs {
		seen[id] = struct{}{}
	}
	for _, id := range b.EvidenceIDs {
		if _, ok := seen[id]; ok {
			return true
		}
	}
	return false
}

func shareSubject(a, b string) bool {
	for _, subject := range []string{"利润", "营收", "收入", "现金流", "资产", "负债"} {
		if strings.Contains(a, subject) && strings.Contains(b, subject) {
			return true
		}
	}
	return false
}

func polarity(text string) int {
	neg := containsAny(text, []string{"下降", "下滑", "减少", "恶化", "并非", "不成立"})
	pos := containsAny(text, []string{"增长", "上升", "改善", "增加"})
	if neg && pos {
		return 0
	}
	if neg {
		return -1
	}
	if pos {
		return 1
	}
	return 0
}

func containsAny(text string, words []string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

func JudgeReport(results []ClaimResult) string {
	hasSup, hasChal, hasMix, hasIns := false, false, false, false
	for _, r := range results {
		switch r.Verdict {
		case "mixed":
			hasMix = true
		case "supported":
			hasSup = true
		case "challenged":
			hasChal = true
		default:
			hasIns = true
		}
	}
	if hasMix || (hasSup && hasChal) {
		return "mixed"
	}
	if hasIns {
		return "insufficient"
	}
	if hasSup && !hasChal {
		return "supported"
	}
	if hasChal && !hasSup {
		return "challenged"
	}
	return "insufficient"
}

func ValidUnknown(s string) bool {
	return UnknownLineOK(s)
}

func ValidateUnknowns(in []string) error {
	return RejectUnknowns(in)
}

func JudgeClaims(items []ClaimItem, support, challenge []Argument) []ClaimResult {
	out := make([]ClaimResult, 0, len(items))
	for _, item := range items {
		out = append(out, JudgeClaim(item, support, challenge))
	}
	return out
}

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
	supIDs := relatedIDs(item, support)
	chalIDs := relatedIDs(item, challenge)
	supOK := len(supIDs) > 0
	chalOK := len(chalIDs) > 0
	switch {
	case supOK && chalOK:
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "mixed", Reason: "同一历史事实存在已核验且未消解的实质矛盾", EvidenceIDs: unique(append(supIDs, chalIDs...))}
	case chalOK && !supOK:
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "challenged", Reason: "仅反驳证据成立", EvidenceIDs: chalIDs}
	case supOK && !chalOK:
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "supported", Reason: "仅支持证据成立", EvidenceIDs: supIDs}
	default:
		return ClaimResult{ClaimID: item.ClaimID, Verdict: "insufficient", Reason: "证据不足", EvidenceIDs: nil}
	}
}

func relatedIDs(item ClaimItem, args []Argument) []string {
	var ids []string
	for _, a := range args {
		if a.ClaimType == "assumption" && len(a.EvidenceIDs) == 0 {
			continue
		}
		if len(a.EvidenceIDs) == 0 {
			continue
		}
		if strings.TrimSpace(a.Text) == "" {
			continue
		}
		ids = append(ids, a.EvidenceIDs...)
	}
	if item.ClaimType == "assumption" {
		return nil
	}
	return unique(ids)
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

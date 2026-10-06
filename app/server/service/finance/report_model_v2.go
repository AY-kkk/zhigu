package finance

import (
	"time"
)

const (
	FactSupported           = "supported"
	FactPrerequisiteMissing = "prerequisite_missing"
	FactContradicted        = "contradicted"
	FactUncertain           = "uncertain"
)

type FactCheck struct {
	ClaimID     string   `json:"claim_id"`
	Status      string   `json:"status"`
	Reason      string   `json:"reason"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type Challenge struct {
	ClaimID     string   `json:"claim_id"`
	Title       string   `json:"title"`
	Argument    string   `json:"argument"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type ReasoningGap struct {
	ClaimID string `json:"claim_id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Missing string `json:"missing"`
}

type TailRisk struct {
	ClaimID     string   `json:"claim_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type TestCondition struct {
	ClaimID          string `json:"claim_id"`
	Metric           string `json:"metric"`
	Baseline         string `json:"baseline"`
	ConfirmDirection string `json:"confirm_direction"`
	FalsifyDirection string `json:"falsify_direction"`
	ReviewAt         string `json:"review_at"`
	Trigger          string `json:"trigger"`
}

type EvidenceRef struct {
	EvidenceID         string `json:"evidence_id"`
	SourceGrade        string `json:"source_grade"`
	VerificationStatus string `json:"verification_status"`
	Relation           string `json:"relation"`
	Locator            string `json:"locator"`
	Title              string `json:"title"`
}

type VerifiedReportV2 = VerifiedReport

func AdjudicateClaim(item ClaimItem, support, challenge []Argument) FactCheck {
	relevantSupport := relevantArgs(item, support)
	relevantChallenge := relevantArgs(item, challenge)
	supIDs := evidenceIDs(relevantSupport)
	chalIDs := evidenceIDs(relevantChallenge)
	allIDs := unique(append(append([]string{}, supIDs...), chalIDs...))

	if LooksLikeOutlook(item.Text, item.ClaimType) {
		return FactCheck{ClaimID: item.ClaimID, Status: FactPrerequisiteMissing, Reason: "展望或预测尚未绑定可核验事实", EvidenceIDs: allIDs}
	}
	if directContradiction(relevantSupport, relevantChallenge) {
		return FactCheck{ClaimID: item.ClaimID, Status: FactContradicted, Reason: "支持与挑战证据存在直接矛盾", EvidenceIDs: allIDs}
	}
	if len(relevantSupport) > 0 && len(relevantChallenge) > 0 {
		return FactCheck{ClaimID: item.ClaimID, Status: FactUncertain, Reason: "支持与挑战证据均存在，尚未消解", EvidenceIDs: allIDs}
	}
	if len(relevantChallenge) > 0 {
		if challengeNegatesClaim(item.Text, relevantChallenge) {
			return FactCheck{ClaimID: item.ClaimID, Status: FactContradicted, Reason: "反方证据直接削弱该主张", EvidenceIDs: chalIDs}
		}
		return FactCheck{ClaimID: item.ClaimID, Status: FactUncertain, Reason: "存在未消解的反方证据", EvidenceIDs: chalIDs}
	}
	if len(relevantSupport) > 0 {
		return FactCheck{ClaimID: item.ClaimID, Status: FactSupported, Reason: "存在直接支持该主张的已登记证据", EvidenceIDs: supIDs}
	}
	if item.ClaimType == "assumption" {
		return FactCheck{ClaimID: item.ClaimID, Status: FactPrerequisiteMissing, Reason: "该假设尚未被独立证据证明", EvidenceIDs: nil}
	}
	return FactCheck{ClaimID: item.ClaimID, Status: FactUncertain, Reason: "证据不足", EvidenceIDs: nil}
}

func AdjudicateClaims(items []ClaimItem, support, challenge []Argument) []FactCheck {
	out := make([]FactCheck, 0, len(items))
	for _, item := range items {
		out = append(out, AdjudicateClaim(item, support, challenge))
	}
	return out
}

func BuildReportV2(runID string, claim Claim, support, challenge []Argument, unknowns, evidenceIDs []string, mode string, asOf string) VerifiedReportV2 {
	factChecks := AdjudicateClaims(claim.Items, support, challenge)
	challenges := make([]Challenge, 0, len(challenge))
	for i, arg := range challenge {
		challenges = append(challenges, Challenge{
			ClaimID: arg.ClaimID, Title: arg.Title,
			Argument: arg.Text, EvidenceIDs: arg.EvidenceIDs,
		})
		if challenges[i].Title == "" {
			challenges[i].Title = "反证 " + itoa(i+1)
		}
	}
	gaps := make([]ReasoningGap, 0)
	assumptions := make([]string, 0)
	testConditions := make([]TestCondition, 0)
	changeConditions := make([]string, 0)
	tailRisks := make([]TailRisk, 0)
	for _, item := range claim.Items {
		if item.ClaimType == "assumption" {
			assumptions = append(assumptions, item.Text)
		}
		if item.ClaimType == "inference" {
			gaps = append(gaps, ReasoningGap{
				ClaimID: item.ClaimID, From: item.Text, To: "研究结论",
				Missing: "尚未用独立证据完整证明“" + item.Text + "”的传导环节。",
			})
		}
		changeConditions = append(changeConditions, "若证实或证伪条件触发，重新评估“"+item.Text+"”。")
		testConditions = append(testConditions, TestCondition{
			ClaimID: item.ClaimID,
			Metric: "与该主张直接相关的已披露基本面、现金流和风险指标",
			Baseline: "以本次研究截止时间的已登记证据为基线",
			ConfirmDirection: "出现独立来源支持“" + item.Text + "”",
			FalsifyDirection: "出现独立来源直接否定“" + item.Text + "”",
			ReviewAt: claim.Horizon,
			Trigger: "出现新公告、财务数据或与原推演相反的证据时重新评估",
		})
		tailRisks = append(tailRisks, TailRisk{
			ClaimID: item.ClaimID,
			Title: "可能推翻主张的未覆盖风险",
			Description: "需要持续跟踪可能否定“" + item.Text + "”的经营、现金流、一次性损益、行业和政策变化。",
		})
	}
	if len(assumptions) == 0 {
		assumptions = []string{"围绕“" + claim.Text + "”未提出可单独核验的明确假设。"}
	}
	return VerifiedReportV2{
		SchemaVersion: "research-report.v2", RunID: runID, Version: 1, Mode: mode,
		AsOf: parseReportTime(asOf), QualityStatus: "incomplete", Verdict: nil,
		Summary: "已生成质证结构，等待证据校验和最终发布。",
		Support: support, Challenge: challenge,
		Assumptions: assumptions, ChangeConditions: changeConditions,
		Unknowns: unknowns, FactChecks: factChecks, Challenges: challenges,
		ReasoningGaps: gaps, TailRisks: tailRisks, TestConditions: testConditions,
		EvidenceIDs: unique(evidenceIDs),
	}
}

func directContradiction(support, challenge []Argument) bool {
	for _, s := range support {
		for _, c := range challenge {
			if shareEvidence(s, c) || shareSubject(s.Text, c.Text) {
				if polarity(s.Text)*polarity(c.Text) < 0 {
					return true
				}
			}
		}
	}
	return false
}

func challengeNegatesClaim(claim string, args []Argument) bool {
	negative := []string{"下降", "下滑", "减少", "恶化", "不成立", "无法", "不足", "削弱", "否定", "并非"}
	for _, arg := range args {
		if containsAny(arg.Text, negative) && overlapScore(claim, arg.Text) > 0 {
			return true
		}
	}
	return false
}

func parseReportTime(value string) (t time.Time) {
	t, _ = time.Parse(time.RFC3339, value)
	return t
}

func verdictFromFactChecks(checks []FactCheck) string {
	if len(checks) == 0 {
		return "insufficient"
	}
	hasSupported := false
	hasChallenge := false
	hasUncertain := false
	for _, check := range checks {
		switch check.Status {
		case FactSupported:
			hasSupported = true
		case FactContradicted:
			hasChallenge = true
		case FactUncertain, FactPrerequisiteMissing:
			hasUncertain = true
		}
	}
	switch {
	case hasChallenge && hasSupported:
		return "partially_supported"
	case hasChallenge:
		return "challenged"
	case hasSupported && hasUncertain:
		return "partially_supported"
	case hasSupported:
		return "supported"
	default:
		return "insufficient"
	}
}

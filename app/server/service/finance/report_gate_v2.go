package finance

import (
	"fmt"
	"html"
	"strings"

	modelfinance "zhigu/server/model/finance"
)

func ValidateReportV2(report VerifiedReport, evidence []modelfinance.Evidence, claim Claim) error {
	if report.SchemaVersion != "research-report.v2" {
		return NewError(400, "validation", "REPORT_SCHEMA", "报告必须使用 research-report.v2")
	}
	if report.RunID == "" || report.Version < 1 || strings.TrimSpace(report.Summary) == "" {
		return NewError(400, "validation", "STRUCTURAL_INVALID", "报告缺少基本字段")
	}
	if report.QualityStatus == "incomplete" && report.Verdict != nil {
		return NewError(400, "validation", "INCOMPLETE_VERDICT", "incomplete 报告不得包含总体结论")
	}
	if report.QualityStatus == "completed" && report.Verdict == nil {
		return NewError(400, "validation", "MISSING_VERDICT", "completed 报告必须包含总体结论")
	}
	allowed := map[string]modelfinance.Evidence{}
	for _, row := range evidence {
		allowed[row.ID] = row
	}
	claimIDs := map[string]struct{}{}
	for _, item := range claim.Items {
		claimIDs[item.ClaimID] = struct{}{}
	}
	for _, check := range report.FactChecks {
		if _, ok := claimIDs[check.ClaimID]; !ok {
			return NewError(400, "validation", "UNKNOWN_CLAIM", "事实核验引用未知主张")
		}
		switch check.Status {
		case FactSupported, FactPrerequisiteMissing, FactContradicted, FactUncertain:
		default:
			return NewError(400, "validation", "INVALID_FACT_STATUS", "事实核验状态无效")
		}
		if err := validateEvidenceIDs(check.EvidenceIDs, allowed); err != nil {
			return err
		}
	}
	if len(claim.Items) > 0 && len(report.FactChecks) != len(claim.Items) {
		return NewError(400, "validation", "REPORT_SECTION_MISSING", "事实核验表未覆盖全部主张")
	}
	for _, section := range []struct {
		name string
		ok   bool
	}{
		{"事实核验", len(report.FactChecks) > 0 || len(claim.Items) == 0},
		{"逐条质疑", true},
		{"推理链缺口", len(report.ReasoningGaps) > 0 || len(claim.Items) == 0},
		{"被忽略的风险", len(report.TailRisks) > 0 || len(claim.Items) == 0},
		{"证实与证伪条件", len(report.TestConditions) > 0 || len(claim.Items) == 0},
		{"证据清单", len(report.EvidenceIndex) > 0 || len(report.EvidenceIDs) == 0},
	} {
		if !section.ok {
			return NewError(400, "validation", "REPORT_SECTION_MISSING", "缺少报告结构块："+section.name)
		}
	}
	for _, arg := range append(append([]Argument{}, report.Support...), report.Challenge...) {
		if (arg.ClaimType == "fact" || arg.ClaimType == "inference") && len(arg.EvidenceIDs) == 0 {
			return NewError(400, "validation", "MISSING_CITATION", "事实/推演必须引用证据")
		}
		if err := validateEvidenceIDs(arg.EvidenceIDs, allowed); err != nil {
			return err
		}
	}
	for _, id := range report.EvidenceIDs {
		if _, ok := allowed[id]; !ok {
			return NewError(400, "validation", "UNREGISTERED_CITATION", "未知引用")
		}
	}
	if err := validateEvidenceIDs(report.EvidenceIDs, allowed); err != nil {
		return err
	}
	for _, ref := range report.EvidenceIndex {
		row, ok := allowed[ref.EvidenceID]
		if !ok {
			return NewError(400, "validation", "UNREGISTERED_CITATION", "证据索引包含未知引用")
		}
		if ref.SourceGrade != row.SourceGrade || ref.VerificationStatus != row.VerificationStatus {
			return NewError(400, "validation", "EVIDENCE_GRADE_MISMATCH", "证据等级与登记记录不一致")
		}
		if row.SourceKind == "user_report" && ref.VerificationStatus != "reported_only" {
			return NewError(400, "validation", "EVIDENCE_GRADE_MISMATCH", "研报证据必须标为 reported_only")
		}
	}
	for _, text := range []string{report.Summary, strings.Join(report.Assumptions, "\n"), strings.Join(report.ChangeConditions, "\n")} {
		if missing := unboundNumbers(text, evidence, report.EvidenceIDs); len(missing) > 0 {
			return NewError(400, "validation", "UNBOUND_NUMBER", "重大数字未绑定证据："+strings.Join(missing, ","))
		}
	}
	return nil
}

func validateEvidenceIDs(ids []string, allowed map[string]modelfinance.Evidence) error {
	for _, id := range ids {
		if _, ok := allowed[id]; !ok {
			return NewError(400, "validation", "UNREGISTERED_CITATION", "未知引用")
		}
	}
	return nil
}

func RenderReportHTML(report VerifiedReport, claim Claim, evidence []modelfinance.Evidence, document *DocumentView) string {
	byID := map[string]modelfinance.Evidence{}
	for _, row := range evidence {
		byID[row.ID] = row
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>投研观点质证报告</title><style>body{font-family:system-ui,-apple-system,sans-serif;max-width:920px;margin:32px auto;padding:0 20px;line-height:1.7;color:#222}h1{font-size:28px}h2{margin-top:32px;border-bottom:1px solid #ddd;padding-bottom:8px}table{width:100%;border-collapse:collapse}th,td{border:1px solid #ddd;padding:8px;text-align:left;vertical-align:top}.meta{color:#666;font-size:14px}.notice{background:#fff4e5;padding:12px;border-left:4px solid #d97706}.small{font-size:13px;color:#666}</style></head><body>`)
	b.WriteString("<h1>投研观点质证报告</h1>")
	b.WriteString(`<p class="notice">本报告仅供研究参考，不构成投资建议。</p>`)
	if document != nil {
		b.WriteString(`<p class="meta">研报：<strong>` + html.EscapeString(document.Filename) + `</strong></p>`)
	}
	b.WriteString(`<p class="meta">原始观点：` + html.EscapeString(claim.Text) + `</p>`)
	b.WriteString(`<p class="meta">数据截止：` + html.EscapeString(report.AsOf.Format("2006-01-02 15:04:05")) + `</p>`)

	writeHeader := func(title string) { b.WriteString("<h2>" + html.EscapeString(title) + "</h2>") }
	writeHeader("核心判断")
	b.WriteString("<p><strong>" + html.EscapeString(verdictLabel(report.Verdict)) + "</strong>：" + html.EscapeString(report.Summary) + "</p>")

	writeHeader("事实核验")
	b.WriteString("<table><thead><tr><th>主张</th><th>状态</th><th>原因</th><th>证据</th></tr></thead><tbody>")
	for _, item := range claim.Items {
		check := FactCheck{ClaimID: item.ClaimID, Status: FactUncertain, Reason: "未提供核验结果"}
		for _, candidate := range report.FactChecks {
			if candidate.ClaimID == item.ClaimID {
				check = candidate
			}
		}
		b.WriteString("<tr><td>" + html.EscapeString(item.Text) + "</td><td>" + html.EscapeString(factStatusLabel(check.Status)) + "</td><td>" + html.EscapeString(check.Reason) + "</td><td>" + html.EscapeString(strings.Join(check.EvidenceIDs, ", ")) + "</td></tr>")
	}
	b.WriteString("</tbody></table>")

	writeHeader("逐条质疑")
	if len(report.Challenges) == 0 {
		b.WriteString("<p>未找到有效反证。</p>")
	}
	for _, item := range report.Challenges {
		b.WriteString("<h3>" + html.EscapeString(item.Title) + "</h3><p>" + html.EscapeString(item.Argument) + "</p><p class=\"small\">证据：" + html.EscapeString(strings.Join(item.EvidenceIDs, ", ")) + "</p>")
	}

	writeHeader("推理链缺口")
	for _, item := range report.ReasoningGaps {
		b.WriteString("<p><strong>" + html.EscapeString(item.From) + " → " + html.EscapeString(item.To) + "：</strong>" + html.EscapeString(item.Missing) + "</p>")
	}
	if len(report.ReasoningGaps) == 0 {
		b.WriteString("<p>未识别到额外推理链缺口。</p>")
	}

	writeHeader("被忽略的风险")
	for _, item := range report.TailRisks {
		b.WriteString("<p><strong>" + html.EscapeString(item.Title) + "：</strong>" + html.EscapeString(item.Description) + "</p>")
	}
	if len(report.TailRisks) == 0 {
		b.WriteString("<p>未发现新的有效风险。</p>")
	}

	writeHeader("证实与证伪条件")
	b.WriteString("<table><thead><tr><th>主张</th><th>指标</th><th>证实方向</th><th>证伪方向</th><th>复核时间</th></tr></thead><tbody>")
	for _, item := range report.TestConditions {
		b.WriteString("<tr><td>" + html.EscapeString(item.ClaimID) + "</td><td>" + html.EscapeString(item.Metric) + "</td><td>" + html.EscapeString(item.ConfirmDirection) + "</td><td>" + html.EscapeString(item.FalsifyDirection) + "</td><td>" + html.EscapeString(item.ReviewAt) + "</td></tr>")
	}
	b.WriteString("</tbody></table>")

	writeHeader("证据清单")
	b.WriteString("<table><thead><tr><th>#</th><th>标题</th><th>来源等级</th><th>核验状态</th><th>定位</th></tr></thead><tbody>")
	for i, ref := range report.EvidenceIndex {
		row := byID[ref.EvidenceID]
		title := ref.Title
		if title == "" {
			title = row.Title
		}
		b.WriteString(fmt.Sprintf("<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>", i+1, html.EscapeString(title), html.EscapeString(ref.SourceGrade), html.EscapeString(ref.VerificationStatus), html.EscapeString(ref.Locator)))
	}
	b.WriteString("</tbody></table>")
	b.WriteString(`<p class="small">报告版本：` + html.EscapeString(fmt.Sprint(report.Version)) + `</p></body></html>`)
	return b.String()
}

func verdictLabel(verdict *string) string {
	if verdict == nil {
		return "证据不足"
	}
	switch *verdict {
	case "supported":
		return "支持"
	case "partially_supported":
		return "部分支持"
	case "challenged":
		return "被挑战"
	case "insufficient":
		return "证据不足"
	default:
		return *verdict
	}
}

func factStatusLabel(status string) string {
	switch status {
	case FactSupported:
		return "成立"
	case FactPrerequisiteMissing:
		return "被前置"
	case FactContradicted:
		return "不成立"
	case FactUncertain:
		return "存疑"
	default:
		return status
	}
}

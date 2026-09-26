package finance

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
)

var materialNumber = regexp.MustCompile(`\d+(?:\.\d+)?(?:%|％|个百分点|万元|亿元|元)`)

func gateReport(tx *gorm.DB, runID string, attempt int, report VerifiedReport, evidence []modelfinance.Evidence) error {
	hash, _ := HashCanonical(report)
	rows := make([]modelfinance.ReportCheck, 0, 4)
	var problems []string
	add := func(pointer, claimType, result string, ids []string, reason string) {
		rawIDs, _ := json.Marshal(ids)
		bindings, _ := json.Marshal(materialNumber.FindAllString(textAt(report, pointer), -1))
		var fail *string
		if reason != "" {
			fail = &reason
		}
		rows = append(rows, modelfinance.ReportCheck{
			ID: "chk_" + uuid.NewString(), RunID: runID, Attempt: attempt,
			CandidateHash: hash, Pointer: pointer, ClaimType: claimType,
			EvidenceIDs: datatypes.JSON(rawIDs), NumericBindings: datatypes.JSON(bindings),
			RuleVersion: "b16_publish_v1", ProgramResult: result, FailureReason: fail,
			CreatedAt: time.Now().UTC(),
		})
		if result != "pass" && reason != "" {
			problems = append(problems, pointer+":"+reason)
		}
	}
	if missing := unboundNumbers(report.Summary, evidence, report.EvidenceIDs); len(missing) > 0 {
		add("/summary", "summary", "fail", report.EvidenceIDs, "未绑定重大数字 "+strings.Join(missing, ","))
	} else {
		add("/summary", "summary", "pass", report.EvidenceIDs, "")
	}
	for i, arg := range report.Support {
		if arg.ClaimType != "fact" && arg.ClaimType != "inference" {
			continue
		}
		if missing := unboundNumbers(arg.Text, evidence, arg.EvidenceIDs); len(missing) > 0 {
			add("/support/"+itoa(i), arg.ClaimType, "fail", arg.EvidenceIDs, "未绑定重大数字 "+strings.Join(missing, ","))
		} else {
			add("/support/"+itoa(i), arg.ClaimType, "pass", arg.EvidenceIDs, "")
		}
	}
	for i, arg := range report.Challenge {
		if arg.ClaimType != "fact" && arg.ClaimType != "inference" {
			continue
		}
		if missing := unboundNumbers(arg.Text, evidence, arg.EvidenceIDs); len(missing) > 0 {
			add("/challenge/"+itoa(i), arg.ClaimType, "fail", arg.EvidenceIDs, "未绑定重大数字 "+strings.Join(missing, ","))
		} else {
			add("/challenge/"+itoa(i), arg.ClaimType, "pass", arg.EvidenceIDs, "")
		}
	}
	if len(rows) > 0 {
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
	}
	if len(problems) > 0 {
		return NewError(400, "validation", "UNBOUND_NUMBER", "重大数字未绑定证据，拒绝发布")
	}
	return nil
}

func textAt(report VerifiedReport, pointer string) string {
	if pointer == "/summary" {
		return report.Summary
	}
	return ""
}

func unboundNumbers(text string, evidence []modelfinance.Evidence, ids []string) []string {
	tokens := materialNumber.FindAllString(text, -1)
	if len(tokens) == 0 {
		return nil
	}
	var missing []string
	for _, token := range tokens {
		if !numberBound(token, evidence, ids) {
			missing = append(missing, token)
		}
	}
	return missing
}

func numberBound(token string, evidence []modelfinance.Evidence, ids []string) bool {
	core := strings.TrimSuffix(token, "个百分点")
	core = strings.TrimRight(core, "%％元")
	core = strings.TrimSuffix(core, "万")
	core = strings.TrimSuffix(core, "亿")
	if core == "" {
		return false
	}
	allowed := map[string]struct{}{}
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	for _, ev := range evidence {
		if _, ok := allowed[ev.ID]; !ok {
			continue
		}
		blob := ev.Text + " " + string(ev.Metrics)
		if strings.Contains(blob, token) || strings.Contains(blob, core) {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

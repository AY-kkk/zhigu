package futures

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

type ExportRecord struct {
	ID            string
	Value         string
	ExportAllowed bool
}

type ExportCalculation struct {
	ID             string
	InputRecordIDs []string
	Value          string
}

type ExportInput struct {
	Title        string
	Text         string
	Records      []ExportRecord
	Calculations []ExportCalculation
}

var externalURL = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s<>"']+`)
var numericToken = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

func ExportHTML(input ExportInput) (string, error) {
	restricted := map[string]bool{}
	restrictedValues := map[string]bool{}
	var recordHTML strings.Builder
	for _, record := range input.Records {
		if !record.ExportAllowed {
			restricted[record.ID] = true
			if strings.TrimSpace(record.Value) != "" {
				restrictedValues[record.Value] = true
			}
			recordHTML.WriteString(`<li>受限数据：原文与数值已隐藏</li>`)
			continue
		}
		recordHTML.WriteString(`<li>` + html.EscapeString(record.ID) + `：` + html.EscapeString(record.Value) + `</li>`)
	}
	var calculationHTML strings.Builder
	for _, calculation := range input.Calculations {
		hidden := false
		for _, id := range calculation.InputRecordIDs {
			if restricted[id] {
				hidden = true
				break
			}
		}
		if hidden {
			calculationHTML.WriteString(`<li>受限数据派生计算已隐藏</li>`)
			continue
		}
		calculationHTML.WriteString(`<li>` + html.EscapeString(calculation.ID) + `：` + html.EscapeString(calculation.Value) + `</li>`)
	}
	title := redactRestrictedProse(input.Title, restrictedValues, len(restricted) > 0)
	cleanText := redactRestrictedProse(input.Text, restrictedValues, len(restricted) > 0)
	cleanText = externalURL.ReplaceAllString(cleanText, "[外部链接已移除]")
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</title></head><body><h1>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</h1><section><h2>研究正文</h2><p>`)
	b.WriteString(strings.ReplaceAll(html.EscapeString(cleanText), "\n", "<br>"))
	b.WriteString(`</p></section><section><h2>证据</h2><ul>`)
	b.WriteString(recordHTML.String())
	b.WriteString(`</ul></section><section><h2>计算</h2><ul>`)
	b.WriteString(calculationHTML.String())
	b.WriteString(`</ul></section></body></html>`)
	return b.String(), nil
}

func redactRestrictedProse(value string, restrictedValues map[string]bool, hasRestricted bool) string {
	for restricted := range restrictedValues {
		value = strings.ReplaceAll(value, restricted, "[受限数据已隐藏]")
	}
	if hasRestricted {
		value = numericToken.ReplaceAllString(value, "[受限数据已隐藏]")
	}
	return value
}

type DeleteImpactState struct {
	Version    string
	Hypotheses []string
}

func ConfirmDeleteImpact(state DeleteImpactState, expectedVersion string, cascade bool) error {
	if state.Version != expectedVersion {
		return ErrRevisionConflict
	}
	if len(state.Hypotheses) > 0 && !cascade {
		return fmt.Errorf("%w: referenced hypotheses require cascade confirmation", ErrInvalidInput)
	}
	return nil
}

type DeletionRequest struct {
	ImpactVersion     string
	CascadeHypotheses bool
}

type DeletionJob struct {
	HiddenAt   time.Time
	PurgeDueAt time.Time
}

func DeleteRunOffline(request DeletionRequest, now time.Time) (DeletionJob, error) {
	if request.ImpactVersion == "" {
		return DeletionJob{}, ErrInvalidInput
	}
	return DeletionJob{HiddenAt: now, PurgeDueAt: now.Add(24 * time.Hour)}, nil
}

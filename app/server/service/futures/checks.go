package futures

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type ConditionKind string

const (
	ConditionMetricCompare ConditionKind = "metric_compare"
	ConditionMetricChange  ConditionKind = "metric_change"
	ConditionConsecutive   ConditionKind = "consecutive_periods"
	ConditionManual        ConditionKind = "manual"
)

type CheckCondition struct {
	Kind      ConditionKind
	SeriesID  string
	Operator  string
	Threshold decimal.Decimal
	Periods   int
	Frequency string
}

type CheckResult struct {
	Result string
	Reason string
}

type CheckEvent struct {
	Notify bool
	Type   string
	Result string
}

func EvaluateCondition(condition CheckCondition, records []ObservationRecord) CheckResult {
	if condition.Kind == ConditionManual {
		return CheckResult{Result: "unknown", Reason: "manual condition requires user registration"}
	}
	if len(records) == 0 {
		return CheckResult{Result: "unknown", Reason: "no observations"}
	}
	switch condition.Kind {
	case ConditionMetricCompare:
		return compare(condition.Operator, records[len(records)-1].Value, condition.Threshold)
	case ConditionMetricChange:
		if len(records) < 2 {
			return CheckResult{Result: "unknown", Reason: "missing previous period"}
		}
		return compare(condition.Operator, records[len(records)-1].Value.Sub(records[len(records)-2].Value), condition.Threshold)
	case ConditionConsecutive:
		if condition.Periods < 1 {
			return CheckResult{Result: "unknown", Reason: "invalid consecutive period count"}
		}
		if len(records) < condition.Periods+1 {
			return CheckResult{Result: "unknown", Reason: "not enough consecutive periods"}
		}
		// The input must be dense and ordered by period; no gap can be skipped.
		for i := 1; i < len(records); i++ {
			if !consecutivePeriods(records[i-1], records[i], condition.Frequency) {
				return CheckResult{Result: "unknown", Reason: "missing or unordered period"}
			}
		}
		tail := records[len(records)-condition.Periods-1:]
		for i := 1; i < len(tail); i++ {
			result := compare(condition.Operator, tail[i].Value.Sub(tail[i-1].Value), condition.Threshold)
			if result.Result != "met" {
				return result
			}
		}
		return CheckResult{Result: "met", Reason: "all consecutive period changes satisfy condition"}
	default:
		return CheckResult{Result: "unknown", Reason: "unsupported condition"}
	}
}

func consecutivePeriods(previous, current ObservationRecord, frequency string) bool {
	if frequency == "" {
		frequency = previous.Frequency
	}
	if frequency == "" {
		frequency = "daily"
	}
	if frequency == "irregular" {
		return previous.PeriodIndex > 0 && current.PeriodIndex == previous.PeriodIndex+1
	}
	previousDate, err := time.Parse("2006-01-02", previous.Period)
	currentDate, err2 := time.Parse("2006-01-02", current.Period)
	if err != nil || err2 != nil {
		return false
	}
	switch frequency {
	case "daily":
		return currentDate.Sub(previousDate) == 24*time.Hour
	case "weekly":
		return currentDate.Sub(previousDate) == 7*24*time.Hour
	case "monthly":
		return previousDate.AddDate(0, 1, 0).Equal(currentDate)
	default:
		return false
	}
}

func compare(operator string, value, threshold decimal.Decimal) CheckResult {
	met := false
	switch operator {
	case "gt":
		met = value.GreaterThan(threshold)
	case "gte":
		met = value.GreaterThanOrEqual(threshold)
	case "lt":
		met = value.LessThan(threshold)
	case "lte":
		met = value.LessThanOrEqual(threshold)
	case "eq":
		met = value.Equal(threshold)
	case "ne":
		met = !value.Equal(threshold)
	default:
		return CheckResult{Result: "unknown", Reason: "invalid operator"}
	}
	if met {
		return CheckResult{Result: "met", Reason: "threshold satisfied"}
	}
	return CheckResult{Result: "not_met", Reason: "threshold not satisfied"}
}

type CheckTracker struct{ previous map[string]string }

func NewCheckTracker() *CheckTracker { return &CheckTracker{previous: map[string]string{}} }

func (t *CheckTracker) Record(hypothesisID string, hypothesisVersion int, conditionID string, version int, result string) CheckEvent {
	key := fmt.Sprintf("%s/%d/%s", hypothesisID, hypothesisVersion, conditionID)
	previous, existed := t.previous[key]
	t.previous[key] = result
	if !existed {
		return CheckEvent{Result: result, Notify: false}
	}
	if previous == result {
		return CheckEvent{Result: result, Notify: false}
	}
	eventType := "revision"
	if strings.TrimSpace(result) == "" {
		eventType = "unknown"
	}
	return CheckEvent{Result: result, Notify: true, Type: eventType}
}

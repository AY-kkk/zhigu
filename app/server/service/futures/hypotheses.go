package futures

import (
	"fmt"
	"strings"
	"time"
)

type HypothesisState struct {
	Version              int
	Lifecycle            string
	UserView             string
	ViewReason           string
	ReviewedCheckVersion *int
	ExpiresAt            time.Time
	NeedsReview          bool
}

func AcknowledgeHypothesis(state HypothesisState, reviewedCheckVersion int) (HypothesisState, error) {
	if state.ReviewedCheckVersion != nil && reviewedCheckVersion < *state.ReviewedCheckVersion {
		return state, ErrRevisionConflict
	}
	next := state
	value := reviewedCheckVersion
	next.ReviewedCheckVersion = &value
	next.NeedsReview = false
	next.Version++
	return next, nil
}

func validateHypothesisConditions(conditions []map[string]any) error {
	if len(conditions) < 2 || len(conditions) > 10 {
		return ErrInvalidInput
	}
	roles := map[string]int{}
	ids := map[string]bool{}
	for _, condition := range conditions {
		id, _ := condition["id"].(string)
		role, _ := condition["role"].(string)
		kind, _ := condition["kind"].(string)
		instruction, _ := condition["instruction"].(string)
		if id == "" || ids[id] || strings.TrimSpace(instruction) == "" {
			return ErrInvalidInput
		}
		ids[id] = true
		if role != "validation" && role != "invalidation" {
			return ErrInvalidInput
		}
		roles[role]++
		switch kind {
		case "metric_compare", "metric_change":
			series, _ := condition["series_id"].(string)
			operator, _ := condition["operator"].(string)
			threshold, _ := condition["threshold"].(string)
			unit, _ := condition["unit"].(string)
			if series == "" || unit == "" || threshold == "" || !validConditionOperator(operator) {
				return ErrInvalidInput
			}
		case "consecutive_periods":
			series, _ := condition["series_id"].(string)
			operator, _ := condition["operator"].(string)
			threshold, _ := condition["threshold"].(string)
			unit, _ := condition["unit"].(string)
			periods, ok := condition["periods"].(float64)
			if series == "" || unit == "" || threshold == "" || !validConditionOperator(operator) || !ok || periods < 2 {
				return ErrInvalidInput
			}
		case "manual":
			if condition["series_id"] != nil || condition["operator"] != nil || condition["threshold"] != nil || condition["unit"] != nil || condition["periods"] != nil {
				return ErrInvalidInput
			}
		default:
			return ErrInvalidInput
		}
	}
	if roles["validation"] < 1 || roles["validation"] > 5 || roles["invalidation"] < 1 || roles["invalidation"] > 5 {
		return ErrInvalidInput
	}
	return nil
}

func validConditionOperator(operator string) bool {
	switch operator {
	case "gt", "gte", "lt", "lte", "eq", "ne":
		return true
	default:
		return false
	}
}

func TransitionHypothesis(state HypothesisState, action string, expectedVersion int) (HypothesisState, error) {
	if state.Version != expectedVersion {
		return state, ErrRevisionConflict
	}
	next := state
	switch action {
	case "activate":
		if state.Lifecycle != "draft" {
			return state, fmt.Errorf("%w: activate", ErrInvalidInput)
		}
		next.Lifecycle = "active"
	case "pause":
		if state.Lifecycle != "active" {
			return state, fmt.Errorf("%w: pause", ErrInvalidInput)
		}
		next.Lifecycle = "paused"
	case "resume":
		if state.Lifecycle != "paused" {
			return state, fmt.Errorf("%w: resume", ErrInvalidInput)
		}
		next.Lifecycle = "active"
	case "close":
		if state.Lifecycle == "deleted" || state.Lifecycle == "closed" {
			return state, fmt.Errorf("%w: close", ErrInvalidInput)
		}
		next.Lifecycle = "closed"
	default:
		return state, fmt.Errorf("%w: unsupported lifecycle action", ErrInvalidInput)
	}
	next.Version++
	return next, nil
}

func DecideHypothesis(state HypothesisState, view, reason string, reviewedCheckVersion int) (HypothesisState, error) {
	if view != "retained" && view != "revised" && view != "rejected" {
		return state, ErrInvalidInput
	}
	if state.ReviewedCheckVersion != nil && reviewedCheckVersion < *state.ReviewedCheckVersion {
		return state, ErrRevisionConflict
	}
	next := state
	next.UserView = view
	next.ViewReason = reason
	value := reviewedCheckVersion
	next.ReviewedCheckVersion = &value
	next.Version++
	return next, nil
}

func ExpireHypothesis(state HypothesisState, now time.Time) HypothesisState {
	if (state.Lifecycle == "active" || state.Lifecycle == "paused") && !state.ExpiresAt.After(now) {
		state.Lifecycle = "expired"
	}
	return state
}

type RecapState struct {
	Facts, Transmission, ContractPerformance, DataSufficiency, Reason string
}

func ValidateRecap(recap RecapState) error {
	for _, value := range []string{recap.Facts, recap.Transmission, recap.ContractPerformance, recap.DataSufficiency, recap.Reason} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: recap requires all fields", ErrInvalidInput)
		}
	}
	if strings.Contains(recap.ContractPerformance, "价格方向") {
		return fmt.Errorf("%w: recap cannot use price direction as the result", ErrInvalidInput)
	}
	return nil
}

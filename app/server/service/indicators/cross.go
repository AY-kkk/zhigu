package indicators

import "github.com/shopspring/decimal"

func CrossesAbove(left, right []*decimal.Decimal, leftReady, rightReady []bool, i int) bool {
	if i < 1 || i >= len(left) || i >= len(right) {
		return false
	}
	if !readyBoth(leftReady, rightReady, i) || !readyBoth(leftReady, rightReady, i-1) {
		return false
	}
	if left[i] == nil || right[i] == nil || left[i-1] == nil || right[i-1] == nil {
		return false
	}
	return left[i-1].LessThanOrEqual(*right[i-1]) && left[i].GreaterThan(*right[i])
}

func CrossesBelow(left, right []*decimal.Decimal, leftReady, rightReady []bool, i int) bool {
	if i < 1 || i >= len(left) || i >= len(right) {
		return false
	}
	if !readyBoth(leftReady, rightReady, i) || !readyBoth(leftReady, rightReady, i-1) {
		return false
	}
	if left[i] == nil || right[i] == nil || left[i-1] == nil || right[i-1] == nil {
		return false
	}
	return left[i-1].GreaterThanOrEqual(*right[i-1]) && left[i].LessThan(*right[i])
}

func readyBoth(a, b []bool, i int) bool {
	if i < 0 || i >= len(a) {
		return false
	}
	ar := a[i]
	br := true
	if b != nil {
		if i >= len(b) {
			return false
		}
		br = b[i]
	}
	return ar && br
}

package futures

import "testing"

func TestOutboxBackoffAndDeadLetter(t *testing.T) {
	want := []int{10, 30, 60, 300, 300, 300, 300, 300, 300, 300}
	for attempt, seconds := range want {
		if got := OutboxBackoff(attempt + 1).Seconds(); got != float64(seconds) {
			t.Fatalf("attempt %d backoff=%v want %d", attempt+1, got, seconds)
		}
	}
	if state := OutboxStateAfterFailure(10, 11); state != "dead" {
		t.Fatalf("state=%s", state)
	}
	if state := OutboxStateAfterFailure(9, 10); state != "pending" {
		t.Fatalf("state=%s", state)
	}
}

package futures

import (
	"encoding/json"
	"strings"
	"testing"

	svc "zhigu/server/service/futures"
)

func TestListEnvelopeAlwaysHasStablePaginationFields(t *testing.T) {
	cursor := "opaque-cursor"
	page := svc.NewPage([]string{"a", "b"}, 20, &cursor, nil)
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"items", "limit", "cursor", "next_cursor"} {
		if _, ok := decoded[field]; !ok {
			t.Fatalf("missing %s in %s", field, raw)
		}
	}
	if decoded["limit"] != float64(20) || decoded["cursor"] != cursor || decoded["next_cursor"] != nil {
		t.Fatalf("unstable pagination envelope: %s", raw)
	}
}

func TestNotificationBatchReadInputRejectsEmptyDuplicateAndOver100(t *testing.T) {
	if err := validateNotificationIDs(nil); err == nil {
		t.Fatal("empty IDs accepted")
	}
	if err := validateNotificationIDs([]string{"a", "a"}); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "notification_" + strings.Repeat("x", i+1)
	}
	if err := validateNotificationIDs(ids); err == nil {
		t.Fatal("over 100 IDs accepted")
	}
	ids = ids[:100]
	for i := range ids {
		ids[i] = "notification_" + strings.Repeat("x", i+1)
	}
	if err := validateNotificationIDs(ids); err != nil {
		t.Fatalf("valid IDs rejected: %v", err)
	}
}

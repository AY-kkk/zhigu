package futures

import "testing"

func TestOutboxEventUsesUnifiedObjectIdentity(t *testing.T) {
	event, err := parseOutboxEvent(map[string]any{"schema_version": "futures.outbox-event.v1", "type": "run_created", "object_type": "run", "object_id": "run-1", "object_version": float64(1), "run_id": "run-1"})
	if err != nil || event.ObjectID != "run-1" || event.ObjectVersion != 1 {
		t.Fatalf("event=%+v err=%v", event, err)
	}
	if _, err := parseOutboxEvent(map[string]any{"type": "run_created", "object_id": "", "object_version": float64(0)}); err == nil {
		t.Fatal("malformed event must fail independently")
	}
}

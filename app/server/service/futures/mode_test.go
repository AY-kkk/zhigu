package futures

import (
	"context"
	"testing"
)

func TestRuntimeModeGatesReadsWritesAndPreservesMaintenance(t *testing.T) {
	off := NewDomain(nil, "").WithMode("off")
	if err := off.AuthorizeAccess(context.Background(), Identity{OwnerID: 1}, "GET", "/api/v1/futures/runs"); err != ErrModuleOff {
		t.Fatalf("off read err=%v", err)
	}
	if err := off.AuthorizeAccess(context.Background(), Identity{OwnerID: 1}, "DELETE", "/api/v1/futures/runs/r1"); err != nil {
		t.Fatalf("off delete err=%v", err)
	}
	if err := off.AuthorizeAccess(context.Background(), Identity{OwnerID: 1}, "POST", "/api/v1/futures/runs/r1/cancel"); err != nil {
		t.Fatalf("off cancel err=%v", err)
	}
	readOnly := NewDomain(nil, "").WithMode("read_only")
	if err := readOnly.AuthorizeAccess(context.Background(), Identity{OwnerID: 1}, "GET", "/api/v1/futures/runs"); err != nil {
		t.Fatal(err)
	}
	if err := readOnly.AuthorizeAccess(context.Background(), Identity{OwnerID: 1}, "POST", "/api/v1/futures/runs"); err != ErrReadOnly {
		t.Fatalf("read-only write err=%v", err)
	}
	live := NewDomain(nil, "").WithMode("live")
	if err := live.AuthorizeAccess(context.Background(), Identity{OwnerID: 1}, "POST", "/api/v1/futures/runs"); err != nil {
		t.Fatal(err)
	}
}

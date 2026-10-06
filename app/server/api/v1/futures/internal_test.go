package futures

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	grant "zhigu/server/service/futures"
)

func TestGrantBindingExpiryReplayAndToolScope(t *testing.T) {
	store := grant.NewMemoryGrantStore()
	lease := grant.TaskLease{Domain: "futures", OwnerID: 1, Mode: "live", RunID: "r1", TaskID: "t1", Generation: 2, ManifestID: "m1", Active: true}
	manager := grant.NewGrantManager(store, grant.StaticLeaseAuthorizer(lease), func() time.Time { return time.Unix(1000, 0) })
	arguments := map[string]any{"record_ids": []string{"obs1"}}
	spec := grant.GrantSpec{
		Domain: "futures", OwnerID: 1, Mode: "live", RunID: "r1", TaskID: "t1", Generation: 2,
		ManifestID: "m1", Stage: "evidence", ToolName: "futures_get_observations", ArgsHash: grant.HashArguments(arguments),
	}
	token, err := manager.Issue(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Use(token, spec); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Use(token, spec); !errors.Is(err, grant.ErrGrantConsumed) {
		t.Fatalf("replay err=%v", err)
	}
	for name, mutate := range map[string]func(*grant.GrantSpec){
		"owner":      func(s *grant.GrantSpec) { s.OwnerID = 2 },
		"run":        func(s *grant.GrantSpec) { s.RunID = "r2" },
		"generation": func(s *grant.GrantSpec) { s.Generation = 3 },
		"manifest":   func(s *grant.GrantSpec) { s.ManifestID = "m2" },
		"args":       func(s *grant.GrantSpec) { s.ArgsHash = hex.EncodeToString(make([]byte, sha256.Size)) },
		"tool":       func(s *grant.GrantSpec) { s.ToolName = "futures_get_evidence" },
	} {
		t.Run(name, func(t *testing.T) {
			badSpec := spec
			mutate(&badSpec)
			token, err := manager.Issue(spec)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Use(token, badSpec); !errors.Is(err, grant.ErrGrantDenied) {
				t.Fatalf("mismatch err=%v", err)
			}
		})
	}
	token, err = manager.Issue(spec)
	if err != nil {
		t.Fatal(err)
	}
	manager = grant.NewGrantManager(store, grant.StaticLeaseAuthorizer(lease), func() time.Time { return time.Unix(1061, 0) })
	if _, err := manager.Use(token, spec); !errors.Is(err, grant.ErrGrantExpired) {
		t.Fatalf("expired err=%v", err)
	}
}

func TestGrantUseRechecksLeaseRevocation(t *testing.T) {
	active := true
	authorizer := grant.LeaseAuthorizerFunc(func(grant.TaskLease) error {
		if !active {
			return grant.ErrGrantDenied
		}
		return nil
	})
	manager := grant.NewGrantManager(grant.NewMemoryGrantStore(), authorizer, func() time.Time { return time.Unix(1000, 0) })
	spec := grant.GrantSpec{
		Domain: "futures", OwnerID: 1, Mode: "live", RunID: "r1", TaskID: "t1", Generation: 2,
		ManifestID: "m1", Stage: "support", ArgsHash: grant.HashArguments(map[string]any{"stage": "support"}),
	}
	token, err := manager.Issue(spec)
	if err != nil {
		t.Fatal(err)
	}
	active = false
	if _, err := manager.Use(token, spec); !errors.Is(err, grant.ErrGrantDenied) {
		t.Fatalf("revoked lease err=%v", err)
	}
}

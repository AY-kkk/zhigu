package strategy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestContractsMatchRuntime asserts the hand-written contract schemas stay in
// sync with the runtime structs that enforce them (§12.7: 必须与运行时校验一致).
func TestContractsMatchRuntime(t *testing.T) {
	base := filepath.Join("..", "..", "..", "..", "contracts", "strategy")

	load := func(name string) map[string]any {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(base, name))
		if err != nil {
			t.Fatalf("read contract %s: %v", name, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("parse contract %s: %v", name, err)
		}
		v, ok := schema["additionalProperties"]
		if strict, isBool := v.(bool); !ok || !isBool || strict {
			t.Fatalf("contract %s must set additionalProperties=false at root", name)
		}
		props, _ := schema["properties"].(map[string]any)
		if len(props) == 0 {
			t.Fatalf("contract %s has no properties", name)
		}
		return props
	}

	assertCovered := func(contract string, props map[string]any, typ reflect.Type) {
		t.Helper()
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if tag == "" || tag == "-" {
				continue
			}
			if _, ok := props[tag]; !ok {
				t.Fatalf("contract %s missing property %q of %s", contract, tag, typ.Name())
			}
		}
	}

	editorProps := load("editor-v1.schema.json")
	assertCovered("editor-v1.schema.json", editorProps, reflect.TypeOf(EditorState{}))

	dslProps := load("dsl-v2.schema.json")
	assertCovered("dsl-v2.schema.json", dslProps, reflect.TypeOf(Document{}))
	if s, _ := dslProps["schema_version"].(map[string]any); s["const"] != SchemaVersionV2 {
		t.Fatalf("dsl-v2 schema_version const = %v, want %s", s, SchemaVersionV2)
	}

	marketProps := load("market-v1.schema.json")
	assertCovered("market-v1.schema.json", marketProps, reflect.TypeOf(MarketTemplate{}))

	// 契约目录四件套齐全（含 OpenAPI）。
	for _, name := range []string{"editor-v1.schema.json", "dsl-v2.schema.json", "market-v1.schema.json", "market.openapi.yaml"} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Fatalf("missing contract file %s: %v", name, err)
		}
	}
}

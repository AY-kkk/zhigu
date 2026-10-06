package initialize

import (
	"strings"
	"testing"
)

func TestResearchViewpointMigration(t *testing.T) {
	db := testDB(t)
	files := realFiles(t)
	if err := MigrateMigrations(db, files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, table := range []string{
		"finance_research_documents",
		"finance_research_document_spans",
		"finance_claim_fact_checks",
	} {
		if !tableExists(t, db, table) {
			t.Fatalf("missing table %s", table)
		}
	}
	for _, col := range []struct {
		table string
		name  string
	}{
		{"finance_claim_drafts", "source_mode"},
		{"finance_claim_drafts", "document_id"},
		{"finance_claim_drafts", "focus_text"},
		{"finance_research_runs", "document_id"},
		{"finance_research_runs", "input_mode"},
	} {
		if !columnExists(t, db, col.table, col.name) {
			t.Fatalf("missing column %s.%s", col.table, col.name)
		}
	}
}

func TestNamedCheckIsVerifiedByConstraintName(t *testing.T) {
	exp, err := parseSchemaObjects(`ALTER TABLE sample ADD CONSTRAINT sample_positive CHECK (value > 0);`)
	if err != nil {
		t.Fatal(err)
	}
	if len(exp.named) != 1 || exp.named[0] != "sample_positive" {
		t.Fatalf("named constraints = %v", exp.named)
	}
	if exp.checks["sample"] != 0 {
		t.Fatalf("named check must not be counted as an unrelated CHECK, got %d", exp.checks["sample"])
	}
}

func TestResearchViewpointMigrationIsPendingOnLegacyBaseline(t *testing.T) {
	files := realFiles(t)
	var migration Migration
	for _, candidate := range files {
		if strings.HasPrefix(candidate.Version, "007_") {
			migration = candidate
			break
		}
	}
	if migration.Version == "" {
		t.Fatal("missing 007 research viewpoint migration")
	}
	exp, err := parseSchemaObjects(migration.SQL)
	if err != nil {
		t.Fatal(err)
	}
	exp = exp.dedup()
	db := testDB(t)
	for _, old := range files[:4] {
		applyRawSQL(t, db, old.SQL)
	}
	cat, err := loadCatalog(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(cat.missing(exp)); got != cat.objectCount(exp) {
		t.Fatalf("legacy baseline must leave all migration objects pending: missing=%d total=%d", got, cat.objectCount(exp))
	}
}

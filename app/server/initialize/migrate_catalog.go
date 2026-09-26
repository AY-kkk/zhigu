package initialize

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// catalog is a read-only snapshot of the public schema used to verify that an
// existing database actually matches what the migration files declare before a
// historical baseline is registered (never "applied because the file exists").
type catalog struct {
	tables  map[string]map[string]bool
	indexes map[string]bool
	named   map[string]bool
	pks     map[string][][]string
	uniques map[string][][]string
	fks     map[string][]fkRef
	checks  map[string]int
}

type catalogColumnRow struct {
	TableName  string
	ColumnName string
}

type catalogIndexRow struct {
	Indexname string
}

type catalogConstraintRow struct {
	Tbl     string
	Conname string
	Contype string
	Cols    string
	Ref     string
}

func loadCatalog(tx *gorm.DB) (*catalog, error) {
	c := &catalog{
		tables:  map[string]map[string]bool{},
		indexes: map[string]bool{},
		named:   map[string]bool{},
		pks:     map[string][][]string{},
		uniques: map[string][][]string{},
		fks:     map[string][]fkRef{},
		checks:  map[string]int{},
	}
	var colRows []catalogColumnRow
	if err := tx.Raw(`SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = 'public'`).Scan(&colRows).Error; err != nil {
		return nil, err
	}
	for _, r := range colRows {
		cols, ok := c.tables[r.TableName]
		if !ok {
			cols = map[string]bool{}
			c.tables[r.TableName] = cols
		}
		cols[r.ColumnName] = true
	}
	var idxRows []catalogIndexRow
	if err := tx.Raw(`SELECT indexname FROM pg_indexes WHERE schemaname = 'public'`).Scan(&idxRows).Error; err != nil {
		return nil, err
	}
	for _, r := range idxRows {
		c.indexes[r.Indexname] = true
	}
	var consRows []catalogConstraintRow
	if err := tx.Raw(`
SELECT con.conrelid::regclass::text AS tbl,
       con.conname AS conname,
       con.contype::text AS contype,
       COALESCE((SELECT string_agg(a.attname, ',' ORDER BY u.ord)
                   FROM unnest(con.conkey) WITH ORDINALITY AS u(attnum, ord)
                   JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = u.attnum), '') AS cols,
       CASE WHEN con.confrelid = 0 THEN '' ELSE con.confrelid::regclass::text END AS ref
  FROM pg_constraint con
 WHERE con.connamespace = 'public'::regnamespace`).Scan(&consRows).Error; err != nil {
		return nil, err
	}
	for _, r := range consRows {
		c.named[r.Conname] = true
		cols := splitCols(r.Cols)
		switch r.Contype {
		case "p":
			c.pks[r.Tbl] = append(c.pks[r.Tbl], cols)
		case "u":
			c.uniques[r.Tbl] = append(c.uniques[r.Tbl], cols)
		case "f":
			c.fks[r.Tbl] = append(c.fks[r.Tbl], fkRef{table: r.Tbl, cols: cols, ref: r.Ref})
		case "c":
			c.checks[r.Tbl]++
		}
	}
	return c, nil
}

// missing lists every declared object not satisfied by the catalog. CHECK
// constraints are verified by per-table count because PostgreSQL rewrites their
// expressions; everything else is matched exactly.
func (c *catalog) missing(e schemaExpect) []string {
	var diffs []string
	tableOK := func(t string) bool { _, ok := c.tables[t]; return ok }
	for _, t := range e.tables {
		if !tableOK(t) {
			diffs = append(diffs, "missing table "+t)
		}
	}
	for _, col := range e.columns {
		if cols, ok := c.tables[col.table]; !ok || !cols[col.name] {
			diffs = append(diffs, "missing column "+col.table+"."+col.name)
		}
	}
	for _, idx := range e.indexes {
		if !c.indexes[idx] {
			diffs = append(diffs, "missing index "+idx)
		}
	}
	for _, n := range e.named {
		if !c.named[n] {
			diffs = append(diffs, "missing constraint "+n)
		}
	}
	for _, pk := range e.pks {
		if !hasKeySet(c.pks[pk.table], pk.cols) {
			diffs = append(diffs, "missing primary key "+keyDesc(pk))
		}
	}
	for _, u := range e.uniques {
		if !hasKeySet(c.uniques[u.table], u.cols) {
			diffs = append(diffs, "missing unique constraint "+keyDesc(u))
		}
	}
	for _, fk := range e.fks {
		if !hasFK(c.fks[fk.table], fk) {
			diffs = append(diffs, "missing foreign key "+fk.table+"("+strings.Join(fk.cols, ",")+") -> "+fk.ref)
		}
	}
	for t, n := range e.checks {
		if c.checks[t] < n {
			diffs = append(diffs, fmt.Sprintf("table %s: want >= %d check constraints, found %d", t, n, c.checks[t]))
		}
	}
	return diffs
}

func (c *catalog) objectCount(e schemaExpect) int {
	n := len(e.tables) + len(e.columns) + len(e.indexes) + len(e.named) + len(e.pks) + len(e.uniques) + len(e.fks)
	for range e.checks {
		n++
	}
	return n
}

func hasKeySet(sets [][]string, want []string) bool {
	for _, s := range sets {
		if sameCols(s, want) {
			return true
		}
	}
	return false
}

func hasFK(fks []fkRef, want fkRef) bool {
	for _, f := range fks {
		if f.ref == want.ref && sameCols(f.cols, want.cols) {
			return true
		}
	}
	return false
}

func sameCols(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, c := range a {
		set[c] = true
	}
	for _, c := range b {
		if !set[c] {
			return false
		}
	}
	return true
}

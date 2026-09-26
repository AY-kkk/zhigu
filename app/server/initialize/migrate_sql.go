package initialize

import (
	"fmt"
	"strings"
)

// splitStatements splits a migration file into complete SQL statements.
// Semicolons inside string literals, dollar-quoted bodies and comments do not
// terminate a statement, so whole files (including plpgsql bodies) execute as written.
func splitStatements(sql string) []string {
	var out []string
	var b strings.Builder
	i := 0
	for i < len(sql) {
		c := sql[i]
		switch {
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			j := i + 2
			for j < len(sql) && sql[j] != '\n' {
				j++
			}
			b.WriteString(sql[i:j])
			i = j
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			j := i + 2
			for j+1 < len(sql) && !(sql[j] == '*' && sql[j+1] == '/') {
				j++
			}
			if j+1 >= len(sql) {
				j = len(sql)
			} else {
				j += 2
			}
			b.WriteString(sql[i:j])
			i = j
		case c == '\'':
			j := i + 1
			for j < len(sql) {
				if sql[j] == '\'' {
					if j+1 < len(sql) && sql[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			b.WriteString(sql[i:j])
			i = j
		case c == '$':
			if end, ok := dollarQuoteEnd(sql, i); ok {
				b.WriteString(sql[i:end])
				i = end
				continue
			}
			b.WriteByte(c)
			i++
		case c == ';':
			if s := strings.TrimSpace(b.String()); s != "" {
				out = append(out, s)
			}
			b.Reset()
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// dollarQuoteEnd returns the end offset (exclusive) of a well-formed dollar-quoted
// body starting at sql[i] == '$'.
func dollarQuoteEnd(sql string, i int) (int, bool) {
	j := i + 1
	for j < len(sql) && (sql[j] == '_' || isASCIILetter(sql[j]) || (j > i+1 && sql[j] >= '0' && sql[j] <= '9')) {
		j++
	}
	if j >= len(sql) || sql[j] != '$' {
		return 0, false
	}
	tag := sql[i : j+1]
	k := strings.Index(sql[j+1:], tag)
	if k < 0 {
		return 0, false
	}
	return j + 1 + k + len(tag), true
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// schemaExpect is the set of schema objects one or more migrations declare.
type schemaExpect struct {
	tables  []string
	columns []columnRef
	indexes []string
	named   []string
	dropped []string
	pks     []keyRef
	uniques []keyRef
	fks     []fkRef
	checks  map[string]int
}

type columnRef struct {
	table string
	name  string
}

type keyRef struct {
	table string
	cols  []string
}

type fkRef struct {
	table string
	cols  []string
	ref   string
}

// parseSchemaObjects extracts the schema objects a migration file declares
// (tables, columns, indexes, named constraints, key sets, FKs, CHECK counts).
// Only object presence is tracked; statements without schema objects (DML) declare nothing.
func parseSchemaObjects(sql string) (schemaExpect, error) {
	exp := schemaExpect{checks: map[string]int{}}
	for _, stmt := range splitStatements(sql) {
		s := skipLead(stmt)
		upper := strings.ToUpper(s)
		switch {
		case strings.HasPrefix(upper, "CREATE TABLE"):
			rest := skipLead(s[len("CREATE TABLE"):])
			if strings.HasPrefix(strings.ToUpper(rest), "IF NOT EXISTS") {
				rest = skipLead(rest[len("IF NOT EXISTS"):])
			}
			name, rest := token(rest)
			inner, _, ok := parenGroup(rest)
			if !ok {
				return exp, fmt.Errorf("cannot parse CREATE TABLE %s", name)
			}
			exp.tables = append(exp.tables, name)
			if err := exp.parseTableBody(name, inner); err != nil {
				return exp, err
			}
		case strings.HasPrefix(upper, "CREATE UNIQUE INDEX"):
			if err := exp.parseCreateIndex(s[len("CREATE UNIQUE INDEX"):]); err != nil {
				return exp, err
			}
		case strings.HasPrefix(upper, "CREATE INDEX"):
			if err := exp.parseCreateIndex(s[len("CREATE INDEX"):]); err != nil {
				return exp, err
			}
		case strings.HasPrefix(upper, "ALTER TABLE"):
			if err := exp.parseAlterTable(s[len("ALTER TABLE"):]); err != nil {
				return exp, err
			}
		case strings.HasPrefix(upper, "INSERT"), strings.HasPrefix(upper, "UPDATE"), strings.HasPrefix(upper, "DELETE"),
			strings.HasPrefix(upper, "DROP"), strings.HasPrefix(upper, "COMMENT"), strings.HasPrefix(upper, "SELECT"),
			strings.HasPrefix(upper, "TRUNCATE"), strings.HasPrefix(upper, "SET"), strings.HasPrefix(upper, "DO"),
			strings.HasPrefix(upper, "CREATE FUNCTION"), strings.HasPrefix(upper, "CREATE OR REPLACE FUNCTION"),
			strings.HasPrefix(upper, "CREATE PROCEDURE"), strings.HasPrefix(upper, "CREATE TRIGGER"):
			// no table-object expectations (functions/triggers are guarded by their host tables)
		default:
			return exp, fmt.Errorf("unsupported migration statement: %q", head(s, 60))
		}
	}
	return exp, nil
}

func (exp *schemaExpect) parseCreateIndex(rest string) error {
	rest = skipLead(rest)
	if strings.HasPrefix(strings.ToUpper(rest), "IF NOT EXISTS") {
		rest = skipLead(rest[len("IF NOT EXISTS"):])
	}
	name, _ := token(rest)
	if name == "" {
		return fmt.Errorf("cannot parse CREATE INDEX")
	}
	exp.indexes = append(exp.indexes, name)
	return nil
}

func (exp *schemaExpect) parseAlterTable(rest string) error {
	rest = skipLead(rest)
	table, rest := token(rest)
	if table == "" {
		return fmt.Errorf("cannot parse ALTER TABLE target")
	}
	rest = skipLead(rest)
	upper := strings.ToUpper(rest)
	switch {
	case strings.HasPrefix(upper, "ADD COLUMN"):
		r := skipLead(rest[len("ADD COLUMN"):])
		if strings.HasPrefix(strings.ToUpper(r), "IF NOT EXISTS") {
			r = skipLead(r[len("IF NOT EXISTS"):])
		}
		col, _ := token(r)
		if col == "" {
			return fmt.Errorf("cannot parse ALTER TABLE %s ADD COLUMN", table)
		}
		exp.columns = append(exp.columns, columnRef{table: table, name: col})
	case strings.HasPrefix(upper, "ADD CONSTRAINT"):
		r := skipLead(rest[len("ADD CONSTRAINT"):])
		name, r := token(r)
		exp.named = append(exp.named, name)
		constraint := skipLead(r)
		if err := exp.parseTableConstraint(table, constraint); err != nil {
			return err
		}
		if strings.HasPrefix(strings.ToUpper(constraint), "CHECK") {
			exp.checks[table]--
			if exp.checks[table] <= 0 {
				delete(exp.checks, table)
			}
		}
		return nil
	case strings.HasPrefix(upper, "ADD "):
		return exp.parseTableConstraint(table, skipLead(rest[len("ADD"):]))
	case strings.HasPrefix(upper, "DROP CONSTRAINT"):
		r := skipLead(rest[len("DROP CONSTRAINT"):])
		if strings.HasPrefix(strings.ToUpper(r), "IF EXISTS") {
			r = skipLead(r[len("IF EXISTS"):])
		}
		name, _ := token(r)
		exp.dropped = append(exp.dropped, name)
		return nil
	default:
		return fmt.Errorf("unsupported ALTER TABLE action on %s: %q", table, head(rest, 40))
	}
	return nil
}

func (exp *schemaExpect) parseTableBody(table, inner string) error {
	for _, part := range splitTopLevel(inner) {
		p := skipLead(part)
		if p == "" {
			continue
		}
		w, r := token(p)
		switch strings.ToUpper(w) {
		case "CONSTRAINT":
			name, r := token(skipLead(r))
			exp.named = append(exp.named, name)
			constraint := skipLead(r)
			if err := exp.parseTableConstraint(table, constraint); err != nil {
				return err
			}
			// A named CHECK is verified by its exact constraint name. Counting it
			// again as an unnamed CHECK lets an unrelated existing constraint
			// incorrectly mark a new migration as already applied.
			if strings.HasPrefix(strings.ToUpper(constraint), "CHECK") {
				exp.checks[table]--
				if exp.checks[table] <= 0 {
					delete(exp.checks, table)
				}
			}
		case "PRIMARY", "UNIQUE", "CHECK", "FOREIGN":
			if err := exp.parseTableConstraint(table, p); err != nil {
				return err
			}
		default:
			exp.columns = append(exp.columns, columnRef{table: table, name: w})
			exp.parseColumnExtras(table, w, r)
		}
	}
	return nil
}

func (exp *schemaExpect) parseTableConstraint(table, s string) error {
	upper := strings.ToUpper(s)
	switch {
	case strings.HasPrefix(upper, "PRIMARY KEY"):
		inner, _, ok := parenGroup(s)
		if !ok {
			return fmt.Errorf("cannot parse PRIMARY KEY on %s", table)
		}
		exp.pks = append(exp.pks, keyRef{table: table, cols: splitCols(inner)})
	case strings.HasPrefix(upper, "UNIQUE"):
		inner, _, ok := parenGroup(s)
		if !ok {
			return fmt.Errorf("cannot parse UNIQUE on %s", table)
		}
		exp.uniques = append(exp.uniques, keyRef{table: table, cols: splitCols(inner)})
	case strings.HasPrefix(upper, "CHECK"):
		if _, _, ok := parenGroup(s); !ok {
			return fmt.Errorf("cannot parse CHECK on %s", table)
		}
		exp.checks[table]++
	case strings.HasPrefix(upper, "FOREIGN KEY"):
		inner, rest, ok := parenGroup(s)
		if !ok {
			return fmt.Errorf("cannot parse FOREIGN KEY on %s", table)
		}
		rest = skipLead(rest)
		if !strings.HasPrefix(strings.ToUpper(rest), "REFERENCES") {
			return fmt.Errorf("cannot parse FOREIGN KEY on %s: missing REFERENCES", table)
		}
		ref, _ := token(skipLead(rest[len("REFERENCES"):]))
		exp.fks = append(exp.fks, fkRef{table: table, cols: splitCols(inner), ref: ref})
	default:
		return fmt.Errorf("unsupported table constraint on %s: %q", table, head(s, 40))
	}
	return nil
}

func (exp *schemaExpect) parseColumnExtras(table, col, def string) {
	upper := " " + strings.ToUpper(def)
	if strings.Contains(upper, "PRIMARY KEY") {
		exp.pks = append(exp.pks, keyRef{table: table, cols: []string{col}})
	}
	if strings.Contains(upper, " UNIQUE") {
		exp.uniques = append(exp.uniques, keyRef{table: table, cols: []string{col}})
	}
	if i := strings.Index(upper, "REFERENCES"); i >= 0 {
		if ref, _ := token(strings.TrimSpace(def[i+len("REFERENCES"):])); ref != "" {
			exp.fks = append(exp.fks, fkRef{table: table, cols: []string{col}, ref: ref})
		}
	}
	if n := strings.Count(upper, "CHECK (") + strings.Count(upper, "CHECK("); n > 0 {
		exp.checks[table] += n
	}
}

func (exp *schemaExpect) merge(other schemaExpect) {
	exp.tables = append(exp.tables, other.tables...)
	exp.columns = append(exp.columns, other.columns...)
	exp.indexes = append(exp.indexes, other.indexes...)
	exp.named = append(exp.named, other.named...)
	exp.dropped = append(exp.dropped, other.dropped...)
	// Only one primary key per table can exist: the latest declaration wins.
	for _, pk := range other.pks {
		replaced := false
		for i := range exp.pks {
			if exp.pks[i].table == pk.table {
				exp.pks[i] = pk
				replaced = true
				break
			}
		}
		if !replaced {
			exp.pks = append(exp.pks, pk)
		}
	}
	exp.uniques = append(exp.uniques, other.uniques...)
	exp.fks = append(exp.fks, other.fks...)
	if exp.checks == nil {
		exp.checks = map[string]int{}
	}
	for t, n := range other.checks {
		exp.checks[t] += n
	}
}

// dedup drops duplicate expectations and names removed by DROP CONSTRAINT.
func (exp schemaExpect) dedup() schemaExpect {
	droppedNamed := map[string]bool{}
	for _, n := range exp.dropped {
		droppedNamed[n] = true
	}
	out := schemaExpect{checks: map[string]int{}}
	seen := map[string]bool{}
	add := func(key string, hit bool, push func()) {
		if hit || seen[key] {
			return
		}
		seen[key] = true
		push()
	}
	for _, t := range exp.tables {
		add("t:"+t, false, func() { out.tables = append(out.tables, t) })
	}
	for _, c := range exp.columns {
		add("c:"+c.table+"."+c.name, false, func() { out.columns = append(out.columns, c) })
	}
	for _, idx := range exp.indexes {
		add("i:"+idx, false, func() { out.indexes = append(out.indexes, idx) })
	}
	for _, n := range exp.named {
		add("n:"+n, droppedNamed[n], func() { out.named = append(out.named, n) })
	}
	for _, pk := range exp.pks {
		add("p:"+keyDesc(pk), false, func() { out.pks = append(out.pks, pk) })
	}
	for _, u := range exp.uniques {
		add("u:"+keyDesc(u), false, func() { out.uniques = append(out.uniques, u) })
	}
	for _, fk := range exp.fks {
		add("f:"+fk.table+"("+strings.Join(fk.cols, ",")+")->"+fk.ref, false, func() { out.fks = append(out.fks, fk) })
	}
	for t, n := range exp.checks {
		out.checks[t] = n
	}
	return out
}

func keyDesc(k keyRef) string {
	return k.table + "(" + strings.Join(k.cols, ",") + ")"
}

func skipLead(s string) string {
	for {
		t := strings.TrimLeft(s, " \t\r\n")
		switch {
		case strings.HasPrefix(t, "--"):
			if n := strings.IndexByte(t, '\n'); n >= 0 {
				t = t[n+1:]
			} else {
				t = ""
			}
		case strings.HasPrefix(t, "/*"):
			if n := strings.Index(t, "*/"); n >= 0 {
				t = t[n+2:]
			} else {
				t = ""
			}
		default:
			return t
		}
		s = t
	}
}

func token(s string) (string, string) {
	s = strings.TrimLeft(s, " \t\r\n")
	if s == "" {
		return "", ""
	}
	if s[0] == '"' {
		if n := strings.IndexByte(s[1:], '"'); n >= 0 {
			return s[1 : 1+n], s[2+n:]
		}
		return s[1:], ""
	}
	i := 0
	for i < len(s) {
		c := s[i]
		if isASCIILetter(c) || c == '_' || (i > 0 && c >= '0' && c <= '9') || c == '.' {
			i++
			continue
		}
		break
	}
	return s[:i], s[i:]
}

func splitCols(inner string) []string {
	var out []string
	for _, p := range strings.Split(inner, ",") {
		c, _ := token(p)
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

func splitTopLevel(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

func parenGroup(s string) (inner string, rest string, ok bool) {
	i := strings.IndexByte(s, '(')
	if i < 0 {
		return "", s, false
	}
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[i+1 : j], s[j+1:], true
			}
		}
	}
	return "", s, false
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

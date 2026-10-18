package httpx

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type registeredError struct {
	Code         string   `json:"code"`
	HTTPStatus   int      `json:"http_status"`
	HTTPStatuses []int    `json:"http_statuses"`
	Retryable    *bool    `json:"retryable"`
	MessageKey   string   `json:"message_key"`
	Interfaces   []string `json:"interfaces"`
}

func serviceErrorCodes(t *testing.T) map[string]map[int]bool {
	t.Helper()
	found := map[string]map[int]bool{}
	codePattern := regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*$`)
	add := func(expr ast.Expr, status int) {
		literal, ok := expr.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return
		}
		code, err := strconv.Unquote(literal.Value)
		if err != nil || !codePattern.MatchString(code) {
			return
		}
		if found[code] == nil {
			found[code] = map[int]bool{}
		}
		if status > 0 {
			found[code][status] = true
		}
	}
	statusOf := func(expr ast.Expr) int {
		if lit, ok := expr.(*ast.BasicLit); ok {
			n, _ := strconv.Atoi(lit.Value)
			return n
		}
		if sel, ok := expr.(*ast.SelectorExpr); ok {
			return map[string]int{"StatusBadRequest": 400, "StatusUnauthorized": 401, "StatusForbidden": 403, "StatusNotFound": 404, "StatusMethodNotAllowed": 405, "StatusConflict": 409, "StatusGone": 410, "StatusRequestEntityTooLarge": 413, "StatusUnprocessableEntity": 422, "StatusTooManyRequests": 429, "StatusInternalServerError": 500, "StatusBadGateway": 502, "StatusServiceUnavailable": 503, "StatusGatewayTimeout": 504}[sel.Sel.Name]
		}
		return 0
	}
	err := filepath.WalkDir("..", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CallExpr:
				name := ""
				switch fun := value.Fun.(type) {
				case *ast.Ident:
					name = fun.Name
				case *ast.SelectorExpr:
					name = fun.Sel.Name
				}
				switch name {
				case "Fail", "FailRetry":
					if len(value.Args) > 3 {
						add(value.Args[2], statusOf(value.Args[1]))
					}
				case "NewError":
					if len(value.Args) > 3 {
						add(value.Args[2], statusOf(value.Args[0]))
					}
				case "newError":
					if len(value.Args) > 2 {
						add(value.Args[1], statusOf(value.Args[0]))
					}
				default:
					lower := strings.ToLower(name)
					if strings.Contains(lower, "fail") || strings.Contains(lower, "error") || strings.Contains(lower, "reject") {
						for _, arg := range value.Args {
							add(arg, 0)
						}
					}
				}
			case *ast.KeyValueExpr:
				if key, ok := value.Key.(*ast.Ident); ok && (key.Name == "Code" || key.Name == "ErrorCode" || key.Name == "code") {
					add(value.Value, 0)
				}
			case *ast.AssignStmt:
				for index, lhs := range value.Lhs {
					name := ""
					switch target := lhs.(type) {
					case *ast.Ident:
						name = target.Name
					case *ast.SelectorExpr:
						name = target.Sel.Name
					}
					if (name == "code" || name == "Code" || name == "errorCode" || name == "ErrorCode") && index < len(value.Rhs) {
						add(value.Rhs[index], 0)
					}
				}
			case *ast.ValueSpec:
				for index, name := range value.Names {
					if strings.Contains(strings.ToLower(name.Name), "errorcode") && index < len(value.Values) {
						add(value.Values[index], 0)
					}
				}
			case *ast.BinaryExpr:
				for _, pair := range [][2]ast.Expr{{value.X, value.Y}, {value.Y, value.X}} {
					if selector, ok := pair[0].(*ast.SelectorExpr); ok && (selector.Sel.Name == "Code" || selector.Sel.Name == "ErrorCode") {
						add(pair[1], 0)
					}
				}
			case *ast.FuncDecl:
				if value.Name.Name == "ErrorCode" {
					ast.Inspect(value.Body, func(n ast.Node) bool {
						if ret, ok := n.(*ast.ReturnStmt); ok {
							for _, expr := range ret.Results {
								add(expr, 0)
							}
						}
						return true
					})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestErrorRegistryCoversGoServices(t *testing.T) {
	codes := serviceErrorCodes(t)
	raw, err := os.ReadFile("../../contracts/error-codes.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		SchemaVersion string            `json:"schema_version"`
		Errors        []registeredError `json:"errors"`
	}
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatal(err)
	}
	if registry.SchemaVersion != "error-codes.v1" {
		t.Fatal("registry version absent")
	}
	registered := map[string]registeredError{}
	for _, item := range registry.Errors {
		if _, exists := registered[item.Code]; exists {
			t.Errorf("duplicate code %s", item.Code)
		}
		if item.Code == "" || item.HTTPStatus < 400 || item.HTTPStatus > 599 || item.Retryable == nil || item.MessageKey == "" || len(item.Interfaces) == 0 {
			t.Errorf("incomplete registry item %+v", item)
		}
		registered[item.Code] = item
	}
	for code, statuses := range codes {
		item, ok := registered[code]
		if !ok {
			t.Errorf("unregistered Go error %s", code)
			continue
		}
		for status := range statuses {
			if status < 400 || status > 599 {
				continue
			}
			accepted := status == item.HTTPStatus
			for _, s := range item.HTTPStatuses {
				accepted = accepted || s == status
			}
			if !accepted {
				t.Errorf("%s emits unregistered status %d", code, status)
			}
		}
	}
	for _, code := range []string{"CSRF_INVALID", "RATE_LIMITED", "NOT_READY", "METHOD_NOT_ALLOWED", "FUTURES_SOURCE_UNAVAILABLE"} {
		if _, ok := registered[code]; !ok {
			t.Errorf("required error absent %s", code)
		}
	}
}

func TestErrorRegistryDeclaresActualInternalHandlerInterfaces(t *testing.T) {
	raw, err := os.ReadFile("../../contracts/error-codes.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Errors []registeredError `json:"errors"`
	}
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatal(err)
	}
	declarations := map[string][]string{}
	for _, entry := range registry.Errors {
		declarations[entry.Code] = entry.Interfaces
	}
	file, err := parser.ParseFile(token.NewFileSet(), "../api/v1/finance/handlers.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]string{}
	handlers := map[string][]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		if assignment, ok := node.(*ast.AssignStmt); ok && len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 {
			if lhs, ok := assignment.Lhs[0].(*ast.Ident); ok {
				if call, ok := assignment.Rhs[0].(*ast.CallExpr); ok {
					if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Group" && len(call.Args) > 0 {
						if literal, ok := call.Args[0].(*ast.BasicLit); ok {
							groups[lhs.Name], _ = strconv.Unquote(literal.Value)
						}
					}
				}
			}
		}
		if call, ok := node.(*ast.CallExpr); ok && len(call.Args) >= 2 {
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
				if group, ok := selector.X.(*ast.Ident); ok && strings.HasPrefix(groups[group.Name], "/internal") {
					if path, ok := call.Args[0].(*ast.BasicLit); ok {
						if handler, ok := call.Args[len(call.Args)-1].(*ast.SelectorExpr); ok {
							suffix, _ := strconv.Unquote(path.Value)
							handlers[handler.Sel.Name] = append(handlers[handler.Sel.Name], groups[group.Name]+suffix)
						}
					}
				}
			}
		}
		return true
	})
	// Propagate route ownership through receiver helpers (ProxyLLMChat and
	// ProxyLLMResponses both delegate validation to a.proxyLLM).
	for pass := 0; pass < len(file.Decls); pass++ {
		changed := false
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List[0].Names) == 0 || len(handlers[function.Name.Name]) == 0 {
				continue
			}
			receiver := function.Recv.List[0].Names[0].Name
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				owner, ok := selector.X.(*ast.Ident)
				if !ok || owner.Name != receiver {
					return true
				}
				for _, route := range handlers[function.Name.Name] {
					exists := false
					for _, known := range handlers[selector.Sel.Name] {
						exists = exists || known == route
					}
					if !exists {
						handlers[selector.Sel.Name] = append(handlers[selector.Sel.Name], route)
						changed = true
					}
				}
				return true
			})
		}
		if !changed {
			break
		}
	}
	checked := 0
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || len(handlers[function.Name.Name]) == 0 {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 4 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Fail" {
				return true
			}
			literal, ok := call.Args[2].(*ast.BasicLit)
			if !ok {
				return true
			}
			code, _ := strconv.Unquote(literal.Value)
			for _, route := range handlers[function.Name.Name] {
				matched := false
				for _, allowed := range declarations[code] {
					matched = matched || route == allowed || strings.HasSuffix(allowed, "*") && strings.HasPrefix(route, strings.TrimSuffix(allowed, "*"))
				}
				if !matched {
					t.Errorf("%s handler %s emits %s without registry interface permission", route, function.Name.Name, code)
				}
				checked++
			}
			return true
		})
	}
	if checked < 5 {
		t.Fatalf("route/error AST scan ineffective: checked %d", checked)
	}
}

func TestEnvelopeStableErrorAndTrace(t *testing.T) {
	for _, status := range []int{400, 429, 503} {
		w := httptest.NewRecorder()
		r := gin.New()
		r.GET("/", func(c *gin.Context) { Fail(c, status, "INVALID_JSON", "safe") })
		r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"data", "error", "trace_id"} {
			if _, ok := body[key]; !ok {
				t.Errorf("missing %s", key)
			}
		}
		id, _ := body["trace_id"].(string)
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("generated trace is not UUID: %s", id)
		}
		if w.Header().Get("X-Trace-ID") != id {
			t.Errorf("trace header/body mismatch")
		}
		e := body["error"].(map[string]any)
		if _, ok := e["retryable"].(bool); !ok {
			t.Errorf("retryable is mandatory")
		}
		if status == 429 || status == 503 {
			if e["retry_after_seconds"] == nil || w.Header().Get("Retry-After") == "" {
				t.Errorf("temporary error lacks retry delay")
			}
		}
	}
}

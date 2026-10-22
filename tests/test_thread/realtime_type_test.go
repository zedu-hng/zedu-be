package test_thread

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

// Publishing a different type from the stored document makes system notices
// render as ordinary messages until the client reloads. Check this invariant
// without requiring the PostgreSQL, search and realtime services.
func TestStoredAndPublishedThreadTypesMatch(t *testing.T) {
	_, testFile, _, _ := runtime.Caller(0)
	fset := token.NewFileSet()
	source, err := parser.ParseFile(fset, filepath.Join(filepath.Dir(testFile), "../../services/thread/mentions.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	for _, declaration := range source.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "SaveThreadMessage" {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "ThreadDocument" && selector.Sel.Name != "FeedMessageRequest") {
				return true
			}
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := pair.Key.(*ast.Ident)
				if ok && key.Name == "Type" {
					var expression bytes.Buffer
					if err := format.Node(&expression, fset, pair.Value); err != nil {
						t.Fatal(err)
					}
					fields[selector.Sel.Name] = expression.String()
				}
			}
			return true
		})
	}
	stored, published := fields["ThreadDocument"], fields["FeedMessageRequest"]
	if stored == "" || published == "" {
		t.Fatal("missing stored or published thread type")
	}
	if stored != published {
		t.Fatalf("stored thread type %s differs from published type %s", stored, published)
	}
}

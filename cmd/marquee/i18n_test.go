package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"marquee/internal/i18n"
)

// Every Bulgarian text the hub writes needs its English: the binary links all
// packages, so all their registrations are in place here.
func TestEveryBulgarianTextHasEnglish(t *testing.T) {
	fset := token.NewFileSet()
	for _, root := range []string{"../../internal", "."} {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.HasPrefix(filepath.Base(path), "en_") ||
				filepath.Base(path) == "app.go" { // the /app page carries its own texts
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil || !hasCyrillic(s) {
					return true
				}
				if strings.Contains(s, "<!doctype") { // templates: their {{t "..."}} texts
					for _, part := range strings.Split(s, `{{t "`)[1:] {
						check(t, fset.Position(lit.Pos()).String(), part[:strings.Index(part, `"`)])
					}
					return true
				}
				check(t, fset.Position(lit.Pos()).String(), s)
				return true
			})
			return nil
		})
	}
	if checked < 100 {
		t.Fatalf("only %d texts found", checked)
	}
}

var checked int

func check(t *testing.T, pos, s string) {
	checked++
	if i18n.In("en", s) == s {
		t.Errorf("%s: no English for %q", pos, s)
	}
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

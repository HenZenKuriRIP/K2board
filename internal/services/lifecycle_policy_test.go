package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Guardrail: AutoDisableExpiredUsers must never Update("enable", false).
// Regression test against re-introducing expire→ban coupling.
func TestAutoDisableExpiredUsers_SourceDoesNotDisableEnable(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	srcPath := filepath.Join(filepath.Dir(thisFile), "lifecycle_svc.go")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)

	// Function body of AutoDisableExpiredUsers should not contain enable=false updates.
	// Parse AST for stronger check.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, srcPath, src, 0)
	if err != nil {
		t.Fatal(err)
	}

	var foundFunc bool
	var badAssign bool
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "AutoDisableExpiredUsers" {
			return true
		}
		foundFunc = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			// look for Update("enable", false) call patterns via source snippet of body
			return true
		})
		return false
	})
	if !foundFunc {
		t.Fatal("AutoDisableExpiredUsers not found")
	}

	// Body text heuristics: must not write enable=false for expiry.
	// Allow comments mentioning the old behavior.
	start := strings.Index(text, "func AutoDisableExpiredUsers")
	if start < 0 {
		t.Fatal("function missing")
	}
	rest := text[start:]
	end := strings.Index(rest, "\nfunc ")
	if end > 0 {
		rest = rest[:end]
	}
	// Strip line comments for heuristic scan
	var code strings.Builder
	for _, line := range strings.Split(rest, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		code.WriteString(line)
		code.WriteByte('\n')
	}
	body := code.String()
	if strings.Contains(body, `Update("enable", false)`) ||
		strings.Contains(body, `Update("enable",false)`) ||
		strings.Contains(body, `"enable": false`) ||
		strings.Contains(body, `"enable":false`) {
		badAssign = true
	}
	if badAssign {
		t.Fatal("AutoDisableExpiredUsers must not set enable=false (expire/ban separation)")
	}
	// Must call repair path or return 0 without disable
	if !strings.Contains(body, "repairLegacyAutoDisabledExpiredUsersOnce") {
		t.Fatal("expected one-time legacy repair entrypoint")
	}
}

// fulfillUser must not force enable=true (ban bypass via payment).
func TestFulfillUser_SourceDoesNotForceEnable(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	srcPath := filepath.Join(filepath.Dir(thisFile), "order_svc.go")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func fulfillUser(")
	if start < 0 {
		t.Fatal("fulfillUser missing")
	}
	rest := text[start:]
	// next top-level func after fulfillUser
	end := strings.Index(rest[1:], "\nfunc ")
	if end > 0 {
		rest = rest[:end+1]
	}
	var code strings.Builder
	for _, line := range strings.Split(rest, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		code.WriteString(line)
		code.WriteByte('\n')
	}
	body := code.String()
	if strings.Contains(body, `"enable":`) && strings.Contains(body, `true`) {
		// tighter: map key enable true
		if strings.Contains(body, `"enable":                true`) ||
			strings.Contains(body, `"enable": true`) ||
			strings.Contains(body, `"enable":true`) {
			t.Fatal("fulfillUser must not force enable=true (would unban on pay)")
		}
	}
}

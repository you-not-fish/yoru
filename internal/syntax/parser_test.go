package syntax

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// Test helpers

func parseFile(t *testing.T, src string) *File {
	t.Helper()
	p := NewParser("test.yoru", strings.NewReader(src), nil)
	f := p.Parse()
	if f == nil {
		t.Fatal("Parse returned nil")
	}
	return f
}

func parseFileWithErrors(t *testing.T, src string) (*File, []string) {
	t.Helper()
	var errs []string
	errh := func(pos Pos, msg string) {
		errs = append(errs, pos.String()+": "+msg)
	}
	p := NewParser("test.yoru", strings.NewReader(src), errh)
	f := p.Parse()
	return f, errs
}

type parseError struct {
	pos Pos
	msg string
}

func parseFileWithErrorDetails(t *testing.T, src string) (*File, []parseError) {
	t.Helper()
	var errs []parseError
	errh := func(pos Pos, msg string) {
		errs = append(errs, parseError{pos: pos, msg: msg})
	}
	p := NewParser("test.yoru", strings.NewReader(src), errh)
	f := p.Parse()
	return f, errs
}

// ----------------------------------------------------------------------------
// Phase 2.1: Basic parsing tests

func TestParsePackage(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantPkg string
	}{
		{"simple", "package main", "main"},
		{"other_name", "package foo", "foo"},
		{"with_newline", "package main\n", "main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFile(t, tt.src)
			if f.PkgName == nil {
				t.Fatal("PkgName is nil")
			}
			if f.PkgName.Value != tt.wantPkg {
				t.Errorf("PkgName = %q, want %q", f.PkgName.Value, tt.wantPkg)
			}
		})
	}
}

func TestParseImport(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantPath string
	}{
		{"simple", "package main\nimport \"fmt\"", "fmt"},
		{"with_path", "package main\nimport \"github.com/foo/bar\"", "github.com/foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFile(t, tt.src)
			if len(f.Imports) != 1 {
				t.Fatalf("got %d imports, want 1", len(f.Imports))
			}
			if f.Imports[0].Path == nil {
				t.Fatal("import path is nil")
			}
			if f.Imports[0].Path.Value != tt.wantPath {
				t.Errorf("import path = %q, want %q", f.Imports[0].Path.Value, tt.wantPath)
			}
		})
	}
}

func TestParseMultipleImports(t *testing.T) {
	src := `package main
import "fmt"
import "os"
import "strings"
`
	f := parseFile(t, src)
	if len(f.Imports) != 3 {
		t.Fatalf("got %d imports, want 3", len(f.Imports))
	}
	wantPaths := []string{"fmt", "os", "strings"}
	for i, want := range wantPaths {
		if f.Imports[i].Path.Value != want {
			t.Errorf("import[%d] = %q, want %q", i, f.Imports[i].Path.Value, want)
		}
	}
}

func TestParseTypeDecl(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantName  string
		wantAlias bool
	}{
		{
			"struct",
			"package main\ntype Point struct { x int }",
			"Point",
			false,
		},
		{
			"alias",
			"package main\ntype Number = int",
			"Number",
			true,
		},
		{
			"pointer",
			"package main\ntype IntPtr = *int",
			"IntPtr",
			true,
		},
		{
			"ref",
			"package main\ntype RefPoint = ref Point",
			"RefPoint",
			true,
		},
		{
			"array",
			"package main\ntype Arr = [10]int",
			"Arr",
			true,
		},
		{
			"pointer_pointer",
			"package main\ntype PtrPtr = **int",
			"PtrPtr",
			true,
		},
		{
			"ref_pointer",
			"package main\ntype RefPtr = ref *Point",
			"RefPtr",
			true,
		},
		{
			"array_of_array",
			"package main\ntype Matrix = [3][4]int",
			"Matrix",
			true,
		},
		{
			"array_of_pointer",
			"package main\ntype PtrArr = [5]*int",
			"PtrArr",
			true,
		},
		{
			"empty_struct",
			"package main\ntype Empty struct {}",
			"Empty",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFile(t, tt.src)
			if len(f.Decls) != 1 {
				t.Fatalf("got %d decls, want 1", len(f.Decls))
			}
			td, ok := f.Decls[0].(*TypeDecl)
			if !ok {
				t.Fatalf("decl is %T, want *TypeDecl", f.Decls[0])
			}
			if td.Name.Value != tt.wantName {
				t.Errorf("name = %q, want %q", td.Name.Value, tt.wantName)
			}
			if td.Alias != tt.wantAlias {
				t.Errorf("alias = %v, want %v", td.Alias, tt.wantAlias)
			}
		})
	}
}

func TestParseStructFields(t *testing.T) {
	src := `package main
type Person struct {
	name string
	age int
	active bool
}
`
	f := parseFile(t, src)
	if len(f.Decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(f.Decls))
	}
	td := f.Decls[0].(*TypeDecl)
	st, ok := td.Type.(*StructType)
	if !ok {
		t.Fatalf("type is %T, want *StructType", td.Type)
	}
	if len(st.Fields) != 3 {
		t.Fatalf("got %d fields, want 3", len(st.Fields))
	}
	wantFields := []struct {
		name string
		typ  string
	}{
		{"name", "string"},
		{"age", "int"},
		{"active", "bool"},
	}
	for i, want := range wantFields {
		if st.Fields[i].Name.Value != want.name {
			t.Errorf("field[%d].Name = %q, want %q", i, st.Fields[i].Name.Value, want.name)
		}
		typeName, ok := st.Fields[i].Type.(*Name)
		if !ok {
			t.Errorf("field[%d].Type is %T, want *Name", i, st.Fields[i].Type)
			continue
		}
		if typeName.Value != want.typ {
			t.Errorf("field[%d].Type = %q, want %q", i, typeName.Value, want.typ)
		}
	}
}

func TestParseVarDecl(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantName  string
		wantType  bool // whether Type is set
		wantValue bool // whether Value is set
	}{
		{
			"typed",
			"package main\nvar x int",
			"x",
			true,
			false,
		},
		{
			"typed_init",
			"package main\nvar x int = 1",
			"x",
			true,
			true,
		},
		{
			"inferred",
			"package main\nvar x = 1",
			"x",
			false,
			true,
		},
		{
			"pointer_type",
			"package main\nvar p *int",
			"p",
			true,
			false,
		},
		{
			"ref_type",
			"package main\nvar r ref Point",
			"r",
			true,
			false,
		},
		{
			"array_type",
			"package main\nvar arr [10]int",
			"arr",
			true,
			false,
		},
		{
			"ref_with_new",
			"package main\nvar r ref Point = new(Point)",
			"r",
			true,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFile(t, tt.src)
			if len(f.Decls) != 1 {
				t.Fatalf("got %d decls, want 1", len(f.Decls))
			}
			vd, ok := f.Decls[0].(*VarDecl)
			if !ok {
				t.Fatalf("decl is %T, want *VarDecl", f.Decls[0])
			}
			if vd.Name.Value != tt.wantName {
				t.Errorf("name = %q, want %q", vd.Name.Value, tt.wantName)
			}
			if (vd.Type != nil) != tt.wantType {
				t.Errorf("Type set = %v, want %v", vd.Type != nil, tt.wantType)
			}
			if (vd.Value != nil) != tt.wantValue {
				t.Errorf("Value set = %v, want %v", vd.Value != nil, tt.wantValue)
			}
		})
	}
}

func TestParseFuncDecl(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantName   string
		wantRecv   bool
		wantParams int
		wantResult bool
	}{
		{
			"simple",
			"package main\nfunc foo() {}",
			"foo",
			false,
			0,
			false,
		},
		{
			"with_params",
			"package main\nfunc add(a int, b int) int { return a }",
			"add",
			false,
			2,
			true,
		},
		{
			"method",
			"package main\nfunc (p Point) Area() int { return 0 }",
			"Area",
			true,
			0,
			true,
		},
		{
			"ptr_receiver",
			"package main\nfunc (p *Point) Move() { }",
			"Move",
			true,
			0,
			false,
		},
		{
			"ref_receiver",
			"package main\nfunc (p ref Point) Reset() { }",
			"Reset",
			true,
			0,
			false,
		},
		{
			"many_params",
			"package main\nfunc process(a int, b int, c int, d string) bool { return true }",
			"process",
			false,
			4,
			true,
		},
		{
			"returns_pointer",
			"package main\nfunc getPtr() *int { return nil }",
			"getPtr",
			false,
			0,
			true,
		},
		{
			"returns_ref",
			"package main\nfunc create() ref Point { return new(Point) }",
			"create",
			false,
			0,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFile(t, tt.src)
			if len(f.Decls) != 1 {
				t.Fatalf("got %d decls, want 1", len(f.Decls))
			}
			fd, ok := f.Decls[0].(*FuncDecl)
			if !ok {
				t.Fatalf("decl is %T, want *FuncDecl", f.Decls[0])
			}
			if fd.Name.Value != tt.wantName {
				t.Errorf("name = %q, want %q", fd.Name.Value, tt.wantName)
			}
			if (fd.Recv != nil) != tt.wantRecv {
				t.Errorf("Recv set = %v, want %v", fd.Recv != nil, tt.wantRecv)
			}
			if len(fd.Params) != tt.wantParams {
				t.Errorf("Params count = %d, want %d", len(fd.Params), tt.wantParams)
			}
			if (fd.Result != nil) != tt.wantResult {
				t.Errorf("Result set = %v, want %v", fd.Result != nil, tt.wantResult)
			}
		})
	}
}

func TestParseMixedDeclarations(t *testing.T) {
	src := `package main

import "fmt"

type Point struct {
	x int
	y int
}

type Number = int

var globalX int
var globalP ref Point = new(Point)

func (p Point) Sum() int {
	return p.x + p.y
}

func main() {
	var p Point
	p.x = 10
}
`
	f := parseFile(t, src)

	// Check package
	if f.PkgName.Value != "main" {
		t.Errorf("package = %q, want main", f.PkgName.Value)
	}

	// Check imports
	if len(f.Imports) != 1 {
		t.Fatalf("got %d imports, want 1", len(f.Imports))
	}

	// Check declarations: 2 types + 2 vars + 2 funcs = 6
	if len(f.Decls) != 6 {
		t.Fatalf("got %d decls, want 6", len(f.Decls))
	}

	// Verify declaration types in order
	if _, ok := f.Decls[0].(*TypeDecl); !ok {
		t.Errorf("decl[0] is %T, want *TypeDecl", f.Decls[0])
	}
	if _, ok := f.Decls[1].(*TypeDecl); !ok {
		t.Errorf("decl[1] is %T, want *TypeDecl", f.Decls[1])
	}
	if _, ok := f.Decls[2].(*VarDecl); !ok {
		t.Errorf("decl[2] is %T, want *VarDecl", f.Decls[2])
	}
	if _, ok := f.Decls[3].(*VarDecl); !ok {
		t.Errorf("decl[3] is %T, want *VarDecl", f.Decls[3])
	}
	if fd, ok := f.Decls[4].(*FuncDecl); !ok {
		t.Errorf("decl[4] is %T, want *FuncDecl", f.Decls[4])
	} else if fd.Recv == nil {
		t.Errorf("decl[4] should be a method with receiver")
	}
	if fd, ok := f.Decls[5].(*FuncDecl); !ok {
		t.Errorf("decl[5] is %T, want *FuncDecl", f.Decls[5])
	} else if fd.Name.Value != "main" {
		t.Errorf("decl[5].Name = %q, want main", fd.Name.Value)
	}
}

func TestParseStatements(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		stmtTyp string
	}{
		{
			"empty",
			"package main\nfunc f() { ; }",
			"*syntax.EmptyStmt",
		},
		{
			"expr_stmt",
			"package main\nfunc f() { foo() }",
			"*syntax.ExprStmt",
		},
		{
			"assign",
			"package main\nfunc f() { x = 1 }",
			"*syntax.AssignStmt",
		},
		{
			"short_decl",
			"package main\nfunc f() { x := 1 }",
			"*syntax.AssignStmt",
		},
		{
			"var_decl",
			"package main\nfunc f() { var x int }",
			"*syntax.DeclStmt",
		},
		{
			"if",
			"package main\nfunc f() { if x > 0 { } }",
			"*syntax.IfStmt",
		},
		{
			"if_else",
			"package main\nfunc f() { if x > 0 { } else { } }",
			"*syntax.IfStmt",
		},
		{
			"for",
			"package main\nfunc f() { for x < 10 { } }",
			"*syntax.ForStmt",
		},
		{
			"return",
			"package main\nfunc f() { return }",
			"*syntax.ReturnStmt",
		},
		{
			"return_value",
			"package main\nfunc f() int { return 1 }",
			"*syntax.ReturnStmt",
		},
		{
			"break",
			"package main\nfunc f() { for { break } }",
			"*syntax.BranchStmt",
		},
		{
			"continue",
			"package main\nfunc f() { for { continue } }",
			"*syntax.BranchStmt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFile(t, tt.src)
			fd, ok := f.Decls[0].(*FuncDecl)
			if !ok || fd.Body == nil {
				t.Fatal("missing function body")
			}

			// Find the first non-empty statement (for break/continue, look inside for loop)
			var stmt Stmt
			if tt.name == "break" || tt.name == "continue" {
				forStmt := fd.Body.Stmts[0].(*ForStmt)
				stmt = forStmt.Body.Stmts[0]
			} else {
				stmt = fd.Body.Stmts[0]
			}

			got := stmtTypeName(stmt)
			if got != tt.stmtTyp {
				t.Errorf("stmt type = %s, want %s", got, tt.stmtTyp)
			}
		})
	}
}

func stmtTypeName(s Stmt) string {
	switch s.(type) {
	case *EmptyStmt:
		return "*syntax.EmptyStmt"
	case *ExprStmt:
		return "*syntax.ExprStmt"
	case *AssignStmt:
		return "*syntax.AssignStmt"
	case *BlockStmt:
		return "*syntax.BlockStmt"
	case *IfStmt:
		return "*syntax.IfStmt"
	case *ForStmt:
		return "*syntax.ForStmt"
	case *ReturnStmt:
		return "*syntax.ReturnStmt"
	case *BranchStmt:
		return "*syntax.BranchStmt"
	case *DeclStmt:
		return "*syntax.DeclStmt"
	default:
		return "*syntax.Unknown"
	}
}

func TestParseExpressions(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		// Literals
		{"123", "BasicLit"},
		{"3.14", "BasicLit"},
		{`"hello"`, "BasicLit"},

		// Binary operations
		{"1 + 2", "Operation"},
		{"1 + 2 * 3", "Operation"}, // precedence test
		{"a && b || c", "Operation"},

		// Unary operations
		{"-x", "Operation"},
		{"!b", "Operation"},
		{"*p", "Operation"},
		{"&x", "Operation"},

		// Postfix
		{"foo()", "CallExpr"},
		{"foo(1, 2)", "CallExpr"},
		{"arr[0]", "IndexExpr"},
		{"p.x", "SelectorExpr"},
		{"p.x.y", "SelectorExpr"},

		// Special
		{"new(Point)", "NewExpr"},
		{"Point{x: 1}", "CompositeLit"},
		{"Result{0, false}", "CompositeLit"},
		{"panic(msg)", "CallExpr"},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			src := "package main\nfunc f() { _ = " + tt.src + " }"
			f := parseFile(t, src)
			fd := f.Decls[0].(*FuncDecl)
			as := fd.Body.Stmts[0].(*AssignStmt)
			expr := as.RHS[0]

			got := exprTypeName(expr)
			if got != tt.want {
				t.Errorf("expr type = %s, want %s", got, tt.want)
			}
		})
	}
}

func exprTypeName(e Expr) string {
	switch e.(type) {
	case *Name:
		return "Name"
	case *BasicLit:
		return "BasicLit"
	case *Operation:
		return "Operation"
	case *CallExpr:
		return "CallExpr"
	case *IndexExpr:
		return "IndexExpr"
	case *SelectorExpr:
		return "SelectorExpr"
	case *ParenExpr:
		return "ParenExpr"
	case *NewExpr:
		return "NewExpr"
	case *CompositeLit:
		return "CompositeLit"
	case *KeyValueExpr:
		return "KeyValueExpr"
	case *ArrayType:
		return "ArrayType"
	case *PointerType:
		return "PointerType"
	case *RefType:
		return "RefType"
	case *StructType:
		return "StructType"
	default:
		return "Unknown"
	}
}

func TestParsePrecedence(t *testing.T) {
	tests := []struct {
		src  string
		want string // expected structure
	}{
		// Multiplicative binds tighter than additive
		{"1 + 2 * 3", "Op{+,1,Op{*,2,3}}"},
		{"1 * 2 + 3", "Op{+,Op{*,1,2},3}"},

		// Comparison binds tighter than logical
		{"a < b && c > d", "Op{&&,Op{<,a,b},Op{>,c,d}}"},

		// Or has lowest precedence
		{"a && b || c && d", "Op{||,Op{&&,a,b},Op{&&,c,d}}"},

		// Left associativity
		{"a + b + c", "Op{+,Op{+,a,b},c}"},
		{"a * b * c", "Op{*,Op{*,a,b},c}"},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			src := "package main\nfunc f() { _ = " + tt.src + " }"
			f := parseFile(t, src)
			fd := f.Decls[0].(*FuncDecl)
			as := fd.Body.Stmts[0].(*AssignStmt)
			expr := as.RHS[0]

			got := exprSummary(expr)
			if got != tt.want {
				t.Errorf("precedence:\ngot:  %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestParseNodePositions(t *testing.T) {
	src := `package main
func f() {
var x int
_ = a + b
_ = foo(1)
_ = arr[0]
_ = p.x
}
`
	f := parseFile(t, src)
	fd := f.Decls[0].(*FuncDecl)

	declStmt := fd.Body.Stmts[0].(*DeclStmt)
	if declStmt.Pos().Line() != 3 || declStmt.Pos().Col() != 1 {
		t.Fatalf("DeclStmt pos = %s, want test.yoru:3:1", declStmt.Pos())
	}

	as1 := fd.Body.Stmts[1].(*AssignStmt)
	bin := as1.RHS[0].(*Operation)
	if bin.Pos().Line() != 4 || bin.Pos().Col() != 5 {
		t.Fatalf("binary op pos = %s, want test.yoru:4:5", bin.Pos())
	}

	as2 := fd.Body.Stmts[2].(*AssignStmt)
	call := as2.RHS[0].(*CallExpr)
	if call.Pos().Line() != 5 || call.Pos().Col() != 5 {
		t.Fatalf("call pos = %s, want test.yoru:5:5", call.Pos())
	}

	as3 := fd.Body.Stmts[3].(*AssignStmt)
	idx := as3.RHS[0].(*IndexExpr)
	if idx.Pos().Line() != 6 || idx.Pos().Col() != 5 {
		t.Fatalf("index pos = %s, want test.yoru:6:5", idx.Pos())
	}

	as4 := fd.Body.Stmts[4].(*AssignStmt)
	sel := as4.RHS[0].(*SelectorExpr)
	if sel.Pos().Line() != 7 || sel.Pos().Col() != 5 {
		t.Fatalf("selector pos = %s, want test.yoru:7:5", sel.Pos())
	}
}

func exprSummary(e Expr) string {
	switch x := e.(type) {
	case *Name:
		return x.Value
	case *BasicLit:
		return x.Value
	case *Operation:
		if x.Y == nil {
			return "Op{" + x.Op.String() + "," + exprSummary(x.X) + "}"
		}
		return "Op{" + x.Op.String() + "," + exprSummary(x.X) + "," + exprSummary(x.Y) + "}"
	case *CallExpr:
		var args []string
		for _, a := range x.Args {
			args = append(args, exprSummary(a))
		}
		return "Call{" + exprSummary(x.Fun) + ",[" + strings.Join(args, ",") + "]}"
	case *IndexExpr:
		return "Index{" + exprSummary(x.X) + "," + exprSummary(x.Index) + "}"
	case *SelectorExpr:
		return "Sel{" + exprSummary(x.X) + "," + x.Sel.Value + "}"
	case *NewExpr:
		return "New{" + exprSummary(x.Type) + "}"
	case *CompositeLit:
		var elems []string
		for _, e := range x.Elems {
			elems = append(elems, exprSummary(e))
		}
		return "Composite{" + exprSummary(x.Type) + ",[" + strings.Join(elems, ",") + "]}"
	case *KeyValueExpr:
		return exprSummary(x.Key) + ":" + exprSummary(x.Value)
	case *ParenExpr:
		return exprSummary(x.X)
	default:
		return "<unknown>"
	}
}

// ----------------------------------------------------------------------------
// Error tests

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string
	}{
		// Missing package
		{"no_package", "func main() {}", "expected package"},

		// Missing identifiers
		{"missing_name", "package main\ntype = int", "expected identifier"},
		{"missing_func_name", "package main\nfunc () {}", "expected identifier"},

		// Missing delimiters
		{"missing_lbrace", "package main\nfunc foo() return", "expected {"},
		{"missing_rbrace", "package main\nfunc foo() { return", "expected }"},
		{"missing_lparen", "package main\nfunc foo) {}", "expected ("},
		{"missing_rparen", "package main\nfunc foo( {}", "expected )"},

		// Expression errors
		{"unexpected_op", "package main\nfunc f() { x = + }", "expected operand"},

		// Type errors ([]int is slice syntax, not supported; parser expects array length)
		{"bad_array_type", "package main\ntype T []int", "expected operand"},

		// Statement errors
		{"bad_if", "package main\nfunc f() { if { } }", "expected"},
		{"bad_return", "package main\nfunc f() { return + }", "expected operand"},
		{"missing_for_cond", "package main\nfunc f() { for { break } }", "expected for condition"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := parseFileWithErrors(t, tt.src)

			if len(errs) == 0 {
				t.Errorf("expected error containing %q", tt.wantErr)
				return
			}

			found := false
			for _, e := range errs {
				if strings.Contains(e, tt.wantErr) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, errs)
			}
		})
	}
}

func TestParseErrorPositions(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantLine   uint32
		wantCol    uint32
		wantSubstr string
	}{
		{
			name:       "missing_package",
			src:        "func main() {}",
			wantLine:   1,
			wantCol:    1,
			wantSubstr: "expected package",
		},
		{
			name:       "bad_operand",
			src:        "package main\nfunc f() { x = + }",
			wantLine:   2,
			wantCol:    16,
			wantSubstr: "expected operand",
		},
		{
			name:       "missing_for_condition",
			src:        "package main\nfunc f() { for { break } }",
			wantLine:   2,
			wantCol:    16,
			wantSubstr: "expected for condition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := parseFileWithErrorDetails(t, tt.src)
			if len(errs) == 0 {
				t.Fatal("expected at least one error")
			}
			first := errs[0]
			if first.pos.Line() != tt.wantLine || first.pos.Col() != tt.wantCol {
				t.Fatalf("first error pos = %s, want test.yoru:%d:%d", first.pos, tt.wantLine, tt.wantCol)
			}
			if !strings.Contains(first.msg, tt.wantSubstr) {
				t.Fatalf("first error msg = %q, want substring %q", first.msg, tt.wantSubstr)
			}
		})
	}
}

func TestParseErrorRecovery(t *testing.T) {
	// Test that parser can recover and report multiple errors
	src := `package main

type = int
var = 1
func foo() {}
`
	_, errs := parseFileWithErrors(t, src)

	// Should have at least 2 errors but still parse the function
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(errs))
	}
}

func TestParseNoAbort(t *testing.T) {
	// Test that parser doesn't panic on bad input
	badInputs := []string{
		"",
		"package",
		"package main\nfunc",
		"package main\ntype T struct {",
		"package main\nfunc f() { if",
		"package main\nfunc f() { for {",
		"package main\n;;;;;;;",
		"package main\nfunc f() { ((((((( }",
	}

	for _, src := range badInputs {
		name := src
		if len(name) > 20 {
			name = name[:20]
		}
		t.Run(name, func(t *testing.T) {
			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("parser panicked: %v", r)
				}
			}()

			p := NewParser("test", strings.NewReader(src), nil)
			_ = p.Parse()
		})
	}
}

// ----------------------------------------------------------------------------
// Complete program test

func TestParseCompleteProgram(t *testing.T) {
	src := `package main

import "fmt"

type Point struct {
	x int
	y float
}

type Number = int

func (p Point) Area() int {
	return p.x * p.y
}

func add(a int, b int) int {
	return a + b
}

func main() {
	var p Point
	p.x = 10
	p.y = 3.14

	result := add(1, 2)

	var arr [5]int
	arr[0] = 1

	var r ref Point = new(Point)
	r.x = 20

	if p.x > 0 {
		println("positive")
	} else {
		println("non-positive")
	}

	var i int = 0
	for i < 10 {
		println(i)
		i = i + 1
	}

	println(p.Area())
}
`

	f, errs := parseFileWithErrors(t, src)

	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	// Verify structure
	if f.PkgName.Value != "main" {
		t.Errorf("package name = %q, want main", f.PkgName.Value)
	}

	if len(f.Imports) != 1 {
		t.Errorf("imports = %d, want 1", len(f.Imports))
	}

	// Should have: Point struct, Number alias, Area method, add func, main func
	if len(f.Decls) != 5 {
		t.Errorf("decls = %d, want 5", len(f.Decls))
	}
}

// ----------------------------------------------------------------------------
// Golden tests

func TestParseGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/parse_*.yoru")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}

			p := NewParser(f, bytes.NewReader(src), nil)
			ast := p.Parse()

			var buf bytes.Buffer
			Fprint(&buf, ast)
			got := buf.String()

			golden := strings.TrimSuffix(f, ".yoru") + ".ast.golden"

			if os.Getenv("UPDATE_GOLDEN") != "" {
				if err := os.WriteFile(golden, []byte(got), 0644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				// If golden file doesn't exist, create it
				if os.IsNotExist(err) {
					if err := os.WriteFile(golden, []byte(got), 0644); err != nil {
						t.Fatal(err)
					}
					t.Logf("created golden file: %s", golden)
					return
				}
				t.Fatal(err)
			}

			if got != string(want) {
				t.Errorf("AST mismatch for %s\nRun with UPDATE_GOLDEN=1 to update", f)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Walk tests

func TestWalk(t *testing.T) {
	src := `package main
func main() {
	x := 1 + 2
}
`
	f := parseFile(t, src)

	var nodeCount int
	var nameCount int
	Walk(f, func(n Node) bool {
		nodeCount++
		if _, ok := n.(*Name); ok {
			nameCount++
		}
		return true
	})

	if nodeCount == 0 {
		t.Error("Walk visited no nodes")
	}
	// Should have at least: main (pkg), main (func), x, 1, 2
	if nameCount < 3 {
		t.Errorf("expected at least 3 Name nodes, got %d", nameCount)
	}
}

func TestInspect(t *testing.T) {
	src := `package main
func f() {
	if x > 0 {
		return 1
	}
}
`
	f := parseFile(t, src)

	var ifCount int
	Inspect(f, func(n Node) bool {
		if _, ok := n.(*IfStmt); ok {
			ifCount++
		}
		return true
	})

	if ifCount != 1 {
		t.Errorf("expected 1 IfStmt, got %d", ifCount)
	}
}

// ----------------------------------------------------------------------------
// Fuzz test

func FuzzParse(f *testing.F) {
	seeds := []string{
		"package main",
		"package main\nfunc main() {}",
		"package main\ntype Point struct { x int\n y float }",
		"package main\nfunc f() { if x > 0 { return 1 } else { return 0 } }",
		"package main\nfunc f() { for i < 10 { i = i + 1 } }",
		"package main\nvar x ref Point = new(Point)",
		"package main\nfunc (p *Point) Move() { p.x = p.x + 1 }",
		"package main\nfunc f() { _ = Result{0, false} }",
		"package main\nfunc f() { panic(\"boom\") }",
		"package main\nimport \"fmt\"\nfunc main() { println(1) }",
		"package main\ntype T = [10]*int",
		"package main\nfunc f() { x := 1 + 2 * 3 - 4 / 5 }",
		"package main\nfunc f() { if a && b || c && d { } }",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		// Syntax errors are acceptable, but parser should not panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("parser panicked on input %q: %v", src, r)
			}
		}()

		errh := func(pos Pos, msg string) {
			// Ignore errors, just ensure no panic
		}

		p := NewParser("fuzz", strings.NewReader(src), errh)
		_ = p.Parse()
	})
}

// ----------------------------------------------------------------------------
// Additional error tests (to reach 20+)

func TestParseMoreErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string
	}{
		// Import errors
		{"bad_import_path", "package main\nimport foo", "expected string literal"},
		{"missing_import_path", "package main\nimport", "expected string literal"},

		// Struct errors
		{"unclosed_struct", "package main\ntype T struct { x int", "expected }"},
		{"bad_field_type", "package main\ntype T struct { x }", "expected type"},

		// Function errors
		{"missing_func_body", "package main\nfunc f()", "expected {"},
		{"bad_param", "package main\nfunc f(int) {}", "expected type"},
		{"unclosed_params", "package main\nfunc f(x int {}", "expected )"},

		// Expression errors
		{"unclosed_paren", "package main\nfunc f() { x = (1 + 2 }", "expected )"},
		{"unclosed_bracket", "package main\nfunc f() { x = arr[0 }", "expected ]"},
		{"bad_selector", "package main\nfunc f() { x = a. }", "expected identifier"},
		{"bad_call_args", "package main\nfunc f() { foo(,) }", "expected operand"},

		// Statement errors
		{"bad_for_body", "package main\nfunc f() { for x }", "expected {"},
		{"missing_for_condition", "package main\nfunc f() { for { break } }", "expected for condition"},
		{"bad_var_init", "package main\nfunc f() { var x int = }", "expected operand"},
		{"unclosed_block", "package main\nfunc f() { { x = 1 }", "expected }"},

		// Composite literal errors
		{"unclosed_composite", "package main\nfunc f() { x := T{ }", "expected"},
		{"bad_key_value", "package main\nfunc f() { x := T{a:} }", "expected operand"},

		// New expression errors
		{"bad_new", "package main\nfunc f() { x := new() }", "expected type"},
		{"unclosed_new", "package main\nfunc f() { x := new(T }", "expected )"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := parseFileWithErrors(t, tt.src)

			if len(errs) == 0 {
				t.Errorf("expected error containing %q", tt.wantErr)
				return
			}

			found := false
			for _, e := range errs {
				if strings.Contains(e, tt.wantErr) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, errs)
			}
		})
	}
}

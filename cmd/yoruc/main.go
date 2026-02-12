// Package main implements the Yoru compiler entry point.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/you-not-fish/yoru/internal/ssa"
	"github.com/you-not-fish/yoru/internal/ssa/passes"
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
	"github.com/you-not-fish/yoru/internal/types2"
)

// Compiler flags
var (
	emitTokens   = flag.Bool("emit-tokens", false, "Output token stream")
	noASI        = flag.Bool("no-asi", false, "Disable automatic semicolon insertion")
	emitAST      = flag.Bool("emit-ast", false, "Output AST")
	astFormat    = flag.String("ast-format", "text", "AST output format (text or json)")
	emitTypedAST = flag.Bool("emit-typed-ast", false, "Output typed AST")
	emitSSA      = flag.Bool("emit-ssa", false, "Output SSA")
	emitLL       = flag.Bool("emit-ll", false, "Output LLVM IR")
	emitLayout   = flag.Bool("emit-layout", false, "Output struct layouts")
	output       = flag.String("o", "", "Output file")
	doctor       = flag.Bool("doctor", false, "Check toolchain")
	version      = flag.Bool("version", false, "Print version")
	trace        = flag.Bool("trace", false, "Output timing trace")
	dumpFunc     = flag.String("dump-func", "", "Only dump specific function")
	ssaVerify    = flag.Bool("ssa-verify", false, "Verify SSA after each pass")
	dumpBefore   = flag.String("dump-before", "", "Dump SSA before pass (name or \"*\")")
	dumpAfter    = flag.String("dump-after", "", "Dump SSA after pass (name or \"*\")")
	gcStats      = flag.Bool("gc-stats", false, "Print GC statistics")
	gcVerbose    = flag.Bool("gc-verbose", false, "Verbose GC output")
	gcStress     = flag.Bool("gc-stress", false, "Trigger GC on every allocation")
)

// Version information
const Version = "0.1.0-dev"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Yoru Compiler %s\n\n", Version)
		fmt.Fprintf(os.Stderr, "Usage: yoruc [options] <file.yoru>\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *version {
		fmt.Printf("yoruc version %s\n", Version)
		fmt.Printf("go version %s\n", runtime.Version())
		os.Exit(0)
	}

	if *doctor {
		os.Exit(runDoctor())
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: no input file")
		fmt.Fprintln(os.Stderr, "usage: yoruc [options] <file.yoru>")
		os.Exit(1)
	}

	filename := args[0]

	// Handle -emit-tokens
	if *emitTokens {
		os.Exit(runEmitTokens(filename))
	}

	// Handle -emit-ast
	if *emitAST {
		os.Exit(runEmitAST(filename))
	}

	// Handle -emit-typed-ast
	if *emitTypedAST {
		os.Exit(runEmitTypedAST(filename))
	}

	// Handle -emit-layout
	if *emitLayout {
		os.Exit(runEmitLayout(filename))
	}

	// Handle -emit-ssa
	if *emitSSA {
		os.Exit(runEmitSSA(filename))
	}

	// TODO: Implement rest of compilation pipeline
	fmt.Fprintf(os.Stderr, "yoruc: compilation not yet implemented\n")
	fmt.Fprintf(os.Stderr, "input file: %s\n", filename)
	os.Exit(1)
}

// runEmitAST parses the input file and outputs the AST.
func runEmitAST(filename string) int {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer f.Close()

	var errs []string
	errh := func(pos syntax.Pos, msg string) {
		errs = append(errs, fmt.Sprintf("%s: %s", pos, msg))
	}

	p := syntax.NewParser(filename, f, errh)
	if *noASI {
		p.SetASIEnabled(false)
	}
	ast := p.Parse()

	// Print errors first
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, e)
	}

	// Output AST
	switch *astFormat {
	case "json":
		if err := syntax.FprintJSON(os.Stdout, ast); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	default:
		syntax.Fprint(os.Stdout, ast)
	}

	if len(errs) > 0 {
		return 1
	}
	return 0
}

// runEmitTokens scans the input file and prints all tokens with positions.
func runEmitTokens(filename string) int {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer f.Close()

	var errors []string
	errh := func(line, col uint32, msg string) {
		errors = append(errors, fmt.Sprintf("%s:%d:%d: %s", filename, line, col, msg))
	}

	s := syntax.NewScanner(filename, f, errh)
	if *noASI {
		s.SetASIEnabled(false)
	}

	// Print header
	fmt.Printf("%-20s %-12s %s\n", "POSITION", "TOKEN", "LITERAL")
	fmt.Printf("%-20s %-12s %s\n", strings.Repeat("-", 20), strings.Repeat("-", 12), strings.Repeat("-", 20))

	for {
		s.Next()
		tok := s.Token()
		pos := s.Pos()
		lit := s.Literal()

		// Format position
		posStr := pos.String()

		// Format literal (escape special characters for display)
		litStr := formatLiteral(lit)

		fmt.Printf("%-20s %-12s %s\n", posStr, tok.String(), litStr)

		if tok.IsEOF() {
			break
		}
	}

	// Print any errors
	if len(errors) > 0 {
		fmt.Println()
		fmt.Println("Errors:")
		for _, e := range errors {
			fmt.Printf("  %s\n", e)
		}
		return 1
	}

	return 0
}

// formatLiteral formats a literal for display, escaping special characters.
func formatLiteral(lit string) string {
	if lit == "" {
		return "\"\""
	}

	// Show the content with escapes visible for readability
	var b strings.Builder
	b.WriteRune('"')
	for _, r := range lit {
		switch r {
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\r':
			b.WriteString("\\r")
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case 0:
			b.WriteString("\\0")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteRune('"')
	return b.String()
}

// runDoctor checks the toolchain and returns an exit code.
func runDoctor() int {
	fmt.Println("Yoru Toolchain Doctor")
	fmt.Println("=====================")
	fmt.Println()

	allOk := true

	// Check Go version
	goVersion := runtime.Version()
	fmt.Printf("Go:      %s", goVersion)
	if checkGoVersion(goVersion) {
		fmt.Println(" ✓")
	} else {
		fmt.Println(" ✗ (need 1.21+)")
		allOk = false
	}

	// Check clang (required)
	clangVersion, clangOk := checkTool("clang", "--version")
	fmt.Printf("clang:   %s", clangVersion)
	if clangOk {
		fmt.Println(" ✓")
	} else {
		fmt.Println(" ✗ (not found)")
		allOk = false
	}

	// Check opt (optional)
	optVersion, optOk := checkTool("opt", "--version")
	fmt.Printf("opt:     %s", optVersion)
	if optOk {
		fmt.Println(" ✓")
	} else {
		fmt.Println(" (optional, not found)")
	}

	// Check llvm-as (optional)
	llvmAsVersion, llvmAsOk := checkTool("llvm-as", "--version")
	fmt.Printf("llvm-as: %s", llvmAsVersion)
	if llvmAsOk {
		fmt.Println(" ✓")
	} else {
		fmt.Println(" (optional, not found)")
	}

	fmt.Println()
	if allOk {
		fmt.Println("All required tools available!")
		return 0
	}

	fmt.Println("Some required tools are missing.")
	fmt.Println("See docs/toolchain.md for installation instructions.")
	return 1
}

// checkGoVersion returns true if the Go version is 1.21 or higher.
func checkGoVersion(v string) bool {
	// Extract version number (e.g., "go1.23.3" -> "1.23")
	if !strings.HasPrefix(v, "go") {
		return false
	}
	v = strings.TrimPrefix(v, "go")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}

	// Check major.minor version
	major := parts[0]
	minor := parts[1]

	// Go 1.21+ is required
	if major == "1" {
		// Parse minor version
		var minorNum int
		fmt.Sscanf(minor, "%d", &minorNum)
		return minorNum >= 21
	}

	// Go 2.x or higher is fine
	return major >= "2"
}

// checkTool runs a tool with the given arguments and returns the first line of output.
func checkTool(name string, args ...string) (string, bool) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}

	// Extract first line
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		line := strings.TrimSpace(lines[0])
		// Truncate long lines
		if len(line) > 60 {
			line = line[:57] + "..."
		}
		return line, true
	}
	return "", false
}

// runEmitTypedAST parses, type-checks, and outputs the typed AST.
func runEmitTypedAST(filename string) int {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer f.Close()

	var parseErrs []string
	parseErrh := func(pos syntax.Pos, msg string) {
		parseErrs = append(parseErrs, fmt.Sprintf("%s: %s", pos, msg))
	}

	p := syntax.NewParser(filename, f, parseErrh)
	if *noASI {
		p.SetASIEnabled(false)
	}
	ast := p.Parse()

	// Print parse errors
	for _, e := range parseErrs {
		fmt.Fprintln(os.Stderr, e)
	}
	if len(parseErrs) > 0 {
		return 1
	}

	// Type check
	var typeErrs []string
	typeErrh := func(pos syntax.Pos, msg string) {
		typeErrs = append(typeErrs, fmt.Sprintf("%s: %s", pos, msg))
	}

	conf := &types2.Config{
		Error: typeErrh,
		Sizes: types.DefaultSizes,
	}
	info := &types2.Info{
		Types:  make(map[syntax.Expr]types2.TypeAndValue),
		Defs:   make(map[*syntax.Name]types.Object),
		Uses:   make(map[*syntax.Name]types.Object),
		Scopes: make(map[syntax.Node]*types.Scope),
	}

	pkg, _ := types2.Check(filename, ast, conf, info)

	// Print type errors
	for _, e := range typeErrs {
		fmt.Fprintln(os.Stderr, e)
	}

	// Output typed AST
	printTypedAST(ast, info, pkg)

	if len(typeErrs) > 0 {
		return 1
	}
	return 0
}

// printTypedAST outputs the AST with type annotations.
func printTypedAST(file *syntax.File, info *types2.Info, pkg *types.Package) {
	fmt.Printf("File\n")
	fmt.Printf("  PkgName: %s\n", file.PkgName.Value)

	if len(file.Imports) > 0 {
		fmt.Printf("  Imports:\n")
		for _, imp := range file.Imports {
			fmt.Printf("    ImportDecl: %s\n", imp.Path.Value)
		}
	}

	fmt.Printf("  Decls:\n")
	for _, decl := range file.Decls {
		printTypedDecl(decl, info, "    ")
	}
}

// printTypedDecl outputs a declaration with type annotations.
func printTypedDecl(decl syntax.Decl, info *types2.Info, indent string) {
	switch d := decl.(type) {
	case *syntax.TypeDecl:
		fmt.Printf("%sTypeDecl\n", indent)
		if obj := info.Defs[d.Name]; obj != nil {
			if obj.Type() != nil {
				fmt.Printf("%s  Name: %s (%s)\n", indent, d.Name.Value, obj.Type())
			} else {
				fmt.Printf("%s  Name: %s\n", indent, d.Name.Value)
			}
		} else {
			fmt.Printf("%s  Name: %s\n", indent, d.Name.Value)
		}

	case *syntax.VarDecl:
		fmt.Printf("%sVarDecl\n", indent)
		if obj := info.Defs[d.Name]; obj != nil {
			fmt.Printf("%s  Name: %s (%s)\n", indent, d.Name.Value, obj.Type())
		} else {
			fmt.Printf("%s  Name: %s\n", indent, d.Name.Value)
		}
		if d.Value != nil {
			fmt.Printf("%s  Value: ", indent)
			printTypedExpr(d.Value, info)
			fmt.Println()
		}

	case *syntax.FuncDecl:
		fmt.Printf("%sFuncDecl\n", indent)
		if obj := info.Defs[d.Name]; obj != nil {
			if obj.Type() != nil {
				fmt.Printf("%s  Name: %s (%s)\n", indent, d.Name.Value, obj.Type())
			} else {
				fmt.Printf("%s  Name: %s\n", indent, d.Name.Value)
			}
		} else {
			fmt.Printf("%s  Name: %s\n", indent, d.Name.Value)
		}
		if d.Body != nil {
			fmt.Printf("%s  Body:\n", indent)
			for _, stmt := range d.Body.Stmts {
				printTypedStmt(stmt, info, indent+"    ")
			}
		}
	}
}

// printTypedStmt outputs a statement with type annotations.
func printTypedStmt(stmt syntax.Stmt, info *types2.Info, indent string) {
	switch s := stmt.(type) {
	case *syntax.ExprStmt:
		fmt.Printf("%sExprStmt\n", indent)
		fmt.Printf("%s  X: ", indent)
		printTypedExpr(s.X, info)
		fmt.Println()

	case *syntax.AssignStmt:
		fmt.Printf("%sAssignStmt (%s)\n", indent, s.Op)
		for i, lhs := range s.LHS {
			fmt.Printf("%s  LHS[%d]: ", indent, i)
			printTypedExpr(lhs, info)
			fmt.Println()
		}
		for i, rhs := range s.RHS {
			fmt.Printf("%s  RHS[%d]: ", indent, i)
			printTypedExpr(rhs, info)
			fmt.Println()
		}

	case *syntax.ReturnStmt:
		fmt.Printf("%sReturnStmt\n", indent)
		if s.Result != nil {
			fmt.Printf("%s  Result: ", indent)
			printTypedExpr(s.Result, info)
			fmt.Println()
		}

	case *syntax.IfStmt:
		fmt.Printf("%sIfStmt\n", indent)
		fmt.Printf("%s  Cond: ", indent)
		printTypedExpr(s.Cond, info)
		fmt.Println()
		fmt.Printf("%s  Then:\n", indent)
		for _, st := range s.Then.Stmts {
			printTypedStmt(st, info, indent+"    ")
		}
		if s.Else != nil {
			fmt.Printf("%s  Else:\n", indent)
			if block, ok := s.Else.(*syntax.BlockStmt); ok {
				for _, st := range block.Stmts {
					printTypedStmt(st, info, indent+"    ")
				}
			}
		}

	case *syntax.ForStmt:
		fmt.Printf("%sForStmt\n", indent)
		if s.Cond != nil {
			fmt.Printf("%s  Cond: ", indent)
			printTypedExpr(s.Cond, info)
			fmt.Println()
		}
		fmt.Printf("%s  Body:\n", indent)
		for _, st := range s.Body.Stmts {
			printTypedStmt(st, info, indent+"    ")
		}

	case *syntax.DeclStmt:
		printTypedDecl(s.Decl, info, indent)

	case *syntax.BlockStmt:
		fmt.Printf("%sBlockStmt\n", indent)
		for _, st := range s.Stmts {
			printTypedStmt(st, info, indent+"  ")
		}

	default:
		fmt.Printf("%s%T\n", indent, stmt)
	}
}

// printTypedExpr outputs an expression with type annotations.
func printTypedExpr(expr syntax.Expr, info *types2.Info) {
	fmt.Print(typedExprString(expr, info))
}

func typedExprString(expr syntax.Expr, info *types2.Info) string {
	tv, ok := info.Types[expr]
	typ := ""
	if ok {
		switch {
		case tv.Type != nil:
			typ = fmt.Sprintf(" (%s)", tv.Type)
		case tv.IsVoid():
			typ = " (void)"
		case tv.IsBuiltin():
			typ = " (builtin)"
		}
	}

	switch e := expr.(type) {
	case *syntax.Name:
		return fmt.Sprintf("Name %q%s", e.Value, typ)
	case *syntax.BasicLit:
		return fmt.Sprintf("BasicLit %q%s", e.Value, typ)
	case *syntax.Operation:
		if e.Y == nil {
			return fmt.Sprintf("Operation %s%s [X=%s]", e.Op, typ, typedExprString(e.X, info))
		}
		return fmt.Sprintf("Operation %s%s [X=%s, Y=%s]", e.Op, typ, typedExprString(e.X, info), typedExprString(e.Y, info))
	case *syntax.CallExpr:
		args := make([]string, len(e.Args))
		for i, arg := range e.Args {
			args[i] = typedExprString(arg, info)
		}
		return fmt.Sprintf("CallExpr%s [Fun=%s, Args=[%s]]", typ, typedExprString(e.Fun, info), strings.Join(args, ", "))
	case *syntax.IndexExpr:
		return fmt.Sprintf("IndexExpr%s [X=%s, Index=%s]", typ, typedExprString(e.X, info), typedExprString(e.Index, info))
	case *syntax.SelectorExpr:
		return fmt.Sprintf("SelectorExpr .%s%s [X=%s]", e.Sel.Value, typ, typedExprString(e.X, info))
	case *syntax.ParenExpr:
		return fmt.Sprintf("ParenExpr%s [X=%s]", typ, typedExprString(e.X, info))
	case *syntax.NewExpr:
		return fmt.Sprintf("NewExpr%s [Type=%s]", typ, typedExprString(e.Type, info))
	case *syntax.CompositeLit:
		elems := make([]string, len(e.Elems))
		for i, elem := range e.Elems {
			elems[i] = typedExprString(elem, info)
		}
		return fmt.Sprintf("CompositeLit%s [Type=%s, Elems=[%s]]", typ, typedExprString(e.Type, info), strings.Join(elems, ", "))
	case *syntax.KeyValueExpr:
		return fmt.Sprintf("KeyValueExpr%s [Key=%s, Value=%s]", typ, typedExprString(e.Key, info), typedExprString(e.Value, info))
	case *syntax.ArrayType:
		return fmt.Sprintf("ArrayType%s [Len=%s, Elem=%s]", typ, typedExprString(e.Len, info), typedExprString(e.Elem, info))
	case *syntax.PointerType:
		return fmt.Sprintf("PointerType%s [Base=%s]", typ, typedExprString(e.Base, info))
	case *syntax.RefType:
		return fmt.Sprintf("RefType%s [Base=%s]", typ, typedExprString(e.Base, info))
	case *syntax.StructType:
		fields := make([]string, len(e.Fields))
		for i, f := range e.Fields {
			name := ""
			if f.Name != nil {
				name = f.Name.Value + " "
			}
			fields[i] = name + typedExprString(f.Type, info)
		}
		return fmt.Sprintf("StructType%s [Fields=[%s]]", typ, strings.Join(fields, ", "))
	default:
		return fmt.Sprintf("%T%s", expr, typ)
	}
}

// runEmitLayout parses, type-checks, and outputs struct layouts.
func runEmitLayout(filename string) int {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer f.Close()

	var parseErrs []string
	parseErrh := func(pos syntax.Pos, msg string) {
		parseErrs = append(parseErrs, fmt.Sprintf("%s: %s", pos, msg))
	}

	p := syntax.NewParser(filename, f, parseErrh)
	if *noASI {
		p.SetASIEnabled(false)
	}
	ast := p.Parse()

	// Print parse errors
	for _, e := range parseErrs {
		fmt.Fprintln(os.Stderr, e)
	}
	if len(parseErrs) > 0 {
		return 1
	}

	// Type check
	var typeErrs []string
	typeErrh := func(pos syntax.Pos, msg string) {
		typeErrs = append(typeErrs, fmt.Sprintf("%s: %s", pos, msg))
	}

	conf := &types2.Config{
		Error: typeErrh,
		Sizes: types.DefaultSizes,
	}
	info := &types2.Info{
		Types:  make(map[syntax.Expr]types2.TypeAndValue),
		Defs:   make(map[*syntax.Name]types.Object),
		Uses:   make(map[*syntax.Name]types.Object),
		Scopes: make(map[syntax.Node]*types.Scope),
	}

	_, _ = types2.Check(filename, ast, conf, info)

	// Print type errors
	for _, e := range typeErrs {
		fmt.Fprintln(os.Stderr, e)
	}

	// Output struct layouts
	fmt.Println("=== Struct Layouts ===")
	fmt.Println()

	for _, decl := range ast.Decls {
		td, ok := decl.(*syntax.TypeDecl)
		if !ok {
			continue
		}

		// Get the type object
		obj := info.Defs[td.Name]
		if obj == nil {
			continue
		}

		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}

		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}

		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}

		// Print struct layout
		fmt.Printf("type %s struct {\n", td.Name.Value)
		for i, field := range st.Fields() {
			offset := st.Offset(i)
			size := types.DefaultSizes.Sizeof(field.Type())
			align := types.DefaultSizes.Alignof(field.Type())
			fmt.Printf("    %-10s %-15s // offset: %d, size: %d, align: %d\n",
				field.Name(), field.Type(), offset, size, align)
		}
		fmt.Printf("}\n")
		fmt.Printf("// size: %d, align: %d\n", st.Size(), st.Align())
		fmt.Println()
	}

	if len(typeErrs) > 0 {
		return 1
	}
	return 0
}

// runEmitSSA parses, type-checks, and outputs SSA for all functions.
func runEmitSSA(filename string) int {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer f.Close()

	var parseErrs []string
	parseErrh := func(pos syntax.Pos, msg string) {
		parseErrs = append(parseErrs, fmt.Sprintf("%s: %s", pos, msg))
	}

	p := syntax.NewParser(filename, f, parseErrh)
	if *noASI {
		p.SetASIEnabled(false)
	}
	ast := p.Parse()

	for _, e := range parseErrs {
		fmt.Fprintln(os.Stderr, e)
	}
	if len(parseErrs) > 0 {
		return 1
	}

	// Type check.
	var typeErrs []string
	typeErrh := func(pos syntax.Pos, msg string) {
		typeErrs = append(typeErrs, fmt.Sprintf("%s: %s", pos, msg))
	}

	conf := &types2.Config{
		Error: typeErrh,
		Sizes: types.DefaultSizes,
	}
	info := &types2.Info{
		Types:  make(map[syntax.Expr]types2.TypeAndValue),
		Defs:   make(map[*syntax.Name]types.Object),
		Uses:   make(map[*syntax.Name]types.Object),
		Scopes: make(map[syntax.Node]*types.Scope),
	}

	_, _ = types2.Check(filename, ast, conf, info)

	for _, e := range typeErrs {
		fmt.Fprintln(os.Stderr, e)
	}
	if len(typeErrs) > 0 {
		return 1
	}

	// Build SSA.
	funcs := ssa.BuildFile(ast, info, types.DefaultSizes)

	// Define pass pipeline.
	pipeline := []passes.Pass{
		{Name: "mem2reg", Fn: passes.Mem2Reg},
	}
	passCfg := passes.Config{
		DumpBefore: *dumpBefore,
		DumpAfter:  *dumpAfter,
		Verify:     *ssaVerify,
		DumpFunc:   *dumpFunc,
	}

	// Run pass pipeline on each function.
	for _, fn := range funcs {
		if *ssaVerify {
			if err := ssa.Verify(fn); err != nil {
				fmt.Fprintf(os.Stderr, "SSA verification failed for %s (before passes):\n%v\n", fn.Name, err)
				return 1
			}
		}
		ssa.ComputeDom(fn)
		if err := passes.Run(fn, pipeline, passCfg); err != nil {
			fmt.Fprintf(os.Stderr, "pass pipeline failed for %s:\n%v\n", fn.Name, err)
			return 1
		}
	}

	// Print SSA functions.
	for i, fn := range funcs {
		if *dumpFunc != "" && fn.Name != *dumpFunc {
			continue
		}
		if i > 0 {
			fmt.Println()
		}
		ssa.Print(fn)
	}

	return 0
}

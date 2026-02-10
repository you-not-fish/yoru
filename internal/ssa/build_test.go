package ssa

import (
	"strings"
	"testing"

	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
	"github.com/you-not-fish/yoru/internal/types2"
)

// buildFromSource parses, type-checks, and builds SSA for the given source.
// It calls t.Fatal on any parse or type errors.
// Each returned function is verified with Verify.
func buildFromSource(t *testing.T, src string) []*Func {
	t.Helper()

	r := strings.NewReader(src)
	var parseErrs []string
	parseErrh := func(pos syntax.Pos, msg string) {
		parseErrs = append(parseErrs, pos.String()+": "+msg)
	}

	p := syntax.NewParser("test.yoru", r, parseErrh)
	file := p.Parse()
	if len(parseErrs) > 0 {
		t.Fatalf("parse errors:\n%s", strings.Join(parseErrs, "\n"))
	}

	var typeErrs []string
	typeErrh := func(pos syntax.Pos, msg string) {
		typeErrs = append(typeErrs, pos.String()+": "+msg)
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

	_, _ = types2.Check("test.yoru", file, conf, info)
	if len(typeErrs) > 0 {
		t.Fatalf("type errors:\n%s", strings.Join(typeErrs, "\n"))
	}

	funcs := BuildFile(file, info, types.DefaultSizes)
	for _, fn := range funcs {
		if err := Verify(fn); err != nil {
			t.Fatalf("Verify(%s) failed:\n%v\nSSA:\n%s", fn.Name, err, Sprint(fn))
		}
	}
	return funcs
}

// getFunc returns the function with the given name from a list, or calls t.Fatal.
func getFunc(t *testing.T, funcs []*Func, name string) *Func {
	t.Helper()
	for _, fn := range funcs {
		if fn.Name == name {
			return fn
		}
	}
	t.Fatalf("function %q not found", name)
	return nil
}

// --- Basic tests ---

func TestBuildEmptyFunc(t *testing.T) {
	src := `package main
func f() {
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")
	// Should have a single entry block with a void return.
	if fn.NumBlocks() != 1 {
		t.Errorf("NumBlocks = %d, want 1", fn.NumBlocks())
	}
	if fn.Entry.Kind != BlockReturn {
		t.Errorf("entry Kind = %v, want BlockReturn", fn.Entry.Kind)
	}
}

func TestBuildReturnConstant(t *testing.T) {
	src := `package main
func f() int {
	return 42
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	if fn.Entry.Kind != BlockReturn {
		t.Errorf("entry Kind = %v, want BlockReturn", fn.Entry.Kind)
	}
	// Should have a Const64 value.
	found := false
	for _, v := range fn.Entry.Values {
		if v.Op == OpConst64 && v.AuxInt == 42 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Const64 [42] in entry block\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildReturnParam(t *testing.T) {
	src := `package main
func f(x int) int {
	return x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Should have: Arg, Alloca, Store, Load.
	hasArg := false
	hasAlloca := false
	hasLoad := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			switch v.Op {
			case OpArg:
				hasArg = true
			case OpAlloca:
				hasAlloca = true
			case OpLoad:
				hasLoad = true
			}
		}
	}
	if !hasArg {
		t.Error("missing OpArg")
	}
	if !hasAlloca {
		t.Error("missing OpAlloca")
	}
	if !hasLoad {
		t.Error("missing OpLoad")
	}
}

func TestBuildArithmetic(t *testing.T) {
	src := `package main
func f(a int, b int) int {
	return a + b * 2
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasMul := false
	hasAdd := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpMul64 {
				hasMul = true
			}
			if v.Op == OpAdd64 {
				hasAdd = true
			}
		}
	}
	if !hasMul {
		t.Errorf("missing OpMul64\nSSA:\n%s", Sprint(fn))
	}
	if !hasAdd {
		t.Errorf("missing OpAdd64\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildVarDecl(t *testing.T) {
	src := `package main
func f() int {
	var x int = 10
	return x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasAlloca := false
	hasStore := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpAlloca {
				hasAlloca = true
			}
			if v.Op == OpStore {
				hasStore = true
			}
		}
	}
	if !hasAlloca {
		t.Error("missing OpAlloca for var x")
	}
	if !hasStore {
		t.Error("missing OpStore for var x = 10")
	}
}

func TestBuildShortDecl(t *testing.T) {
	src := `package main
func f() int {
	x := 5
	return x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasAlloca := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpAlloca {
				hasAlloca = true
			}
		}
	}
	if !hasAlloca {
		t.Error("missing OpAlloca for short decl x := 5")
	}
}

func TestBuildReassignment(t *testing.T) {
	src := `package main
func f() int {
	x := 1
	x = 2
	return x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Should have exactly 2 stores to x (one from := and one from =).
	storeCount := 0
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpStore {
				storeCount++
			}
		}
	}
	if storeCount != 2 {
		t.Errorf("expected 2 stores, got %d\nSSA:\n%s", storeCount, Sprint(fn))
	}
}

func TestBuildVarDeclZeroInit(t *testing.T) {
	src := `package main
func f() int {
	var x int
	return x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasZero := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpZero {
				hasZero = true
			}
		}
	}
	if !hasZero {
		t.Errorf("missing OpZero for zero-initialized var\nSSA:\n%s", Sprint(fn))
	}
}

// --- Control flow tests ---

func TestBuildIfNoElse(t *testing.T) {
	src := `package main
func f(x int) int {
	if x > 0 {
		return x
	}
	return 0
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Should have: entry (If), then (Return), done (Return).
	hasIf := false
	returnCount := 0
	for _, b := range fn.Blocks {
		if b.Kind == BlockIf {
			hasIf = true
		}
		if b.Kind == BlockReturn {
			returnCount++
		}
	}
	if !hasIf {
		t.Errorf("missing BlockIf\nSSA:\n%s", Sprint(fn))
	}
	if returnCount < 2 {
		t.Errorf("expected at least 2 return blocks, got %d\nSSA:\n%s", returnCount, Sprint(fn))
	}
}

func TestBuildIfElse(t *testing.T) {
	src := `package main
func f(x int) int {
	if x > 0 {
		return 1
	} else {
		return -1
	}
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasIf := false
	for _, b := range fn.Blocks {
		if b.Kind == BlockIf {
			hasIf = true
			if len(b.Succs) != 2 {
				t.Errorf("If block has %d succs, want 2", len(b.Succs))
			}
		}
	}
	if !hasIf {
		t.Errorf("missing BlockIf\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildElseIfChain(t *testing.T) {
	src := `package main
func f(x int) int {
	if x > 0 {
		return 1
	} else if x < 0 {
		return -1
	} else {
		return 0
	}
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Should have 2 If blocks (one for each condition).
	ifCount := 0
	for _, b := range fn.Blocks {
		if b.Kind == BlockIf {
			ifCount++
		}
	}
	if ifCount != 2 {
		t.Errorf("expected 2 If blocks for else-if chain, got %d\nSSA:\n%s", ifCount, Sprint(fn))
	}
}

func TestBuildForLoop(t *testing.T) {
	src := `package main
func f() int {
	var i int = 0
	for i < 10 {
		i = i + 1
	}
	return i
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// For loop generates a header block with an If.
	hasIf := false
	for _, b := range fn.Blocks {
		if b.Kind == BlockIf {
			hasIf = true
		}
	}
	if !hasIf {
		t.Errorf("missing BlockIf for loop condition\nSSA:\n%s", Sprint(fn))
	}

	// Should have a back-edge (header has itself as a predecessor's successor path).
	// At minimum: entry, header, body, exit blocks.
	if fn.NumBlocks() < 4 {
		t.Errorf("expected at least 4 blocks for a for loop, got %d\nSSA:\n%s", fn.NumBlocks(), Sprint(fn))
	}
}

func TestBuildForBreak(t *testing.T) {
	src := `package main
func f() int {
	var i int = 0
	for i < 100 {
		if i == 10 {
			break
		}
		i = i + 1
	}
	return i
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Should have multiple blocks including the break target.
	if fn.NumBlocks() < 5 {
		t.Errorf("expected at least 5 blocks with break, got %d\nSSA:\n%s", fn.NumBlocks(), Sprint(fn))
	}
}

func TestBuildForContinue(t *testing.T) {
	src := `package main
func f() int {
	var i int = 0
	var sum int = 0
	for i < 10 {
		i = i + 1
		if i == 5 {
			continue
		}
		sum = sum + i
	}
	return sum
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	if fn.NumBlocks() < 5 {
		t.Errorf("expected at least 5 blocks with continue, got %d\nSSA:\n%s", fn.NumBlocks(), Sprint(fn))
	}
}

func TestBuildBothBranchesReturn(t *testing.T) {
	src := `package main
func f(x int) int {
	if x > 0 {
		return 1
	} else {
		return -1
	}
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Both branches return, so there should be no Plain→done block used.
	// We simply verify it compiles and passes verification.
	_ = Sprint(fn)
}

// --- Short-circuit tests ---

func TestBuildShortCircuitAnd(t *testing.T) {
	src := `package main
func f(a bool, b bool) bool {
	return a && b
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// && produces a Phi node.
	hasPhi := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpPhi {
				hasPhi = true
			}
		}
	}
	if !hasPhi {
		t.Errorf("expected Phi for && short-circuit\nSSA:\n%s", Sprint(fn))
	}
	// Should have at least 4 blocks: entry, short, right, merge.
	if fn.NumBlocks() < 4 {
		t.Errorf("expected at least 4 blocks for &&, got %d\nSSA:\n%s", fn.NumBlocks(), Sprint(fn))
	}
}

func TestBuildShortCircuitOr(t *testing.T) {
	src := `package main
func f(a bool, b bool) bool {
	return a || b
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasPhi := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpPhi {
				hasPhi = true
			}
		}
	}
	if !hasPhi {
		t.Errorf("expected Phi for || short-circuit\nSSA:\n%s", Sprint(fn))
	}
}

// --- Builtin tests ---

func TestBuildPrintln(t *testing.T) {
	src := `package main
func f() {
	println(42)
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasPrintln := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpPrintln {
				hasPrintln = true
			}
		}
	}
	if !hasPrintln {
		t.Errorf("missing OpPrintln\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildPanic(t *testing.T) {
	src := `package main
func f() {
	panic("boom")
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasPanic := false
	hasExit := false
	for _, b := range fn.Blocks {
		if b.Kind == BlockExit {
			hasExit = true
		}
		for _, v := range b.Values {
			if v.Op == OpPanic {
				hasPanic = true
			}
		}
	}
	if !hasPanic {
		t.Errorf("missing OpPanic\nSSA:\n%s", Sprint(fn))
	}
	if !hasExit {
		t.Errorf("missing BlockExit\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildNew(t *testing.T) {
	src := `package main
type Point struct {
	x int
	y int
}
func f() ref Point {
	return new(Point)
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasNewAlloc := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpNewAlloc {
				hasNewAlloc = true
			}
		}
	}
	if !hasNewAlloc {
		t.Errorf("missing OpNewAlloc\nSSA:\n%s", Sprint(fn))
	}
}

// --- Composite type tests ---

func TestBuildStructLiteral(t *testing.T) {
	src := `package main
type Point struct {
	x int
	y int
}
func f() Point {
	return Point{x: 1, y: 2}
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasFieldPtr := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpStructFieldPtr {
				hasFieldPtr = true
			}
		}
	}
	if !hasFieldPtr {
		t.Errorf("missing OpStructFieldPtr for struct literal\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildFieldRead(t *testing.T) {
	src := `package main
type Point struct {
	x int
	y int
}
func f() int {
	var p Point
	return p.x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasFieldPtr := false
	hasLoad := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpStructFieldPtr {
				hasFieldPtr = true
			}
			if v.Op == OpLoad {
				hasLoad = true
			}
		}
	}
	if !hasFieldPtr {
		t.Errorf("missing OpStructFieldPtr\nSSA:\n%s", Sprint(fn))
	}
	if !hasLoad {
		t.Errorf("missing OpLoad\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildFieldWrite(t *testing.T) {
	src := `package main
type Point struct {
	x int
	y int
}
func f() {
	var p Point
	p.x = 42
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasFieldPtr := false
	hasStore := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpStructFieldPtr {
				hasFieldPtr = true
			}
			if v.Op == OpStore {
				hasStore = true
			}
		}
	}
	if !hasFieldPtr {
		t.Errorf("missing OpStructFieldPtr\nSSA:\n%s", Sprint(fn))
	}
	if !hasStore {
		t.Errorf("missing OpStore\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildArrayIndexRead(t *testing.T) {
	src := `package main
func f() int {
	var arr [5]int
	return arr[0]
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasArrayIdxPtr := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpArrayIndexPtr {
				hasArrayIdxPtr = true
			}
		}
	}
	if !hasArrayIdxPtr {
		t.Errorf("missing OpArrayIndexPtr\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildArrayIndexWrite(t *testing.T) {
	src := `package main
func f() {
	var arr [5]int
	arr[0] = 42
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasArrayIdxPtr := false
	hasStore := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpArrayIndexPtr {
				hasArrayIdxPtr = true
			}
			if v.Op == OpStore {
				hasStore = true
			}
		}
	}
	if !hasArrayIdxPtr {
		t.Errorf("missing OpArrayIndexPtr\nSSA:\n%s", Sprint(fn))
	}
	if !hasStore {
		t.Errorf("missing OpStore\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildAddressOf(t *testing.T) {
	// &x is used within the same function (no escape).
	src := `package main
func f() int {
	var x int = 10
	var p *int = &x
	return *p
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Verify it compiles and passes verification.
	hasLoad := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpLoad {
				hasLoad = true
			}
		}
	}
	if !hasLoad {
		t.Errorf("missing OpLoad for *p dereference\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildDeref(t *testing.T) {
	src := `package main
func f(p *int) int {
	return *p
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasLoad := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpLoad {
				hasLoad = true
			}
		}
	}
	if !hasLoad {
		t.Errorf("missing OpLoad for dereference\nSSA:\n%s", Sprint(fn))
	}
}

// --- Float operations ---

func TestBuildFloatOps(t *testing.T) {
	src := `package main
func f(a float, b float) float {
	return a + b
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasAddF64 := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpAddF64 {
				hasAddF64 = true
			}
		}
	}
	if !hasAddF64 {
		t.Errorf("expected OpAddF64 for float addition\nSSA:\n%s", Sprint(fn))
	}
}

// --- Boolean operations ---

func TestBuildBoolComparison(t *testing.T) {
	src := `package main
func f(x int) bool {
	return x > 5
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasGt := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpGt64 {
				hasGt = true
			}
		}
	}
	if !hasGt {
		t.Errorf("missing OpGt64\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildNot(t *testing.T) {
	src := `package main
func f(x bool) bool {
	return !x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasNot := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpNot {
				hasNot = true
			}
		}
	}
	if !hasNot {
		t.Errorf("missing OpNot\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildNegate(t *testing.T) {
	src := `package main
func f(x int) int {
	return -x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasNeg := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpNeg64 {
				hasNeg = true
			}
		}
	}
	if !hasNeg {
		t.Errorf("missing OpNeg64\nSSA:\n%s", Sprint(fn))
	}
}

// --- Function calls ---

func TestBuildFuncCall(t *testing.T) {
	src := `package main
func add(a int, b int) int {
	return a + b
}
func f() int {
	return add(1, 2)
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasStaticCall := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpStaticCall {
				hasStaticCall = true
			}
		}
	}
	if !hasStaticCall {
		t.Errorf("missing OpStaticCall\nSSA:\n%s", Sprint(fn))
	}
}

// --- Method calls ---

func TestBuildMethodCall(t *testing.T) {
	src := `package main
type Point struct {
	x int
	y int
}
func (p Point) Sum() int {
	return p.x + p.y
}
func f() int {
	var p Point
	return p.Sum()
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasStaticCall := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpStaticCall {
				hasStaticCall = true
			}
		}
	}
	if !hasStaticCall {
		t.Errorf("missing OpStaticCall for method call\nSSA:\n%s", Sprint(fn))
	}
}

// --- Nested loops ---

func TestBuildNestedLoops(t *testing.T) {
	src := `package main
func f() int {
	var sum int = 0
	var i int = 0
	for i < 3 {
		var j int = 0
		for j < 3 {
			sum = sum + 1
			j = j + 1
		}
		i = i + 1
	}
	return sum
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Nested loops should have 2 If blocks (one per loop header).
	ifCount := 0
	for _, b := range fn.Blocks {
		if b.Kind == BlockIf {
			ifCount++
		}
	}
	if ifCount != 2 {
		t.Errorf("expected 2 If blocks for nested loops, got %d\nSSA:\n%s", ifCount, Sprint(fn))
	}
}

// --- Multiple functions ---

func TestBuildMultipleFuncs(t *testing.T) {
	src := `package main
func add(a int, b int) int {
	return a + b
}
func sub(a int, b int) int {
	return a - b
}
func main() {
	println(add(1, 2))
	println(sub(5, 3))
}
`
	funcs := buildFromSource(t, src)
	if len(funcs) != 3 {
		t.Fatalf("expected 3 functions, got %d", len(funcs))
	}
	_ = getFunc(t, funcs, "add")
	_ = getFunc(t, funcs, "sub")
	_ = getFunc(t, funcs, "main")
}

// --- Block statement (scoping) ---

func TestBuildBlockStmt(t *testing.T) {
	src := `package main
func f() int {
	var x int = 1
	{
		var y int = 2
		x = y
	}
	return x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	// Should compile and verify fine; scoping is handled by type checker.
	_ = Sprint(fn)
}

// --- Ref field access ---

func TestBuildRefFieldAccess(t *testing.T) {
	src := `package main
type Point struct {
	x int
	y int
}
func f() int {
	var p ref Point = new(Point)
	return p.x
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasFieldPtr := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpStructFieldPtr {
				hasFieldPtr = true
			}
		}
	}
	if !hasFieldPtr {
		t.Errorf("missing OpStructFieldPtr for ref field access\nSSA:\n%s", Sprint(fn))
	}
}

// --- ExprStmt (void call) ---

func TestBuildExprStmt(t *testing.T) {
	src := `package main
func sideEffect() {
	println(1)
}
func f() {
	sideEffect()
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasCall := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpStaticCall {
				hasCall = true
			}
		}
	}
	if !hasCall {
		t.Errorf("missing OpStaticCall in ExprStmt\nSSA:\n%s", Sprint(fn))
	}
}

// --- Constant types ---

func TestBuildConstBool(t *testing.T) {
	src := `package main
func f() bool {
	return true
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasBool := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpConstBool && v.AuxInt == 1 {
				hasBool = true
			}
		}
	}
	if !hasBool {
		t.Errorf("missing OpConstBool [1]\nSSA:\n%s", Sprint(fn))
	}
}

func TestBuildConstString(t *testing.T) {
	src := `package main
func f() {
	println("hello")
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	hasStr := false
	for _, b := range fn.Blocks {
		for _, v := range b.Values {
			if v.Op == OpConstString {
				hasStr = true
			}
		}
	}
	if !hasStr {
		t.Errorf("missing OpConstString\nSSA:\n%s", Sprint(fn))
	}
}

// --- Golden test: SSA output format ---

func TestBuildGoldenSimple(t *testing.T) {
	src := `package main
func add(a int, b int) int {
	return a + b
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "add")
	got := Sprint(fn)

	// Verify key structural properties rather than exact output
	// (value IDs may shift).
	mustContain := []string{
		"func add(a int, b int) int:",
		"b0: (entry)",
		"Arg <int>",
		"Alloca <*int>",
		"Store",
		"Load <int>",
		"Add64 <int>",
		"Return",
	}
	for _, s := range mustContain {
		if !strings.Contains(got, s) {
			t.Errorf("output missing %q\ngot:\n%s", s, got)
		}
	}
}

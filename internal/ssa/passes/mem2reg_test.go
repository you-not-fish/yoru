package passes

import (
	"strings"
	"testing"

	"github.com/you-not-fish/yoru/internal/ssa"
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
	"github.com/you-not-fish/yoru/internal/types2"
)

// buildFromSource parses, type-checks, and builds SSA, then runs mem2reg.
func buildFromSource(t *testing.T, src string) []*ssa.Func {
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

	funcs := ssa.BuildFile(file, info, types.DefaultSizes)
	for _, fn := range funcs {
		if err := ssa.Verify(fn); err != nil {
			t.Fatalf("Verify(%s) before mem2reg failed:\n%v\nSSA:\n%s", fn.Name, err, ssa.Sprint(fn))
		}
	}
	return funcs
}

// buildAndRun builds SSA from source, runs mem2reg, and verifies.
func buildAndRun(t *testing.T, src string) []*ssa.Func {
	t.Helper()
	funcs := buildFromSource(t, src)
	for _, fn := range funcs {
		Mem2Reg(fn)
		if err := ssa.Verify(fn); err != nil {
			t.Fatalf("Verify(%s) after mem2reg failed:\n%v\nSSA:\n%s", fn.Name, err, ssa.Sprint(fn))
		}
	}
	return funcs
}

func getFunc(t *testing.T, funcs []*ssa.Func, name string) *ssa.Func {
	t.Helper()
	for _, fn := range funcs {
		if fn.Name == name {
			return fn
		}
	}
	t.Fatalf("function %q not found", name)
	return nil
}

func countOp(f *ssa.Func, op ssa.Op) int {
	n := 0
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			if v.Op == op {
				n++
			}
		}
	}
	return n
}

// --- Tests ---

func TestMem2RegSimpleReturn(t *testing.T) {
	src := `package main
func f() int {
	var x int = 42
	return x
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
	if n := countOp(fn, ssa.OpLoad); n != 0 {
		t.Errorf("loads after mem2reg = %d, want 0", n)
	}
	if n := countOp(fn, ssa.OpStore); n != 0 {
		t.Errorf("stores after mem2reg = %d, want 0", n)
	}
}

func TestMem2RegParameter(t *testing.T) {
	src := `package main
func f(x int) int {
	return x
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
	// Should have an OpArg still.
	if n := countOp(fn, ssa.OpArg); n != 1 {
		t.Errorf("args after mem2reg = %d, want 1", n)
	}
}

func TestMem2RegReassignment(t *testing.T) {
	src := `package main
func f() int {
	var x int = 1
	x = 2
	return x
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}

	// The return block should have Const64[2] as control.
	for _, b := range fn.Blocks {
		if b.Kind == ssa.BlockReturn && len(b.Controls) > 0 {
			ctrl := b.Controls[0]
			if ctrl.Op != ssa.OpConst64 || ctrl.AuxInt != 2 {
				t.Errorf("return value = %s, want Const64[2]", ctrl.LongString())
			}
		}
	}
}

func TestMem2RegDiamondPhi(t *testing.T) {
	src := `package main
func f(x int) int {
	var r int
	if x > 0 {
		r = 1
	} else {
		r = 2
	}
	return r
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
	if n := countOp(fn, ssa.OpPhi); n < 1 {
		t.Errorf("phis after mem2reg = %d, want >= 1", n)
	}
}

func TestMem2RegLoopPhi(t *testing.T) {
	src := `package main
func f() int {
	var i int = 0
	for i < 10 {
		i = i + 1
	}
	return i
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
	if n := countOp(fn, ssa.OpPhi); n < 1 {
		t.Errorf("phis after mem2reg = %d, want >= 1", n)
	}
}

func TestMem2RegNonPromotableStructField(t *testing.T) {
	src := `package main
type Point struct {
	x int
	y int
}
func f() int {
	var p Point
	p.x = 42
	return p.x
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	// The struct alloca should NOT be promoted (uses OpStructFieldPtr).
	if n := countOp(fn, ssa.OpAlloca); n == 0 {
		t.Error("struct alloca was incorrectly promoted")
	}
}

func TestMem2RegNonPromotableAddressTaken(t *testing.T) {
	// Test using manual SSA construction since Yoru's type system prevents
	// passing *T to functions. We construct an alloca with an OpAddr use.
	sig := types.NewFunc(nil, nil, types.Typ[types.Int])
	f := ssa.NewFunc("f", sig)
	entry := f.Entry

	// alloca for x
	alloca := f.NewValue(entry, ssa.OpAlloca, types.NewPointer(types.Typ[types.Int]))
	alloca.Aux = "x"
	// Store 42
	c42 := f.NewValue(entry, ssa.OpConst64, types.Typ[types.Int])
	c42.AuxInt = 42
	store := f.NewValue(entry, ssa.OpStore, nil, alloca, c42)
	_ = store
	// OpAddr (take address — makes alloca non-promotable)
	addr := f.NewValue(entry, ssa.OpAddr, types.NewPointer(types.Typ[types.Int]), alloca)
	_ = addr
	// Load and return
	load := f.NewValue(entry, ssa.OpLoad, types.Typ[types.Int], alloca)
	entry.Kind = ssa.BlockReturn
	entry.SetControl(load)

	Mem2Reg(f)

	// x's alloca should NOT be promoted (address taken via OpAddr).
	if countOp(f, ssa.OpAlloca) == 0 {
		t.Error("address-taken alloca was incorrectly promoted")
	}
}

func TestMem2RegShortCircuitCoexistence(t *testing.T) {
	src := `package main
func f(a bool, b bool) bool {
	return a && b
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	// Param allocas should be promoted but existing phis (from &&) preserved.
	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
	// && creates a phi — should still exist.
	if n := countOp(fn, ssa.OpPhi); n < 1 {
		t.Errorf("phis after mem2reg = %d, want >= 1", n)
	}
}

func TestMem2RegZeroInit(t *testing.T) {
	src := `package main
func f() int {
	var x int
	return x
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}

	// Return value should be Const64[0] or the zero value.
	for _, b := range fn.Blocks {
		if b.Kind == ssa.BlockReturn && len(b.Controls) > 0 {
			ctrl := b.Controls[0]
			if ctrl.Op != ssa.OpConst64 || ctrl.AuxInt != 0 {
				t.Errorf("return value = %s, want Const64[0]", ctrl.LongString())
			}
		}
	}
}

func TestMem2RegMultipleVarsWithPhi(t *testing.T) {
	src := `package main
func f(n int) int {
	var x int = 1
	var y int = 2
	if n > 0 {
		x = 10
		y = 20
	}
	return x + y
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
	// There should be phis for both x and y at the merge point.
	if n := countOp(fn, ssa.OpPhi); n < 2 {
		t.Errorf("phis after mem2reg = %d, want >= 2", n)
	}
}

func TestMem2RegAllocaCount(t *testing.T) {
	src := `package main
func f(x int, y int) int {
	var z int = x + y
	return z
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")
	before := countOp(fn, ssa.OpAlloca)

	Mem2Reg(fn)

	after := countOp(fn, ssa.OpAlloca)
	if after >= before {
		t.Errorf("alloca count: before=%d, after=%d; expected reduction", before, after)
	}
	if after != 0 {
		t.Errorf("alloca count after = %d, want 0", after)
	}
}

func TestMem2RegBoolVar(t *testing.T) {
	src := `package main
func f() bool {
	var x bool
	return x
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
	for _, b := range fn.Blocks {
		if b.Kind == ssa.BlockReturn && len(b.Controls) > 0 {
			ctrl := b.Controls[0]
			if ctrl.Op != ssa.OpConstBool || ctrl.AuxInt != 0 {
				t.Errorf("return value = %s, want ConstBool[0]", ctrl.LongString())
			}
		}
	}
}

func TestMem2RegStringVar(t *testing.T) {
	src := `package main
func f() string {
	var s string = "hello"
	return s
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
}

func TestMem2RegFloatVar(t *testing.T) {
	src := `package main
func f() float {
	var x float = 3.14
	return x
}
`
	funcs := buildAndRun(t, src)
	fn := getFunc(t, funcs, "f")

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after mem2reg = %d, want 0", n)
	}
}

// TestMem2RegExistingBuildTests verifies all existing build_test.go patterns
// still pass after mem2reg by running a comprehensive source.
func TestMem2RegExistingBuildTests(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"empty", `package main
func f() {}
`},
		{"const_return", `package main
func f() int {
	return 42
}
`},
		{"param_return", `package main
func f(x int) int {
	return x
}
`},
		{"add", `package main
func f(x int, y int) int {
	return x + y
}
`},
		{"if_else", `package main
func f(x int) int {
	if x > 0 {
		return 1
	} else {
		return -1
	}
}
`},
		{"for_loop", `package main
func f() int {
	var i int = 0
	for i < 10 {
		i = i + 1
	}
	return i
}
`},
		{"short_circuit_and", `package main
func f(a bool, b bool) bool {
	return a && b
}
`},
		{"short_circuit_or", `package main
func f(a bool, b bool) bool {
	return a || b
}
`},
		{"nested_if", `package main
func f(x int) int {
	var r int = 0
	if x > 0 {
		if x > 10 {
			r = 2
		} else {
			r = 1
		}
	}
	return r
}
`},
		{"multi_param", `package main
func f(a int, b int, c int) int {
	return a + b + c
}
`},
		{"void_func", `package main
func f() {
	println(42)
}
`},
		{"break_continue", `package main
func f() int {
	var i int = 0
	for i < 100 {
		if i > 5 {
			break
		}
		i = i + 1
	}
	return i
}
`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			funcs := buildAndRun(t, tt.src)
			if len(funcs) == 0 {
				t.Fatal("no functions built")
			}
		})
	}
}

func TestMem2RegPassRunner(t *testing.T) {
	src := `package main
func f(x int) int {
	var y int = x + 1
	return y
}
`
	funcs := buildFromSource(t, src)
	fn := getFunc(t, funcs, "f")

	pipeline := []Pass{
		{Name: "mem2reg", Fn: Mem2Reg},
	}

	err := Run(fn, pipeline, Config{Verify: true})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if n := countOp(fn, ssa.OpAlloca); n != 0 {
		t.Errorf("allocas after pass runner = %d, want 0", n)
	}
}

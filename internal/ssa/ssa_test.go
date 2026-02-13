package ssa

import (
	"strings"
	"testing"

	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// nopos is the zero position for convenience in tests.
var nopos syntax.Pos

// makeAddFunc builds: func add(x, y int) int { return x + y }
func makeAddFunc() *Func {
	sig := types.NewFunc(
		nil, // no receiver
		[]*types.Var{
			types.NewVar(nopos, "x", types.Typ[types.Int]),
			types.NewVar(nopos, "y", types.Typ[types.Int]),
		},
		types.Typ[types.Int],
	)

	f := NewFunc("add", sig)
	entry := f.Entry

	// v0 = Arg <int> {x}
	v0 := f.NewValue(entry, OpArg, types.Typ[types.Int])
	v0.AuxInt = 0
	v0.Aux = "x"

	// v1 = Arg <int> {y}
	v1 := f.NewValue(entry, OpArg, types.Typ[types.Int])
	v1.AuxInt = 1
	v1.Aux = "y"

	// v2 = Add64 <int> v0 v1
	v2 := f.NewValue(entry, OpAdd64, types.Typ[types.Int], v0, v1)

	// Return v2
	entry.Kind = BlockReturn
	entry.SetControl(v2)

	return f
}

func TestManualConstruct(t *testing.T) {
	f := makeAddFunc()

	// Basic structure checks
	if f.Name != "add" {
		t.Errorf("Name = %q, want %q", f.Name, "add")
	}
	if f.NumBlocks() != 1 {
		t.Errorf("NumBlocks = %d, want 1", f.NumBlocks())
	}
	if f.NumValues() != 3 {
		t.Errorf("NumValues = %d, want 3", f.NumValues())
	}

	entry := f.Entry
	if entry.Kind != BlockReturn {
		t.Errorf("entry Kind = %v, want BlockReturn", entry.Kind)
	}
	if len(entry.Values) != 3 {
		t.Errorf("entry has %d values, want 3", len(entry.Values))
	}

	// Check the add value
	addVal := entry.Values[2]
	if addVal.Op != OpAdd64 {
		t.Errorf("value[2].Op = %v, want OpAdd64", addVal.Op)
	}
	if len(addVal.Args) != 2 {
		t.Errorf("add has %d args, want 2", len(addVal.Args))
	}

	// Verify passes
	if err := Verify(f); err != nil {
		t.Errorf("Verify failed: %v", err)
	}
}

func TestPrintFormat(t *testing.T) {
	f := makeAddFunc()
	got := Sprint(f)

	// Check key parts of the output
	want := `func add(x int, y int) int:
  b0: (entry)
    v0 = Arg <int> {x}
    v1 = Arg <int> [1] {y}
    v2 = Add64 <int> v0 v1
    Return v2
`
	if got != want {
		t.Errorf("Sprint output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintIfBlock(t *testing.T) {
	// Build: func abs(n int) int { if n < 0 { return -n } return n }
	sig := types.NewFunc(nil,
		[]*types.Var{types.NewVar(nopos, "n", types.Typ[types.Int])},
		types.Typ[types.Int])
	f := NewFunc("abs", sig)
	entry := f.Entry

	v0 := f.NewValue(entry, OpArg, types.Typ[types.Int])
	v0.Aux = "n"

	v1 := f.NewValue(entry, OpConst64, types.Typ[types.Int])
	v1.AuxInt = 0

	v2 := f.NewValue(entry, OpLt64, types.Typ[types.Bool], v0, v1)

	// then block: return -n
	bThen := f.NewBlock(BlockReturn)
	v3 := f.NewValue(bThen, OpNeg64, types.Typ[types.Int], v0)
	bThen.SetControl(v3)

	// else block: return n
	bElse := f.NewBlock(BlockReturn)
	bElse.SetControl(v0)

	// Wire entry as If block
	entry.Kind = BlockIf
	entry.SetControl(v2)
	entry.AddSucc(bThen)
	entry.AddSucc(bElse)

	if err := Verify(f); err != nil {
		t.Errorf("Verify failed: %v", err)
	}

	got := Sprint(f)
	// Just check that it contains the expected patterns
	if !strings.Contains(got, "If v2 -> b1 b2") {
		t.Errorf("output missing If terminator, got:\n%s", got)
	}
	if !strings.Contains(got, "v3 = Neg64 <int> v0") {
		t.Errorf("output missing Neg64, got:\n%s", got)
	}
}

func TestPrintPhiBlock(t *testing.T) {
	// Build a function with a phi node
	sig := types.NewFunc(nil,
		[]*types.Var{types.NewVar(nopos, "x", types.Typ[types.Int])},
		types.Typ[types.Int])
	f := NewFunc("phi_test", sig)
	entry := f.Entry

	v0 := f.NewValue(entry, OpArg, types.Typ[types.Int])
	v0.Aux = "x"
	v1 := f.NewValue(entry, OpConst64, types.Typ[types.Int])
	v1.AuxInt = 1

	// merge block
	merge := f.NewBlock(BlockReturn)
	phi := f.NewValue(merge, OpPhi, types.Typ[types.Int], v0, v1)
	merge.SetControl(phi)

	// Wire: entry -> merge (two paths via dummy setup)
	// For testing phi, we need exactly 2 preds
	entry2 := f.NewBlock(BlockPlain)
	entry2.AddSucc(merge)
	entry.Kind = BlockPlain
	entry.Succs = nil // clear default
	entry.AddSucc(merge)

	if err := Verify(f); err != nil {
		t.Errorf("Verify failed: %v", err)
	}

	got := Sprint(f)
	if !strings.Contains(got, "Phi <int> v0 v1") {
		t.Errorf("output missing Phi, got:\n%s", got)
	}
}

func TestVerifyNilType(t *testing.T) {
	f := NewFunc("bad_nil_type", nil)
	entry := f.Entry

	// Create a non-void value with nil type — should fail
	v := f.NewValue(entry, OpAdd64, nil)
	_ = v

	entry.Kind = BlockReturn

	err := Verify(f)
	if err == nil {
		t.Fatal("Verify should fail for nil type on non-void value")
	}
	if !strings.Contains(err.Error(), "nil Type") {
		t.Errorf("error should mention nil Type, got: %v", err)
	}
}

func TestVerifyNoTerminator(t *testing.T) {
	f := NewFunc("bad_no_term", nil)
	_ = f.Entry

	// Entry is BlockPlain by default but has no successors
	// This should trigger "plain block has 0 succs, want 1"

	err := Verify(f)
	if err == nil {
		t.Fatal("Verify should fail for plain block with no successors")
	}
	if !strings.Contains(err.Error(), "plain block has 0 succs") {
		t.Errorf("error should mention succs, got: %v", err)
	}
}

func TestVerifyPhiArgCount(t *testing.T) {
	sig := types.NewFunc(nil,
		[]*types.Var{types.NewVar(nopos, "x", types.Typ[types.Int])},
		types.Typ[types.Int])
	f := NewFunc("bad_phi", sig)
	entry := f.Entry

	v0 := f.NewValue(entry, OpArg, types.Typ[types.Int])
	v0.Aux = "x"

	// merge with 2 preds but phi with 1 arg
	merge := f.NewBlock(BlockReturn)
	phi := f.NewValue(merge, OpPhi, types.Typ[types.Int], v0) // only 1 arg
	merge.SetControl(phi)

	entry2 := f.NewBlock(BlockPlain)
	entry2.AddSucc(merge)
	entry.Kind = BlockPlain
	entry.AddSucc(merge)

	// merge has 2 preds but phi has 1 arg
	err := Verify(f)
	if err == nil {
		t.Fatal("Verify should fail for phi arg count mismatch")
	}
	if !strings.Contains(err.Error(), "phi has 1 args but block has 2 preds") {
		t.Errorf("error should mention phi arg count, got: %v", err)
	}
}

func TestVerifyInconsistentEdges(t *testing.T) {
	f := NewFunc("bad_edges", nil)
	entry := f.Entry
	entry.Kind = BlockReturn

	// Create a block that claims entry as a pred, but entry doesn't have it as succ
	orphan := f.NewBlock(BlockReturn)
	orphan.Preds = append(orphan.Preds, entry)
	// Deliberately do NOT add orphan to entry.Succs

	err := Verify(f)
	if err == nil {
		t.Fatal("Verify should fail for inconsistent edges")
	}
	if !strings.Contains(err.Error(), "does not have") {
		t.Errorf("error should mention edge inconsistency, got: %v", err)
	}
}

func TestVerifyEntryNoPreds(t *testing.T) {
	f := NewFunc("bad_entry", nil)
	entry := f.Entry
	entry.Kind = BlockReturn

	// Manually add a predecessor to entry
	extra := f.NewBlock(BlockPlain)
	extra.AddSucc(entry)

	err := Verify(f)
	if err == nil {
		t.Fatal("Verify should fail for entry with predecessors")
	}
	if !strings.Contains(err.Error(), "entry block") && !strings.Contains(err.Error(), "predecessors") {
		t.Errorf("error should mention entry preds, got: %v", err)
	}
}

func TestVerifyBlockInvalidKind(t *testing.T) {
	f := NewFunc("bad_kind", nil)
	entry := f.Entry
	entry.Kind = BlockReturn

	// Create a block with invalid kind
	bad := f.NewBlock(BlockInvalid)
	_ = bad

	err := Verify(f)
	if err == nil {
		t.Fatal("Verify should fail for invalid block kind")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("error should mention invalid kind, got: %v", err)
	}
}

func TestVerifyValueBlockMismatch(t *testing.T) {
	f := NewFunc("bad_vblock", nil)
	entry := f.Entry
	entry.Kind = BlockReturn

	b2 := f.NewBlock(BlockReturn)

	// Create a value in entry but set its Block to b2
	v := f.NewValue(entry, OpConst64, types.Typ[types.Int])
	v.Block = b2 // mismatch!

	err := Verify(f)
	if err == nil {
		t.Fatal("Verify should fail for value Block mismatch")
	}
	if !strings.Contains(err.Error(), "Block pointer") {
		t.Errorf("error should mention Block pointer, got: %v", err)
	}
}

func TestVerifyNilArg(t *testing.T) {
	f := NewFunc("bad_nil_arg", nil)
	entry := f.Entry
	entry.Kind = BlockReturn

	v := f.NewValue(entry, OpAdd64, types.Typ[types.Int])
	v.Args = []*Value{nil} // nil arg

	err := Verify(f)
	if err == nil {
		t.Fatal("Verify should fail for nil arg")
	}
	if !strings.Contains(err.Error(), "arg[0] is nil") {
		t.Errorf("error should mention nil arg, got: %v", err)
	}
}

func TestVerifyValid(t *testing.T) {
	f := makeAddFunc()
	if err := Verify(f); err != nil {
		t.Errorf("Verify failed on valid function: %v", err)
	}
}

func TestOpIsPure(t *testing.T) {
	pureOps := []Op{
		OpConst64, OpConstFloat, OpConstBool, OpConstString, OpConstNil,
		OpAdd64, OpSub64, OpMul64, OpDiv64, OpMod64, OpNeg64,
		OpAddF64, OpSubF64, OpMulF64, OpDivF64, OpNegF64,
		OpEq64, OpNeq64, OpLt64, OpLeq64, OpGt64, OpGeq64,
		OpEqF64, OpNeqF64, OpLtF64, OpLeqF64, OpGtF64, OpGeqF64,
		OpEqPtr, OpNeqPtr,
		OpNot, OpAndBool, OpOrBool,
		OpStructFieldPtr, OpArrayIndexPtr,
		OpIntToFloat, OpFloatToInt,
		OpPhi, OpCopy, OpArg,
		OpAddr,
		OpStringLen, OpStringPtr,
	}

	for _, op := range pureOps {
		if !op.IsPure() {
			t.Errorf("Op %s should be pure", op)
		}
	}

	impureOps := []Op{
		OpAlloca, OpLoad, OpStore, OpZero,
		OpStaticCall, OpCall,
		OpNewAlloc,
		OpPrintln, OpPanic,
		OpNilCheck,
	}

	for _, op := range impureOps {
		if op.IsPure() {
			t.Errorf("Op %s should NOT be pure", op)
		}
	}
}

func TestOpIsVoid(t *testing.T) {
	voidOps := []Op{OpStore, OpZero, OpPrintln, OpPanic, OpNilCheck}
	for _, op := range voidOps {
		if !op.IsVoid() {
			t.Errorf("Op %s should be void", op)
		}
	}

	nonVoidOps := []Op{
		OpConst64, OpAdd64, OpLoad, OpAlloca, OpStaticCall,
		OpPhi, OpArg, OpNewAlloc,
	}
	for _, op := range nonVoidOps {
		if op.IsVoid() {
			t.Errorf("Op %s should NOT be void", op)
		}
	}
}

func TestOpString(t *testing.T) {
	tests := []struct {
		op   Op
		want string
	}{
		{OpAdd64, "Add64"},
		{OpConst64, "Const64"},
		{OpPhi, "Phi"},
		{OpStaticCall, "StaticCall"},
		{OpStore, "Store"},
		{OpInvalid, "Invalid"},
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("Op(%d).String() = %q, want %q", tt.op, got, tt.want)
		}
	}
}

func TestBlockKindString(t *testing.T) {
	tests := []struct {
		kind BlockKind
		want string
	}{
		{BlockPlain, "plain"},
		{BlockIf, "if"},
		{BlockReturn, "ret"},
		{BlockExit, "exit"},
		{BlockInvalid, "invalid"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("BlockKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestValueString(t *testing.T) {
	f := NewFunc("test", nil)
	entry := f.Entry
	entry.Kind = BlockReturn

	v := f.NewValue(entry, OpConst64, types.Typ[types.Int])
	v.AuxInt = 42

	if got := v.String(); got != "v0" {
		t.Errorf("Value.String() = %q, want %q", got, "v0")
	}

	longStr := v.LongString()
	if !strings.Contains(longStr, "Const64") {
		t.Errorf("LongString missing op name: %s", longStr)
	}
	if !strings.Contains(longStr, "[42]") {
		t.Errorf("LongString missing AuxInt: %s", longStr)
	}
}

func TestValueUseCount(t *testing.T) {
	f := NewFunc("test", nil)
	entry := f.Entry
	entry.Kind = BlockReturn

	v0 := f.NewValue(entry, OpConst64, types.Typ[types.Int])
	v0.AuxInt = 1

	v1 := f.NewValue(entry, OpConst64, types.Typ[types.Int])
	v1.AuxInt = 2

	// v2 uses v0 and v1
	_ = f.NewValue(entry, OpAdd64, types.Typ[types.Int], v0, v1)

	if v0.Uses != 1 {
		t.Errorf("v0.Uses = %d, want 1", v0.Uses)
	}
	if v1.Uses != 1 {
		t.Errorf("v1.Uses = %d, want 1", v1.Uses)
	}

	// v3 also uses v0
	_ = f.NewValue(entry, OpNeg64, types.Typ[types.Int], v0)

	if v0.Uses != 2 {
		t.Errorf("v0.Uses = %d, want 2 after second use", v0.Uses)
	}
}

func TestFuncNewBlock(t *testing.T) {
	f := NewFunc("test", nil)

	// Entry block is b0
	if f.Entry.ID != 0 {
		t.Errorf("entry ID = %d, want 0", f.Entry.ID)
	}

	b1 := f.NewBlock(BlockReturn)
	if b1.ID != 1 {
		t.Errorf("new block ID = %d, want 1", b1.ID)
	}

	if f.NumBlocks() != 2 {
		t.Errorf("NumBlocks = %d, want 2", f.NumBlocks())
	}
}

func TestNewFuncCreatesEntry(t *testing.T) {
	f := NewFunc("test", nil)
	if f.Entry == nil {
		t.Fatal("NewFunc should create an entry block")
	}
	if len(f.Blocks) != 1 {
		t.Errorf("NewFunc should have 1 block, got %d", len(f.Blocks))
	}
	if f.Blocks[0] != f.Entry {
		t.Error("Blocks[0] should be the entry block")
	}
}

// TestNewVar0 verifies the types.NewVar signature works with our usage.
// This is a compile-time check more than runtime.
func TestTypesIntegration(t *testing.T) {
	sig := types.NewFunc(
		nil,
		[]*types.Var{
			types.NewVar(nopos, "a", types.Typ[types.Int]),
			types.NewVar(nopos, "b", types.Typ[types.Float]),
		},
		types.Typ[types.Bool],
	)

	if sig.NumParams() != 2 {
		t.Errorf("NumParams = %d, want 2", sig.NumParams())
	}
	if sig.Result() != types.Typ[types.Bool] {
		t.Errorf("Result = %v, want bool", sig.Result())
	}
}

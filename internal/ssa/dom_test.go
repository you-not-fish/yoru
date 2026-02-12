package ssa

import (
	"testing"

	"github.com/you-not-fish/yoru/internal/types"
)

// makeSig is a helper that returns a simple void function signature.
func makeSig() *types.Func {
	return types.NewFunc(nil, nil, nil)
}

// TestDomSingleBlock verifies that a single-block function has Idom=nil.
func TestDomSingleBlock(t *testing.T) {
	f := NewFunc("f", makeSig())
	f.Entry.Kind = BlockReturn

	ComputeDom(f)

	if f.Entry.Idom != nil {
		t.Errorf("entry Idom = %v, want nil", f.Entry.Idom)
	}
	if len(f.Entry.Dominees) != 0 {
		t.Errorf("entry Dominees = %d, want 0", len(f.Entry.Dominees))
	}
}

// TestDomLinearChain verifies: b0 → b1 → b2
func TestDomLinearChain(t *testing.T) {
	f := NewFunc("f", makeSig())
	b0 := f.Entry
	b1 := f.NewBlock(BlockPlain)
	b2 := f.NewBlock(BlockReturn)

	b0.AddSucc(b1)
	b1.AddSucc(b2)

	ComputeDom(f)

	if b0.Idom != nil {
		t.Errorf("b0.Idom = %v, want nil", b0.Idom)
	}
	if b1.Idom != b0 {
		t.Errorf("b1.Idom = %v, want %v", b1.Idom, b0)
	}
	if b2.Idom != b1 {
		t.Errorf("b2.Idom = %v, want %v", b2.Idom, b1)
	}
}

// TestDomDiamond verifies:
//
//	b0
//	├→ b1 ─┐
//	└→ b2 ─┘
//	   b3
func TestDomDiamond(t *testing.T) {
	f := NewFunc("f", makeSig())
	b0 := f.Entry
	b1 := f.NewBlock(BlockPlain)
	b2 := f.NewBlock(BlockPlain)
	b3 := f.NewBlock(BlockReturn)

	// b0 branches to b1 and b2.
	cond := f.NewValue(b0, OpConstBool, types.Typ[types.Bool])
	cond.AuxInt = 1
	b0.Kind = BlockIf
	b0.SetControl(cond)
	b0.AddSucc(b1)
	b0.AddSucc(b2)

	b1.AddSucc(b3)
	b2.AddSucc(b3)

	ComputeDom(f)

	if b0.Idom != nil {
		t.Errorf("b0.Idom = %v, want nil", b0.Idom)
	}
	if b1.Idom != b0 {
		t.Errorf("b1.Idom = %v, want %v", b1.Idom, b0)
	}
	if b2.Idom != b0 {
		t.Errorf("b2.Idom = %v, want %v", b2.Idom, b0)
	}
	if b3.Idom != b0 {
		t.Errorf("b3.Idom = %v, want %v", b3.Idom, b0)
	}

	// Dominance frontier: DF(b1)={b3}, DF(b2)={b3}.
	df := ComputeDomFrontier(f)
	assertDF(t, df, b0, nil)
	assertDF(t, df, b1, []*Block{b3})
	assertDF(t, df, b2, []*Block{b3})
	assertDF(t, df, b3, nil)
}

// TestDomLoop verifies:
//
//	b0 → b1 → b2
//	      ↑    │
//	      └────┘
//	      b1 → b3
func TestDomLoop(t *testing.T) {
	f := NewFunc("f", makeSig())
	b0 := f.Entry
	b1 := f.NewBlock(BlockIf)
	b2 := f.NewBlock(BlockPlain)
	b3 := f.NewBlock(BlockReturn)

	b0.AddSucc(b1)

	// b1: if cond → b2 (body) else b3 (exit)
	cond := f.NewValue(b1, OpConstBool, types.Typ[types.Bool])
	cond.AuxInt = 1
	b1.SetControl(cond)
	b1.AddSucc(b2)
	b1.AddSucc(b3)

	// b2 loops back to b1.
	b2.AddSucc(b1)

	ComputeDom(f)

	if b0.Idom != nil {
		t.Errorf("b0.Idom = %v, want nil", b0.Idom)
	}
	if b1.Idom != b0 {
		t.Errorf("b1.Idom = %v, want %v", b1.Idom, b0)
	}
	if b2.Idom != b1 {
		t.Errorf("b2.Idom = %v, want %v", b2.Idom, b1)
	}
	if b3.Idom != b1 {
		t.Errorf("b3.Idom = %v, want %v", b3.Idom, b1)
	}

	// DF(b2) = {b1} (back-edge).
	df := ComputeDomFrontier(f)
	assertDF(t, df, b2, []*Block{b1})
}

// TestRPOOrdering verifies that RPO visits blocks in correct order.
func TestRPOOrdering(t *testing.T) {
	f := NewFunc("f", makeSig())
	b0 := f.Entry
	b1 := f.NewBlock(BlockPlain)
	b2 := f.NewBlock(BlockPlain)
	b3 := f.NewBlock(BlockReturn)

	cond := f.NewValue(b0, OpConstBool, types.Typ[types.Bool])
	cond.AuxInt = 1
	b0.Kind = BlockIf
	b0.SetControl(cond)
	b0.AddSucc(b1)
	b0.AddSucc(b2)
	b1.AddSucc(b3)
	b2.AddSucc(b3)

	rpo := ReversePostOrder(f)

	if len(rpo) != 4 {
		t.Fatalf("RPO len = %d, want 4", len(rpo))
	}
	if rpo[0] != b0 {
		t.Errorf("RPO[0] = %v, want %v", rpo[0], b0)
	}
	// b3 must be last since it has the most predecessors to visit.
	if rpo[3] != b3 {
		t.Errorf("RPO[3] = %v, want %v", rpo[3], b3)
	}
}

// TestDomComplex verifies a more complex nested structure:
//
//	   b0
//	   ├→ b1 → b3
//	   └→ b2 → b3
//	          b3
//	          ├→ b4 → b6
//	          └→ b5 → b6
//	               b6
func TestDomComplex(t *testing.T) {
	f := NewFunc("f", makeSig())
	b0 := f.Entry
	b1 := f.NewBlock(BlockPlain)
	b2 := f.NewBlock(BlockPlain)
	b3 := f.NewBlock(BlockIf)
	b4 := f.NewBlock(BlockPlain)
	b5 := f.NewBlock(BlockPlain)
	b6 := f.NewBlock(BlockReturn)

	cond0 := f.NewValue(b0, OpConstBool, types.Typ[types.Bool])
	cond0.AuxInt = 1
	b0.Kind = BlockIf
	b0.SetControl(cond0)
	b0.AddSucc(b1)
	b0.AddSucc(b2)

	b1.AddSucc(b3)
	b2.AddSucc(b3)

	cond3 := f.NewValue(b3, OpConstBool, types.Typ[types.Bool])
	cond3.AuxInt = 1
	b3.SetControl(cond3)
	b3.AddSucc(b4)
	b3.AddSucc(b5)

	b4.AddSucc(b6)
	b5.AddSucc(b6)

	ComputeDom(f)

	// b0 dominates everything.
	if b0.Idom != nil {
		t.Errorf("b0.Idom = %v, want nil", b0.Idom)
	}
	if b1.Idom != b0 {
		t.Errorf("b1.Idom = %v, want %v", b1.Idom, b0)
	}
	if b2.Idom != b0 {
		t.Errorf("b2.Idom = %v, want %v", b2.Idom, b0)
	}
	if b3.Idom != b0 {
		t.Errorf("b3.Idom = %v, want %v", b3.Idom, b0)
	}
	if b4.Idom != b3 {
		t.Errorf("b4.Idom = %v, want %v", b4.Idom, b3)
	}
	if b5.Idom != b3 {
		t.Errorf("b5.Idom = %v, want %v", b5.Idom, b3)
	}
	if b6.Idom != b3 {
		t.Errorf("b6.Idom = %v, want %v", b6.Idom, b3)
	}

	df := ComputeDomFrontier(f)
	assertDF(t, df, b1, []*Block{b3})
	assertDF(t, df, b2, []*Block{b3})
	assertDF(t, df, b4, []*Block{b6})
	assertDF(t, df, b5, []*Block{b6})
}

// TestDomNestedLoop verifies:
//
//	b0 → b1 → b2 → b3 → b2 (inner loop back-edge)
//	           ↑         │
//	           └─────────┘ (outer: b2→b1 not present, b1→b4)
//
// Simplified: b0 → b1 → b2, b2 → b1 (back), b1 → b3
func TestDomNestedLoop(t *testing.T) {
	f := NewFunc("f", makeSig())
	b0 := f.Entry
	b1 := f.NewBlock(BlockIf)
	b2 := f.NewBlock(BlockPlain)
	b3 := f.NewBlock(BlockReturn)

	b0.AddSucc(b1)

	cond := f.NewValue(b1, OpConstBool, types.Typ[types.Bool])
	cond.AuxInt = 1
	b1.SetControl(cond)
	b1.AddSucc(b2) // body
	b1.AddSucc(b3) // exit

	b2.AddSucc(b1) // back-edge

	ComputeDom(f)

	if b1.Idom != b0 {
		t.Errorf("b1.Idom = %v, want %v", b1.Idom, b0)
	}
	if b2.Idom != b1 {
		t.Errorf("b2.Idom = %v, want %v", b2.Idom, b1)
	}
	if b3.Idom != b1 {
		t.Errorf("b3.Idom = %v, want %v", b3.Idom, b1)
	}

	df := ComputeDomFrontier(f)
	assertDF(t, df, b2, []*Block{b1})
}

// TestDomUnreachable verifies that unreachable blocks get no Idom.
func TestDomUnreachable(t *testing.T) {
	f := NewFunc("f", makeSig())
	b0 := f.Entry
	b0.Kind = BlockReturn

	// b1 is unreachable.
	b1 := f.NewBlock(BlockReturn)

	ComputeDom(f)

	if b0.Idom != nil {
		t.Errorf("b0.Idom = %v, want nil", b0.Idom)
	}
	if b1.Idom != nil {
		t.Errorf("unreachable b1.Idom = %v, want nil", b1.Idom)
	}
}

// assertDF checks that the dominance frontier of b equals the expected set.
func assertDF(t *testing.T, df map[*Block][]*Block, b *Block, want []*Block) {
	t.Helper()
	got := df[b]
	if len(got) != len(want) {
		t.Errorf("DF(%v) = %v (len %d), want %v (len %d)", b, got, len(got), want, len(want))
		return
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DF(%v) missing %v, got %v", b, w, got)
		}
	}
}

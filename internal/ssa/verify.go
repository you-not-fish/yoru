package ssa

import (
	"fmt"
	"strings"
)

// Verify checks the structural integrity of an SSA function.
// It returns an error describing all violations found, or nil if valid.
func Verify(f *Func) error {
	var errs []string

	add := func(format string, args ...interface{}) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	if f.Entry == nil {
		add("func %s: entry block is nil", f.Name)
		return combineErrors(errs)
	}

	if len(f.Blocks) == 0 {
		add("func %s: no blocks", f.Name)
		return combineErrors(errs)
	}

	if f.Blocks[0] != f.Entry {
		add("func %s: Blocks[0] is not the entry block", f.Name)
	}

	// 1. Entry block has no predecessors
	if len(f.Entry.Preds) != 0 {
		add("func %s: entry block %s has %d predecessors, want 0",
			f.Name, f.Entry, len(f.Entry.Preds))
	}

	// Build a set of all blocks for membership checks.
	blockSet := make(map[*Block]bool, len(f.Blocks))
	for _, b := range f.Blocks {
		blockSet[b] = true
	}

	// Build a set of all values for reference checks.
	valueSet := make(map[*Value]bool)

	for _, b := range f.Blocks {
		// 2. Every block has a valid Kind
		if b.Kind == BlockInvalid {
			add("func %s, %s: block has invalid kind", f.Name, b)
		}

		// 3. Block's Func pointer matches
		if b.Func != f {
			add("func %s, %s: block Func pointer mismatch", f.Name, b)
		}

		// Check values
		for _, v := range b.Values {
			valueSet[v] = true

			// 4. Every Value's Block pointer matches its containing block
			if v.Block != b {
				add("func %s, %s, %s: value Block pointer is %s, want %s",
					f.Name, b, v, v.Block, b)
			}

			// 5. Non-void values must have non-nil Type
			// Exception: StaticCall/Call may have nil Type for void-returning functions.
			if !v.Op.IsVoid() && v.Type == nil && v.Op != OpStaticCall && v.Op != OpCall {
				add("func %s, %s, %s (%s): non-void value has nil Type",
					f.Name, b, v, v.Op)
			}

			// 6. Args are non-nil
			for i, arg := range v.Args {
				if arg == nil {
					add("func %s, %s, %s: arg[%d] is nil", f.Name, b, v, i)
				}
			}

			// 7. Phi args count == Preds count
			if v.Op == OpPhi {
				if len(v.Args) != len(b.Preds) {
					add("func %s, %s, %s: phi has %d args but block has %d preds",
						f.Name, b, v, len(v.Args), len(b.Preds))
				}
			}
		}

		// 8. Terminator checks based on Kind
		switch b.Kind {
		case BlockPlain:
			if len(b.Succs) != 1 {
				add("func %s, %s: plain block has %d succs, want 1",
					f.Name, b, len(b.Succs))
			}
		case BlockIf:
			if len(b.Controls) != 1 {
				add("func %s, %s: if block has %d controls, want 1",
					f.Name, b, len(b.Controls))
			}
			if len(b.Succs) != 2 {
				add("func %s, %s: if block has %d succs, want 2",
					f.Name, b, len(b.Succs))
			}
		case BlockReturn:
			if len(b.Succs) != 0 {
				add("func %s, %s: return block has %d succs, want 0",
					f.Name, b, len(b.Succs))
			}
		case BlockExit:
			if len(b.Succs) != 0 {
				add("func %s, %s: exit block has %d succs, want 0",
					f.Name, b, len(b.Succs))
			}
		}

		// 9. Succs/Preds edge consistency
		for _, succ := range b.Succs {
			if !blockSet[succ] {
				add("func %s, %s: successor %s not in function", f.Name, b, succ)
				continue
			}
			if !containsBlock(succ.Preds, b) {
				add("func %s, %s: successor %s does not have %s as predecessor",
					f.Name, b, succ, b)
			}
		}
		for _, pred := range b.Preds {
			if !blockSet[pred] {
				add("func %s, %s: predecessor %s not in function", f.Name, b, pred)
				continue
			}
			if !containsBlock(pred.Succs, b) {
				add("func %s, %s: predecessor %s does not have %s as successor",
					f.Name, b, pred, b)
			}
		}

		// 10. Control values must reference existing values
		for i, c := range b.Controls {
			if c == nil {
				// nil control is allowed for BlockReturn (void return)
				if b.Kind != BlockReturn {
					add("func %s, %s: control[%d] is nil", f.Name, b, i)
				}
			}
		}
	}

	// 11. Verify all value args are in the function
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			for i, arg := range v.Args {
				if arg != nil && !valueSet[arg] {
					add("func %s, %s, %s: arg[%d] (%s) not found in function",
						f.Name, b, v, i, arg)
				}
			}
		}
		for i, c := range b.Controls {
			if c != nil && !valueSet[c] {
				add("func %s, %s: control[%d] (%s) not found in function",
					f.Name, b, i, c)
			}
		}
	}

	return combineErrors(errs)
}

// containsBlock checks whether bs contains b.
func containsBlock(bs []*Block, b *Block) bool {
	for _, x := range bs {
		if x == b {
			return true
		}
	}
	return false
}

// VerifyDom checks dominance properties of an SSA function.
// ComputeDom must have been called before this.
// It calls Verify first, then checks dominance invariants.
func VerifyDom(f *Func) error {
	if err := Verify(f); err != nil {
		return err
	}

	var errs []string
	add := func(format string, args ...interface{}) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	// Build reachability set.
	reachable := make(map[*Block]bool)
	var walk func(b *Block)
	walk = func(b *Block) {
		if reachable[b] {
			return
		}
		reachable[b] = true
		for _, s := range b.Succs {
			walk(s)
		}
	}
	walk(f.Entry)

	// 1. Entry Idom must be nil.
	if f.Entry.Idom != nil {
		add("func %s: entry %s has non-nil Idom %s", f.Name, f.Entry, f.Entry.Idom)
	}

	// 2. All reachable non-entry blocks must have non-nil Idom != self.
	for _, b := range f.Blocks {
		if !reachable[b] || b == f.Entry {
			continue
		}
		if b.Idom == nil {
			add("func %s, %s: reachable block has nil Idom", f.Name, b)
		} else if b.Idom == b {
			add("func %s, %s: block is its own Idom", f.Name, b)
		}
	}

	// Build value-to-index maps for same-block ordering checks.
	valIdx := make(map[*Value]int)
	for _, b := range f.Blocks {
		for i, v := range b.Values {
			valIdx[v] = i
		}
	}

	// dominates returns true if a dominates b (a is on the path from b to the entry
	// via Idom chain, or a == b).
	dominates := func(a, b *Block) bool {
		for b != nil {
			if b == a {
				return true
			}
			b = b.Idom
		}
		return false
	}

	// 3. For non-phi values: each arg's block must dominate the use block
	// (or if same block, arg must appear before use).
	for _, b := range f.Blocks {
		if !reachable[b] {
			continue
		}
		for _, v := range b.Values {
			if v.Op == OpPhi {
				continue
			}
			for i, arg := range v.Args {
				if arg == nil {
					continue
				}
				defBlock := arg.Block
				if defBlock == b {
					// Same block: arg must appear before use.
					if valIdx[arg] >= valIdx[v] {
						add("func %s, %s, %s: arg[%d] %s defined at index %d, used at index %d (same block)",
							f.Name, b, v, i, arg, valIdx[arg], valIdx[v])
					}
				} else if !dominates(defBlock, b) {
					add("func %s, %s, %s: arg[%d] %s defined in %s which does not dominate %s",
						f.Name, b, v, i, arg, defBlock, b)
				}
			}
		}
	}

	// 4. For phi values: each arg[i]'s block must dominate Preds[i].
	for _, b := range f.Blocks {
		if !reachable[b] {
			continue
		}
		for _, v := range b.Values {
			if v.Op != OpPhi {
				continue
			}
			for i, arg := range v.Args {
				if arg == nil || i >= len(b.Preds) {
					continue
				}
				pred := b.Preds[i]
				defBlock := arg.Block
				if !dominates(defBlock, pred) {
					add("func %s, %s, %s: phi arg[%d] %s defined in %s which does not dominate pred %s",
						f.Name, b, v, i, arg, defBlock, pred)
				}
			}
		}
	}

	// 5. Control values must dominate their block.
	for _, b := range f.Blocks {
		if !reachable[b] {
			continue
		}
		for i, c := range b.Controls {
			if c == nil {
				continue
			}
			defBlock := c.Block
			if defBlock != b && !dominates(defBlock, b) {
				add("func %s, %s: control[%d] %s defined in %s which does not dominate %s",
					f.Name, b, i, c, defBlock, b)
			}
		}
	}

	return combineErrors(errs)
}

// combineErrors creates an error from a list of error strings, or returns nil.
func combineErrors(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("SSA verification failed:\n  %s", strings.Join(errs, "\n  "))
}

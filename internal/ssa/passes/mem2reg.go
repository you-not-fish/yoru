package passes

import (
	"github.com/you-not-fish/yoru/internal/ssa"
	"github.com/you-not-fish/yoru/internal/types"
)

// Mem2Reg promotes stack allocas to SSA registers by inserting phi nodes
// and renaming variables. Only "simple" allocas (used only by load/store/zero)
// are promoted; allocas whose address escapes are left intact.
func Mem2Reg(f *ssa.Func) {
	// Ensure dominance tree is available.
	ssa.ComputeDom(f)

	allocas := findPromotable(f)
	if len(allocas) == 0 {
		return
	}

	df := ssa.ComputeDomFrontier(f)

	// For each alloca, find blocks that define (store/zero to) it.
	defBlocks := make(map[*ssa.Value][]*ssa.Block, len(allocas))
	for _, a := range allocas {
		defBlocks[a] = findDefBlocks(f, a)
	}

	// Insert phi nodes at iterated dominance frontier.
	phiMap := insertPhis(f, allocas, defBlocks, df)

	// Rename variables using domtree preorder walk.
	rename(f, allocas, phiMap)

	// Remove dead loads, stores, zeros, and allocas.
	cleanup(f, allocas)
}

// findPromotable returns all allocas that can be promoted to registers.
// An alloca is promotable if every use is an OpLoad (ptr), OpStore (dst),
// or OpZero (ptr) with the alloca as Args[0].
func findPromotable(f *ssa.Func) []*ssa.Value {
	// Collect all allocas.
	var allAllocas []*ssa.Value
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			if v.Op == ssa.OpAlloca {
				allAllocas = append(allAllocas, v)
			}
		}
	}

	// Build a set of allocas for quick lookup.
	allocaSet := make(map[*ssa.Value]bool, len(allAllocas))
	for _, a := range allAllocas {
		allocaSet[a] = true
	}

	// Check each use of each alloca.
	// First pass: mark non-promotable allocas.
	nonPromotable := make(map[*ssa.Value]bool)
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			for i, arg := range v.Args {
				if !allocaSet[arg] {
					continue
				}
				switch v.Op {
				case ssa.OpLoad:
					if i != 0 {
						nonPromotable[arg] = true
					}
				case ssa.OpStore:
					if i == 0 {
						// alloca as store destination — OK
					} else {
						// alloca used as the stored value (address escapes)
						nonPromotable[arg] = true
					}
				case ssa.OpZero:
					if i != 0 {
						nonPromotable[arg] = true
					}
				default:
					// Any other use (OpAddr, OpStructFieldPtr, etc.) is non-promotable.
					nonPromotable[arg] = true
				}
			}
		}
	}

	var promotable []*ssa.Value
	for _, a := range allAllocas {
		if !nonPromotable[a] {
			promotable = append(promotable, a)
		}
	}
	return promotable
}

// findDefBlocks returns the blocks containing stores/zeros to the given alloca.
func findDefBlocks(f *ssa.Func, alloca *ssa.Value) []*ssa.Block {
	seen := make(map[*ssa.Block]bool)
	var blocks []*ssa.Block
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			if (v.Op == ssa.OpStore || v.Op == ssa.OpZero) && v.Args[0] == alloca {
				if !seen[b] {
					seen[b] = true
					blocks = append(blocks, b)
				}
			}
		}
	}
	return blocks
}

// insertPhis places phi nodes at the iterated dominance frontier for each alloca.
// Returns phiMap[block][alloca] = phi value.
func insertPhis(
	f *ssa.Func,
	allocas []*ssa.Value,
	defBlocks map[*ssa.Value][]*ssa.Block,
	df map[*ssa.Block][]*ssa.Block,
) map[*ssa.Block]map[*ssa.Value]*ssa.Value {
	phiMap := make(map[*ssa.Block]map[*ssa.Value]*ssa.Value)

	for _, alloca := range allocas {
		elemType := alloca.Type.(*types.Pointer).Elem()

		// Compute iterated dominance frontier.
		idf := iteratedDF(defBlocks[alloca], df)

		for _, b := range idf {
			phi := f.NewValueAtFront(b, ssa.OpPhi, elemType)
			// Pre-allocate Args with nil entries (one per predecessor).
			phi.Args = make([]*ssa.Value, len(b.Preds))

			if phiMap[b] == nil {
				phiMap[b] = make(map[*ssa.Value]*ssa.Value)
			}
			phiMap[b][alloca] = phi
		}
	}

	return phiMap
}

// iteratedDF computes the iterated dominance frontier from a set of defining blocks.
func iteratedDF(defs []*ssa.Block, df map[*ssa.Block][]*ssa.Block) []*ssa.Block {
	var result []*ssa.Block
	inResult := make(map[*ssa.Block]bool)
	worklist := make([]*ssa.Block, len(defs))
	copy(worklist, defs)
	inWorklist := make(map[*ssa.Block]bool, len(defs))
	for _, b := range defs {
		inWorklist[b] = true
	}

	for len(worklist) > 0 {
		b := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]

		for _, d := range df[b] {
			if !inResult[d] {
				inResult[d] = true
				result = append(result, d)
				if !inWorklist[d] {
					inWorklist[d] = true
					worklist = append(worklist, d)
				}
			}
		}
	}
	return result
}

// rename walks the dominator tree in preorder, tracking reaching definitions
// for each alloca and wiring up phi arguments.
func rename(f *ssa.Func, allocas []*ssa.Value, phiMap map[*ssa.Block]map[*ssa.Value]*ssa.Value) {
	// Create zero constants for each alloca's element type (in entry block).
	zeroVals := make(map[*ssa.Value]*ssa.Value, len(allocas))
	for _, a := range allocas {
		elemType := a.Type.(*types.Pointer).Elem()
		zeroVals[a] = makeZero(f, elemType)
	}

	// Stacks of reaching definitions.
	stacks := make(map[*ssa.Value][]*ssa.Value, len(allocas))
	for _, a := range allocas {
		stacks[a] = []*ssa.Value{zeroVals[a]}
	}

	// Set of promotable allocas for fast lookup.
	allocaSet := make(map[*ssa.Value]bool, len(allocas))
	for _, a := range allocas {
		allocaSet[a] = true
	}

	// Track values to remove.
	dead := make(map[*ssa.Value]bool)

	var visit func(b *ssa.Block)
	visit = func(b *ssa.Block) {
		// Count definitions pushed in this block to pop later.
		pushCounts := make(map[*ssa.Value]int, len(allocas))

		// 1. Process phis in this block — they are new definitions.
		if pm, ok := phiMap[b]; ok {
			for alloca, phi := range pm {
				stacks[alloca] = append(stacks[alloca], phi)
				pushCounts[alloca]++
			}
		}

		// 2. Process values in order.
		for _, v := range b.Values {
			switch v.Op {
			case ssa.OpLoad:
				if allocaSet[v.Args[0]] {
					alloca := v.Args[0]
					stack := stacks[alloca]
					reachingDef := stack[len(stack)-1]
					f.ReplaceUses(v, reachingDef)
					dead[v] = true
				}
			case ssa.OpStore:
				if allocaSet[v.Args[0]] {
					alloca := v.Args[0]
					storedVal := v.Args[1]
					stacks[alloca] = append(stacks[alloca], storedVal)
					pushCounts[alloca]++
					dead[v] = true
				}
			case ssa.OpZero:
				if allocaSet[v.Args[0]] {
					alloca := v.Args[0]
					stacks[alloca] = append(stacks[alloca], zeroVals[alloca])
					pushCounts[alloca]++
					dead[v] = true
				}
			}
		}

		// 3. Fill successor phis.
		for _, s := range b.Succs {
			pm, ok := phiMap[s]
			if !ok {
				continue
			}
			// Find pred index of b in s.Preds.
			predIdx := -1
			for i, p := range s.Preds {
				if p == b {
					predIdx = i
					break
				}
			}
			if predIdx < 0 {
				continue
			}
			for alloca, phi := range pm {
				stack := stacks[alloca]
				val := stack[len(stack)-1]
				phi.Args[predIdx] = val
				val.Uses++
			}
		}

		// 4. Recurse into dominated blocks.
		for _, child := range b.Dominees {
			visit(child)
		}

		// 5. Pop definitions pushed in this block.
		for alloca, count := range pushCounts {
			stacks[alloca] = stacks[alloca][:len(stacks[alloca])-count]
		}
	}

	visit(f.Entry)

	// Mark dead values globally so cleanup can find them.
	// Store the dead set in a package-level scope is not needed;
	// we just remove them now.
	removeDead(f, dead, allocaSet)
}

// makeZero creates a zero constant for the given type in the entry block.
func makeZero(f *ssa.Func, t types.Type) *ssa.Value {
	switch typ := t.Underlying().(type) {
	case *types.Basic:
		switch typ.Kind() {
		case types.Int:
			v := f.NewValue(f.Entry, ssa.OpConst64, t)
			v.AuxInt = 0
			return v
		case types.Float:
			v := f.NewValue(f.Entry, ssa.OpConstFloat, t)
			v.AuxFloat = 0
			return v
		case types.Bool:
			v := f.NewValue(f.Entry, ssa.OpConstBool, t)
			v.AuxInt = 0
			return v
		case types.String:
			v := f.NewValue(f.Entry, ssa.OpConstString, t)
			v.Aux = ""
			return v
		}
	case *types.Pointer, *types.Ref:
		return f.NewValue(f.Entry, ssa.OpConstNil, t)
	}
	// For struct/array types, this shouldn't happen since they aren't promotable.
	// Fallback: create a zero int (should not be reached for valid programs).
	v := f.NewValue(f.Entry, ssa.OpConst64, t)
	v.AuxInt = 0
	return v
}

// removeDead removes dead loads/stores/zeros and unused allocas.
func removeDead(f *ssa.Func, dead map[*ssa.Value]bool, allocaSet map[*ssa.Value]bool) {
	for _, b := range f.Blocks {
		var live []*ssa.Value
		for _, v := range b.Values {
			if dead[v] {
				// Decrement use counts for this dead value's args.
				for _, arg := range v.Args {
					arg.Uses--
				}
				continue
			}
			live = append(live, v)
		}
		b.Values = live
	}

	// Remove promoted allocas with Uses==0.
	for _, b := range f.Blocks {
		var live []*ssa.Value
		for _, v := range b.Values {
			if allocaSet[v] && v.Uses == 0 {
				continue
			}
			live = append(live, v)
		}
		b.Values = live
	}
}

// cleanup removes trivial phis (all args the same or self-referential).
func cleanup(f *ssa.Func, allocas []*ssa.Value) {
	changed := true
	for changed {
		changed = false
		for _, b := range f.Blocks {
			for _, v := range b.Values {
				if v.Op != ssa.OpPhi {
					continue
				}
				if trivial := trivialPhi(v); trivial != nil {
					f.ReplaceUses(v, trivial)
					changed = true
				}
			}
		}
		// Remove dead phis.
		if changed {
			for _, b := range f.Blocks {
				var live []*ssa.Value
				for _, v := range b.Values {
					if v.Op == ssa.OpPhi && v.Uses == 0 {
						for _, arg := range v.Args {
							if arg != nil {
								arg.Uses--
							}
						}
						continue
					}
					live = append(live, v)
				}
				b.Values = live
			}
		}
	}
}

// trivialPhi returns the single non-self value if the phi is trivial
// (all args are the same value or self-references), or nil if non-trivial.
func trivialPhi(phi *ssa.Value) *ssa.Value {
	var unique *ssa.Value
	for _, arg := range phi.Args {
		if arg == nil || arg == phi {
			continue
		}
		if unique == nil {
			unique = arg
		} else if arg != unique {
			return nil // multiple distinct args
		}
	}
	return unique // may be nil if all args are self or nil
}

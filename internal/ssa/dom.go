package ssa

// ReversePostOrder returns the blocks of f in reverse post-order,
// starting from f.Entry. Unreachable blocks are excluded.
func ReversePostOrder(f *Func) []*Block {
	visited := make(map[*Block]bool, len(f.Blocks))
	var order []*Block

	var dfs func(b *Block)
	dfs = func(b *Block) {
		if visited[b] {
			return
		}
		visited[b] = true
		for _, s := range b.Succs {
			dfs(s)
		}
		order = append(order, b)
	}
	dfs(f.Entry)

	// Reverse the post-order to get RPO.
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// ComputeDom computes the immediate dominator tree for f using
// Cooper, Harvey, and Kennedy's "A Simple, Fast Dominance Algorithm".
// It populates Block.Idom and Block.Dominees for all reachable blocks.
func ComputeDom(f *Func) {
	rpo := ReversePostOrder(f)
	if len(rpo) == 0 {
		return
	}

	// Assign RPO numbers.
	rpoNum := make(map[*Block]int, len(rpo))
	for i, b := range rpo {
		rpoNum[b] = i
	}

	// intersect finds the closest common dominator.
	intersect := func(b1, b2 *Block) *Block {
		for b1 != b2 {
			for rpoNum[b1] > rpoNum[b2] {
				b1 = b1.Idom
			}
			for rpoNum[b2] > rpoNum[b1] {
				b2 = b2.Idom
			}
		}
		return b1
	}

	// Initialize: entry dominates itself (sentinel).
	entry := rpo[0]
	entry.Idom = entry

	// Clear old domtree data.
	for _, b := range f.Blocks {
		if b != entry {
			b.Idom = nil
		}
		b.Dominees = nil
	}

	// Iterate until convergence.
	changed := true
	for changed {
		changed = false
		for _, b := range rpo[1:] { // skip entry
			// Find first predecessor with Idom already computed.
			var newIdom *Block
			for _, p := range b.Preds {
				if p.Idom != nil {
					newIdom = p
					break
				}
			}
			if newIdom == nil {
				continue
			}

			// Intersect with remaining processed predecessors.
			for _, p := range b.Preds {
				if p == newIdom {
					continue
				}
				if p.Idom != nil {
					newIdom = intersect(p, newIdom)
				}
			}

			if b.Idom != newIdom {
				b.Idom = newIdom
				changed = true
			}
		}
	}

	// Fix entry: Idom = nil (was sentinel).
	entry.Idom = nil

	// Build Dominees lists from Idom relationships.
	for _, b := range rpo {
		if b.Idom != nil {
			b.Idom.Dominees = append(b.Idom.Dominees, b)
		}
	}
}

// ComputeDomFrontier computes the dominance frontier for each block in f.
// ComputeDom must have been called first.
func ComputeDomFrontier(f *Func) map[*Block][]*Block {
	df := make(map[*Block][]*Block)

	for _, b := range f.Blocks {
		if len(b.Preds) < 2 {
			continue
		}
		for _, p := range b.Preds {
			runner := p
			for runner != nil && runner != b.Idom {
				df[runner] = appendUnique(df[runner], b)
				runner = runner.Idom
			}
		}
	}

	return df
}

// appendUnique appends b to list if not already present.
func appendUnique(list []*Block, b *Block) []*Block {
	for _, x := range list {
		if x == b {
			return list
		}
	}
	return append(list, b)
}

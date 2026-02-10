package ssa

import "fmt"

// BlockKind describes how a basic block terminates.
type BlockKind int

const (
	BlockInvalid BlockKind = iota
	BlockPlain                    // unconditional jump to Succs[0]
	BlockIf                       // conditional branch: if Controls[0] then Succs[0] else Succs[1]
	BlockReturn                   // function return; Controls[0] = return value (may be nil)
	BlockExit                     // program exit (e.g., panic); no successors
)

// blockKindNames maps BlockKind to its string representation.
var blockKindNames = [...]string{
	BlockInvalid: "invalid",
	BlockPlain:   "plain",
	BlockIf:      "if",
	BlockReturn:  "ret",
	BlockExit:    "exit",
}

// String returns the string representation of the block kind.
func (k BlockKind) String() string {
	if int(k) < len(blockKindNames) {
		return blockKindNames[k]
	}
	return "unknown"
}

// Block represents a basic block in the control flow graph.
// A block contains a sequence of non-branching Values, followed by
// a terminator indicated by its Kind.
type Block struct {
	// ID is a unique identifier within the containing Func.
	ID ID

	// Kind describes how this block terminates.
	Kind BlockKind

	// Controls holds the terminator's operand values.
	// For BlockIf: Controls[0] = branch condition.
	// For BlockReturn: Controls[0] = return value (nil for void return).
	// For BlockPlain/BlockExit: empty.
	Controls []*Value

	// Succs lists the successor blocks in the CFG.
	// For BlockPlain: Succs[0] = target.
	// For BlockIf: Succs[0] = then, Succs[1] = else.
	// For BlockReturn/BlockExit: empty.
	Succs []*Block

	// Preds lists the predecessor blocks in the CFG.
	Preds []*Block

	// Values is the ordered list of values computed in this block.
	Values []*Value

	// Func is the function containing this block.
	Func *Func

	// Dominance tree fields (populated in Phase 4C).
	Idom     *Block   // immediate dominator
	Dominees []*Block // blocks dominated by this block
}

// String returns a short string representation (e.g., "b3").
func (b *Block) String() string {
	return fmt.Sprintf("b%d", b.ID)
}

// AddSucc adds a successor block, updating both Succs and the successor's Preds.
func (b *Block) AddSucc(succ *Block) {
	b.Succs = append(b.Succs, succ)
	succ.Preds = append(succ.Preds, b)
}

// SetControl sets the branch/return control value.
func (b *Block) SetControl(v *Value) {
	b.Controls = []*Value{v}
	if v != nil {
		v.Uses++
	}
}

// AddControl appends a control value.
func (b *Block) AddControl(v *Value) {
	b.Controls = append(b.Controls, v)
	if v != nil {
		v.Uses++
	}
}

// NumSuccs returns the number of successor blocks.
func (b *Block) NumSuccs() int { return len(b.Succs) }

// NumPreds returns the number of predecessor blocks.
func (b *Block) NumPreds() int { return len(b.Preds) }

// NumValues returns the number of values in this block.
func (b *Block) NumValues() int { return len(b.Values) }

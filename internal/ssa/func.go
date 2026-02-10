package ssa

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// Func represents an SSA function.
// It contains a control flow graph of Blocks, each containing Values.
type Func struct {
	// Name is the function name.
	Name string

	// Sig is the function signature from the type checker.
	Sig *types.Func

	// Blocks is the list of basic blocks. Blocks[0] is always the entry block.
	Blocks []*Block

	// Entry is the entry block (same as Blocks[0]).
	Entry *Block

	// nextValueID is the next available value ID.
	nextValueID ID

	// nextBlockID is the next available block ID.
	nextBlockID ID
}

// NewFunc creates a new SSA function with the given name and signature.
// An entry block is automatically created.
func NewFunc(name string, sig *types.Func) *Func {
	f := &Func{
		Name: name,
		Sig:  sig,
	}
	// Create entry block.
	entry := f.NewBlock(BlockPlain)
	f.Entry = entry
	return f
}

// NewBlock creates a new basic block with the given kind and appends it to the function.
func (f *Func) NewBlock(kind BlockKind) *Block {
	b := &Block{
		ID:   f.nextBlockID,
		Kind: kind,
		Func: f,
	}
	f.nextBlockID++
	f.Blocks = append(f.Blocks, b)
	return b
}

// NewValue creates a new Value in the given block.
func (f *Func) NewValue(b *Block, op Op, typ types.Type, args ...*Value) *Value {
	v := &Value{
		ID:    f.nextValueID,
		Op:    op,
		Type:  typ,
		Block: b,
	}
	f.nextValueID++
	for _, arg := range args {
		v.AddArg(arg)
	}
	b.Values = append(b.Values, v)
	return v
}

// NewValuePos creates a new Value with source position in the given block.
func (f *Func) NewValuePos(b *Block, op Op, typ types.Type, pos syntax.Pos, args ...*Value) *Value {
	v := f.NewValue(b, op, typ, args...)
	v.Pos = pos
	return v
}

// NumBlocks returns the number of blocks in the function.
func (f *Func) NumBlocks() int { return len(f.Blocks) }

// NumValues returns the total number of values across all blocks.
func (f *Func) NumValues() int {
	n := 0
	for _, b := range f.Blocks {
		n += len(b.Values)
	}
	return n
}

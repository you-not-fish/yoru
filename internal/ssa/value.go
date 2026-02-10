package ssa

import (
	"fmt"

	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// ID is a unique identifier for Values and Blocks within a Func.
type ID int32

// Value represents a single SSA computation.
// Each Value has exactly one definition and may be used by other Values.
type Value struct {
	// ID is a unique identifier within the containing Func.
	ID ID

	// Op is the operation this value computes.
	Op Op

	// Type is the result type of this value.
	// Nil for void operations (Store, Println, Panic).
	Type types.Type

	// Args are the input values to this operation.
	Args []*Value

	// Block is the basic block that contains this value.
	Block *Block

	// AuxInt holds an auxiliary integer (e.g., constant value, field index).
	AuxInt int64

	// AuxFloat holds an auxiliary float (for OpConstFloat).
	AuxFloat float64

	// Aux holds arbitrary auxiliary data (e.g., string constant, *types.FuncObj).
	Aux interface{}

	// Uses tracks the number of references to this value.
	// Used by DCE to identify dead values.
	Uses int32

	// Pos is the source position associated with this value.
	Pos syntax.Pos
}

// String returns a short string representation of the value (e.g., "v5").
func (v *Value) String() string {
	return fmt.Sprintf("v%d", v.ID)
}

// LongString returns a detailed string representation including op, type, and args.
func (v *Value) LongString() string {
	s := fmt.Sprintf("v%d = %s", v.ID, v.Op)
	if v.Type != nil {
		s += fmt.Sprintf(" <%s>", v.Type)
	}
	if v.AuxInt != 0 || v.Op == OpConst64 || v.Op == OpConstBool {
		s += fmt.Sprintf(" [%d]", v.AuxInt)
	}
	if v.AuxFloat != 0 || v.Op == OpConstFloat {
		s += fmt.Sprintf(" [%g]", v.AuxFloat)
	}
	if v.Aux != nil {
		s += fmt.Sprintf(" {%v}", v.Aux)
	}
	for _, arg := range v.Args {
		s += " " + arg.String()
	}
	return s
}

// AddArg appends a value to the argument list and increments the arg's use count.
func (v *Value) AddArg(arg *Value) {
	v.Args = append(v.Args, arg)
	arg.Uses++
}

// SetArgs replaces the argument list, adjusting use counts.
func (v *Value) SetArgs(args []*Value) {
	// Decrement old uses
	for _, old := range v.Args {
		old.Uses--
	}
	v.Args = args
	// Increment new uses
	for _, arg := range args {
		arg.Uses++
	}
}

// ReplaceArg replaces the argument at index i, adjusting use counts.
func (v *Value) ReplaceArg(i int, new *Value) {
	old := v.Args[i]
	old.Uses--
	v.Args[i] = new
	new.Uses++
}

// IsPure returns true if this value's op has no side effects.
func (v *Value) IsPure() bool {
	return v.Op.IsPure()
}

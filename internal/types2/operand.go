package types2

import (
	"go/constant"

	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// operandMode describes the mode of an operand.
type operandMode int

const (
	invalid   operandMode = iota // operand is invalid
	novalue                      // operand has no value (void function call)
	builtin                      // operand is a built-in function
	typexpr                      // operand is a type expression
	constant_                    // operand is a constant value
	variable                     // operand is an addressable variable
	value                        // operand is a computed value (not addressable)
)

// operand represents the result of evaluating an expression.
type operand struct {
	mode operandMode
	pos  syntax.Pos
	typ  types.Type
	val  constant.Value // constant value (only valid when mode == constant_)
	expr syntax.Expr    // source expression (for error reporting)
}

// String returns a string representation of the operand for debugging.
func (x *operand) String() string {
	if x.mode == invalid {
		return "invalid operand"
	}
	if x.typ == nil {
		return "operand without type"
	}
	return x.typ.String()
}

// isNil reports whether the operand is the nil value.
func (x *operand) isNil() bool {
	return types.IsNil(x.typ)
}

// setConst sets the operand to a constant value.
func (x *operand) setConst(pos syntax.Pos, typ types.Type, val constant.Value) {
	x.mode = constant_
	x.pos = pos
	x.typ = typ
	x.val = val
}

// setVar sets the operand to a variable.
func (x *operand) setVar(pos syntax.Pos, typ types.Type) {
	x.mode = variable
	x.pos = pos
	x.typ = typ
	x.val = nil
}

// setValue sets the operand to a computed value.
func (x *operand) setValue(pos syntax.Pos, typ types.Type) {
	x.mode = value
	x.pos = pos
	x.typ = typ
	x.val = nil
}

// setInvalid sets the operand to invalid.
func (x *operand) setInvalid(pos syntax.Pos) {
	x.mode = invalid
	x.pos = pos
	x.typ = nil
	x.val = nil
}

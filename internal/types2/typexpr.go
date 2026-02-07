package types2

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// typExpr evaluates a type expression and sets x to the resulting type.
func (c *Checker) typExpr(x *operand, e syntax.Expr) {
	x.mode = typexpr
	x.pos = e.Pos()

	switch e := e.(type) {
	case *syntax.Name:
		c.typeName(x, e)
	case *syntax.ArrayType:
		c.arrayType(x, e)
	case *syntax.PointerType:
		c.pointerType(x, e)
	case *syntax.RefType:
		c.refType(x, e)
	case *syntax.StructType:
		c.structType(x, e)
	default:
		c.errorf(e.Pos(), "%T is not a type", e)
		x.mode = invalid
	}
}

// typeName resolves a type name.
func (c *Checker) typeName(x *operand, name *syntax.Name) {
	obj := c.resolve(name)
	if obj == nil {
		x.mode = invalid
		return
	}

	switch obj := obj.(type) {
	case *types.TypeName:
		if typ := obj.Type(); typ != nil {
			x.typ = typ
			return
		}
		c.errorf(name.Pos(), "invalid type %s", name.Value)
		x.mode = invalid
		return
	case *types.Builtin:
		// new is used as new(T), not as a type
		c.errorf(name.Pos(), "%s is not a type", name.Value)
		x.mode = invalid
	default:
		c.errorf(name.Pos(), "%s is not a type", name.Value)
		x.mode = invalid
	}
}

// arrayType resolves an array type [N]Elem.
func (c *Checker) arrayType(x *operand, e *syntax.ArrayType) {
	// Evaluate length expression (must be a constant integer)
	var length int64 = -1
	if e.Len != nil {
		var lenOp operand
		c.expr(&lenOp, e.Len)
		if lenOp.mode == constant_ {
			if n, ok := c.constInt64(&lenOp); ok {
				if n < 0 {
					c.errorf(e.Len.Pos(), "array length must be non-negative")
				} else {
					length = n
				}
			}
		} else {
			c.errorf(e.Len.Pos(), "array length must be a constant expression")
		}
	} else {
		c.errorf(e.Pos(), "missing array length")
	}

	// Resolve element type
	elem := c.resolveType(e.Elem)
	if elem == nil {
		x.mode = invalid
		return
	}

	if length < 0 {
		// Use 0 as fallback for error recovery
		length = 0
	}

	x.typ = types.NewArray(length, elem)
}

// pointerType resolves a pointer type *T.
func (c *Checker) pointerType(x *operand, e *syntax.PointerType) {
	base := c.resolveType(e.Base)
	if base == nil {
		x.mode = invalid
		return
	}
	x.typ = types.NewPointer(base)
}

// refType resolves a reference type ref T.
func (c *Checker) refType(x *operand, e *syntax.RefType) {
	base := c.resolveType(e.Base)
	if base == nil {
		x.mode = invalid
		return
	}
	x.typ = types.NewRef(base)
}

// structType resolves a struct type.
func (c *Checker) structType(x *operand, e *syntax.StructType) {
	fields := make([]*types.Var, len(e.Fields))
	seen := make(map[string]bool)

	for i, field := range e.Fields {
		// Resolve field type
		fieldType := c.resolveType(field.Type)
		if fieldType == nil {
			x.mode = invalid
			return
		}

		// Check for duplicate field names
		name := ""
		if field.Name != nil {
			name = field.Name.Value
			if seen[name] {
				c.errorf(field.Name.Pos(), "duplicate field %s", name)
			}
			seen[name] = true
		}

		fields[i] = types.NewField(field.Pos(), name, fieldType)
	}

	st := types.NewStruct(fields)
	// Compute layout
	c.conf.Sizes.ComputeLayout(st)
	x.typ = st
}

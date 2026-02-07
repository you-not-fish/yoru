package types2

import (
	"go/constant"
	"strconv"
	"strings"

	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// expr evaluates an expression and sets x to the result.
func (c *Checker) expr(x *operand, e syntax.Expr) {
	c.exprInternal(x, e)

	// Record type information
	if x.mode != invalid {
		c.recordType(e, x)
	}
}

// exprInternal is the main expression checking function.
func (c *Checker) exprInternal(x *operand, e syntax.Expr) {
	x.mode = invalid
	x.pos = e.Pos()
	x.expr = e

	switch e := e.(type) {
	case *syntax.Name:
		c.ident(x, e)
	case *syntax.BasicLit:
		c.basicLit(x, e)
	case *syntax.Operation:
		if e.Y == nil {
			c.unary(x, e)
		} else {
			c.binary(x, e)
		}
	case *syntax.CallExpr:
		c.call(x, e)
	case *syntax.IndexExpr:
		c.index(x, e)
	case *syntax.SelectorExpr:
		c.selector(x, e)
	case *syntax.NewExpr:
		c.newExpr(x, e)
	case *syntax.CompositeLit:
		c.compositeLit(x, e)
	case *syntax.ParenExpr:
		c.exprInternal(x, e.X)
	case *syntax.ArrayType, *syntax.PointerType, *syntax.RefType, *syntax.StructType:
		c.typExpr(x, e)
	default:
		c.errorf(e.Pos(), "unexpected expression %T", e)
	}
}

// ident evaluates an identifier.
func (c *Checker) ident(x *operand, name *syntax.Name) {
	obj := c.resolve(name)
	if obj == nil {
		x.mode = invalid
		return
	}

	switch obj := obj.(type) {
	case *types.Var:
		x.mode = variable
		x.typ = obj.Type()
		// Handle true/false as constants
		if obj.Name() == "true" || obj.Name() == "false" {
			x.mode = constant_
			x.val = constant.MakeBool(obj.Name() == "true")
		}
	case *types.TypeName:
		x.mode = typexpr
		x.typ = obj.Type()
	case *types.FuncObj:
		x.mode = value
		x.typ = obj.Signature()
	case *types.Builtin:
		x.mode = builtin
		x.typ = nil // builtins don't have a regular type
	case *types.Nil:
		x.mode = constant_
		x.typ = types.Typ[types.UntypedNil]
		x.val = nil
	default:
		c.errorf(name.Pos(), "unexpected object %T", obj)
		x.mode = invalid
	}
}

// basicLit evaluates a basic literal (int, float, string).
func (c *Checker) basicLit(x *operand, lit *syntax.BasicLit) {
	x.mode = constant_
	x.pos = lit.Pos()

	switch lit.Kind {
	case syntax.IntLit:
		// Parse integer literal
		val, err := strconv.ParseInt(lit.Value, 0, 64)
		if err != nil {
			c.errorf(lit.Pos(), "invalid integer literal: %s", lit.Value)
			x.mode = invalid
			return
		}
		x.typ = types.Typ[types.UntypedInt]
		x.val = constant.MakeInt64(val)

	case syntax.FloatLit:
		// Parse float literal
		val, err := strconv.ParseFloat(lit.Value, 64)
		if err != nil {
			c.errorf(lit.Pos(), "invalid float literal: %s", lit.Value)
			x.mode = invalid
			return
		}
		x.typ = types.Typ[types.UntypedFloat]
		x.val = constant.MakeFloat64(val)

	case syntax.StringLit:
		// String literal value is already decoded by scanner
		x.typ = types.Typ[types.UntypedString]
		x.val = constant.MakeString(lit.Value)

	default:
		c.errorf(lit.Pos(), "unknown literal kind")
		x.mode = invalid
	}
}

// unary evaluates a unary operation.
func (c *Checker) unary(x *operand, e *syntax.Operation) {
	c.expr(x, e.X)
	if x.mode == invalid {
		return
	}

	switch e.Op {
	case syntax.Not: // !
		if !isBoolean(x.typ) {
			c.errorf(e.Pos(), "operator ! requires boolean operand")
			x.mode = invalid
			return
		}
		if x.mode == constant_ {
			x.val = constant.UnaryOp(0, x.val, 0) // token.NOT
		}
		x.typ = types.Typ[types.UntypedBool]
		if !types.IsUntypedType(x.typ) {
			x.typ = types.Typ[types.Bool]
		}

	case syntax.Sub: // -
		if !isNumeric(x.typ) {
			c.errorf(e.Pos(), "operator - requires numeric operand")
			x.mode = invalid
			return
		}
		if x.mode == constant_ {
			x.val = constant.UnaryOp(12, x.val, 0) // token.SUB
		}

	case syntax.And: // &
		if x.mode != variable {
			c.errorf(e.Pos(), "cannot take address of %s", e.X)
			x.mode = invalid
			return
		}
		x.mode = value
		x.typ = types.NewPointer(x.typ)

	case syntax.Mul: // *
		// Dereference
		switch t := x.typ.Underlying().(type) {
		case *types.Pointer:
			x.mode = variable
			x.typ = t.Elem()
		case *types.Ref:
			x.mode = variable
			x.typ = t.Elem()
		default:
			c.errorf(e.Pos(), "cannot dereference non-pointer type %s", x.typ)
			x.mode = invalid
		}

	default:
		c.errorf(e.Pos(), "unknown unary operator")
		x.mode = invalid
	}
}

// binary evaluates a binary operation.
func (c *Checker) binary(x *operand, e *syntax.Operation) {
	var y operand
	c.expr(x, e.X)
	c.expr(&y, e.Y)

	if x.mode == invalid || y.mode == invalid {
		x.mode = invalid
		return
	}

	op := e.Op
	prec := op.Precedence()

	// Comparison operators
	if prec == 3 {
		c.comparison(x, &y, op)
		return
	}

	// Logical operators
	if prec == 1 || prec == 2 {
		c.logical(x, &y, op)
		return
	}

	// Arithmetic operators
	c.arithmetic(x, &y, op)
}

// comparison handles comparison operators (==, !=, <, <=, >, >=).
func (c *Checker) comparison(x, y *operand, op syntax.Token) {
	// Check that operands are comparable
	if !c.comparable(x, y) {
		c.errorf(x.pos, "cannot compare %s and %s", x.typ, y.typ)
		x.mode = invalid
		return
	}

	// For ordering operators, check that types are ordered
	if op.Precedence() == 3 && (op != syntax.Token(28) && op != syntax.Token(29)) { // not == or !=
		if !types.Ordered(x.typ) {
			c.errorf(x.pos, "operator %s not defined for %s", op, x.typ)
			x.mode = invalid
			return
		}
	}

	// Result is always bool
	x.mode = value
	x.typ = types.Typ[types.Bool]

	// Constant folding
	if x.mode == constant_ && y.mode == constant_ {
		x.val = c.evalComparison(x.val, y.val, op)
		x.mode = constant_
		x.typ = types.Typ[types.UntypedBool]
	}
}

// logical handles logical operators (&&, ||).
func (c *Checker) logical(x, y *operand, op syntax.Token) {
	if !isBoolean(x.typ) || !isBoolean(y.typ) {
		c.errorf(x.pos, "operator %s requires boolean operands", op)
		x.mode = invalid
		return
	}

	x.mode = value
	if types.IsUntypedType(x.typ) && types.IsUntypedType(y.typ) {
		x.typ = types.Typ[types.UntypedBool]
	} else {
		x.typ = types.Typ[types.Bool]
	}

	// Constant folding
	if x.mode == constant_ && y.mode == constant_ {
		x.val = c.evalLogical(x.val, y.val, op)
		x.mode = constant_
	}
}

// arithmetic handles arithmetic operators (+, -, *, /, %, etc.).
func (c *Checker) arithmetic(x, y *operand, op syntax.Token) {
	// String concatenation
	if isStringType(x.typ) && isStringType(y.typ) {
		if op == syntax.Token(36) { // _Add
			x.mode = value
			if types.IsUntypedType(x.typ) && types.IsUntypedType(y.typ) {
				x.typ = types.Typ[types.UntypedString]
			} else {
				x.typ = types.Typ[types.String]
			}
			if x.mode == constant_ && y.mode == constant_ {
				x.val = constant.BinaryOp(x.val, 12, y.val) // token.ADD
				x.mode = constant_
			}
			return
		}
		c.errorf(x.pos, "operator %s not defined for strings", op)
		x.mode = invalid
		return
	}

	// Numeric arithmetic
	if !isNumeric(x.typ) || !isNumeric(y.typ) {
		c.errorf(x.pos, "operator %s requires numeric operands", op)
		x.mode = invalid
		return
	}

	// Check for % on floats
	if op == syntax.Token(44) { // _Rem
		if isFloat(x.typ) || isFloat(y.typ) {
			c.errorf(x.pos, "operator %% not defined for float")
			x.mode = invalid
			return
		}
	}

	// Determine result type
	x.mode = value
	if types.IsUntypedType(x.typ) && types.IsUntypedType(y.typ) {
		// Both untyped: result is untyped
		if isFloat(x.typ) || isFloat(y.typ) {
			x.typ = types.Typ[types.UntypedFloat]
		} else {
			x.typ = types.Typ[types.UntypedInt]
		}
	} else if types.IsUntypedType(x.typ) {
		x.typ = y.typ
	} else if types.IsUntypedType(y.typ) {
		// x.typ stays the same
	} else {
		// Both typed: must be identical
		if !types.Identical(x.typ, y.typ) {
			c.errorf(x.pos, "mismatched types %s and %s", x.typ, y.typ)
			x.mode = invalid
			return
		}
	}

	// Constant folding
	if x.mode == constant_ && y.mode == constant_ {
		x.val = c.evalArithmetic(x.val, y.val, op)
		x.mode = constant_
	}
}

// comparable reports whether x and y can be compared.
func (c *Checker) comparable(x, y *operand) bool {
	// nil can be compared to any pointer or ref type
	if x.isNil() && types.IsPointerOrRef(y.typ) {
		return true
	}
	if y.isNil() && types.IsPointerOrRef(x.typ) {
		return true
	}

	// Types must be compatible
	if types.AssignableTo(x.typ, y.typ) || types.AssignableTo(y.typ, x.typ) {
		return types.Comparable(x.typ) || types.Comparable(y.typ)
	}

	return false
}

// Helper functions for type checking
func isBoolean(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsBoolean != 0
}

func isNumeric(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsNumeric != 0
}

func isInteger(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsInteger != 0
}

func isFloat(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsFloat != 0
}

func isStringType(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

// index evaluates an index expression x[i].
func (c *Checker) index(x *operand, e *syntax.IndexExpr) {
	c.expr(x, e.X)
	if x.mode == invalid {
		return
	}

	// Check that x is an array or pointer to array
	var elemType types.Type
	switch t := x.typ.Underlying().(type) {
	case *types.Array:
		elemType = t.Elem()
		x.mode = variable // array elements are addressable
	case *types.Pointer:
		if arr, ok := t.Elem().Underlying().(*types.Array); ok {
			elemType = arr.Elem()
			x.mode = variable
		} else {
			c.errorf(e.Pos(), "cannot index into %s", x.typ)
			x.mode = invalid
			return
		}
	case *types.Ref:
		if arr, ok := t.Elem().Underlying().(*types.Array); ok {
			elemType = arr.Elem()
			x.mode = variable
		} else {
			c.errorf(e.Pos(), "cannot index into %s", x.typ)
			x.mode = invalid
			return
		}
	default:
		c.errorf(e.Pos(), "cannot index into %s", x.typ)
		x.mode = invalid
		return
	}

	// Check index
	var idx operand
	c.expr(&idx, e.Index)
	if idx.mode == invalid {
		x.mode = invalid
		return
	}

	if !isInteger(idx.typ) {
		c.errorf(e.Index.Pos(), "index must be an integer")
		x.mode = invalid
		return
	}

	x.typ = elemType
}

// selector evaluates a selector expression x.sel.
func (c *Checker) selector(x *operand, e *syntax.SelectorExpr) {
	c.expr(x, e.X)
	if x.mode == invalid {
		return
	}

	sel := e.Sel.Value

	// Try field access
	if field := c.lookupField(x.typ, sel); field != nil {
		c.recordUse(e.Sel, field)
		x.mode = variable
		x.typ = field.Type()
		return
	}

	// Check for method (for better error message)
	if method := c.lookupMethodObj(x.typ, sel); method != nil {
		// Method selectors cannot be used as values in Yoru
		c.errorf(e.Pos(), "cannot use method %s.%s as value (method expressions not supported)", x.typ, sel)
		x.mode = invalid
		return
	}

	c.errorf(e.Sel.Pos(), "%s has no field or method %s", x.typ, sel)
	x.mode = invalid
}

// lookupField looks up a field by name in type T.
func (c *Checker) lookupField(T types.Type, name string) *types.Var {
	if T == nil {
		return nil
	}

	// Auto-dereference pointers and refs
	t := T.Underlying()
	if t == nil {
		return nil
	}

	switch t := t.(type) {
	case *types.Pointer:
		return c.lookupField(t.Elem(), name)
	case *types.Ref:
		return c.lookupField(t.Elem(), name)
	case *types.Struct:
		for _, f := range t.Fields() {
			if f.Name() == name {
				return f
			}
		}
	case *types.Named:
		return c.lookupField(t.Underlying(), name)
	}
	return nil
}

// lookupMethodObj looks up a method by name in type T.
func (c *Checker) lookupMethodObj(T types.Type, name string) *types.FuncObj {
	if T == nil {
		return nil
	}

	// Get the base named type
	t := T.Underlying()
	if t == nil {
		return nil
	}

	switch t := t.(type) {
	case *types.Pointer:
		return c.lookupMethodObj(t.Elem(), name)
	case *types.Ref:
		return c.lookupMethodObj(t.Elem(), name)
	}

	if named, ok := T.(*types.Named); ok {
		return named.LookupMethod(name)
	}
	return nil
}

// newExpr evaluates a new(T) expression.
func (c *Checker) newExpr(x *operand, e *syntax.NewExpr) {
	// Resolve the type argument
	T := c.resolveType(e.Type)
	if T == nil {
		x.mode = invalid
		return
	}

	// new(T) returns ref T
	x.mode = value
	x.typ = types.NewRef(T)
}

// compositeLit evaluates a composite literal Type{...}.
func (c *Checker) compositeLit(x *operand, e *syntax.CompositeLit) {
	// Resolve the type
	T := c.resolveType(e.Type)
	if T == nil {
		x.mode = invalid
		return
	}

	st, ok := T.Underlying().(*types.Struct)
	if !ok {
		c.errorf(e.Pos(), "invalid composite literal type %s", T)
		x.mode = invalid
		return
	}

	// Check elements
	if len(e.Elems) > len(st.Fields()) {
		c.errorf(e.Pos(), "too many values in struct literal")
	}

	hasKeys := false
	if len(e.Elems) > 0 {
		_, hasKeys = e.Elems[0].(*syntax.KeyValueExpr)
	}

	if hasKeys {
		// Keyed elements
		seen := make(map[string]bool)
		for _, elem := range e.Elems {
			kv, ok := elem.(*syntax.KeyValueExpr)
			if !ok {
				c.errorf(elem.Pos(), "mixture of field:value and value elements in struct literal")
				continue
			}

			key, ok := kv.Key.(*syntax.Name)
			if !ok {
				c.errorf(kv.Key.Pos(), "invalid field name")
				continue
			}

			if seen[key.Value] {
				c.errorf(key.Pos(), "duplicate field name %s", key.Value)
				continue
			}
			seen[key.Value] = true

			// Find field
			var field *types.Var
			for _, f := range st.Fields() {
				if f.Name() == key.Value {
					field = f
					break
				}
			}
			if field == nil {
				c.errorf(key.Pos(), "unknown field %s", key.Value)
				continue
			}

			// Check value
			var val operand
			c.expr(&val, kv.Value)
			if val.mode != invalid {
				c.assignment(&val, field.Type(), "struct literal")
			}
		}
	} else {
		// Unkeyed elements
		for i, elem := range e.Elems {
			if i >= len(st.Fields()) {
				break
			}
			field := st.Field(i)
			var val operand
			c.expr(&val, elem)
			if val.mode != invalid {
				c.assignment(&val, field.Type(), "struct literal")
			}
		}
	}

	x.mode = value
	x.typ = T
}

// constInt64 returns the int64 value of a constant operand.
func (c *Checker) constInt64(x *operand) (int64, bool) {
	if x.mode != constant_ {
		return 0, false
	}
	if x.val == nil {
		return 0, false
	}
	if x.val.Kind() != constant.Int {
		return 0, false
	}
	n, exact := constant.Int64Val(x.val)
	if !exact {
		c.errorf(x.pos, "constant %s overflows int64", x.val)
		return 0, false
	}
	return n, true
}

// assignment checks whether x can be assigned to type T.
func (c *Checker) assignment(x *operand, T types.Type, context string) {
	if x.mode == invalid {
		return
	}

	// Check ref T -> *T conversion (forbidden)
	if types.IsPointer(T) && types.IsRef(x.typ) {
		c.errorf(x.pos, "cannot convert %s to %s (would cause use-after-free)", x.typ, T)
		x.mode = invalid
		return
	}

	if types.AssignableTo(x.typ, T) {
		// Convert untyped to typed
		if types.IsUntypedType(x.typ) {
			x.typ = T
		}
		return
	}

	c.errorf(x.pos, "cannot use %s as %s in %s", x.typ, T, context)
	x.mode = invalid
}

// Constant evaluation helpers
func (c *Checker) evalComparison(x, y constant.Value, op syntax.Token) constant.Value {
	// Map syntax tokens to constant comparison tokens
	var cmp int
	switch op.String() {
	case "==":
		return constant.MakeBool(constant.Compare(x, 0, y)) // token.EQL
	case "!=":
		return constant.MakeBool(!constant.Compare(x, 0, y))
	case "<":
		cmp = -1
		return constant.MakeBool(constant.Compare(x, 40, y)) // token.LSS
	case "<=":
		return constant.MakeBool(constant.Compare(x, 43, y)) // token.LEQ
	case ">":
		return constant.MakeBool(constant.Compare(x, 41, y)) // token.GTR
	case ">=":
		return constant.MakeBool(constant.Compare(x, 44, y)) // token.GEQ
	}
	_ = cmp
	return constant.MakeBool(false)
}

func (c *Checker) evalLogical(x, y constant.Value, op syntax.Token) constant.Value {
	xb := constant.BoolVal(x)
	yb := constant.BoolVal(y)
	switch op.String() {
	case "&&":
		return constant.MakeBool(xb && yb)
	case "||":
		return constant.MakeBool(xb || yb)
	}
	return constant.MakeBool(false)
}

func (c *Checker) evalArithmetic(x, y constant.Value, op syntax.Token) constant.Value {
	switch strings.TrimSpace(op.String()) {
	case "+":
		return constant.BinaryOp(x, 12, y) // token.ADD
	case "-":
		return constant.BinaryOp(x, 13, y) // token.SUB
	case "*":
		return constant.BinaryOp(x, 14, y) // token.MUL
	case "/":
		return constant.BinaryOp(x, 15, y) // token.QUO
	case "%":
		return constant.BinaryOp(x, 16, y) // token.REM
	}
	return nil
}

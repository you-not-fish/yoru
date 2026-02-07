package types2

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// checkPointerEscape checks if a *T value is escaping to a forbidden location.
// Forbidden locations: global variables, heap object fields.
func (c *Checker) checkPointerEscape(lhs syntax.Expr, rhs *operand) {
	if !types.IsPointer(rhs.typ) {
		return
	}

	// Check 1: Global variable
	if name, ok := lhs.(*syntax.Name); ok {
		obj := c.lookup(name.Value)
		if obj != nil && obj.Parent() == c.pkg.Scope() {
			c.errorf(lhs.Pos(), "*T cannot escape to global variable %s", name.Value)
			return
		}
	}

	// Check 2: Heap object field (ref T field)
	if sel, ok := lhs.(*syntax.SelectorExpr); ok {
		var base operand
		c.expr(&base, sel.X)
		if types.IsRef(base.typ) {
			c.errorf(lhs.Pos(), "*T cannot escape to heap object field")
			return
		}
	}

	// Check 3: Array element of a ref type
	if idx, ok := lhs.(*syntax.IndexExpr); ok {
		var base operand
		c.expr(&base, idx.X)
		if types.IsRef(base.typ) {
			c.errorf(lhs.Pos(), "*T cannot escape to heap object element")
			return
		}
	}
}

// checkReturnEscape checks if a *T value is being returned.
// Returning *T is forbidden because it would escape the stack frame.
func (c *Checker) checkReturnEscape(s *syntax.ReturnStmt, x *operand) {
	if !types.IsPointer(x.typ) {
		return
	}

	c.errorf(s.Pos(), "cannot return *T from function (use ref T for heap allocation)")
}

// checkCallArgEscape checks if *T values are passed to function arguments.
// v1 conservative rule: prohibit passing *T to any non-builtin function.
func (c *Checker) checkCallArgEscape(e *syntax.CallExpr, args []*operand) {
	// Check if the callee is a builtin (builtins are safe)
	isBuiltin := false
	if name, ok := e.Fun.(*syntax.Name); ok {
		obj := c.lookup(name.Value)
		if obj != nil {
			_, isBuiltin = obj.(*types.Builtin)
		}
	}

	for i, arg := range args {
		if arg == nil || arg.mode == invalid {
			continue
		}
		if !types.IsPointer(arg.typ) {
			continue
		}

		if !isBuiltin {
			c.errorf(e.Args[i].Pos(),
				"*T cannot be passed to function (may escape); use ref T for heap data")
		}
	}
}

// checkRefToPtrConversion checks for illegal ref T -> *T conversions.
// This is checked during assignment.
func (c *Checker) checkRefToPtrConversion(x *operand, target types.Type) bool {
	if types.IsPointer(target) && types.IsRef(x.typ) {
		c.errorf(x.pos,
			"cannot convert %s to %s (would cause use-after-free)", x.typ, target)
		return false
	}
	return true
}

package types2

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// call checks a function call expression.
func (c *Checker) call(x *operand, e *syntax.CallExpr) {
	// Check if this is a method call
	if sel, ok := e.Fun.(*syntax.SelectorExpr); ok {
		c.methodCall(x, e, sel)
		return
	}

	// Evaluate the function expression
	c.expr(x, e.Fun)
	if x.mode == invalid {
		return
	}

	// Handle builtin functions
	if x.mode == builtin {
		c.builtinCall(x, e)
		return
	}

	// Regular function call
	c.regularCall(x, e)
}

// regularCall handles regular function calls.
func (c *Checker) regularCall(x *operand, e *syntax.CallExpr) {
	// Get function signature
	sig, ok := x.typ.(*types.Func)
	if !ok {
		c.errorf(e.Fun.Pos(), "cannot call non-function %s", x.typ)
		x.mode = invalid
		return
	}

	// Check arguments
	args := c.checkCallArgs(e, sig)

	// Check escape for *T arguments (v1 conservative rule)
	c.checkCallArgEscape(e, args)

	// Set result
	if sig.Result() != nil {
		x.mode = value
		x.typ = sig.Result()
	} else {
		x.mode = novalue
		x.typ = nil
	}
}

// methodCall handles method calls (x.M(...)).
func (c *Checker) methodCall(x *operand, e *syntax.CallExpr, sel *syntax.SelectorExpr) {
	// Evaluate the receiver
	c.expr(x, sel.X)
	if x.mode == invalid {
		return
	}

	// Look up the method
	method, needAddr, _ := c.lookupMethod(x.typ, sel.Sel.Value)
	if method == nil {
		c.errorf(sel.Sel.Pos(), "%s has no method %s", x.typ, sel.Sel.Value)
		x.mode = invalid
		return
	}

	// Check that we can auto-address if needed
	if needAddr && x.mode != variable {
		c.errorf(sel.Pos(), "cannot call pointer method on non-addressable %s", x.typ)
		x.mode = invalid
		return
	}

	c.recordUse(sel.Sel, method)

	sig := method.Signature()
	if sig == nil {
		c.errorf(sel.Pos(), "method %s has no signature", sel.Sel.Value)
		x.mode = invalid
		return
	}

	// Check arguments
	args := c.checkCallArgs(e, sig)

	// Check escape for *T arguments
	c.checkCallArgEscape(e, args)

	// Set result
	if sig.Result() != nil {
		x.mode = value
		x.typ = sig.Result()
	} else {
		x.mode = novalue
		x.typ = nil
	}
}

// lookupMethod looks up a method by name on type T.
// Returns the method, whether auto-addressing is needed, and whether auto-dereferencing is needed.
func (c *Checker) lookupMethod(T types.Type, name string) (*types.FuncObj, bool, bool) {
	var needAddr, needDeref bool

	// Dereference pointers and refs
	base := T
	switch t := T.Underlying().(type) {
	case *types.Pointer:
		base = t.Elem()
		needDeref = true
	case *types.Ref:
		base = t.Elem()
		needDeref = true
	}

	// Find named type
	named, ok := base.(*types.Named)
	if !ok {
		return nil, false, false
	}

	// Look up method
	method := named.LookupMethod(name)
	if method == nil {
		return nil, false, false
	}

	// Check if receiver is pointer type
	if method.Signature() != nil && method.Signature().Recv() != nil {
		recv := method.Signature().Recv()
		if _, isPtr := recv.Type().Underlying().(*types.Pointer); isPtr && !needDeref {
			// Value type calling pointer method - need auto-address
			needAddr = true
		}
	}

	return method, needAddr, needDeref
}

// checkCallArgs checks function call arguments.
func (c *Checker) checkCallArgs(e *syntax.CallExpr, sig *types.Func) []*operand {
	args := make([]*operand, len(e.Args))

	// Check argument count
	expected := sig.NumParams()
	got := len(e.Args)
	if got != expected {
		c.errorf(e.Pos(), "wrong number of arguments: got %d, want %d", got, expected)
		// Continue checking what we can
	}

	// Check each argument
	for i, arg := range e.Args {
		args[i] = &operand{}
		c.expr(args[i], arg)
		if args[i].mode == invalid {
			continue
		}

		if i < expected {
			param := sig.Param(i)
			c.assignment(args[i], param.Type(), "argument")
		}
	}

	return args
}

// builtinCall handles builtin function calls (println, new, panic).
func (c *Checker) builtinCall(x *operand, e *syntax.CallExpr) {
	// Get builtin name
	name, ok := e.Fun.(*syntax.Name)
	if !ok {
		c.errorf(e.Fun.Pos(), "unexpected builtin expression")
		x.mode = invalid
		return
	}

	obj := c.lookup(name.Value)
	if obj == nil {
		x.mode = invalid
		return
	}

	builtin, ok := obj.(*types.Builtin)
	if !ok {
		c.errorf(name.Pos(), "%s is not a builtin", name.Value)
		x.mode = invalid
		return
	}

	switch builtin.Kind() {
	case types.BuiltinPrintln:
		c.builtinPrintln(x, e)
	case types.BuiltinNew:
		c.builtinNew(x, e)
	case types.BuiltinPanic:
		c.builtinPanic(x, e)
	default:
		c.errorf(name.Pos(), "unknown builtin %s", name.Value)
		x.mode = invalid
	}
}

// builtinPrintln handles println(args...).
func (c *Checker) builtinPrintln(x *operand, e *syntax.CallExpr) {
	// println accepts any number of arguments of basic types
	for _, arg := range e.Args {
		var a operand
		c.expr(&a, arg)
		if a.mode == invalid {
			continue
		}

		// Check that argument is printable
		if !c.isPrintable(a.typ) {
			c.errorf(arg.Pos(), "cannot print value of type %s", a.typ)
		}
	}

	x.mode = novalue
	x.typ = nil
}

// isPrintable reports whether a type can be printed.
func (c *Checker) isPrintable(t types.Type) bool {
	switch t := t.Underlying().(type) {
	case *types.Basic:
		// All basic types are printable
		return t.Kind() != types.Invalid
	case *types.Pointer, *types.Ref:
		// Pointers and refs are printable (as addresses)
		return true
	default:
		return false
	}
}

// builtinNew handles new(T).
func (c *Checker) builtinNew(x *operand, e *syntax.CallExpr) {
	if len(e.Args) != 1 {
		c.errorf(e.Pos(), "new requires exactly one argument")
		x.mode = invalid
		return
	}

	// The argument must be a type
	T := c.resolveType(e.Args[0])
	if T == nil {
		x.mode = invalid
		return
	}

	// new(T) returns ref T
	x.mode = value
	x.typ = types.NewRef(T)
}

// builtinPanic handles panic(msg).
func (c *Checker) builtinPanic(x *operand, e *syntax.CallExpr) {
	if len(e.Args) != 1 {
		c.errorf(e.Pos(), "panic requires exactly one argument")
		x.mode = invalid
		return
	}

	var arg operand
	c.expr(&arg, e.Args[0])
	if arg.mode == invalid {
		x.mode = invalid
		return
	}

	// Argument must be a string
	if !isStringType(arg.typ) {
		c.errorf(e.Args[0].Pos(), "panic argument must be a string")
	}

	x.mode = novalue
	x.typ = nil
}

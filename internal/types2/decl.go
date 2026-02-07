package types2

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// checkTypeDecl type-checks a type declaration.
// It reports whether the declaration's resolved type changed.
func (c *Checker) checkTypeDecl(decl *syntax.TypeDecl) bool {
	obj := c.lookup(decl.Name.Value)
	if obj == nil {
		return false // already reported in collectDecls
	}
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return false
	}

	// Resolve the underlying type
	underlying := c.resolveType(decl.Type)
	if underlying == nil {
		return false
	}

	if decl.Alias {
		// Type alias: type T = U
		if tn.Type() == underlying {
			return false
		}
		tn.SetType(underlying)
		return true
	}

	// Type definition: type T struct { ... }
	// Reuse named placeholder created during collection when available.
	if named, ok := tn.Type().(*types.Named); ok {
		if named.Underlying() == underlying {
			return false
		}
		named.SetUnderlying(underlying)
		return true
	}
	types.NewNamed(tn, underlying)
	return true
}

// checkVarDecl type-checks a top-level variable declaration.
func (c *Checker) checkVarDecl(decl *syntax.VarDecl) {
	obj := c.lookup(decl.Name.Value)
	if obj == nil {
		return
	}
	v, ok := obj.(*types.Var)
	if !ok {
		return
	}

	var typ types.Type
	var val operand

	// Determine type from explicit annotation or initializer
	if decl.Type != nil {
		typ = c.resolveType(decl.Type)
		if typ == nil {
			return
		}
	}

	if decl.Value != nil {
		c.expr(&val, decl.Value)
		if val.mode == invalid {
			return
		}
		if val.mode == novalue {
			c.errorf(decl.Value.Pos(), "cannot use no-value expression as variable initializer")
			return
		}

		if typ == nil {
			// Type inference
			if types.IsUntypedType(val.typ) {
				typ = types.DefaultType(val.typ)
			} else {
				typ = val.typ
			}
		} else {
			// Check assignment
			c.assignment(&val, typ, "variable declaration")
		}
	}

	if typ == nil {
		c.errorf(decl.Pos(), "missing type or initializer in variable declaration")
		return
	}

	// Update the Var's type
	v.SetType(typ)
}

// checkFuncSignature type-checks a function signature.
func (c *Checker) checkFuncSignature(decl *syntax.FuncDecl) {
	fn := c.funcDecls[decl]
	if fn == nil {
		return
	}

	// Resolve parameter types
	params := make([]*types.Var, len(decl.Params))
	for i, p := range decl.Params {
		ptype := c.resolveType(p.Type)
		if ptype == nil {
			return
		}
		name := ""
		if p.Name != nil {
			name = p.Name.Value
		}
		params[i] = types.NewVar(p.Pos(), name, ptype)
	}

	// Resolve return type
	var result types.Type
	if decl.Result != nil {
		result = c.resolveType(decl.Result)
		if result == nil {
			return
		}
	}

	// Resolve receiver (for methods)
	var recv *types.Var
	if decl.Recv != nil {
		recvType := c.resolveType(decl.Recv.Type)
		if recvType == nil {
			return
		}
		name := ""
		if decl.Recv.Name != nil {
			name = decl.Recv.Name.Value
		}
		recv = types.NewVar(decl.Recv.Pos(), name, recvType)

		// Add method to receiver type
		c.addMethod(decl.Name.Pos(), recvType, fn)
	}

	// Create function signature
	sig := types.NewFunc(recv, params, result)
	fn.SetSignature(sig)
}

// addMethod adds a method to the receiver's named type.
func (c *Checker) addMethod(pos syntax.Pos, recvType types.Type, method *types.FuncObj) {
	// Get base type (strip pointer/ref)
	base := recvType
	if ptr, ok := recvType.(*types.Pointer); ok {
		base = ptr.Elem()
	}
	if _, ok := recvType.(*types.Ref); ok {
		c.errorf(pos, "method receiver cannot be ref type %s", recvType)
		return
	}

	// Find the named type
	if named, ok := base.(*types.Named); ok {
		if existing := named.LookupMethod(method.Name()); existing != nil {
			c.errorf(pos, "method %s already declared for %s", method.Name(), named)
			return
		}
		named.AddMethod(method)
		return
	}
	c.errorf(pos, "method receiver must be a named type or pointer to named type")
}

// checkFuncBody type-checks a function body.
func (c *Checker) checkFuncBody(decl *syntax.FuncDecl) {
	if decl.Body == nil {
		return // No body to check
	}

	// Get function object
	fn := c.funcDecls[decl]
	if fn == nil {
		return
	}

	sig := fn.Signature()
	if sig == nil {
		return
	}

	// Save function context
	oldFuncSig := c.funcSig
	c.funcSig = sig

	// Create function scope
	c.openScope(decl.Body, "function "+decl.Name.Value)

	// Add receiver to scope
	if sig.Recv() != nil {
		recv := sig.Recv()
		if recv.Name() != "" {
			c.scope.Insert(recv)
		}
	}

	// Add parameters to scope
	for _, p := range sig.Params() {
		if p.Name() != "" {
			c.scope.Insert(p)
		}
	}

	// Check body statements
	c.stmts(decl.Body.Stmts)

	// Check return completeness: all paths must return when a result type exists.
	if sig.Result() != nil && !c.blockMustReturn(decl.Body.Stmts) {
		c.errorf(decl.Body.Rbrace, "missing return statement")
	}

	c.closeScope()

	// Restore function context
	c.funcSig = oldFuncSig
}

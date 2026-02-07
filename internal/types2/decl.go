package types2

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// checkTypeDecl type-checks a type declaration.
func (c *Checker) checkTypeDecl(decl *syntax.TypeDecl) {
	obj := c.lookup(decl.Name.Value)
	if obj == nil {
		return // already reported in collectDecls
	}
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return
	}

	// Resolve the underlying type
	underlying := c.resolveType(decl.Type)
	if underlying == nil {
		return
	}

	if decl.Alias {
		// Type alias: type T = U
		// The TypeName simply refers to the aliased type
		tn.Type() // already set in collectTypeDecl
		// Update the type to the resolved type
		// For aliases, we don't wrap in Named
	} else {
		// Type definition: type T struct { ... }
		// Create a Named type
		named := types.NewNamed(tn, underlying)
		_ = named // TypeName.typ is already set by NewNamed
	}
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
	obj, _ := c.scope.LookupParent(decl.Name.Value)
	if obj == nil {
		return
	}
	fn, ok := obj.(*types.FuncObj)
	if !ok {
		// Might be a method, handled differently
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
		c.addMethod(recvType, fn)
	}

	// Create function signature
	sig := types.NewFunc(recv, params, result)
	fn.SetSignature(sig)
}

// addMethod adds a method to the receiver's named type.
func (c *Checker) addMethod(recvType types.Type, method *types.FuncObj) {
	// Get base type (strip pointer/ref)
	base := recvType
	if ptr, ok := recvType.(*types.Pointer); ok {
		base = ptr.Elem()
	}

	// Find the named type
	if named, ok := base.(*types.Named); ok {
		named.AddMethod(method)
	}
}

// checkFuncBody type-checks a function body.
func (c *Checker) checkFuncBody(decl *syntax.FuncDecl) {
	if decl.Body == nil {
		return // No body to check
	}

	// Get function object
	obj, _ := c.scope.LookupParent(decl.Name.Value)
	if obj == nil {
		return
	}
	fn, ok := obj.(*types.FuncObj)
	if !ok {
		return
	}

	sig := fn.Signature()
	if sig == nil {
		return
	}

	// Save function context
	oldFuncSig := c.funcSig
	oldHasReturn := c.hasReturn
	c.funcSig = sig
	c.hasReturn = false

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

	// Check return
	if sig.Result() != nil && !c.hasReturn {
		c.errorf(decl.Body.Rbrace, "missing return statement")
	}

	c.closeScope()

	// Restore function context
	c.funcSig = oldFuncSig
	c.hasReturn = oldHasReturn
}

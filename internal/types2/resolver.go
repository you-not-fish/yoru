package types2

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// collectDecls collects all top-level declarations and creates
// placeholder objects for them in the package scope.
func (c *Checker) collectDecls(decls []syntax.Decl) {
	for _, d := range decls {
		switch decl := d.(type) {
		case *syntax.TypeDecl:
			c.collectTypeDecl(decl)
		case *syntax.VarDecl:
			c.collectVarDecl(decl)
		case *syntax.FuncDecl:
			c.collectFuncDecl(decl)
		case *syntax.ImportDecl:
			// Imports are parsed but not semantically analyzed in v1
			c.errorf(decl.Pos(), "import statements are not supported")
		}
	}
}

// collectTypeDecl collects a type declaration.
func (c *Checker) collectTypeDecl(decl *syntax.TypeDecl) {
	// Create a TypeName with a named placeholder for forward references.
	obj := types.NewTypeName(decl.Name.Pos(), decl.Name.Value, nil)

	// Create a named placeholder immediately
	// so recursive types (e.g. struct fields pointing to the same named type)
	// can resolve before the underlying type is finalized.
	types.NewNamed(obj, nil)

	c.declare(decl.Name, obj)
}

// collectVarDecl collects a variable declaration.
func (c *Checker) collectVarDecl(decl *syntax.VarDecl) {
	// Create a Var object with nil type
	// The type will be resolved in checkVarDecl
	obj := types.NewVar(decl.Name.Pos(), decl.Name.Value, nil)
	c.declare(decl.Name, obj)
}

// collectFuncDecl collects a function declaration.
func (c *Checker) collectFuncDecl(decl *syntax.FuncDecl) {
	name := decl.Name.Value
	obj := types.NewFuncObj(decl.Name.Pos(), name)
	c.funcDecls[decl] = obj

	// Methods are attached to their receiver type; they are not package-scope
	// declarations and therefore are not inserted into the package scope.
	if decl.Recv != nil {
		if c.info != nil {
			c.info.Defs[decl.Name] = obj
		}
		return
	}

	// Regular function
	c.declare(decl.Name, obj)
}

// resolve resolves a name to an object.
// Reports an error if the name is undefined.
func (c *Checker) resolve(name *syntax.Name) types.Object {
	obj := c.lookup(name.Value)
	if obj == nil {
		c.errorf(name.Pos(), "undefined: %s", name.Value)
		return nil
	}
	c.recordUse(name, obj)
	return obj
}

// resolveType resolves a type expression and returns the resulting type.
func (c *Checker) resolveType(e syntax.Expr) types.Type {
	var x operand
	c.typExpr(&x, e)
	if x.mode == invalid || x.mode != typexpr || x.typ == nil {
		return nil
	}
	return x.typ
}

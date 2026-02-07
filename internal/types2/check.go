package types2

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// Checker is the type checker.
type Checker struct {
	conf *Config
	info *Info
	pkg  *types.Package

	// Current checking context
	scope *types.Scope // current scope
	pos   syntax.Pos   // current position (for error reporting)

	// Function context
	funcSig *types.Func // current function signature

	// Control-flow context
	loopDepth int // nested loop depth (for break/continue validation)

	// Declaration objects keyed by AST node.
	// Methods are not in package scope, so they must be tracked separately.
	// Lifecycle: allocated per Check invocation and used only while checking one file.
	funcDecls map[*syntax.FuncDecl]*types.FuncObj

	// Error tracking
	errors int        // error count
	first  *TypeError // first error
}

// checkFile type-checks a single file.
func (c *Checker) checkFile(file *syntax.File) {
	// Create package
	pkgName := "main"
	if file.PkgName != nil {
		pkgName = file.PkgName.Value
	}
	c.pkg = types.NewPackage(pkgName)
	c.scope = c.pkg.Scope()

	// Record file scope
	if c.info != nil {
		c.info.Scopes[file] = c.scope
	}

	// Phase 1: Collect all top-level declarations
	c.collectDecls(file.Decls)

	// Phase 2: Check type declarations (resolve underlying types)
	var typeDecls []*syntax.TypeDecl
	for _, decl := range file.Decls {
		if td, ok := decl.(*syntax.TypeDecl); ok {
			typeDecls = append(typeDecls, td)
		}
	}
	// Run multiple passes so forward aliases can settle to final types.
	// Example: type A = B; type B = int
	for pass := 0; pass < len(typeDecls); pass++ {
		changed := false
		for _, td := range typeDecls {
			if c.checkTypeDecl(td) {
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Phase 3: Check function signatures
	for _, decl := range file.Decls {
		if fd, ok := decl.(*syntax.FuncDecl); ok {
			c.checkFuncSignature(fd)
		}
	}

	// Phase 4: Check variable declarations
	for _, decl := range file.Decls {
		if vd, ok := decl.(*syntax.VarDecl); ok {
			c.checkVarDecl(vd)
		}
	}

	// Phase 5: Check function bodies
	for _, decl := range file.Decls {
		if fd, ok := decl.(*syntax.FuncDecl); ok {
			c.checkFuncBody(fd)
		}
	}
}

// openScope creates a new scope as a child of the current scope.
func (c *Checker) openScope(n syntax.Node, comment string) *types.Scope {
	s := types.NewScope(c.scope, n.Pos(), n.End(), comment)
	c.scope = s
	if c.info != nil {
		c.info.Scopes[n] = s
	}
	return s
}

// closeScope returns to the parent scope.
func (c *Checker) closeScope() {
	c.scope = c.scope.Parent()
}

// lookup looks up a name in the current scope chain.
func (c *Checker) lookup(name string) types.Object {
	obj, _ := c.scope.LookupParent(name)
	return obj
}

// declare declares an object in the current scope.
// Reports an error if the name is already declared.
func (c *Checker) declare(name *syntax.Name, obj types.Object) {
	if existing := c.scope.Insert(obj); existing != nil {
		c.errorf(name.Pos(), "%s redeclared in this block", name.Value)
		return
	}
	if c.info != nil {
		c.info.Defs[name] = obj
	}
}

// recordType records the type information for an expression.
func (c *Checker) recordType(e syntax.Expr, x *operand) {
	if c.info == nil {
		return
	}
	c.info.Types[e] = TypeAndValue{
		Type:  x.typ,
		Value: x.val,
		mode:  x.mode,
	}
}

// recordUse records a use of an object.
func (c *Checker) recordUse(name *syntax.Name, obj types.Object) {
	if c.info != nil {
		c.info.Uses[name] = obj
	}
}

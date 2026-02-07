package types2

import (
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// stmts checks a list of statements.
func (c *Checker) stmts(list []syntax.Stmt) {
	for _, s := range list {
		c.stmt(s)
	}
}

// stmt checks a single statement.
func (c *Checker) stmt(s syntax.Stmt) {
	switch s := s.(type) {
	case *syntax.EmptyStmt:
		// Nothing to check

	case *syntax.ExprStmt:
		c.exprStmt(s)

	case *syntax.AssignStmt:
		c.assignStmt(s)

	case *syntax.BlockStmt:
		c.blockStmt(s)

	case *syntax.IfStmt:
		c.ifStmt(s)

	case *syntax.ForStmt:
		c.forStmt(s)

	case *syntax.ReturnStmt:
		c.returnStmt(s)

	case *syntax.BranchStmt:
		c.branchStmt(s)

	case *syntax.DeclStmt:
		c.declStmt(s)

	default:
		c.errorf(s.Pos(), "unexpected statement %T", s)
	}
}

// exprStmt checks an expression statement.
func (c *Checker) exprStmt(s *syntax.ExprStmt) {
	var x operand
	c.expr(&x, s.X)
	// Expression statements are typically function calls
	// The result (if any) is discarded
}

// blockStmt checks a block statement.
func (c *Checker) blockStmt(s *syntax.BlockStmt) {
	c.openScope(s, "block")
	c.stmts(s.Stmts)
	c.closeScope()
}

// ifStmt checks an if statement.
func (c *Checker) ifStmt(s *syntax.IfStmt) {
	// Check condition
	var cond operand
	c.expr(&cond, s.Cond)
	if cond.mode != invalid && !isBoolean(cond.typ) {
		c.errorf(s.Cond.Pos(), "non-boolean condition in if statement")
	}

	// Check then branch
	c.openScope(s.Then, "if then")
	c.stmts(s.Then.Stmts)
	c.closeScope()

	// Check else branch
	if s.Else != nil {
		switch els := s.Else.(type) {
		case *syntax.BlockStmt:
			c.openScope(els, "if else")
			c.stmts(els.Stmts)
			c.closeScope()
		case *syntax.IfStmt:
			c.ifStmt(els)
		}
	}
}

// forStmt checks a for statement.
func (c *Checker) forStmt(s *syntax.ForStmt) {
	// Open a scope for the loop
	c.openScope(s.Body, "for")
	c.loopDepth++
	defer func() {
		c.loopDepth--
		c.closeScope()
	}()

	// Check condition
	if s.Cond != nil {
		var cond operand
		c.expr(&cond, s.Cond)
		if cond.mode != invalid && !isBoolean(cond.typ) {
			c.errorf(s.Cond.Pos(), "non-boolean condition in for statement")
		}
	}

	// Check body
	c.stmts(s.Body.Stmts)
}

// returnStmt checks a return statement.
func (c *Checker) returnStmt(s *syntax.ReturnStmt) {
	if c.funcSig == nil {
		c.errorf(s.Pos(), "return statement outside function")
		return
	}

	resultType := c.funcSig.Result()

	if s.Result == nil {
		// Bare return
		if resultType != nil {
			c.errorf(s.Pos(), "missing return value")
		}
		return
	}

	// Check return value
	var x operand
	c.expr(&x, s.Result)
	if x.mode == invalid {
		return
	}

	if resultType == nil {
		c.errorf(s.Pos(), "unexpected return value in void function")
		return
	}

	// Check escape: *T cannot be returned
	c.checkReturnEscape(s, &x)

	// Check assignment
	c.assignment(&x, resultType, "return statement")
}

// branchStmt checks a break or continue statement.
func (c *Checker) branchStmt(s *syntax.BranchStmt) {
	if c.loopDepth > 0 {
		return
	}
	if s.Tok.IsBreak() {
		c.errorf(s.Pos(), "break not in for loop")
		return
	}
	if s.Tok.IsContinue() {
		c.errorf(s.Pos(), "continue not in for loop")
		return
	}
	c.errorf(s.Pos(), "unexpected branch statement")
}

// declStmt checks a declaration statement (var inside function body).
func (c *Checker) declStmt(s *syntax.DeclStmt) {
	switch decl := s.Decl.(type) {
	case *syntax.VarDecl:
		c.localVarDecl(decl)
	default:
		c.errorf(s.Pos(), "unexpected declaration in statement context")
	}
}

// localVarDecl checks a local variable declaration.
func (c *Checker) localVarDecl(decl *syntax.VarDecl) {
	var typ types.Type
	var val operand

	// Determine type
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

	// Create and declare variable
	v := types.NewVar(decl.Name.Pos(), decl.Name.Value, typ)
	c.declare(decl.Name, v)
}

// assignStmt checks an assignment statement.
func (c *Checker) assignStmt(s *syntax.AssignStmt) {
	if len(s.LHS) != len(s.RHS) {
		c.errorf(s.Pos(), "assignment mismatch: %d variables but %d values", len(s.LHS), len(s.RHS))
		return
	}

	for i := range s.LHS {
		if s.Op.IsDefine() {
			// Short variable declaration :=
			c.shortVarDecl(s.LHS[i], s.RHS[i])
		} else {
			// Regular assignment =
			c.regularAssign(s.LHS[i], s.RHS[i])
		}
	}
}

// shortVarDecl handles short variable declaration (x := expr).
func (c *Checker) shortVarDecl(lhs syntax.Expr, rhs syntax.Expr) {
	name, ok := lhs.(*syntax.Name)
	if !ok {
		c.errorf(lhs.Pos(), "non-name on left side of :=")
		return
	}

	var val operand
	c.expr(&val, rhs)
	if val.mode == invalid {
		return
	}
	if val.mode == novalue {
		c.errorf(rhs.Pos(), "cannot use no-value expression in := declaration")
		return
	}

	// Determine type
	typ := val.typ
	if types.IsUntypedType(typ) {
		typ = types.DefaultType(typ)
	}

	// Create and declare variable
	v := types.NewVar(name.Pos(), name.Value, typ)
	c.declare(name, v)
}

// regularAssign handles regular assignment (lhs = rhs).
func (c *Checker) regularAssign(lhs syntax.Expr, rhs syntax.Expr) {
	var left, right operand

	c.expr(&left, lhs)
	c.expr(&right, rhs)

	if left.mode == invalid || right.mode == invalid {
		return
	}
	if right.mode == novalue {
		c.errorf(rhs.Pos(), "cannot assign no-value expression")
		return
	}

	// Check that lhs is assignable
	if left.mode != variable {
		c.errorf(lhs.Pos(), "cannot assign to %s", lhs)
		return
	}

	// Check escape: *T cannot be assigned to certain locations
	if types.IsPointer(right.typ) {
		c.checkPointerEscape(lhs, &right)
	}

	// Check assignment compatibility
	c.assignment(&right, left.typ, "assignment")
}

// blockMustReturn reports whether all control-flow paths in this statement list return.
// This is conservative: loops are treated as potentially non-terminating paths.
// Yoru syntax rejects bare "for { ... }", so we don't model infinite-loop proofs here.
func (c *Checker) blockMustReturn(stmts []syntax.Stmt) bool {
	for _, s := range stmts {
		if c.stmtMustReturn(s) {
			return true
		}
	}
	return false
}

func (c *Checker) stmtMustReturn(s syntax.Stmt) bool {
	switch s := s.(type) {
	case *syntax.ReturnStmt:
		return true
	case *syntax.BlockStmt:
		return c.blockMustReturn(s.Stmts)
	case *syntax.IfStmt:
		if s.Else == nil {
			return false
		}
		thenReturns := c.blockMustReturn(s.Then.Stmts)
		switch els := s.Else.(type) {
		case *syntax.BlockStmt:
			return thenReturns && c.blockMustReturn(els.Stmts)
		case *syntax.IfStmt:
			return thenReturns && c.stmtMustReturn(els)
		default:
			return false
		}
	}
	return false
}

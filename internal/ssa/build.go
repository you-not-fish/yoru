package ssa

import (
	"fmt"

	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
	"github.com/you-not-fish/yoru/internal/types2"
)

// builder holds the state for lowering a single function from Typed AST to SSA.
type builder struct {
	info  *types2.Info  // type-checker output (read-only)
	sizes *types.Sizes  // DefaultSizes for layout queries

	fn *Func  // current SSA function
	b  *Block // current block (nil = unreachable)

	vars map[types.Object]*Value // Object → alloca mapping

	breakTarget    *Block // innermost loop exit
	continueTarget *Block // innermost loop header
}

// BuildFile builds SSA functions for all function declarations in the file.
// It returns a list of SSA functions (one per FuncDecl with a body).
func BuildFile(file *syntax.File, info *types2.Info, sizes *types.Sizes) []*Func {
	var funcs []*Func
	for _, decl := range file.Decls {
		fd, ok := decl.(*syntax.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		fn := buildFunc(fd, info, sizes)
		funcs = append(funcs, fn)
	}
	return funcs
}

// buildFunc builds an SSA function from a FuncDecl.
func buildFunc(fd *syntax.FuncDecl, info *types2.Info, sizes *types.Sizes) *Func {
	// Look up the FuncObj for this declaration.
	obj := info.Defs[fd.Name]
	if obj == nil {
		panic(fmt.Sprintf("ssa.buildFunc: no object for func %s", fd.Name.Value))
	}
	funcObj, ok := obj.(*types.FuncObj)
	if !ok {
		panic(fmt.Sprintf("ssa.buildFunc: expected *types.FuncObj, got %T", obj))
	}
	sig := funcObj.Signature()

	fn := NewFunc(fd.Name.Value, sig)

	b := &builder{
		info:  info,
		sizes: sizes,
		fn:    fn,
		b:     fn.Entry,
		vars:  make(map[types.Object]*Value),
	}

	// Emit receiver as OpArg + OpAlloca + OpStore (if method).
	if fd.Recv != nil && sig.Recv() != nil {
		recv := sig.Recv()
		argVal := fn.NewValue(fn.Entry, OpArg, recv.Type())
		argVal.AuxInt = -1 // receiver distinguished from params
		argVal.Aux = recv.Name()

		alloca := b.entryAlloca(recv.Type(), recv.Name())
		fn.NewValue(fn.Entry, OpStore, nil, alloca, argVal)

		// Map the receiver object. The type checker inserts sig.Recv()
		// directly into the scope, so info.Uses will map to it.
		b.vars[recv] = alloca
	}

	// Emit parameters: OpArg + OpAlloca + OpStore for each.
	for i := 0; i < sig.NumParams(); i++ {
		param := sig.Param(i)
		argVal := fn.NewValue(fn.Entry, OpArg, param.Type())
		argVal.AuxInt = int64(i)
		argVal.Aux = param.Name()

		alloca := b.entryAlloca(param.Type(), param.Name())
		fn.NewValue(fn.Entry, OpStore, nil, alloca, argVal)

		// Map the parameter object. The type checker inserts sig.Param(i)
		// into the scope directly, so info.Uses maps to the same *types.Var.
		b.vars[param] = alloca
	}

	// Lower function body.
	b.stmts(fd.Body.Stmts)

	// Implicit void return: if the current block is still open and unterminated,
	// emit a void return.
	if b.b != nil && b.b.Kind == BlockPlain && len(b.b.Succs) == 0 {
		b.b.Kind = BlockReturn
		// No control value for void return.
	}

	return fn
}

// entryAlloca creates an alloca in the entry block for a variable of the given type.
// All allocas go into the entry block to satisfy the mem2reg prerequisite.
func (b *builder) entryAlloca(typ types.Type, name string) *Value {
	ptrTyp := types.NewPointer(typ)
	alloca := b.fn.NewValue(b.fn.Entry, OpAlloca, ptrTyp)
	alloca.Aux = name
	return alloca
}

// stmts lowers a list of statements.
func (b *builder) stmts(list []syntax.Stmt) {
	for _, s := range list {
		if b.b == nil {
			// Unreachable code after return/break/continue/panic.
			break
		}
		b.stmt(s)
	}
}

// stmt dispatches a statement to the appropriate lowering method.
func (b *builder) stmt(s syntax.Stmt) {
	if b.b == nil {
		return
	}
	switch s := s.(type) {
	case *syntax.EmptyStmt:
		// no-op

	case *syntax.ExprStmt:
		// Evaluate for side effects, discard result.
		b.expr(s.X)

	case *syntax.DeclStmt:
		b.declStmt(s)

	case *syntax.AssignStmt:
		b.assignStmt(s)

	case *syntax.ReturnStmt:
		b.returnStmt(s)

	case *syntax.IfStmt:
		b.ifStmt(s)

	case *syntax.ForStmt:
		b.forStmt(s)

	case *syntax.BranchStmt:
		b.branchStmt(s)

	case *syntax.BlockStmt:
		b.stmts(s.Stmts)

	default:
		panic(fmt.Sprintf("ssa.builder.stmt: unhandled %T", s))
	}
}

// declStmt handles variable declarations inside function bodies.
func (b *builder) declStmt(s *syntax.DeclStmt) {
	switch d := s.Decl.(type) {
	case *syntax.VarDecl:
		b.varDecl(d)
	default:
		// Type declarations inside functions are ignored for SSA.
	}
}

// varDecl lowers a var declaration: var x T = init
func (b *builder) varDecl(d *syntax.VarDecl) {
	obj := b.info.Defs[d.Name]
	if obj == nil {
		return
	}

	alloca := b.entryAlloca(obj.Type(), d.Name.Value)
	b.vars[obj] = alloca

	if d.Value != nil {
		// var x T = expr
		val := b.expr(d.Value)
		b.fn.NewValue(b.b, OpStore, nil, alloca, val)
	} else {
		// var x T (zero-initialized)
		size := b.sizes.Sizeof(obj.Type())
		zero := b.fn.NewValue(b.b, OpZero, nil, alloca)
		zero.AuxInt = size
	}
}

// assignStmt handles assignment (=) and short declaration (:=).
func (b *builder) assignStmt(s *syntax.AssignStmt) {
	if s.Op.IsDefine() {
		// Short declaration: x := expr
		// LHS[0] must be a Name.
		for i, lhs := range s.LHS {
			name, ok := lhs.(*syntax.Name)
			if !ok {
				continue
			}
			obj := b.info.Defs[name]
			if obj == nil {
				continue
			}

			alloca := b.entryAlloca(obj.Type(), name.Value)
			b.vars[obj] = alloca

			if i < len(s.RHS) {
				val := b.expr(s.RHS[i])
				b.fn.NewValue(b.b, OpStore, nil, alloca, val)
			}
		}
		return
	}

	// Regular assignment: lhs = rhs
	for i, lhs := range s.LHS {
		if i >= len(s.RHS) {
			break
		}
		ptr := b.addr(lhs)
		val := b.expr(s.RHS[i])
		b.fn.NewValue(b.b, OpStore, nil, ptr, val)
	}
}

// returnStmt handles: return [expr]
func (b *builder) returnStmt(s *syntax.ReturnStmt) {
	if s.Result != nil {
		val := b.expr(s.Result)
		// b.b may have changed due to expr evaluation (e.g., short-circuit).
		b.b.Kind = BlockReturn
		b.b.SetControl(val)
	} else {
		b.b.Kind = BlockReturn
	}
	b.b = nil // subsequent code is unreachable
}

// ifStmt handles: if cond { then } [else { ... }]
func (b *builder) ifStmt(s *syntax.IfStmt) {
	cond := b.expr(s.Cond)

	bThen := b.fn.NewBlock(BlockPlain)
	bDone := b.fn.NewBlock(BlockPlain)

	var bElse *Block
	if s.Else != nil {
		bElse = b.fn.NewBlock(BlockPlain)
	} else {
		bElse = bDone
	}

	// Current block becomes If.
	b.b.Kind = BlockIf
	b.b.SetControl(cond)
	b.b.AddSucc(bThen)
	b.b.AddSucc(bElse)

	// Lower then branch.
	b.b = bThen
	b.stmts(s.Then.Stmts)
	if b.b != nil {
		b.b.AddSucc(bDone)
	}

	// Lower else branch.
	if s.Else != nil {
		b.b = bElse
		switch els := s.Else.(type) {
		case *syntax.BlockStmt:
			b.stmts(els.Stmts)
		case *syntax.IfStmt:
			b.ifStmt(els)
		}
		if b.b != nil {
			b.b.AddSucc(bDone)
		}
	}

	// Continue in the done block (if reachable).
	if len(bDone.Preds) > 0 {
		b.b = bDone
	} else {
		// Both branches terminated — bDone is dead. Remove it.
		b.removeDead(bDone)
		b.b = nil
	}
}

// forStmt handles: for cond { body }
func (b *builder) forStmt(s *syntax.ForStmt) {
	bHeader := b.fn.NewBlock(BlockPlain)
	bBody := b.fn.NewBlock(BlockPlain)
	bExit := b.fn.NewBlock(BlockPlain)

	// Jump from current block to header.
	b.b.AddSucc(bHeader)

	// Header: evaluate condition.
	b.b = bHeader
	if s.Cond != nil {
		cond := b.expr(s.Cond)
		b.b.Kind = BlockIf
		b.b.SetControl(cond)
		b.b.AddSucc(bBody)
		b.b.AddSucc(bExit)
	} else {
		// Infinite loop (for {}).
		b.b.AddSucc(bBody)
	}

	// Save/restore loop targets.
	savedBreak := b.breakTarget
	savedContinue := b.continueTarget
	b.breakTarget = bExit
	b.continueTarget = bHeader

	// Lower body.
	b.b = bBody
	b.stmts(s.Body.Stmts)
	if b.b != nil {
		// Back-edge to header.
		b.b.AddSucc(bHeader)
	}

	// Restore loop targets.
	b.breakTarget = savedBreak
	b.continueTarget = savedContinue

	// Continue in exit block.
	b.b = bExit
}

// branchStmt handles break and continue.
func (b *builder) branchStmt(s *syntax.BranchStmt) {
	if s.Tok.IsBreak() {
		if b.breakTarget != nil {
			b.b.AddSucc(b.breakTarget)
		}
	} else {
		// continue
		if b.continueTarget != nil {
			b.b.AddSucc(b.continueTarget)
		}
	}
	b.b = nil // subsequent code is unreachable
}

// removeDead removes a dead block from the function's block list.
// The block must have no predecessors and no successors.
func (b *builder) removeDead(dead *Block) {
	blocks := b.fn.Blocks
	for i, blk := range blocks {
		if blk == dead {
			b.fn.Blocks = append(blocks[:i], blocks[i+1:]...)
			return
		}
	}
}

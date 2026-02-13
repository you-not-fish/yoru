package ssa

import (
	"fmt"
	"go/constant"

	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
	"github.com/you-not-fish/yoru/internal/types2"
)

// expr lowers an expression to an SSA value.
func (b *builder) expr(e syntax.Expr) *Value {
	// Check for constant expressions first.
	if tv, ok := b.info.Types[e]; ok && tv.IsConstant() {
		return b.constValue(e, tv)
	}

	switch e := e.(type) {
	case *syntax.Name:
		return b.nameExpr(e)

	case *syntax.BasicLit:
		// Should have been handled by constValue above.
		// Fall through to generate from the literal directly.
		return b.basicLitExpr(e)

	case *syntax.Operation:
		return b.operationExpr(e)

	case *syntax.CallExpr:
		return b.callExpr(e)

	case *syntax.SelectorExpr:
		return b.selectorExpr(e)

	case *syntax.IndexExpr:
		return b.indexExpr(e)

	case *syntax.CompositeLit:
		return b.compositeLitExpr(e)

	case *syntax.ParenExpr:
		return b.expr(e.X)

	case *syntax.NewExpr:
		return b.newExpr(e)

	default:
		panic(fmt.Sprintf("ssa.builder.expr: unhandled %T", e))
	}
}

// constValue generates an SSA constant value from a type-checked constant.
func (b *builder) constValue(e syntax.Expr, tv types2.TypeAndValue) *Value {
	typ := tv.Type
	if types.IsUntypedType(typ) {
		typ = types.DefaultType(typ)
	}

	val := tv.Value
	if val == nil {
		// nil constant
		v := b.fn.NewValue(b.b, OpConstNil, typ)
		return v
	}

	// Determine the target kind from the type.
	// go/constant may represent an integer-valued result as constant.Float
	// (e.g., 10/3 as a rational). We must respect the target type.
	isTargetInt := false
	if basic, ok := typ.Underlying().(*types.Basic); ok {
		isTargetInt = basic.Kind() == types.Int || basic.Kind() == types.UntypedInt
	}

	switch val.Kind() {
	case constant.Int:
		n, _ := constant.Int64Val(val)
		v := b.fn.NewValue(b.b, OpConst64, typ)
		v.AuxInt = n
		return v

	case constant.Float:
		if isTargetInt {
			// go/constant represents integer division results (e.g., 10/3)
			// as exact rationals with kind=Float. Truncate to int.
			f, _ := constant.Float64Val(val)
			v := b.fn.NewValue(b.b, OpConst64, typ)
			v.AuxInt = int64(f)
			return v
		}
		f, _ := constant.Float64Val(val)
		v := b.fn.NewValue(b.b, OpConstFloat, typ)
		v.AuxFloat = f
		return v

	case constant.Bool:
		v := b.fn.NewValue(b.b, OpConstBool, typ)
		if constant.BoolVal(val) {
			v.AuxInt = 1
		}
		return v

	case constant.String:
		s := constant.StringVal(val)
		v := b.fn.NewValue(b.b, OpConstString, typ)
		v.Aux = s
		return v

	default:
		panic(fmt.Sprintf("ssa.constValue: unhandled constant kind %v", val.Kind()))
	}
}

// basicLitExpr handles literals that weren't resolved as constants by the type checker.
// This should be rare; most literals are typed as constants.
func (b *builder) basicLitExpr(e *syntax.BasicLit) *Value {
	tv, ok := b.info.Types[e]
	if ok {
		return b.constValue(e, tv)
	}
	panic(fmt.Sprintf("ssa.basicLitExpr: no type info for %q", e.Value))
}

// nameExpr lowers a name reference to a load from its alloca.
func (b *builder) nameExpr(e *syntax.Name) *Value {
	obj := b.info.Uses[e]
	if obj == nil {
		// Could be a definition site or a builtin.
		obj = b.info.Defs[e]
		if obj == nil {
			panic(fmt.Sprintf("ssa.nameExpr: no object for %q", e.Value))
		}
	}

	// Check for nil literal.
	if _, isNil := obj.(*types.Nil); isNil {
		tv := b.info.Types[e]
		typ := tv.Type
		if types.IsUntypedType(typ) {
			typ = types.DefaultType(typ)
		}
		v := b.fn.NewValue(b.b, OpConstNil, typ)
		return v
	}

	alloca, ok := b.vars[obj]
	if !ok {
		panic(fmt.Sprintf("ssa.nameExpr: no alloca for %q", e.Value))
	}

	// Load from the alloca.
	loadTyp := obj.Type()
	return b.fn.NewValue(b.b, OpLoad, loadTyp, alloca)
}

// operationExpr handles unary and binary operations.
func (b *builder) operationExpr(e *syntax.Operation) *Value {
	if e.Y == nil {
		return b.unaryExpr(e)
	}
	return b.binaryExpr(e)
}

// unaryExpr handles unary operations.
func (b *builder) unaryExpr(e *syntax.Operation) *Value {
	switch e.Op {
	case syntax.Not: // !
		x := b.expr(e.X)
		return b.fn.NewValue(b.b, OpNot, types.Typ[types.Bool], x)

	case syntax.Sub: // - (negate)
		x := b.expr(e.X)
		xTyp := b.exprType(e.X)
		if isFloat(xTyp) {
			return b.fn.NewValue(b.b, OpNegF64, xTyp, x)
		}
		return b.fn.NewValue(b.b, OpNeg64, xTyp, x)

	case syntax.And: // & (address-of)
		ptr := b.addr(e.X)
		return ptr

	case syntax.Mul: // * (dereference)
		x := b.expr(e.X)
		// Determine the element type.
		xTyp := b.exprType(e.X)
		elemTyp := derefType(xTyp)
		return b.fn.NewValue(b.b, OpLoad, elemTyp, x)

	default:
		panic(fmt.Sprintf("ssa.unaryExpr: unhandled unary op %s", e.Op))
	}
}

// binaryExpr handles binary operations.
func (b *builder) binaryExpr(e *syntax.Operation) *Value {
	// Short-circuit logical operators.
	if e.Op.IsLogical() {
		return b.shortCircuit(e)
	}

	x := b.expr(e.X)
	y := b.expr(e.Y)

	xTyp := b.exprType(e.X)
	resTyp := b.exprType(e)

	op := b.binOp(e.Op, xTyp)
	return b.fn.NewValue(b.b, op, resTyp, x, y)
}

// shortCircuit implements short-circuit evaluation for && and ||.
func (b *builder) shortCircuit(e *syntax.Operation) *Value {
	left := b.expr(e.X)

	bRight := b.fn.NewBlock(BlockPlain)
	bShort := b.fn.NewBlock(BlockPlain)
	bMerge := b.fn.NewBlock(BlockPlain)

	b.b.Kind = BlockIf
	b.b.SetControl(left)

	isAnd := e.Op.String() == "&&"
	if isAnd {
		// && : if true, eval right; if false, short-circuit to false
		b.b.AddSucc(bRight) // true  → eval right
		b.b.AddSucc(bShort) // false → short-circuit
	} else {
		// || : if true, short-circuit to true; if false, eval right
		b.b.AddSucc(bShort) // true  → short-circuit
		b.b.AddSucc(bRight) // false → eval right
	}

	// Short-circuit block: produce the constant.
	b.b = bShort
	var shortVal *Value
	if isAnd {
		// && short-circuits to false
		shortVal = b.fn.NewValue(bShort, OpConstBool, types.Typ[types.Bool])
		shortVal.AuxInt = 0
	} else {
		// || short-circuits to true
		shortVal = b.fn.NewValue(bShort, OpConstBool, types.Typ[types.Bool])
		shortVal.AuxInt = 1
	}
	bShort.AddSucc(bMerge)

	// Right block: evaluate right operand.
	b.b = bRight
	right := b.expr(e.Y)
	// Capture the current block (may have changed due to nested short-circuit).
	bRightEnd := b.b
	bRightEnd.AddSucc(bMerge)

	// Merge block: phi.
	b.b = bMerge
	phi := b.fn.NewValue(bMerge, OpPhi, types.Typ[types.Bool], shortVal, right)

	return phi
}

// binOp maps a syntax token + operand type to an SSA Op.
func (b *builder) binOp(tok syntax.Token, opType types.Type) Op {
	if isFloat(opType) {
		return floatBinOp(tok)
	}
	if isPointerOrRef(opType) {
		return ptrBinOp(tok)
	}
	// Integer/bool.
	return intBinOp(tok)
}

func intBinOp(tok syntax.Token) Op {
	switch tok.String() {
	case "+":
		return OpAdd64
	case "-":
		return OpSub64
	case "*":
		return OpMul64
	case "/":
		return OpDiv64
	case "%":
		return OpMod64
	case "==":
		return OpEq64
	case "!=":
		return OpNeq64
	case "<":
		return OpLt64
	case "<=":
		return OpLeq64
	case ">":
		return OpGt64
	case ">=":
		return OpGeq64
	default:
		panic(fmt.Sprintf("ssa.intBinOp: unhandled token %s", tok))
	}
}

func floatBinOp(tok syntax.Token) Op {
	switch tok.String() {
	case "+":
		return OpAddF64
	case "-":
		return OpSubF64
	case "*":
		return OpMulF64
	case "/":
		return OpDivF64
	case "==":
		return OpEqF64
	case "!=":
		return OpNeqF64
	case "<":
		return OpLtF64
	case "<=":
		return OpLeqF64
	case ">":
		return OpGtF64
	case ">=":
		return OpGeqF64
	default:
		panic(fmt.Sprintf("ssa.floatBinOp: unhandled token %s", tok))
	}
}

func ptrBinOp(tok syntax.Token) Op {
	switch tok.String() {
	case "==":
		return OpEqPtr
	case "!=":
		return OpNeqPtr
	default:
		panic(fmt.Sprintf("ssa.ptrBinOp: unhandled token %s", tok))
	}
}

// callExpr handles function calls.
func (b *builder) callExpr(e *syntax.CallExpr) *Value {
	// Check for method call: e.Fun is SelectorExpr.
	if sel, ok := e.Fun.(*syntax.SelectorExpr); ok {
		return b.methodCallExpr(e, sel)
	}

	// Check for builtin.
	if tv, ok := b.info.Types[e.Fun]; ok && tv.IsBuiltin() {
		return b.builtinCall(e)
	}

	// Regular function call.
	return b.regularCall(e)
}

// regularCall handles a direct function call.
func (b *builder) regularCall(e *syntax.CallExpr) *Value {
	funName, ok := e.Fun.(*syntax.Name)
	if !ok {
		panic("ssa.regularCall: non-name function expression")
	}

	obj := b.info.Uses[funName]
	funcObj, ok := obj.(*types.FuncObj)
	if !ok {
		panic(fmt.Sprintf("ssa.regularCall: expected *types.FuncObj, got %T", obj))
	}

	// Evaluate arguments.
	args := make([]*Value, len(e.Args))
	for i, arg := range e.Args {
		args[i] = b.expr(arg)
	}

	// Determine result type.
	sig := funcObj.Signature()
	var resTyp types.Type
	if sig.Result() != nil {
		resTyp = sig.Result()
	}

	v := b.fn.NewValue(b.b, OpStaticCall, resTyp, args...)
	v.Aux = funcObj
	return v
}

// methodCallExpr handles a method call: recv.Method(args...)
func (b *builder) methodCallExpr(e *syntax.CallExpr, sel *syntax.SelectorExpr) *Value {
	// Look up the method.
	methodObj := b.info.Uses[sel.Sel]
	if methodObj == nil {
		panic(fmt.Sprintf("ssa.methodCallExpr: no object for method %s", sel.Sel.Value))
	}
	funcObj, ok := methodObj.(*types.FuncObj)
	if !ok {
		panic(fmt.Sprintf("ssa.methodCallExpr: expected *types.FuncObj, got %T", methodObj))
	}

	sig := funcObj.Signature()

	// Evaluate receiver.
	recv := b.expr(sel.X)

	// Auto-address: if the method has a pointer receiver but we have a value,
	// take the address.
	if sig.Recv() != nil {
		recvParamType := sig.Recv().Type()
		recvExprType := b.exprType(sel.X)
		if isPointerOrRef(recvParamType) && !isPointerOrRef(recvExprType) {
			// Need to take address of receiver.
			ptr := b.addr(sel.X)
			recv = ptr
		}
	}

	// Build args: receiver first, then call args.
	args := make([]*Value, 0, 1+len(e.Args))
	args = append(args, recv)
	for _, arg := range e.Args {
		args = append(args, b.expr(arg))
	}

	var resTyp types.Type
	if sig.Result() != nil {
		resTyp = sig.Result()
	}

	v := b.fn.NewValue(b.b, OpStaticCall, resTyp, args...)
	v.Aux = funcObj
	return v
}

// builtinCall handles calls to builtin functions.
func (b *builder) builtinCall(e *syntax.CallExpr) *Value {
	funName, ok := e.Fun.(*syntax.Name)
	if !ok {
		panic("ssa.builtinCall: non-name builtin")
	}

	obj := b.info.Uses[funName]
	bi, ok := obj.(*types.Builtin)
	if !ok {
		panic(fmt.Sprintf("ssa.builtinCall: expected *types.Builtin, got %T", obj))
	}

	switch bi.Kind() {
	case types.BuiltinPrintln:
		args := make([]*Value, len(e.Args))
		for i, arg := range e.Args {
			args[i] = b.expr(arg)
		}
		b.fn.NewValue(b.b, OpPrintln, nil, args...)
		// println returns void; return a dummy value (not used).
		return nil

	case types.BuiltinNew:
		// new(T) → OpNewAlloc
		tv := b.info.Types[e]
		resTyp := tv.Type
		v := b.fn.NewValue(b.b, OpNewAlloc, resTyp)
		// Store the element type in Aux.
		if refTyp, ok := resTyp.Underlying().(*types.Ref); ok {
			v.Aux = refTyp.Elem()
		}
		return v

	case types.BuiltinPanic:
		var arg *Value
		if len(e.Args) > 0 {
			arg = b.expr(e.Args[0])
		}
		if arg != nil {
			b.fn.NewValue(b.b, OpPanic, nil, arg)
		} else {
			b.fn.NewValue(b.b, OpPanic, nil)
		}
		b.b.Kind = BlockExit
		b.b = nil
		return nil

	default:
		panic(fmt.Sprintf("ssa.builtinCall: unhandled builtin %s", bi.Name()))
	}
}

// selectorExpr handles field access: x.field
func (b *builder) selectorExpr(e *syntax.SelectorExpr) *Value {
	// Determine the struct type and field index.
	xTyp := b.exprType(e.X)
	st, fieldIdx := b.resolveField(xTyp, e.Sel.Value)
	if st == nil {
		panic(fmt.Sprintf("ssa.selectorExpr: cannot find field %s", e.Sel.Value))
	}

	fieldType := st.Field(fieldIdx).Type()

	var basePtr *Value
	if isPointerOrRef(xTyp) {
		// X is a pointer/ref — evaluate it as a pointer.
		basePtr = b.expr(e.X)
	} else {
		// X is a struct value — take its address.
		basePtr = b.addr(e.X)
	}

	fieldPtr := b.fn.NewValue(b.b, OpStructFieldPtr, types.NewPointer(fieldType), basePtr)
	fieldPtr.AuxInt = int64(fieldIdx)

	return b.fn.NewValue(b.b, OpLoad, fieldType, fieldPtr)
}

// indexExpr handles array index: x[i]
func (b *builder) indexExpr(e *syntax.IndexExpr) *Value {
	xTyp := b.exprType(e.X)
	var elemType types.Type
	var basePtr *Value

	switch t := xTyp.Underlying().(type) {
	case *types.Array:
		elemType = t.Elem()
		basePtr = b.addr(e.X)
	case *types.Pointer:
		if arr, ok := t.Elem().Underlying().(*types.Array); ok {
			elemType = arr.Elem()
			basePtr = b.expr(e.X)
		} else {
			panic("ssa.indexExpr: pointer to non-array")
		}
	case *types.Ref:
		if arr, ok := t.Elem().Underlying().(*types.Array); ok {
			elemType = arr.Elem()
			basePtr = b.expr(e.X)
		} else {
			panic("ssa.indexExpr: ref to non-array")
		}
	default:
		panic(fmt.Sprintf("ssa.indexExpr: cannot index %s", xTyp))
	}

	idx := b.expr(e.Index)
	elemPtr := b.fn.NewValue(b.b, OpArrayIndexPtr, types.NewPointer(elemType), basePtr, idx)
	return b.fn.NewValue(b.b, OpLoad, elemType, elemPtr)
}

// compositeLitExpr handles struct literals: T{f: v, ...}
func (b *builder) compositeLitExpr(e *syntax.CompositeLit) *Value {
	tv := b.info.Types[e]
	litTyp := tv.Type

	// Allocate space.
	alloca := b.entryAlloca(litTyp, "")
	size := b.sizes.Sizeof(litTyp)
	zero := b.fn.NewValue(b.b, OpZero, nil, alloca)
	zero.AuxInt = size

	// Get the underlying struct.
	st, ok := litTyp.Underlying().(*types.Struct)
	if !ok {
		panic(fmt.Sprintf("ssa.compositeLitExpr: non-struct type %s", litTyp))
	}

	// Initialize fields.
	for i, elem := range e.Elems {
		var fieldIdx int
		var val *Value

		switch kv := elem.(type) {
		case *syntax.KeyValueExpr:
			// Keyed: find field by name.
			keyName, ok := kv.Key.(*syntax.Name)
			if !ok {
				panic("ssa.compositeLitExpr: non-name key")
			}
			for j := 0; j < st.NumFields(); j++ {
				if st.Field(j).Name() == keyName.Value {
					fieldIdx = j
					break
				}
			}
			val = b.expr(kv.Value)
		default:
			// Positional.
			fieldIdx = i
			val = b.expr(elem)
		}

		fieldType := st.Field(fieldIdx).Type()
		fieldPtr := b.fn.NewValue(b.b, OpStructFieldPtr, types.NewPointer(fieldType), alloca)
		fieldPtr.AuxInt = int64(fieldIdx)
		b.fn.NewValue(b.b, OpStore, nil, fieldPtr, val)
	}

	// Load the whole struct.
	return b.fn.NewValue(b.b, OpLoad, litTyp, alloca)
}

// newExpr handles new(T) — delegates to builtinCall path.
// This handles the case where new(T) is represented as a NewExpr AST node
// (parsed as keyword, not as a call).
func (b *builder) newExpr(e *syntax.NewExpr) *Value {
	tv := b.info.Types[e]
	resTyp := tv.Type
	v := b.fn.NewValue(b.b, OpNewAlloc, resTyp)
	if refTyp, ok := resTyp.Underlying().(*types.Ref); ok {
		v.Aux = refTyp.Elem()
	}
	return v
}

// addr computes the address of an expression (for assignment LHS or address-of).
// Returns a *Value that is a pointer to the storage location.
func (b *builder) addr(e syntax.Expr) *Value {
	switch e := e.(type) {
	case *syntax.Name:
		obj := b.info.Uses[e]
		if obj == nil {
			obj = b.info.Defs[e]
		}
		if obj == nil {
			panic(fmt.Sprintf("ssa.addr: no object for %q", e.Value))
		}
		alloca, ok := b.vars[obj]
		if !ok {
			panic(fmt.Sprintf("ssa.addr: no alloca for %q", e.Value))
		}
		return alloca

	case *syntax.SelectorExpr:
		// Field address: &x.field
		xTyp := b.exprType(e.X)
		st, fieldIdx := b.resolveField(xTyp, e.Sel.Value)
		if st == nil {
			panic(fmt.Sprintf("ssa.addr: cannot find field %s", e.Sel.Value))
		}
		fieldType := st.Field(fieldIdx).Type()

		var basePtr *Value
		if isPointerOrRef(xTyp) {
			basePtr = b.expr(e.X)
		} else {
			basePtr = b.addr(e.X)
		}

		fieldPtr := b.fn.NewValue(b.b, OpStructFieldPtr, types.NewPointer(fieldType), basePtr)
		fieldPtr.AuxInt = int64(fieldIdx)
		return fieldPtr

	case *syntax.IndexExpr:
		// Array element address: &x[i]
		xTyp := b.exprType(e.X)
		var elemType types.Type
		var basePtr *Value

		switch t := xTyp.Underlying().(type) {
		case *types.Array:
			elemType = t.Elem()
			basePtr = b.addr(e.X)
		case *types.Pointer:
			if arr, ok := t.Elem().Underlying().(*types.Array); ok {
				elemType = arr.Elem()
				basePtr = b.expr(e.X)
			}
		case *types.Ref:
			if arr, ok := t.Elem().Underlying().(*types.Array); ok {
				elemType = arr.Elem()
				basePtr = b.expr(e.X)
			}
		}

		idx := b.expr(e.Index)
		return b.fn.NewValue(b.b, OpArrayIndexPtr, types.NewPointer(elemType), basePtr, idx)

	case *syntax.Operation:
		// Dereference on LHS: *p = val → store to p
		if e.Op == syntax.Mul && e.Y == nil {
			return b.expr(e.X)
		}
		panic(fmt.Sprintf("ssa.addr: cannot take address of operation %s", e.Op))

	case *syntax.ParenExpr:
		return b.addr(e.X)

	default:
		panic(fmt.Sprintf("ssa.addr: cannot take address of %T", e))
	}
}

// exprType returns the concrete type of an expression.
func (b *builder) exprType(e syntax.Expr) types.Type {
	tv, ok := b.info.Types[e]
	if !ok {
		panic(fmt.Sprintf("ssa.exprType: no type info for %T", e))
	}
	typ := tv.Type
	if types.IsUntypedType(typ) {
		typ = types.DefaultType(typ)
	}
	return typ
}

// resolveField finds a struct field by name, traversing through pointers/refs/named types.
// Returns the struct type and field index.
func (b *builder) resolveField(typ types.Type, name string) (*types.Struct, int) {
	// Dereference pointers and refs.
	t := typ.Underlying()
	switch pt := t.(type) {
	case *types.Pointer:
		t = pt.Elem().Underlying()
	case *types.Ref:
		t = pt.Elem().Underlying()
	}

	st, ok := t.(*types.Struct)
	if !ok {
		return nil, -1
	}

	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i).Name() == name {
			return st, i
		}
	}
	return nil, -1
}

// Helper type predicates.

func isFloat(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Float || b.Kind() == types.UntypedFloat)
}

func isPointerOrRef(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Ref:
		return true
	}
	return false
}

// derefType returns the element type of a pointer or ref type.
func derefType(t types.Type) types.Type {
	switch pt := t.Underlying().(type) {
	case *types.Pointer:
		return pt.Elem()
	case *types.Ref:
		return pt.Elem()
	}
	panic(fmt.Sprintf("ssa.derefType: not a pointer type: %s", t))
}


package codegen

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/you-not-fish/yoru/internal/rtabi"
	"github.com/you-not-fish/yoru/internal/ssa"
	"github.com/you-not-fish/yoru/internal/types"
)

// lowerFunc emits the LLVM IR for a single SSA function.
func (g *generator) lowerFunc(fn *ssa.Func) {
	retType := "void"
	if fn.Sig != nil {
		retType = llvmReturnType(fn.Sig)
	}

	// Build parameter list.
	var params []string
	if fn.Sig != nil {
		if fn.Sig.Recv() != nil {
			params = append(params, llvmType(fn.Sig.Recv().Type())+" %recv")
		}
		for i := 0; i < fn.Sig.NumParams(); i++ {
			p := fn.Sig.Param(i)
			params = append(params, fmt.Sprintf("%s %%%s", llvmType(p.Type()), p.Name()))
		}
	}

	funcName := fn.Name
	if funcName == "main" {
		funcName = rtabi.YoruMain
	}

	g.e.emit("define %s @%s(%s) {", retType, funcName, strings.Join(params, ", "))

	for _, b := range fn.Blocks {
		g.lowerBlock(b, fn)
	}

	g.e.emit("}")
}

// lowerBlock emits the LLVM IR for a single basic block.
func (g *generator) lowerBlock(b *ssa.Block, fn *ssa.Func) {
	g.e.emitLabel(b)

	for _, v := range b.Values {
		g.lowerValue(v)
	}

	g.lowerTerminator(b)
}

// lowerValue emits the LLVM IR for a single SSA value.
func (g *generator) lowerValue(v *ssa.Value) {
	switch v.Op {
	// Simple constants are inlined at use sites — no instruction emitted.
	case ssa.OpConst64, ssa.OpConstFloat, ssa.OpConstBool, ssa.OpConstNil:
		return

	// String constants need to build a {ptr, i64} struct.
	case ssa.OpConstString:
		g.lowerConstString(v)
		return

	// Integer arithmetic
	case ssa.OpAdd64:
		g.emitBinOp("add", "i64", v)
	case ssa.OpSub64:
		g.emitBinOp("sub", "i64", v)
	case ssa.OpMul64:
		g.emitBinOp("mul", "i64", v)
	case ssa.OpDiv64:
		g.emitBinOp("sdiv", "i64", v)
	case ssa.OpMod64:
		g.emitBinOp("srem", "i64", v)
	case ssa.OpNeg64:
		g.e.emitInst("%s = sub i64 0, %s", valueName(v), g.operand(v.Args[0]))

	// Float arithmetic
	case ssa.OpAddF64:
		g.emitBinOp("fadd", "double", v)
	case ssa.OpSubF64:
		g.emitBinOp("fsub", "double", v)
	case ssa.OpMulF64:
		g.emitBinOp("fmul", "double", v)
	case ssa.OpDivF64:
		g.emitBinOp("fdiv", "double", v)
	case ssa.OpNegF64:
		g.e.emitInst("%s = fneg double %s", valueName(v), g.operand(v.Args[0]))

	// Integer comparison
	case ssa.OpEq64:
		g.emitICmp("eq", v)
	case ssa.OpNeq64:
		g.emitICmp("ne", v)
	case ssa.OpLt64:
		g.emitICmp("slt", v)
	case ssa.OpLeq64:
		g.emitICmp("sle", v)
	case ssa.OpGt64:
		g.emitICmp("sgt", v)
	case ssa.OpGeq64:
		g.emitICmp("sge", v)

	// Float comparison
	case ssa.OpEqF64:
		g.emitFCmp("oeq", v)
	case ssa.OpNeqF64:
		g.emitFCmp("une", v)
	case ssa.OpLtF64:
		g.emitFCmp("olt", v)
	case ssa.OpLeqF64:
		g.emitFCmp("ole", v)
	case ssa.OpGtF64:
		g.emitFCmp("ogt", v)
	case ssa.OpGeqF64:
		g.emitFCmp("oge", v)

	// Pointer comparison
	case ssa.OpEqPtr:
		g.e.emitInst("%s = icmp eq ptr %s, %s", valueName(v), g.operand(v.Args[0]), g.operand(v.Args[1]))
	case ssa.OpNeqPtr:
		g.e.emitInst("%s = icmp ne ptr %s, %s", valueName(v), g.operand(v.Args[0]), g.operand(v.Args[1]))

	// Boolean
	case ssa.OpNot:
		g.e.emitInst("%s = xor i1 %s, true", valueName(v), g.operand(v.Args[0]))
	case ssa.OpAndBool:
		g.e.emitInst("%s = and i1 %s, %s", valueName(v), g.operand(v.Args[0]), g.operand(v.Args[1]))
	case ssa.OpOrBool:
		g.e.emitInst("%s = or i1 %s, %s", valueName(v), g.operand(v.Args[0]), g.operand(v.Args[1]))

	// Conversion
	case ssa.OpIntToFloat:
		g.e.emitInst("%s = sitofp i64 %s to double", valueName(v), g.operand(v.Args[0]))
	case ssa.OpFloatToInt:
		g.e.emitInst("%s = fptosi double %s to i64", valueName(v), g.operand(v.Args[0]))

	// Memory
	case ssa.OpAlloca:
		elemType := allocaElemType(v)
		g.e.emitInst("%s = alloca %s", valueName(v), elemType)
	case ssa.OpLoad:
		lt := llvmType(v.Type)
		g.e.emitInst("%s = load %s, ptr %s", valueName(v), lt, g.operand(v.Args[0]))
	case ssa.OpStore:
		storeType := llvmType(v.Args[1].Type)
		g.e.emitInst("store %s %s, ptr %s", storeType, g.operand(v.Args[1]), g.operand(v.Args[0]))
	case ssa.OpZero:
		size := v.AuxInt
		g.e.emitInst("call void @llvm.memset.p0.i64(ptr %s, i8 0, i64 %d, i1 false)", g.operand(v.Args[0]), size)

	// Struct/Array access
	case ssa.OpStructFieldPtr:
		basePtr := g.operand(v.Args[0])
		fieldIdx := v.AuxInt
		// Determine the struct type from the pointer's element type.
		structType := structTypeFromPtr(v.Args[0])
		g.e.emitInst("%s = getelementptr %s, ptr %s, i32 0, i32 %d", valueName(v), structType, basePtr, fieldIdx)
	case ssa.OpArrayIndexPtr:
		basePtr := g.operand(v.Args[0])
		idx := g.operand(v.Args[1])
		arrayType := arrayTypeFromPtr(v.Args[0])
		g.e.emitInst("%s = getelementptr %s, ptr %s, i64 0, i64 %s", valueName(v), arrayType, basePtr, idx)

	// Address
	case ssa.OpAddr:
		// OpAddr produces the same pointer as its alloca argument.
		// This is handled by operand() returning the alloca name.
		// But if someone takes the result as a value, we need a copy.
		g.e.emitInst("%s = bitcast ptr %s to ptr", valueName(v), g.operand(v.Args[0]))

	// SSA
	case ssa.OpPhi:
		g.lowerPhi(v)
	case ssa.OpCopy:
		// Copy: just emit as an alias. Use a bitcast as identity for the type.
		lt := llvmType(v.Type)
		if lt == "ptr" {
			g.e.emitInst("%s = bitcast ptr %s to ptr", valueName(v), g.operand(v.Args[0]))
		} else {
			g.e.emitInst("%s = add %s %s, 0", valueName(v), lt, g.operand(v.Args[0]))
		}
	case ssa.OpArg:
		// Args are accessed via parameter names directly; operand() handles this.
		return

	// Builtins
	case ssa.OpPrintln:
		g.lowerPrintln(v)
	case ssa.OpPanic:
		g.lowerPanic(v)

	// Heap allocation
	case ssa.OpNewAlloc:
		g.lowerNewAlloc(v)

	// Nil check
	case ssa.OpNilCheck:
		g.lowerNilCheck(v)

	// String operations
	case ssa.OpStringLen:
		// Extract length from {ptr, i64} string.
		g.e.emitInst("%s = extractvalue { ptr, i64 } %s, 1", valueName(v), g.operand(v.Args[0]))
	case ssa.OpStringPtr:
		g.e.emitInst("%s = extractvalue { ptr, i64 } %s, 0", valueName(v), g.operand(v.Args[0]))

	// Calls
	case ssa.OpStaticCall:
		g.lowerStaticCall(v)
	case ssa.OpCall:
		g.lowerIndirectCall(v)

	default:
		g.e.emitInst("; TODO: unhandled op %s", v.Op)
	}
}

// lowerTerminator emits the block terminator instruction.
func (g *generator) lowerTerminator(b *ssa.Block) {
	switch b.Kind {
	case ssa.BlockPlain:
		if len(b.Succs) > 0 {
			g.e.emitInst("br label %%%s", blockName(b.Succs[0]))
		} else {
			g.e.emitInst("unreachable")
		}
	case ssa.BlockIf:
		cond := g.operand(b.Controls[0])
		g.e.emitInst("br i1 %s, label %%%s, label %%%s",
			cond, blockName(b.Succs[0]), blockName(b.Succs[1]))
	case ssa.BlockReturn:
		if len(b.Controls) > 0 && b.Controls[0] != nil {
			retVal := b.Controls[0]
			retType := llvmType(retVal.Type)
			g.e.emitInst("ret %s %s", retType, g.operand(retVal))
		} else {
			g.e.emitInst("ret void")
		}
	case ssa.BlockExit:
		g.e.emitInst("unreachable")
	default:
		g.e.emitInst("; unknown block kind")
		g.e.emitInst("unreachable")
	}
}

// operand returns the LLVM IR operand string for an SSA value.
// Constants are inlined, others use their %vN name.
func (g *generator) operand(v *ssa.Value) string {
	switch v.Op {
	case ssa.OpConst64:
		return strconv.FormatInt(v.AuxInt, 10)
	case ssa.OpConstFloat:
		return formatFloat(v.AuxFloat)
	case ssa.OpConstBool:
		if v.AuxInt != 0 {
			return "true"
		}
		return "false"
	case ssa.OpConstString:
		// The {ptr, i64} was built by lowerConstString and named %vN.
		return valueName(v)
	case ssa.OpConstNil:
		return "null"
	case ssa.OpArg:
		// Return the parameter name.
		name, ok := v.Aux.(string)
		if ok && name != "" {
			if v.AuxInt == -1 {
				return "%recv"
			}
			return "%" + name
		}
		return fmt.Sprintf("%%arg%d", v.AuxInt)
	}
	return valueName(v)
}

// emitBinOp emits a binary operation instruction.
func (g *generator) emitBinOp(inst, ty string, v *ssa.Value) {
	g.e.emitInst("%s = %s %s %s, %s", valueName(v), inst, ty, g.operand(v.Args[0]), g.operand(v.Args[1]))
}

// emitICmp emits an integer comparison.
func (g *generator) emitICmp(cond string, v *ssa.Value) {
	g.e.emitInst("%s = icmp %s i64 %s, %s", valueName(v), cond, g.operand(v.Args[0]), g.operand(v.Args[1]))
}

// emitFCmp emits a floating-point comparison.
func (g *generator) emitFCmp(cond string, v *ssa.Value) {
	g.e.emitInst("%s = fcmp %s double %s, %s", valueName(v), cond, g.operand(v.Args[0]), g.operand(v.Args[1]))
}

// lowerPhi emits a phi node.
func (g *generator) lowerPhi(v *ssa.Value) {
	lt := llvmType(v.Type)
	parts := make([]string, len(v.Args))
	for i, arg := range v.Args {
		pred := v.Block.Preds[i]
		parts[i] = fmt.Sprintf("[ %s, %%%s ]", g.operand(arg), blockName(pred))
	}
	g.e.emitInst("%s = phi %s %s", valueName(v), lt, strings.Join(parts, ", "))
}

// lowerConstString builds a {ptr, i64} struct value from a string global.
func (g *generator) lowerConstString(v *ssa.Value) {
	s := v.Aux.(string)
	idx := g.stringIndex(s)
	strGlobal := fmt.Sprintf("@.str.%d", idx)
	t0 := g.e.nextTmp()
	g.e.emitInst("%s = insertvalue { ptr, i64 } undef, ptr %s, 0", t0, strGlobal)
	g.e.emitInst("%s = insertvalue { ptr, i64 } %s, i64 %d, 1", valueName(v), t0, len(s))
}

// lowerPrintln emits the sequence of runtime calls for println.
func (g *generator) lowerPrintln(v *ssa.Value) {
	for i, arg := range v.Args {
		// Space separator between args.
		if i > 0 {
			g.emitPrintSpace()
		}
		g.emitPrintArg(arg)
	}
	// Trailing newline.
	g.e.emitInst("call void @%s()", rtabi.FnPrintln)
}

// emitPrintArg emits a single runtime print call for one argument.
func (g *generator) emitPrintArg(arg *ssa.Value) {
	t := arg.Type
	if t == nil {
		return
	}

	switch {
	case isIntType(t):
		g.e.emitInst("call void @%s(i64 %s)", rtabi.FnPrintI64, g.operand(arg))
	case isFloatType(t):
		g.e.emitInst("call void @%s(double %s)", rtabi.FnPrintF64, g.operand(arg))
	case isBoolType(t):
		// Bool is i1 in SSA, but rt_print_bool expects i8.
		tmp := g.e.nextTmp()
		g.e.emitInst("%s = zext i1 %s to i8", tmp, g.operand(arg))
		g.e.emitInst("call void @%s(i8 %s)", rtabi.FnPrintBool, tmp)
	case isStringType(t):
		g.emitPrintString(arg)
	default:
		g.e.emitInst("; TODO: print type %s", t)
	}
}

// emitPrintString emits a print call for a string argument.
// The value is already a {ptr, i64} (built by lowerConstString or other means).
func (g *generator) emitPrintString(arg *ssa.Value) {
	g.e.emitInst("call void @%s({ ptr, i64 } %s)", rtabi.FnPrintString, g.operand(arg))
}

// emitPrintSpace emits a call to print a single space.
func (g *generator) emitPrintSpace() {
	idx := g.stringIndex(" ")
	strGlobal := fmt.Sprintf("@.str.%d", idx)
	t0 := g.e.nextTmp()
	t1 := g.e.nextTmp()
	g.e.emitInst("%s = insertvalue { ptr, i64 } undef, ptr %s, 0", t0, strGlobal)
	g.e.emitInst("%s = insertvalue { ptr, i64 } %s, i64 1, 1", t1, t0)
	g.e.emitInst("call void @%s({ ptr, i64 } %s)", rtabi.FnPrintString, t1)
}

// lowerPanic emits the runtime panic call.
func (g *generator) lowerPanic(v *ssa.Value) {
	if len(v.Args) > 0 {
		arg := v.Args[0]
		if isStringType(arg.Type) {
			// String values are already {ptr, i64}.
			g.e.emitInst("call void @%s({ ptr, i64 } %s)", rtabi.FnPanicString, g.operand(arg))
		} else {
			g.e.emitInst("call void @%s(ptr %s)", rtabi.FnPanic, g.operand(arg))
		}
	} else {
		// Panic with no argument: build a "panic" string.
		idx := g.stringIndex("panic")
		strGlobal := fmt.Sprintf("@.str.%d", idx)
		t0 := g.e.nextTmp()
		t1 := g.e.nextTmp()
		g.e.emitInst("%s = insertvalue { ptr, i64 } undef, ptr %s, 0", t0, strGlobal)
		g.e.emitInst("%s = insertvalue { ptr, i64 } %s, i64 5, 1", t1, t0)
		g.e.emitInst("call void @%s({ ptr, i64 } %s)", rtabi.FnPanicString, t1)
	}
}

// lowerStaticCall emits a direct function call.
func (g *generator) lowerStaticCall(v *ssa.Value) {
	funcObj := v.Aux.(*types.FuncObj)
	sig := funcObj.Signature()
	retType := llvmReturnType(sig)

	calleeName := funcObj.Name()
	if calleeName == "main" {
		calleeName = rtabi.YoruMain
	}

	// Build argument list.
	var argStrs []string
	argIdx := 0
	if sig.Recv() != nil {
		argStrs = append(argStrs, fmt.Sprintf("%s %s", llvmType(sig.Recv().Type()), g.operand(v.Args[argIdx])))
		argIdx++
	}
	for i := 0; i < sig.NumParams(); i++ {
		p := sig.Param(i)
		argStrs = append(argStrs, fmt.Sprintf("%s %s", llvmType(p.Type()), g.operand(v.Args[argIdx])))
		argIdx++
	}

	if retType == "void" {
		g.e.emitInst("call void @%s(%s)", calleeName, strings.Join(argStrs, ", "))
	} else {
		g.e.emitInst("%s = call %s @%s(%s)", valueName(v), retType, calleeName, strings.Join(argStrs, ", "))
	}
}

// lowerIndirectCall emits an indirect function call.
func (g *generator) lowerIndirectCall(v *ssa.Value) {
	g.e.emitInst("; TODO: indirect call")
	g.e.emitInst("unreachable")
}

// lowerNewAlloc emits a heap allocation via rt_alloc.
func (g *generator) lowerNewAlloc(v *ssa.Value) {
	// v.Aux contains the element type for new(T).
	elemType, ok := v.Aux.(types.Type)
	if !ok {
		g.e.emitInst("; ERROR: OpNewAlloc without element type")
		return
	}
	size := g.sizes.Sizeof(elemType)
	// For now, pass null as the type descriptor (Phase 5C will generate proper TypeDescs).
	g.e.emitInst("%s = call ptr @%s(i64 %d, ptr null)", valueName(v), rtabi.FnAlloc, size)
}

// lowerNilCheck emits a nil check with panic.
func (g *generator) lowerNilCheck(v *ssa.Value) {
	cmp := g.e.nextTmp()
	g.e.emitInst("%s = icmp eq ptr %s, null", cmp, g.operand(v.Args[0]))
	thenLabel := fmt.Sprintf("nilchk.fail.%d", v.ID)
	contLabel := fmt.Sprintf("nilchk.ok.%d", v.ID)
	g.e.emitInst("br i1 %s, label %%%s, label %%%s", cmp, thenLabel, contLabel)
	g.e.emit("%s:", thenLabel)
	idx := g.stringIndex("nil pointer dereference")
	strGlobal := fmt.Sprintf("@.str.%d", idx)
	t0 := g.e.nextTmp()
	t1 := g.e.nextTmp()
	g.e.emitInst("%s = insertvalue { ptr, i64 } undef, ptr %s, 0", t0, strGlobal)
	g.e.emitInst("%s = insertvalue { ptr, i64 } %s, i64 %d, 1", t1, t0, len("nil pointer dereference"))
	g.e.emitInst("call void @%s({ ptr, i64 } %s)", rtabi.FnPanicString, t1)
	g.e.emitInst("unreachable")
	g.e.emit("%s:", contLabel)
}

// allocaElemType returns the LLVM element type for an alloca instruction.
// The alloca value has type *T, so we extract T.
func allocaElemType(v *ssa.Value) string {
	if v.Type == nil {
		return "i8"
	}
	switch pt := v.Type.Underlying().(type) {
	case *types.Pointer:
		return llvmType(pt.Elem())
	case *types.Ref:
		return llvmType(pt.Elem())
	}
	return "i8"
}

// structTypeFromPtr extracts the LLVM struct type from a pointer-to-struct value.
func structTypeFromPtr(v *ssa.Value) string {
	if v.Type == nil {
		return "i8"
	}
	switch pt := v.Type.Underlying().(type) {
	case *types.Pointer:
		return llvmType(pt.Elem())
	case *types.Ref:
		return llvmType(pt.Elem())
	}
	return "i8"
}

// arrayTypeFromPtr extracts the LLVM array type from a pointer-to-array value.
func arrayTypeFromPtr(v *ssa.Value) string {
	return structTypeFromPtr(v) // same logic
}

// formatFloat formats a float64 as an LLVM IR floating-point literal.
// LLVM requires hex representation for non-finite and non-simple values.
func formatFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "0x7FF0000000000000"
	}
	if math.IsInf(f, -1) {
		return "0xFFF0000000000000"
	}
	if math.IsNaN(f) {
		return "0x7FF8000000000000"
	}
	// Use hex encoding for exact representation.
	bits := math.Float64bits(f)
	return fmt.Sprintf("0x%016X", bits)
}

// stringIndex returns the index of a string in the global string table,
// adding it if not present.
func (g *generator) stringIndex(s string) int {
	if idx, ok := g.stringMap[s]; ok {
		return idx
	}
	idx := len(g.strings)
	g.strings = append(g.strings, s)
	g.stringMap[s] = idx
	return idx
}

// llvmEscapeString returns an LLVM IR escaped string literal.
// Non-printable characters and backslash are escaped as \HH.
func llvmEscapeString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' || c == '"' || c < 0x20 || c >= 0x7f {
			fmt.Fprintf(&b, "\\%02X", c)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

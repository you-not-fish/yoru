// Package ssa implements the SSA (Static Single Assignment) intermediate
// representation for the Yoru compiler.
package ssa

// Op represents an SSA operation code.
type Op int

const (
	OpInvalid Op = iota

	// Constants
	OpConst64     // integer constant; AuxInt = value
	OpConstFloat  // float constant; AuxFloat = value
	OpConstBool   // bool constant; AuxInt = 0 or 1
	OpConstString // string constant; Aux = string value
	OpConstNil    // nil constant

	// Integer arithmetic
	OpAdd64 // int + int
	OpSub64 // int - int
	OpMul64 // int * int
	OpDiv64 // int / int
	OpMod64 // int % int
	OpNeg64 // -int (unary)

	// Float arithmetic
	OpAddF64 // float + float
	OpSubF64 // float - float
	OpMulF64 // float * float
	OpDivF64 // float / float
	OpNegF64 // -float (unary)

	// Integer comparison
	OpEq64  // int == int
	OpNeq64 // int != int
	OpLt64  // int < int
	OpLeq64 // int <= int
	OpGt64  // int > int
	OpGeq64 // int >= int

	// Float comparison
	OpEqF64  // float == float
	OpNeqF64 // float != float
	OpLtF64  // float < float
	OpLeqF64 // float <= float
	OpGtF64  // float > float
	OpGeqF64 // float >= float

	// Pointer comparison
	OpEqPtr  // ptr == ptr (or ref == ref)
	OpNeqPtr // ptr != ptr (or ref != ref)

	// Boolean
	OpNot     // !bool
	OpAndBool // bool && bool (already short-circuit lowered)
	OpOrBool  // bool || bool (already short-circuit lowered)

	// Memory
	OpAlloca // stack allocation; Type = *T; Aux = optional name
	OpLoad   // load from pointer; Args[0] = ptr
	OpStore  // store to pointer; Args[0] = ptr, Args[1] = val; void
	OpZero   // zero-fill memory; Args[0] = ptr; AuxInt = size; void

	// Struct/Array access
	OpStructFieldPtr // &s.field; Args[0] = struct ptr; AuxInt = field index
	OpArrayIndexPtr  // &a[i]; Args[0] = array ptr, Args[1] = index

	// Conversion
	OpIntToFloat // int → float
	OpFloatToInt // float → int

	// Calls
	OpStaticCall // direct function call; Aux = *types.FuncObj; Args = arguments
	OpCall       // indirect call; Args[0] = func ptr, Args[1:] = arguments

	// Heap allocation
	OpNewAlloc // new(T) → ref T; calls rt_alloc; Aux = TypeDesc info

	// SSA-specific
	OpPhi  // φ function; Args = one per predecessor
	OpCopy // value copy (identity)
	OpArg  // function argument; AuxInt = param index; Aux = param name

	// Address
	OpAddr // address of local (&x → ptr to alloca); Args[0] = alloca

	// Builtins
	OpPrintln // println(...); Args = values to print; void
	OpPanic   // panic(msg); Args[0] = string; void

	// Nil check
	OpNilCheck // nil check; Args[0] = pointer; panics if nil

	// String operations
	OpStringLen // string length; Args[0] = string
	OpStringPtr // string data pointer; Args[0] = string

	opCount // sentinel; must be last
)

// OpInfo holds metadata about an SSA operation.
type OpInfo struct {
	Name   string // human-readable name
	IsPure bool   // true if the op has no side effects and can be CSE'd/DCE'd
	IsVoid bool   // true if the op produces no value (Store, Println, etc.)
}

// opInfoTable maps each Op to its OpInfo.
// Index by Op value.
var opInfoTable = [opCount]OpInfo{
	OpInvalid: {Name: "Invalid"},

	// Constants — all pure
	OpConst64:     {Name: "Const64", IsPure: true},
	OpConstFloat:  {Name: "ConstFloat", IsPure: true},
	OpConstBool:   {Name: "ConstBool", IsPure: true},
	OpConstString: {Name: "ConstString", IsPure: true},
	OpConstNil:    {Name: "ConstNil", IsPure: true},

	// Integer arithmetic — all pure
	OpAdd64: {Name: "Add64", IsPure: true},
	OpSub64: {Name: "Sub64", IsPure: true},
	OpMul64: {Name: "Mul64", IsPure: true},
	OpDiv64: {Name: "Div64", IsPure: true},
	OpMod64: {Name: "Mod64", IsPure: true},
	OpNeg64: {Name: "Neg64", IsPure: true},

	// Float arithmetic — all pure
	OpAddF64: {Name: "AddF64", IsPure: true},
	OpSubF64: {Name: "SubF64", IsPure: true},
	OpMulF64: {Name: "MulF64", IsPure: true},
	OpDivF64: {Name: "DivF64", IsPure: true},
	OpNegF64: {Name: "NegF64", IsPure: true},

	// Integer comparison — all pure
	OpEq64:  {Name: "Eq64", IsPure: true},
	OpNeq64: {Name: "Neq64", IsPure: true},
	OpLt64:  {Name: "Lt64", IsPure: true},
	OpLeq64: {Name: "Leq64", IsPure: true},
	OpGt64:  {Name: "Gt64", IsPure: true},
	OpGeq64: {Name: "Geq64", IsPure: true},

	// Float comparison — all pure
	OpEqF64:  {Name: "EqF64", IsPure: true},
	OpNeqF64: {Name: "NeqF64", IsPure: true},
	OpLtF64:  {Name: "LtF64", IsPure: true},
	OpLeqF64: {Name: "LeqF64", IsPure: true},
	OpGtF64:  {Name: "GtF64", IsPure: true},
	OpGeqF64: {Name: "GeqF64", IsPure: true},

	// Pointer comparison — pure
	OpEqPtr:  {Name: "EqPtr", IsPure: true},
	OpNeqPtr: {Name: "NeqPtr", IsPure: true},

	// Boolean — pure
	OpNot:     {Name: "Not", IsPure: true},
	OpAndBool: {Name: "AndBool", IsPure: true},
	OpOrBool:  {Name: "OrBool", IsPure: true},

	// Memory — NOT pure (side effects)
	OpAlloca: {Name: "Alloca"},
	OpLoad:   {Name: "Load"},
	OpStore:  {Name: "Store", IsVoid: true},
	OpZero:   {Name: "Zero", IsVoid: true},

	// Struct/Array — pure (just pointer arithmetic)
	OpStructFieldPtr: {Name: "StructFieldPtr", IsPure: true},
	OpArrayIndexPtr:  {Name: "ArrayIndexPtr", IsPure: true},

	// Conversion — pure
	OpIntToFloat: {Name: "IntToFloat", IsPure: true},
	OpFloatToInt: {Name: "FloatToInt", IsPure: true},

	// Calls — NOT pure (side effects)
	OpStaticCall: {Name: "StaticCall"},
	OpCall:       {Name: "Call"},

	// Heap allocation — NOT pure
	OpNewAlloc: {Name: "NewAlloc"},

	// SSA — Phi and Copy are pure; Arg is pure
	OpPhi:  {Name: "Phi", IsPure: true},
	OpCopy: {Name: "Copy", IsPure: true},
	OpArg:  {Name: "Arg", IsPure: true},

	// Address — pure (just computes pointer)
	OpAddr: {Name: "Addr", IsPure: true},

	// Builtins — NOT pure (side effects)
	OpPrintln: {Name: "Println", IsVoid: true},
	OpPanic:   {Name: "Panic", IsVoid: true},

	// Nil check — NOT pure (may panic)
	OpNilCheck: {Name: "NilCheck"},

	// String — pure
	OpStringLen: {Name: "StringLen", IsPure: true},
	OpStringPtr: {Name: "StringPtr", IsPure: true},
}

// String returns the human-readable name of the op.
func (o Op) String() string {
	if o >= 0 && int(o) < len(opInfoTable) {
		return opInfoTable[o].Name
	}
	return "unknown"
}

// Info returns the OpInfo for this op.
func (o Op) Info() OpInfo {
	if o >= 0 && int(o) < len(opInfoTable) {
		return opInfoTable[o]
	}
	return OpInfo{Name: "unknown"}
}

// IsPure returns true if this op has no side effects.
func (o Op) IsPure() bool {
	if o >= 0 && int(o) < len(opInfoTable) {
		return opInfoTable[o].IsPure
	}
	return false
}

// IsVoid returns true if this op produces no value.
func (o Op) IsVoid() bool {
	if o >= 0 && int(o) < len(opInfoTable) {
		return opInfoTable[o].IsVoid
	}
	return false
}

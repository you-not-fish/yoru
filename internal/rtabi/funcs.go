// Package rtabi defines the ABI constants shared between the compiler and runtime.
package rtabi

// Runtime function names (must match runtime/runtime.h declarations)
const (
	// Initialization
	FnInit     = "rt_init"
	FnShutdown = "rt_shutdown"

	// Memory allocation
	FnAlloc = "rt_alloc"

	// Garbage collection
	FnCollect = "rt_collect"

	// Error handling
	FnPanic       = "rt_panic"
	FnPanicString = "rt_panic_string"

	// I/O functions
	FnPrintI64    = "rt_print_i64"
	FnPrintF64    = "rt_print_f64"
	FnPrintBool   = "rt_print_bool"
	FnPrintString = "rt_print_string"
	FnPrintln     = "rt_println"

	// Bounds checking
	FnBoundsCheck = "rt_bounds_check"

	// Statistics (debug)
	FnGetStats   = "rt_get_stats"
	FnPrintStats = "rt_print_stats"
)

// Runtime type descriptor names
const (
	TypeDescInt    = "rt_type_int"
	TypeDescFloat  = "rt_type_float"
	TypeDescBool   = "rt_type_bool"
	TypeDescString = "rt_type_string"
)

// User program entry point
const (
	// YoruMain is the name of the user's main function.
	YoruMain = "yoru_main"
)

// LLVM intrinsics used by the runtime
const (
	// LLVMGCRoot is the llvm.gcroot intrinsic.
	LLVMGCRoot = "llvm.gcroot"
)

// FuncSignature describes a runtime function's signature for code generation.
type FuncSignature struct {
	Name       string   // Function name
	ReturnType string   // LLVM return type ("void", "ptr", etc.)
	ParamTypes []string // LLVM parameter types
	NoReturn   bool     // Whether function has noreturn attribute
}

// RuntimeFunctions returns the signatures of all runtime functions.
func RuntimeFunctions() []FuncSignature {
	return []FuncSignature{
		// Initialization
		{Name: FnInit, ReturnType: "void", ParamTypes: nil},
		{Name: FnShutdown, ReturnType: "void", ParamTypes: nil},

		// Memory allocation
		{Name: FnAlloc, ReturnType: "ptr", ParamTypes: []string{"i64", "ptr"}},

		// Garbage collection
		{Name: FnCollect, ReturnType: "void", ParamTypes: nil},

		// Error handling
		{Name: FnPanic, ReturnType: "void", ParamTypes: []string{"ptr"}, NoReturn: true},
		{Name: FnPanicString, ReturnType: "void", ParamTypes: []string{LLVMTypeString}, NoReturn: true},

		// I/O functions
		{Name: FnPrintI64, ReturnType: "void", ParamTypes: []string{"i64"}},
		{Name: FnPrintF64, ReturnType: "void", ParamTypes: []string{"double"}},
		{Name: FnPrintBool, ReturnType: "void", ParamTypes: []string{"i8"}},
		{Name: FnPrintString, ReturnType: "void", ParamTypes: []string{LLVMTypeString}},
		{Name: FnPrintln, ReturnType: "void", ParamTypes: nil},

		// Bounds checking
		{Name: FnBoundsCheck, ReturnType: "void", ParamTypes: []string{"i64", "i64"}},
	}
}

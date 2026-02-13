package codegen

import (
	"fmt"

	"github.com/you-not-fish/yoru/internal/rtabi"
	"github.com/you-not-fish/yoru/internal/types"
)

// llvmType maps a Yoru type to its LLVM IR type string.
func llvmType(t types.Type) string {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return llvmBasicType(u)
	case *types.Pointer, *types.Ref:
		return rtabi.LLVMTypePtr
	case *types.Func:
		return rtabi.LLVMTypePtr
	case *types.Array:
		return fmt.Sprintf("[%d x %s]", u.Len(), llvmType(u.Elem()))
	case *types.Struct:
		return llvmStructType(u)
	}
	return "void"
}

// llvmBasicType maps a basic type to LLVM IR.
func llvmBasicType(b *types.Basic) string {
	switch b.Kind() {
	case types.Int, types.UntypedInt:
		return rtabi.LLVMTypeInt
	case types.Float, types.UntypedFloat:
		return rtabi.LLVMTypeFloat
	case types.Bool, types.UntypedBool:
		// In SSA flow, booleans are i1.
		return rtabi.LLVMTypeBoolI1
	case types.String, types.UntypedString:
		return rtabi.LLVMTypeString
	case types.UntypedNil:
		return rtabi.LLVMTypePtr
	}
	return "void"
}

// llvmStructType returns the LLVM struct type literal for a Yoru struct.
func llvmStructType(s *types.Struct) string {
	if s.NumFields() == 0 {
		return "{}"
	}
	result := "{ "
	for i := 0; i < s.NumFields(); i++ {
		if i > 0 {
			result += ", "
		}
		result += llvmType(s.Field(i).Type())
	}
	result += " }"
	return result
}

// llvmReturnType returns the LLVM return type for a function signature.
// Returns "void" if the function has no result.
func llvmReturnType(sig *types.Func) string {
	if sig.Result() == nil {
		return "void"
	}
	return llvmType(sig.Result())
}

// isBoolType returns true if t is a boolean type (used for i1↔i8 conversion).
func isBoolType(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Bool || b.Kind() == types.UntypedBool)
}

// isStringType returns true if t is a string type.
func isStringType(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.String || b.Kind() == types.UntypedString)
}

// isFloatType returns true if t is a float type.
func isFloatType(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Float || b.Kind() == types.UntypedFloat)
}

// isIntType returns true if t is an int type.
func isIntType(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Int || b.Kind() == types.UntypedInt)
}

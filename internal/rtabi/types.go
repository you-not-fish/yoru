// Package rtabi defines the ABI constants shared between the compiler and runtime.
// These values must be kept in sync with runtime/runtime.h.
package rtabi

// Target configuration (Phase 0 locked)
const (
	// TargetTriple is the LLVM target triple for code generation.
	TargetTriple = "arm64-apple-macosx26.0.0"

	// DataLayout is the LLVM data layout string matching the target.
	DataLayout = "e-m:o-i64:64-i128:128-n32:64-S128-Fn32"
)

// Basic type sizes in bytes
const (
	SizeInt    = 8  // int64_t
	SizeFloat  = 8  // double
	SizeBool   = 1  // int8_t (stored), i1 (SSA)
	SizePtr    = 8  // pointer
	SizeString = 16 // { ptr, len }
)

// Basic type alignments in bytes
const (
	AlignInt    = 8
	AlignFloat  = 8
	AlignBool   = 1
	AlignPtr    = 8
	AlignString = 8 // aligned to pointer
)

// Object header layout
const (
	// ObjHeaderSize is the size of the object header (TypeDesc* + next_mark).
	ObjHeaderSize = 16

	// ObjHeaderTypeOffset is the offset of the type field in the header.
	ObjHeaderTypeOffset = 0

	// ObjHeaderNextMarkOffset is the offset of the next_mark field.
	ObjHeaderNextMarkOffset = 8
)

// TypeDesc layout (matches runtime/runtime.h TypeDesc)
const (
	// TypeDescSize is the size of TypeDesc struct.
	TypeDescSize = 24 // size(8) + num_ptrs(8) + offsets*(8)

	// TypeDescSizeOffset is the offset of the size field.
	TypeDescSizeOffset = 0

	// TypeDescNumPtrsOffset is the offset of the num_ptrs field.
	TypeDescNumPtrsOffset = 8

	// TypeDescOffsetsOffset is the offset of the offsets pointer field.
	TypeDescOffsetsOffset = 16
)

// GC constants
const (
	// GCStrategy is the LLVM GC strategy name.
	GCStrategy = "shadow-stack"

	// GCMarkBit is the bit used for GC marking in next_mark.
	GCMarkBit = 0
)

// LLVM type names for code generation
const (
	LLVMTypeInt    = "i64"
	LLVMTypeFloat  = "double"
	LLVMTypeBool   = "i8"    // in memory
	LLVMTypeBoolI1 = "i1"    // in SSA
	LLVMTypePtr    = "ptr"   // opaque pointer (LLVM 15+)
	LLVMTypeString = "{ ptr, i64 }"
)

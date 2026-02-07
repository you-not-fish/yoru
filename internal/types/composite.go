package types

import (
	"fmt"
	"strings"
)

// Array represents an array type [N]Elem.
type Array struct {
	typ
	len  int64
	elem Type
}

// NewArray creates a new array type with the given length and element type.
func NewArray(len int64, elem Type) *Array {
	return &Array{len: len, elem: elem}
}

// Len returns the array length.
func (a *Array) Len() int64 {
	return a.len
}

// Elem returns the array element type.
func (a *Array) Elem() Type {
	return a.elem
}

// Underlying implements Type.
func (a *Array) Underlying() Type {
	return a
}

// String implements Type.
func (a *Array) String() string {
	return fmt.Sprintf("[%d]%s", a.len, a.elem)
}

// Struct represents a struct type.
type Struct struct {
	typ
	fields  []*Var  // field declarations
	size    int64   // computed size (0 if not yet computed)
	align   int64   // computed alignment (0 if not yet computed)
	offsets []int64 // field offsets (nil if not yet computed)
}

// NewStruct creates a new struct type with the given fields.
func NewStruct(fields []*Var) *Struct {
	return &Struct{fields: fields}
}

// NumFields returns the number of fields.
func (s *Struct) NumFields() int {
	return len(s.fields)
}

// Field returns the field at the given index.
func (s *Struct) Field(i int) *Var {
	return s.fields[i]
}

// Fields returns all fields.
func (s *Struct) Fields() []*Var {
	return s.fields
}

// Size returns the struct size in bytes.
// Must be called after layout is computed.
func (s *Struct) Size() int64 {
	return s.size
}

// Align returns the struct alignment in bytes.
// Must be called after layout is computed.
func (s *Struct) Align() int64 {
	return s.align
}

// Offset returns the offset of field i in bytes.
// Must be called after layout is computed.
func (s *Struct) Offset(i int) int64 {
	return s.offsets[i]
}

// Offsets returns all field offsets.
// Must be called after layout is computed.
func (s *Struct) Offsets() []int64 {
	return s.offsets
}

// SetLayout sets the computed layout information.
func (s *Struct) SetLayout(size, align int64, offsets []int64) {
	s.size = size
	s.align = align
	s.offsets = offsets
}

// LayoutDone reports whether layout has been computed.
func (s *Struct) LayoutDone() bool {
	return s.offsets != nil
}

// Underlying implements Type.
func (s *Struct) Underlying() Type {
	return s
}

// String implements Type.
func (s *Struct) String() string {
	var buf strings.Builder
	buf.WriteString("struct{")
	for i, f := range s.fields {
		if i > 0 {
			buf.WriteString("; ")
		}
		buf.WriteString(f.Name())
		buf.WriteString(" ")
		buf.WriteString(f.Type().String())
	}
	buf.WriteString("}")
	return buf.String()
}

// Pointer represents a pointer type *T.
// In Yoru, pointers are stack-only and cannot escape.
type Pointer struct {
	typ
	base Type
}

// NewPointer creates a new pointer type.
func NewPointer(base Type) *Pointer {
	return &Pointer{base: base}
}

// Elem returns the base type that the pointer points to.
func (p *Pointer) Elem() Type {
	return p.base
}

// Underlying implements Type.
func (p *Pointer) Underlying() Type {
	return p
}

// String implements Type.
func (p *Pointer) String() string {
	return "*" + p.base.String()
}

// Ref represents a GC-managed reference type ref T.
type Ref struct {
	typ
	base Type
}

// NewRef creates a new reference type.
func NewRef(base Type) *Ref {
	return &Ref{base: base}
}

// Elem returns the base type that the reference points to.
func (r *Ref) Elem() Type {
	return r.base
}

// Underlying implements Type.
func (r *Ref) Underlying() Type {
	return r
}

// String implements Type.
func (r *Ref) String() string {
	return "ref " + r.base.String()
}

// Func represents a function type.
type Func struct {
	typ
	recv   *Var   // receiver (nil for non-method functions)
	params []*Var // parameters
	result Type   // return type (nil for void functions)
}

// NewFunc creates a new function type.
func NewFunc(recv *Var, params []*Var, result Type) *Func {
	return &Func{recv: recv, params: params, result: result}
}

// Recv returns the receiver, or nil if this is not a method.
func (f *Func) Recv() *Var {
	return f.recv
}

// Params returns the parameter list.
func (f *Func) Params() []*Var {
	return f.params
}

// NumParams returns the number of parameters.
func (f *Func) NumParams() int {
	return len(f.params)
}

// Param returns the parameter at index i.
func (f *Func) Param(i int) *Var {
	return f.params[i]
}

// Result returns the result type, or nil for void functions.
func (f *Func) Result() Type {
	return f.result
}

// Underlying implements Type.
func (f *Func) Underlying() Type {
	return f
}

// String implements Type.
func (f *Func) String() string {
	var buf strings.Builder
	buf.WriteString("func")
	if f.recv != nil {
		buf.WriteString("(")
		buf.WriteString(f.recv.Name())
		buf.WriteString(" ")
		buf.WriteString(f.recv.Type().String())
		buf.WriteString(") ")
	}
	buf.WriteString("(")
	for i, p := range f.params {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(p.Name())
		buf.WriteString(" ")
		buf.WriteString(p.Type().String())
	}
	buf.WriteString(")")
	if f.result != nil {
		buf.WriteString(" ")
		buf.WriteString(f.result.String())
	}
	return buf.String()
}

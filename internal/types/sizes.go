package types

import "github.com/you-not-fish/yoru/internal/rtabi"

// Sizes provides size and alignment calculations for types.
// It uses the rtabi constants to ensure ABI consistency with the runtime.
type Sizes struct{}

// DefaultSizes is the default Sizes implementation.
var DefaultSizes = &Sizes{}

// Sizeof returns the size of type T in bytes.
func (s *Sizes) Sizeof(T Type) int64 {
	switch t := T.Underlying().(type) {
	case *Basic:
		return s.basicSize(t.Kind())
	case *Array:
		return t.Len() * s.Sizeof(t.Elem())
	case *Struct:
		s.ComputeLayout(t)
		return t.Size()
	case *Pointer, *Ref:
		return rtabi.SizePtr
	case *Func:
		return rtabi.SizePtr
	case *Named:
		return s.Sizeof(t.Underlying())
	}
	return 0
}

// Alignof returns the alignment of type T in bytes.
func (s *Sizes) Alignof(T Type) int64 {
	switch t := T.Underlying().(type) {
	case *Basic:
		return s.basicAlign(t.Kind())
	case *Array:
		if t.Len() == 0 {
			return 1
		}
		return s.Alignof(t.Elem())
	case *Struct:
		s.ComputeLayout(t)
		return t.Align()
	case *Pointer, *Ref:
		return rtabi.AlignPtr
	case *Func:
		return rtabi.AlignPtr
	case *Named:
		return s.Alignof(t.Underlying())
	}
	return 1
}

// Offsetof returns the offset of field i in struct type T.
func (s *Sizes) Offsetof(T *Struct, i int) int64 {
	s.ComputeLayout(T)
	return T.Offset(i)
}

// ComputeLayout computes the size, alignment, and field offsets for a struct.
// This function is idempotent and safe to call multiple times.
func (s *Sizes) ComputeLayout(st *Struct) {
	if st.LayoutDone() {
		return
	}

	var offset int64
	var maxAlign int64 = 1
	offsets := make([]int64, len(st.fields))

	for i, f := range st.fields {
		fieldSize := s.Sizeof(f.Type())
		fieldAlign := s.Alignof(f.Type())

		// Align offset to field alignment
		offset = align(offset, fieldAlign)
		offsets[i] = offset
		offset += fieldSize

		if fieldAlign > maxAlign {
			maxAlign = fieldAlign
		}
	}

	// Add padding at end for struct alignment
	size := align(offset, maxAlign)

	st.SetLayout(size, maxAlign, offsets)
}

// basicSize returns the size of a basic type in bytes.
func (s *Sizes) basicSize(kind BasicKind) int64 {
	switch kind {
	case Bool:
		return rtabi.SizeBool
	case Int:
		return rtabi.SizeInt
	case Float:
		return rtabi.SizeFloat
	case String:
		return rtabi.SizeString
	default:
		// Untyped types have no concrete size
		return 0
	}
}

// basicAlign returns the alignment of a basic type in bytes.
func (s *Sizes) basicAlign(kind BasicKind) int64 {
	switch kind {
	case Bool:
		return rtabi.AlignBool
	case Int:
		return rtabi.AlignInt
	case Float:
		return rtabi.AlignFloat
	case String:
		return rtabi.AlignString
	default:
		// Untyped types have no concrete alignment
		return 1
	}
}

// align returns x rounded up to a multiple of a.
func align(x, a int64) int64 {
	return (x + a - 1) &^ (a - 1)
}

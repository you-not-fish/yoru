package types

import (
	"testing"

	"github.com/you-not-fish/yoru/internal/rtabi"
	"github.com/you-not-fish/yoru/internal/syntax"
)

func TestSizeof(t *testing.T) {
	sizes := DefaultSizes

	tests := []struct {
		typ  Type
		want int64
	}{
		{Typ[Bool], rtabi.SizeBool},
		{Typ[Int], rtabi.SizeInt},
		{Typ[Float], rtabi.SizeFloat},
		{Typ[String], rtabi.SizeString},
		{NewPointer(Typ[Int]), rtabi.SizePtr},
		{NewRef(Typ[Int]), rtabi.SizePtr},
	}

	for _, tt := range tests {
		t.Run(tt.typ.String(), func(t *testing.T) {
			got := sizes.Sizeof(tt.typ)
			if got != tt.want {
				t.Errorf("Sizeof(%s) = %d, want %d", tt.typ, got, tt.want)
			}
		})
	}
}

func TestAlignof(t *testing.T) {
	sizes := DefaultSizes

	tests := []struct {
		typ  Type
		want int64
	}{
		{Typ[Bool], rtabi.AlignBool},
		{Typ[Int], rtabi.AlignInt},
		{Typ[Float], rtabi.AlignFloat},
		{Typ[String], rtabi.AlignString},
		{NewPointer(Typ[Int]), rtabi.AlignPtr},
		{NewRef(Typ[Int]), rtabi.AlignPtr},
	}

	for _, tt := range tests {
		t.Run(tt.typ.String(), func(t *testing.T) {
			got := sizes.Alignof(tt.typ)
			if got != tt.want {
				t.Errorf("Alignof(%s) = %d, want %d", tt.typ, got, tt.want)
			}
		})
	}
}

func TestArraySize(t *testing.T) {
	sizes := DefaultSizes

	tests := []struct {
		len  int64
		elem Type
		want int64
	}{
		{10, Typ[Int], 10 * rtabi.SizeInt},
		{5, Typ[Bool], 5 * rtabi.SizeBool},
		{3, NewPointer(Typ[Int]), 3 * rtabi.SizePtr},
	}

	for _, tt := range tests {
		arr := NewArray(tt.len, tt.elem)
		t.Run(arr.String(), func(t *testing.T) {
			got := sizes.Sizeof(arr)
			if got != tt.want {
				t.Errorf("Sizeof(%s) = %d, want %d", arr, got, tt.want)
			}
		})
	}
}

func TestStructLayout(t *testing.T) {
	sizes := DefaultSizes

	// struct { a int; b bool; c int }
	// Expected layout with padding:
	// offset 0: a (8 bytes)
	// offset 8: b (1 byte)
	// offset 9-15: padding (7 bytes)
	// offset 16: c (8 bytes)
	// Total: 24 bytes, align: 8
	fields := []*Var{
		NewField(syntax.Pos{}, "a", Typ[Int]),
		NewField(syntax.Pos{}, "b", Typ[Bool]),
		NewField(syntax.Pos{}, "c", Typ[Int]),
	}
	st := NewStruct(fields)
	sizes.ComputeLayout(st)

	if st.Offset(0) != 0 {
		t.Errorf("Offset(0) = %d, want 0", st.Offset(0))
	}
	if st.Offset(1) != 8 {
		t.Errorf("Offset(1) = %d, want 8", st.Offset(1))
	}
	if st.Offset(2) != 16 {
		t.Errorf("Offset(2) = %d, want 16", st.Offset(2))
	}
	if st.Size() != 24 {
		t.Errorf("Size() = %d, want 24", st.Size())
	}
	if st.Align() != 8 {
		t.Errorf("Align() = %d, want 8", st.Align())
	}
}

func TestStructLayoutCompact(t *testing.T) {
	sizes := DefaultSizes

	// struct { a bool; b bool; c bool }
	// No padding needed between bool fields
	fields := []*Var{
		NewField(syntax.Pos{}, "a", Typ[Bool]),
		NewField(syntax.Pos{}, "b", Typ[Bool]),
		NewField(syntax.Pos{}, "c", Typ[Bool]),
	}
	st := NewStruct(fields)
	sizes.ComputeLayout(st)

	if st.Offset(0) != 0 {
		t.Errorf("Offset(0) = %d, want 0", st.Offset(0))
	}
	if st.Offset(1) != 1 {
		t.Errorf("Offset(1) = %d, want 1", st.Offset(1))
	}
	if st.Offset(2) != 2 {
		t.Errorf("Offset(2) = %d, want 2", st.Offset(2))
	}
	if st.Size() != 3 {
		t.Errorf("Size() = %d, want 3", st.Size())
	}
	if st.Align() != 1 {
		t.Errorf("Align() = %d, want 1", st.Align())
	}
}

func TestStructLayoutEmpty(t *testing.T) {
	sizes := DefaultSizes

	st := NewStruct(nil)
	sizes.ComputeLayout(st)

	if st.Size() != 0 {
		t.Errorf("Size() = %d, want 0", st.Size())
	}
	if st.Align() != 1 {
		t.Errorf("Align() = %d, want 1", st.Align())
	}
}

func TestStructLayoutWithPointers(t *testing.T) {
	sizes := DefaultSizes

	// struct { p *int; x int; q ref int }
	fields := []*Var{
		NewField(syntax.Pos{}, "p", NewPointer(Typ[Int])),
		NewField(syntax.Pos{}, "x", Typ[Int]),
		NewField(syntax.Pos{}, "q", NewRef(Typ[Int])),
	}
	st := NewStruct(fields)
	sizes.ComputeLayout(st)

	// All fields are 8-byte aligned
	if st.Offset(0) != 0 {
		t.Errorf("Offset(0) = %d, want 0", st.Offset(0))
	}
	if st.Offset(1) != 8 {
		t.Errorf("Offset(1) = %d, want 8", st.Offset(1))
	}
	if st.Offset(2) != 16 {
		t.Errorf("Offset(2) = %d, want 16", st.Offset(2))
	}
	if st.Size() != 24 {
		t.Errorf("Size() = %d, want 24", st.Size())
	}
}

func TestNestedStructLayout(t *testing.T) {
	sizes := DefaultSizes

	// Inner: struct { x int; y int }
	innerFields := []*Var{
		NewField(syntax.Pos{}, "x", Typ[Int]),
		NewField(syntax.Pos{}, "y", Typ[Int]),
	}
	inner := NewStruct(innerFields)
	sizes.ComputeLayout(inner)

	// Outer: struct { a bool; inner Inner }
	outerFields := []*Var{
		NewField(syntax.Pos{}, "a", Typ[Bool]),
		NewField(syntax.Pos{}, "inner", inner),
	}
	outer := NewStruct(outerFields)
	sizes.ComputeLayout(outer)

	// a at 0 (1 byte)
	// padding 7 bytes
	// inner at 8 (16 bytes)
	// Total: 24 bytes
	if outer.Offset(0) != 0 {
		t.Errorf("Offset(0) = %d, want 0", outer.Offset(0))
	}
	if outer.Offset(1) != 8 {
		t.Errorf("Offset(1) = %d, want 8", outer.Offset(1))
	}
	if outer.Size() != 24 {
		t.Errorf("Size() = %d, want 24", outer.Size())
	}
}

func TestStringSize(t *testing.T) {
	// String is a special type: ptr + len = 16 bytes
	size := DefaultSizes.Sizeof(Typ[String])
	if size != rtabi.SizeString {
		t.Errorf("Sizeof(string) = %d, want %d", size, rtabi.SizeString)
	}
}

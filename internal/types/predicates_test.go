package types

import (
	"testing"

	"github.com/you-not-fish/yoru/internal/syntax"
)

func TestIdentical(t *testing.T) {
	tests := []struct {
		name string
		a, b Type
		want bool
	}{
		{"same basic", Typ[Int], Typ[Int], true},
		{"diff basic", Typ[Int], Typ[Float], false},
		{"same array", NewArray(10, Typ[Int]), NewArray(10, Typ[Int]), true},
		{"diff array len", NewArray(10, Typ[Int]), NewArray(5, Typ[Int]), false},
		{"diff array elem", NewArray(10, Typ[Int]), NewArray(10, Typ[Float]), false},
		{"same ptr", NewPointer(Typ[Int]), NewPointer(Typ[Int]), true},
		{"diff ptr", NewPointer(Typ[Int]), NewPointer(Typ[Float]), false},
		{"same ref", NewRef(Typ[Int]), NewRef(Typ[Int]), true},
		{"diff ref", NewRef(Typ[Int]), NewRef(Typ[Float]), false},
		{"ptr vs ref", NewPointer(Typ[Int]), NewRef(Typ[Int]), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Identical(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Identical(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIdenticalStruct(t *testing.T) {
	// Struct identity is by structure, not name
	fields1 := []*Var{
		NewField(syntax.Pos{}, "x", Typ[Int]),
		NewField(syntax.Pos{}, "y", Typ[Float]),
	}
	fields2 := []*Var{
		NewField(syntax.Pos{}, "x", Typ[Int]),
		NewField(syntax.Pos{}, "y", Typ[Float]),
	}
	fields3 := []*Var{
		NewField(syntax.Pos{}, "a", Typ[Int]), // Different name
		NewField(syntax.Pos{}, "b", Typ[Float]),
	}
	fields4 := []*Var{
		NewField(syntax.Pos{}, "x", Typ[Int]),
		NewField(syntax.Pos{}, "y", Typ[Int]), // Different type
	}

	s1 := NewStruct(fields1)
	s2 := NewStruct(fields2)
	s3 := NewStruct(fields3)
	s4 := NewStruct(fields4)

	if !Identical(s1, s2) {
		t.Error("Identical structs with same fields should be identical")
	}
	if Identical(s1, s3) {
		t.Error("Structs with different field names should not be identical")
	}
	if Identical(s1, s4) {
		t.Error("Structs with different field types should not be identical")
	}
}

func TestIdenticalFunc(t *testing.T) {
	// func(int) bool
	f1 := NewFunc(nil, []*Var{NewVar(syntax.Pos{}, "x", Typ[Int])}, Typ[Bool])
	f2 := NewFunc(nil, []*Var{NewVar(syntax.Pos{}, "y", Typ[Int])}, Typ[Bool]) // Different param name
	f3 := NewFunc(nil, []*Var{NewVar(syntax.Pos{}, "x", Typ[Float])}, Typ[Bool])
	f4 := NewFunc(nil, []*Var{NewVar(syntax.Pos{}, "x", Typ[Int])}, Typ[Int])

	if !Identical(f1, f2) {
		t.Error("Functions with same signature but different param names should be identical")
	}
	if Identical(f1, f3) {
		t.Error("Functions with different param types should not be identical")
	}
	if Identical(f1, f4) {
		t.Error("Functions with different result types should not be identical")
	}
}

func TestIdenticalNamed(t *testing.T) {
	// Named types are identical only if they refer to same TypeName
	obj1 := NewTypeName(syntax.Pos{}, "T", nil)
	obj2 := NewTypeName(syntax.Pos{}, "T", nil) // Different object, same name

	n1 := NewNamed(obj1, Typ[Int])
	n2 := NewNamed(obj1, Typ[Int]) // Same object
	n3 := NewNamed(obj2, Typ[Int]) // Different object

	if !Identical(n1, n2) {
		t.Error("Named types with same TypeName should be identical")
	}
	if Identical(n1, n3) {
		t.Error("Named types with different TypeName should not be identical")
	}
}

func TestAssignableTo(t *testing.T) {
	tests := []struct {
		name string
		V, T Type
		want bool
	}{
		{"same type", Typ[Int], Typ[Int], true},
		{"diff type", Typ[Int], Typ[Float], false},
		{"untyped int to int", Typ[UntypedInt], Typ[Int], true},
		{"untyped int to float", Typ[UntypedInt], Typ[Float], true},
		{"untyped float to float", Typ[UntypedFloat], Typ[Float], true},
		{"untyped float to int", Typ[UntypedFloat], Typ[Int], false},
		{"untyped bool to bool", Typ[UntypedBool], Typ[Bool], true},
		{"untyped bool to int", Typ[UntypedBool], Typ[Int], false},
		{"untyped nil to ptr", Typ[UntypedNil], NewPointer(Typ[Int]), true},
		{"untyped nil to ref", Typ[UntypedNil], NewRef(Typ[Int]), true},
		{"untyped nil to int", Typ[UntypedNil], Typ[Int], false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AssignableTo(tt.V, tt.T)
			if got != tt.want {
				t.Errorf("AssignableTo(%s, %s) = %v, want %v", tt.V, tt.T, got, tt.want)
			}
		})
	}
}

func TestDefaultType(t *testing.T) {
	tests := []struct {
		typ  Type
		want Type
	}{
		{Typ[UntypedInt], Typ[Int]},
		{Typ[UntypedFloat], Typ[Float]},
		{Typ[UntypedBool], Typ[Bool]},
		{Typ[UntypedString], Typ[String]},
		{Typ[Int], Typ[Int]}, // Non-untyped stays same
	}

	for _, tt := range tests {
		t.Run(tt.typ.String(), func(t *testing.T) {
			got := DefaultType(tt.typ)
			if !Identical(got, tt.want) {
				t.Errorf("DefaultType(%s) = %s, want %s", tt.typ, got, tt.want)
			}
		})
	}
}

func TestIsPointer(t *testing.T) {
	tests := []struct {
		typ  Type
		want bool
	}{
		{NewPointer(Typ[Int]), true},
		{NewRef(Typ[Int]), false},
		{Typ[Int], false},
		{NewArray(10, Typ[Int]), false},
	}

	for _, tt := range tests {
		t.Run(tt.typ.String(), func(t *testing.T) {
			got := IsPointer(tt.typ)
			if got != tt.want {
				t.Errorf("IsPointer(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestIsRef(t *testing.T) {
	tests := []struct {
		typ  Type
		want bool
	}{
		{NewRef(Typ[Int]), true},
		{NewPointer(Typ[Int]), false},
		{Typ[Int], false},
	}

	for _, tt := range tests {
		t.Run(tt.typ.String(), func(t *testing.T) {
			got := IsRef(tt.typ)
			if got != tt.want {
				t.Errorf("IsRef(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestIsNil(t *testing.T) {
	tests := []struct {
		typ  Type
		want bool
	}{
		{Typ[UntypedNil], true},
		{Typ[Int], false},
		{NewPointer(Typ[Int]), false},
	}

	for _, tt := range tests {
		t.Run(tt.typ.String(), func(t *testing.T) {
			got := IsNil(tt.typ)
			if got != tt.want {
				t.Errorf("IsNil(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestIsUntypedType(t *testing.T) {
	tests := []struct {
		typ  Type
		want bool
	}{
		{Typ[UntypedInt], true},
		{Typ[UntypedFloat], true},
		{Typ[UntypedBool], true},
		{Typ[UntypedString], true},
		{Typ[UntypedNil], true},
		{Typ[Int], false},
		{Typ[Float], false},
		{Typ[Bool], false},
		{Typ[String], false},
	}

	for _, tt := range tests {
		t.Run(tt.typ.String(), func(t *testing.T) {
			got := IsUntypedType(tt.typ)
			if got != tt.want {
				t.Errorf("IsUntypedType(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		typ  Type
		want bool
	}{
		{Typ[Int], true},
		{Typ[Float], true},
		{Typ[UntypedInt], true},
		{Typ[UntypedFloat], true},
		{Typ[Bool], false},
		{Typ[String], false},
	}

	for _, tt := range tests {
		t.Run(tt.typ.String(), func(t *testing.T) {
			b, ok := tt.typ.Underlying().(*Basic)
			got := ok && b.Info()&IsNumeric != 0
			if got != tt.want {
				t.Errorf("IsNumeric(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

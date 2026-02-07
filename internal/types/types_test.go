package types

import (
	"testing"

	"github.com/you-not-fish/yoru/internal/syntax"
)

func TestBasicTypes(t *testing.T) {
	tests := []struct {
		kind BasicKind
		name string
		info BasicInfo
	}{
		{Bool, "bool", IsBoolean},
		{Int, "int", IsInteger | IsNumeric},
		{Float, "float", IsFloat | IsNumeric},
		{String, "string", IsString},
		{UntypedBool, "untyped bool", IsBoolean | IsUntyped},
		{UntypedInt, "untyped int", IsInteger | IsNumeric | IsUntyped},
		{UntypedFloat, "untyped float", IsFloat | IsNumeric | IsUntyped},
		{UntypedString, "untyped string", IsString | IsUntyped},
		{UntypedNil, "untyped nil", IsUntyped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := Typ[tt.kind]
			if typ == nil {
				t.Fatalf("Typ[%d] is nil", tt.kind)
			}
			if typ.Kind() != tt.kind {
				t.Errorf("Kind() = %v, want %v", typ.Kind(), tt.kind)
			}
			if typ.Info() != tt.info {
				t.Errorf("Info() = %v, want %v", typ.Info(), tt.info)
			}
			if typ.Name() != tt.name {
				t.Errorf("Name() = %q, want %q", typ.Name(), tt.name)
			}
			if typ.String() != tt.name {
				t.Errorf("String() = %q, want %q", typ.String(), tt.name)
			}
			// Basic type's underlying is itself
			if typ.Underlying() != typ {
				t.Errorf("Underlying() != self")
			}
		})
	}
}

func TestArrayType(t *testing.T) {
	elem := Typ[Int]
	arr := NewArray(10, elem)

	if arr.Len() != 10 {
		t.Errorf("Len() = %d, want 10", arr.Len())
	}
	if arr.Elem() != elem {
		t.Errorf("Elem() != expected element type")
	}
	if arr.String() != "[10]int" {
		t.Errorf("String() = %q, want %q", arr.String(), "[10]int")
	}
	if arr.Underlying() != arr {
		t.Errorf("Underlying() != self")
	}
}

func TestPointerType(t *testing.T) {
	base := Typ[Int]
	ptr := NewPointer(base)

	if ptr.Elem() != base {
		t.Errorf("Elem() != expected base type")
	}
	if ptr.String() != "*int" {
		t.Errorf("String() = %q, want %q", ptr.String(), "*int")
	}
	if ptr.Underlying() != ptr {
		t.Errorf("Underlying() != self")
	}
}

func TestRefType(t *testing.T) {
	base := Typ[Int]
	ref := NewRef(base)

	if ref.Elem() != base {
		t.Errorf("Elem() != expected base type")
	}
	if ref.String() != "ref int" {
		t.Errorf("String() = %q, want %q", ref.String(), "ref int")
	}
	if ref.Underlying() != ref {
		t.Errorf("Underlying() != self")
	}
}

func TestStructType(t *testing.T) {
	fields := []*Var{
		NewField(syntax.Pos{}, "x", Typ[Int]),
		NewField(syntax.Pos{}, "y", Typ[Float]),
	}
	st := NewStruct(fields)

	if st.NumFields() != 2 {
		t.Errorf("NumFields() = %d, want 2", st.NumFields())
	}
	if st.Field(0).Name() != "x" {
		t.Errorf("Field(0).Name() = %q, want %q", st.Field(0).Name(), "x")
	}
	if st.Field(1).Name() != "y" {
		t.Errorf("Field(1).Name() = %q, want %q", st.Field(1).Name(), "y")
	}

	expected := "struct{x int; y float}"
	if st.String() != expected {
		t.Errorf("String() = %q, want %q", st.String(), expected)
	}
}

func TestFuncType(t *testing.T) {
	params := []*Var{
		NewVar(syntax.Pos{}, "a", Typ[Int]),
		NewVar(syntax.Pos{}, "b", Typ[Float]),
	}
	result := Typ[Bool]
	fn := NewFunc(nil, params, result)

	if fn.NumParams() != 2 {
		t.Errorf("NumParams() = %d, want 2", fn.NumParams())
	}
	if fn.Result() != result {
		t.Errorf("Result() != expected result type")
	}
	if fn.Recv() != nil {
		t.Errorf("Recv() != nil for non-method")
	}

	expected := "func(a int, b float) bool"
	if fn.String() != expected {
		t.Errorf("String() = %q, want %q", fn.String(), expected)
	}
}

func TestFuncTypeVoid(t *testing.T) {
	params := []*Var{
		NewVar(syntax.Pos{}, "x", Typ[Int]),
	}
	fn := NewFunc(nil, params, nil)

	if fn.Result() != nil {
		t.Errorf("Result() should be nil for void function")
	}

	expected := "func(x int)"
	if fn.String() != expected {
		t.Errorf("String() = %q, want %q", fn.String(), expected)
	}
}

func TestFuncTypeNoParams(t *testing.T) {
	fn := NewFunc(nil, nil, Typ[Int])

	if fn.NumParams() != 0 {
		t.Errorf("NumParams() = %d, want 0", fn.NumParams())
	}

	expected := "func() int"
	if fn.String() != expected {
		t.Errorf("String() = %q, want %q", fn.String(), expected)
	}
}

func TestMethodType(t *testing.T) {
	recv := NewVar(syntax.Pos{}, "self", NewPointer(Typ[Int]))
	params := []*Var{
		NewVar(syntax.Pos{}, "x", Typ[Int]),
	}
	fn := NewFunc(recv, params, Typ[Bool])

	if fn.Recv() != recv {
		t.Errorf("Recv() != expected receiver")
	}

	expected := "func(self *int) (x int) bool"
	if fn.String() != expected {
		t.Errorf("String() = %q, want %q", fn.String(), expected)
	}
}

func TestNamedType(t *testing.T) {
	obj := NewTypeName(syntax.Pos{}, "Point", nil)
	fields := []*Var{
		NewField(syntax.Pos{}, "x", Typ[Int]),
		NewField(syntax.Pos{}, "y", Typ[Int]),
	}
	st := NewStruct(fields)
	named := NewNamed(obj, st)

	if named.Obj() != obj {
		t.Errorf("Obj() != expected object")
	}
	if named.Underlying() != st {
		t.Errorf("Underlying() != struct")
	}
	if named.String() != "Point" {
		t.Errorf("String() = %q, want %q", named.String(), "Point")
	}
	if named.NumMethods() != 0 {
		t.Errorf("NumMethods() = %d, want 0", named.NumMethods())
	}
}

func TestNamedTypeWithMethods(t *testing.T) {
	obj := NewTypeName(syntax.Pos{}, "Counter", nil)
	st := NewStruct(nil)
	named := NewNamed(obj, st)

	// Add a method
	method := NewFuncObj(syntax.Pos{}, "Inc")
	named.AddMethod(method)

	if named.NumMethods() != 1 {
		t.Errorf("NumMethods() = %d, want 1", named.NumMethods())
	}
	if named.Method(0) != method {
		t.Errorf("Method(0) != expected method")
	}
}

func TestNestedTypes(t *testing.T) {
	// [5]*ref int
	ref := NewRef(Typ[Int])
	ptr := NewPointer(ref)
	arr := NewArray(5, ptr)

	expected := "[5]*ref int"
	if arr.String() != expected {
		t.Errorf("String() = %q, want %q", arr.String(), expected)
	}
}

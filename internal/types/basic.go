package types

// BasicKind describes the kind of basic type.
type BasicKind int

const (
	Invalid BasicKind = iota // invalid type

	// Concrete basic types
	Bool
	Int
	Float
	String

	// Untyped basic types (for constant expressions)
	UntypedBool
	UntypedInt
	UntypedFloat
	UntypedString
	UntypedNil
)

// BasicInfo describes properties of a basic type.
type BasicInfo int

const (
	IsBoolean BasicInfo = 1 << iota
	IsInteger
	IsFloat
	IsString
	IsUntyped
	IsNumeric = IsInteger | IsFloat
)

// Basic represents a basic type: bool, int, float, string, and untyped variants.
type Basic struct {
	typ
	kind BasicKind
	info BasicInfo
	name string
}

// Kind returns the kind of the basic type.
func (b *Basic) Kind() BasicKind {
	return b.kind
}

// Info returns information about the basic type.
func (b *Basic) Info() BasicInfo {
	return b.info
}

// Name returns the name of the basic type.
func (b *Basic) Name() string {
	return b.name
}

// Underlying implements Type.
func (b *Basic) Underlying() Type {
	return b
}

// String implements Type.
func (b *Basic) String() string {
	return b.name
}

// Typ holds the predeclared basic types, indexed by BasicKind.
// Typ[Invalid] is nil, representing an invalid type.
var Typ = []*Basic{
	Invalid:       nil,
	Bool:          {kind: Bool, info: IsBoolean, name: "bool"},
	Int:           {kind: Int, info: IsInteger | IsNumeric, name: "int"},
	Float:         {kind: Float, info: IsFloat | IsNumeric, name: "float"},
	String:        {kind: String, info: IsString, name: "string"},
	UntypedBool:   {kind: UntypedBool, info: IsBoolean | IsUntyped, name: "untyped bool"},
	UntypedInt:    {kind: UntypedInt, info: IsInteger | IsNumeric | IsUntyped, name: "untyped int"},
	UntypedFloat:  {kind: UntypedFloat, info: IsFloat | IsNumeric | IsUntyped, name: "untyped float"},
	UntypedString: {kind: UntypedString, info: IsString | IsUntyped, name: "untyped string"},
	UntypedNil:    {kind: UntypedNil, info: IsUntyped, name: "untyped nil"},
}

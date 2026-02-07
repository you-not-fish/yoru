package types

import "github.com/you-not-fish/yoru/internal/syntax"

// Object represents a declared entity: variable, type, function, builtin, or nil.
type Object interface {
	Name() string    // object name
	Type() Type      // object type
	Pos() syntax.Pos // declaration position
	Parent() *Scope  // enclosing scope

	setParent(*Scope) // internal: set parent scope
	aObject()         // marker method to restrict implementations
}

// object is the base struct for all objects.
type object struct {
	name   string
	typ    Type
	pos    syntax.Pos
	parent *Scope
}

func (o *object) Name() string       { return o.name }
func (o *object) Type() Type         { return o.typ }
func (o *object) Pos() syntax.Pos    { return o.pos }
func (o *object) Parent() *Scope     { return o.parent }
func (o *object) setParent(s *Scope) { o.parent = s }
func (*object) aObject()             {}

// Var represents a variable or struct field.
type Var struct {
	object
	isField bool // true if this is a struct field
}

// NewVar creates a new variable object.
func NewVar(pos syntax.Pos, name string, typ Type) *Var {
	return &Var{object: object{name: name, typ: typ, pos: pos}}
}

// NewField creates a new struct field object.
func NewField(pos syntax.Pos, name string, typ Type) *Var {
	return &Var{object: object{name: name, typ: typ, pos: pos}, isField: true}
}

// IsField reports whether this variable is a struct field.
func (v *Var) IsField() bool {
	return v.isField
}

// SetType sets the variable's type.
// This is called during type checking once the type is resolved.
func (v *Var) SetType(typ Type) {
	v.typ = typ
}

// TypeName represents a declared type name.
type TypeName struct {
	object
}

// NewTypeName creates a new type name object.
func NewTypeName(pos syntax.Pos, name string, typ Type) *TypeName {
	return &TypeName{object: object{name: name, typ: typ, pos: pos}}
}

// SetType sets the type associated with the type name.
// This is used during type checking once the declaration is resolved.
func (t *TypeName) SetType(typ Type) {
	t.typ = typ
}

// FuncObj represents a declared function or method.
type FuncObj struct {
	object
	sig *Func // function signature (set after construction)
}

// NewFuncObj creates a new function object.
// The signature should be set later using SetSignature.
func NewFuncObj(pos syntax.Pos, name string) *FuncObj {
	return &FuncObj{object: object{name: name, pos: pos}}
}

// Signature returns the function signature.
func (f *FuncObj) Signature() *Func {
	return f.sig
}

// SetSignature sets the function signature.
// This is called during type checking once the signature is resolved.
func (f *FuncObj) SetSignature(sig *Func) {
	f.sig = sig
	f.typ = sig
}

// BuiltinKind identifies a builtin function.
type BuiltinKind int

const (
	BuiltinPrintln BuiltinKind = iota
	BuiltinNew
	BuiltinPanic
)

// Builtin represents a built-in function.
type Builtin struct {
	object
	kind BuiltinKind
}

// NewBuiltin creates a new builtin function object.
func NewBuiltin(name string, kind BuiltinKind) *Builtin {
	return &Builtin{object: object{name: name}, kind: kind}
}

// Kind returns the builtin function kind.
func (b *Builtin) Kind() BuiltinKind {
	return b.kind
}

// Nil represents the predeclared nil value.
type Nil struct {
	object
}

// NewNil creates a new nil object.
func NewNil() *Nil {
	return &Nil{object: object{name: "nil", typ: Typ[UntypedNil]}}
}

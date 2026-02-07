package types

import "github.com/you-not-fish/yoru/internal/syntax"

// NoPos is the zero position value, used for predeclared objects.
var NoPos syntax.Pos

// Universe is the root scope containing all predeclared objects.
var Universe *Scope

// Predeclared objects accessible via the Universe scope.
var (
	// Types
	universeInt    *TypeName
	universeFloat  *TypeName
	universeBool   *TypeName
	universeString *TypeName

	// Constants
	universeTrue  Object
	universeFalse Object
	universeNil   *Nil

	// Builtins
	universePrintln *Builtin
	universeNew     *Builtin
	universePanic   *Builtin
)

func init() {
	// Create Universe scope
	Universe = NewScope(nil, NoPos, NoPos, "universe")

	// Define predeclared types
	defPredeclaredTypes()

	// Define predeclared constants
	defPredeclaredConsts()

	// Define predeclared builtins
	defPredeclaredBuiltins()
}

// defPredeclaredTypes defines int, float, bool, string in Universe.
func defPredeclaredTypes() {
	for _, kind := range []BasicKind{Bool, Int, Float, String} {
		typ := Typ[kind]
		obj := NewTypeName(NoPos, typ.name, typ)
		Universe.Insert(obj)

		switch kind {
		case Bool:
			universeBool = obj
		case Int:
			universeInt = obj
		case Float:
			universeFloat = obj
		case String:
			universeString = obj
		}
	}
}

// defPredeclaredConsts defines true, false, nil in Universe.
func defPredeclaredConsts() {
	// true and false are Var objects with untyped bool type
	universeTrue = NewVar(NoPos, "true", Typ[UntypedBool])
	Universe.Insert(universeTrue)

	universeFalse = NewVar(NoPos, "false", Typ[UntypedBool])
	Universe.Insert(universeFalse)

	// nil is a Nil object
	universeNil = NewNil()
	Universe.Insert(universeNil)
}

// defPredeclaredBuiltins defines println, new, panic in Universe.
func defPredeclaredBuiltins() {
	universePrintln = NewBuiltin("println", BuiltinPrintln)
	Universe.Insert(universePrintln)

	universeNew = NewBuiltin("new", BuiltinNew)
	Universe.Insert(universeNew)

	universePanic = NewBuiltin("panic", BuiltinPanic)
	Universe.Insert(universePanic)
}

// Predeclared type accessors
func UniverseInt() *TypeName    { return universeInt }
func UniverseFloat() *TypeName  { return universeFloat }
func UniverseBool() *TypeName   { return universeBool }
func UniverseString() *TypeName { return universeString }

// Predeclared constant accessors
func UniverseTrue() Object  { return universeTrue }
func UniverseFalse() Object { return universeFalse }
func UniverseNil() *Nil     { return universeNil }

// Predeclared builtin accessors
func UniversePrintln() *Builtin { return universePrintln }
func UniverseNew() *Builtin     { return universeNew }
func UniversePanic() *Builtin   { return universePanic }

package types

// Identical reports whether x and y are identical types.
func Identical(x, y Type) bool {
	if x == y {
		return true
	}
	if x == nil || y == nil {
		return false
	}
	return identical(x, y)
}

func identical(x, y Type) bool {
	// Handle named types
	xn, xNamed := x.(*Named)
	yn, yNamed := y.(*Named)
	if xNamed && yNamed {
		// Two named types are identical only if they are the same named type
		return xn.obj == yn.obj
	}
	if xNamed != yNamed {
		// One named, one not
		return false
	}

	// Neither is named, compare underlying structures
	switch x := x.(type) {
	case *Basic:
		if y, ok := y.(*Basic); ok {
			return x.kind == y.kind
		}
	case *Array:
		if y, ok := y.(*Array); ok {
			return x.len == y.len && Identical(x.elem, y.elem)
		}
	case *Struct:
		if y, ok := y.(*Struct); ok {
			return identicalStructs(x, y)
		}
	case *Pointer:
		if y, ok := y.(*Pointer); ok {
			return Identical(x.base, y.base)
		}
	case *Ref:
		if y, ok := y.(*Ref); ok {
			return Identical(x.base, y.base)
		}
	case *Func:
		if y, ok := y.(*Func); ok {
			return identicalFuncs(x, y)
		}
	}
	return false
}

func identicalStructs(x, y *Struct) bool {
	if len(x.fields) != len(y.fields) {
		return false
	}
	for i := range x.fields {
		if x.fields[i].Name() != y.fields[i].Name() {
			return false
		}
		if !Identical(x.fields[i].Type(), y.fields[i].Type()) {
			return false
		}
	}
	return true
}

func identicalFuncs(x, y *Func) bool {
	// Check receivers
	if (x.recv == nil) != (y.recv == nil) {
		return false
	}
	if x.recv != nil && !Identical(x.recv.Type(), y.recv.Type()) {
		return false
	}

	// Check parameters
	if len(x.params) != len(y.params) {
		return false
	}
	for i := range x.params {
		if !Identical(x.params[i].Type(), y.params[i].Type()) {
			return false
		}
	}

	// Check results
	if (x.result == nil) != (y.result == nil) {
		return false
	}
	if x.result != nil && !Identical(x.result, y.result) {
		return false
	}

	return true
}

// AssignableTo reports whether a value of type V is assignable to type T.
func AssignableTo(V, T Type) bool {
	// Identical types are always assignable
	if Identical(V, T) {
		return true
	}

	// Untyped nil is assignable to any pointer or ref type
	if isUntyped(V) {
		Vb, ok := V.(*Basic)
		if ok && Vb.kind == UntypedNil {
			Tu := T.Underlying()
			switch Tu.(type) {
			case *Pointer, *Ref:
				return true
			}
		}
	}

	// Untyped constants can be assigned to compatible concrete types
	if isUntyped(V) {
		return isRepresentableAs(V, T)
	}

	return false
}

// isRepresentableAs reports whether an untyped value can be represented as type T.
func isRepresentableAs(V, T Type) bool {
	Vb, ok := V.(*Basic)
	if !ok {
		return false
	}

	Tu := T.Underlying()
	Tb, ok := Tu.(*Basic)
	if !ok {
		return false
	}

	switch Vb.kind {
	case UntypedBool:
		return Tb.kind == Bool
	case UntypedInt:
		// Untyped int can be assigned to int or float
		return Tb.kind == Int || Tb.kind == Float
	case UntypedFloat:
		// Untyped float can only be assigned to float
		return Tb.kind == Float
	case UntypedString:
		return Tb.kind == String
	}
	return false
}

// isUntyped reports whether T is an untyped type.
func isUntyped(T Type) bool {
	b, ok := T.(*Basic)
	return ok && b.info&IsUntyped != 0
}

// isBoolean reports whether T is a boolean type.
func isBoolean(T Type) bool {
	b, ok := T.Underlying().(*Basic)
	return ok && b.info&IsBoolean != 0
}

// isInteger reports whether T is an integer type.
func isInteger(T Type) bool {
	b, ok := T.Underlying().(*Basic)
	return ok && (b.kind == Int || b.kind == UntypedInt)
}

// isFloat reports whether T is a floating-point type.
func isFloat(T Type) bool {
	b, ok := T.Underlying().(*Basic)
	return ok && (b.kind == Float || b.kind == UntypedFloat)
}

// isNumeric reports whether T is a numeric type (integer or float).
func isNumeric(T Type) bool {
	b, ok := T.Underlying().(*Basic)
	return ok && b.info&IsNumeric != 0
}

// isStringType reports whether T is a string type.
func isStringType(T Type) bool {
	b, ok := T.Underlying().(*Basic)
	return ok && b.info&IsString != 0
}

// IsUntypedType reports whether T is an untyped type (exported version).
func IsUntypedType(T Type) bool {
	return isUntyped(T)
}

// IsPointer reports whether T is a pointer type (*T).
func IsPointer(T Type) bool {
	_, ok := T.Underlying().(*Pointer)
	return ok
}

// IsRef reports whether T is a reference type (ref T).
func IsRef(T Type) bool {
	_, ok := T.Underlying().(*Ref)
	return ok
}

// IsPointerOrRef reports whether T is a pointer or reference type.
func IsPointerOrRef(T Type) bool {
	return IsPointer(T) || IsRef(T)
}

// IsNil reports whether T is the untyped nil type.
func IsNil(T Type) bool {
	b, ok := T.(*Basic)
	return ok && b.kind == UntypedNil
}

// DefaultType returns the default type for an untyped type.
// For typed types, returns the type itself.
func DefaultType(T Type) Type {
	b, ok := T.(*Basic)
	if !ok {
		return T
	}
	switch b.kind {
	case UntypedBool:
		return Typ[Bool]
	case UntypedInt:
		return Typ[Int]
	case UntypedFloat:
		return Typ[Float]
	case UntypedString:
		return Typ[String]
	default:
		return T
	}
}

// Comparable reports whether values of type T can be compared with == or !=.
func Comparable(T Type) bool {
	switch t := T.Underlying().(type) {
	case *Basic:
		// All basic types are comparable (except invalid)
		return t.kind != Invalid
	case *Pointer, *Ref:
		// Pointers and refs can be compared
		return true
	case *Array:
		// Arrays are comparable if their element type is comparable
		return Comparable(t.elem)
	case *Struct:
		// Structs are comparable if all fields are comparable
		for _, f := range t.fields {
			if !Comparable(f.Type()) {
				return false
			}
		}
		return true
	default:
		// Functions are not comparable
		return false
	}
}

// Ordered reports whether values of type T can be ordered with <, <=, >, >=.
func Ordered(T Type) bool {
	b, ok := T.Underlying().(*Basic)
	if !ok {
		return false
	}
	// Only numeric and string types are ordered
	return b.info&(IsNumeric|IsString) != 0
}

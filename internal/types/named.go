package types

// Named represents a named type (type T ...).
type Named struct {
	typ
	obj        *TypeName  // type name object
	underlying Type       // underlying type
	methods    []*FuncObj // methods associated with this type
}

// NewNamed creates a new named type.
// The underlying type should be set later using SetUnderlying.
func NewNamed(obj *TypeName, underlying Type) *Named {
	n := &Named{obj: obj, underlying: underlying}
	if obj != nil {
		obj.typ = n
	}
	return n
}

// Obj returns the type name object.
func (n *Named) Obj() *TypeName {
	return n.obj
}

// SetUnderlying sets the underlying type.
// This is called during type checking once the underlying type is resolved.
func (n *Named) SetUnderlying(underlying Type) {
	n.underlying = underlying
}

// Underlying implements Type.
// For named types, returns the underlying type of the named type.
func (n *Named) Underlying() Type {
	return n.underlying
}

// String implements Type.
func (n *Named) String() string {
	if n.obj != nil {
		return n.obj.Name()
	}
	return "unnamed"
}

// NumMethods returns the number of methods.
func (n *Named) NumMethods() int {
	return len(n.methods)
}

// Method returns the method at index i.
func (n *Named) Method(i int) *FuncObj {
	return n.methods[i]
}

// Methods returns all methods.
func (n *Named) Methods() []*FuncObj {
	return n.methods
}

// AddMethod adds a method to this named type.
func (n *Named) AddMethod(m *FuncObj) {
	n.methods = append(n.methods, m)
}

// LookupMethod looks up a method by name.
// Returns nil if not found.
func (n *Named) LookupMethod(name string) *FuncObj {
	for _, m := range n.methods {
		if m.Name() == name {
			return m
		}
	}
	return nil
}

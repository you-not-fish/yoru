package types

import (
	"fmt"
	"strings"

	"github.com/you-not-fish/yoru/internal/syntax"
)

// Scope represents a lexical scope.
// Scopes form a tree starting from the Universe scope.
type Scope struct {
	parent   *Scope
	children []*Scope
	elems    map[string]Object
	pos, end syntax.Pos
	comment  string // debugging comment (e.g., "function foo", "block")
}

// NewScope creates a new scope with the given parent.
func NewScope(parent *Scope, pos, end syntax.Pos, comment string) *Scope {
	s := &Scope{
		parent:  parent,
		elems:   make(map[string]Object),
		pos:     pos,
		end:     end,
		comment: comment,
	}
	if parent != nil {
		parent.children = append(parent.children, s)
	}
	return s
}

// Parent returns the parent scope, or nil for the Universe scope.
func (s *Scope) Parent() *Scope {
	return s.parent
}

// Children returns the list of child scopes.
func (s *Scope) Children() []*Scope {
	return s.children
}

// NumChildren returns the number of child scopes.
func (s *Scope) NumChildren() int {
	return len(s.children)
}

// Pos returns the start position of the scope in source.
func (s *Scope) Pos() syntax.Pos {
	return s.pos
}

// End returns the end position of the scope in source.
func (s *Scope) End() syntax.Pos {
	return s.end
}

// Comment returns the scope's comment (for debugging).
func (s *Scope) Comment() string {
	return s.comment
}

// Lookup returns the object with the given name in the current scope.
// Returns nil if not found in this scope (does not search parent scopes).
func (s *Scope) Lookup(name string) Object {
	return s.elems[name]
}

// LookupParent returns the object with the given name by searching
// from the current scope up through all parent scopes.
// Returns the object and the scope in which it was found.
// Returns (nil, nil) if not found.
func (s *Scope) LookupParent(name string) (Object, *Scope) {
	for scope := s; scope != nil; scope = scope.parent {
		if obj := scope.elems[name]; obj != nil {
			return obj, scope
		}
	}
	return nil, nil
}

// Insert inserts an object into the scope.
// If an object with the same name already exists, returns the existing object.
// Otherwise, returns nil.
func (s *Scope) Insert(obj Object) Object {
	name := obj.Name()
	if existing := s.elems[name]; existing != nil {
		return existing
	}
	s.elems[name] = obj
	obj.setParent(s)
	return nil
}

// Names returns the names of all objects in the scope, sorted alphabetically.
func (s *Scope) Names() []string {
	names := make([]string, 0, len(s.elems))
	for name := range s.elems {
		names = append(names, name)
	}
	// Simple sort for deterministic output
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] > names[j] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

// NumObjects returns the number of objects in the scope.
func (s *Scope) NumObjects() int {
	return len(s.elems)
}

// String returns a string representation of the scope for debugging.
func (s *Scope) String() string {
	var buf strings.Builder
	s.writeTo(&buf, 0)
	return buf.String()
}

func (s *Scope) writeTo(buf *strings.Builder, indent int) {
	prefix := strings.Repeat("  ", indent)
	fmt.Fprintf(buf, "%sscope %s {\n", prefix, s.comment)
	for _, name := range s.Names() {
		obj := s.elems[name]
		fmt.Fprintf(buf, "%s  %s: %s\n", prefix, name, obj.Type())
	}
	for _, child := range s.children {
		child.writeTo(buf, indent+1)
	}
	fmt.Fprintf(buf, "%s}\n", prefix)
}

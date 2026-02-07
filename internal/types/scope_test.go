package types

import (
	"testing"

	"github.com/you-not-fish/yoru/internal/syntax"
)

// Helper function to create a scope for testing
func testScope(parent *Scope, comment string) *Scope {
	return NewScope(parent, syntax.Pos{}, syntax.Pos{}, comment)
}

func TestScopeInsertAndLookup(t *testing.T) {
	scope := testScope(nil, "test")

	obj := NewVar(syntax.Pos{}, "x", Typ[Int])
	existing := scope.Insert(obj)

	if existing != nil {
		t.Errorf("Insert() returned non-nil for first insert")
	}

	found := scope.Lookup("x")
	if found != obj {
		t.Errorf("Lookup() did not return inserted object")
	}

	// Insert duplicate
	obj2 := NewVar(syntax.Pos{}, "x", Typ[Float])
	existing = scope.Insert(obj2)
	if existing != obj {
		t.Errorf("Insert() should return first object for duplicate")
	}
}

func TestScopeLookupParent(t *testing.T) {
	parent := testScope(nil, "parent")
	child := testScope(parent, "child")

	obj := NewVar(syntax.Pos{}, "x", Typ[Int])
	parent.Insert(obj)

	// Lookup in child should find parent's object
	found, foundScope := child.LookupParent("x")
	if found != obj {
		t.Errorf("LookupParent() did not find parent's object")
	}
	if foundScope != parent {
		t.Errorf("LookupParent() returned wrong scope")
	}

	// Direct lookup in child should fail
	if child.Lookup("x") != nil {
		t.Errorf("Lookup() should not find parent's object")
	}
}

func TestScopeShadowing(t *testing.T) {
	parent := testScope(nil, "parent")
	child := testScope(parent, "child")

	parentObj := NewVar(syntax.Pos{}, "x", Typ[Int])
	parent.Insert(parentObj)

	childObj := NewVar(syntax.Pos{}, "x", Typ[Float])
	child.Insert(childObj)

	// LookupParent in child should find child's object (shadowing)
	found, foundScope := child.LookupParent("x")
	if found != childObj {
		t.Errorf("LookupParent() should find child's shadowing object")
	}
	if foundScope != child {
		t.Errorf("LookupParent() should return child scope")
	}
}

func TestScopeHierarchy(t *testing.T) {
	// Universe -> Package -> Function -> Block
	universe := testScope(nil, "universe")
	pkg := testScope(universe, "package")
	fn := testScope(pkg, "function")
	block := testScope(fn, "block")

	// Insert at different levels
	universe.Insert(NewTypeName(syntax.Pos{}, "int", Typ[Int]))
	pkg.Insert(NewVar(syntax.Pos{}, "globalX", Typ[Int]))
	fn.Insert(NewVar(syntax.Pos{}, "param", Typ[Int]))
	block.Insert(NewVar(syntax.Pos{}, "local", Typ[Int]))

	// All should be visible from block
	tests := []string{"int", "globalX", "param", "local"}
	for _, name := range tests {
		found, _ := block.LookupParent(name)
		if found == nil {
			t.Errorf("LookupParent(%q) failed from block", name)
		}
	}
}

func TestScopeNames(t *testing.T) {
	scope := testScope(nil, "test")

	scope.Insert(NewVar(syntax.Pos{}, "a", Typ[Int]))
	scope.Insert(NewVar(syntax.Pos{}, "b", Typ[Float]))
	scope.Insert(NewVar(syntax.Pos{}, "c", Typ[Bool]))

	names := scope.Names()
	if len(names) != 3 {
		t.Errorf("Names() returned %d names, want 3", len(names))
	}

	// Names should be sorted
	expected := []string{"a", "b", "c"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("Names()[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestScopeParent(t *testing.T) {
	parent := testScope(nil, "parent")
	child := testScope(parent, "child")

	if child.Parent() != parent {
		t.Errorf("Parent() != expected parent")
	}
	if parent.Parent() != nil {
		t.Errorf("Parent() should be nil for root scope")
	}
}

func TestScopeComment(t *testing.T) {
	scope := testScope(nil, "my comment")
	if scope.Comment() != "my comment" {
		t.Errorf("Comment() = %q, want %q", scope.Comment(), "my comment")
	}
}

func TestObjectParentScope(t *testing.T) {
	scope := testScope(nil, "test")
	obj := NewVar(syntax.Pos{}, "x", Typ[Int])

	if obj.Parent() != nil {
		t.Errorf("Parent() should be nil before insertion")
	}

	scope.Insert(obj)

	if obj.Parent() != scope {
		t.Errorf("Parent() should be set after insertion")
	}
}

func TestScopeChildren(t *testing.T) {
	parent := testScope(nil, "parent")
	child1 := testScope(parent, "child1")
	child2 := testScope(parent, "child2")

	if parent.NumChildren() != 2 {
		t.Errorf("NumChildren() = %d, want 2", parent.NumChildren())
	}

	children := parent.Children()
	if len(children) != 2 {
		t.Errorf("Children() returned %d, want 2", len(children))
	}
	if children[0] != child1 {
		t.Errorf("Children()[0] != child1")
	}
	if children[1] != child2 {
		t.Errorf("Children()[1] != child2")
	}
}

func TestUniverse(t *testing.T) {
	// Universe should be initialized
	if Universe == nil {
		t.Fatal("Universe is nil")
	}

	// Check predeclared types
	for _, name := range []string{"int", "float", "bool", "string"} {
		obj := Universe.Lookup(name)
		if obj == nil {
			t.Errorf("Universe.Lookup(%q) = nil", name)
		}
		tn, ok := obj.(*TypeName)
		if !ok {
			t.Errorf("Universe.Lookup(%q) is not TypeName", name)
		}
		if tn.Name() != name {
			t.Errorf("TypeName.Name() = %q, want %q", tn.Name(), name)
		}
	}

	// Check predeclared constants
	for _, name := range []string{"true", "false", "nil"} {
		obj := Universe.Lookup(name)
		if obj == nil {
			t.Errorf("Universe.Lookup(%q) = nil", name)
		}
	}

	// Check predeclared builtins
	for _, name := range []string{"println", "new", "panic"} {
		obj := Universe.Lookup(name)
		if obj == nil {
			t.Errorf("Universe.Lookup(%q) = nil", name)
		}
		_, ok := obj.(*Builtin)
		if !ok {
			t.Errorf("Universe.Lookup(%q) is not Builtin", name)
		}
	}
}

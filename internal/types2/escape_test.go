package types2

import (
	"testing"
)

// TestPointerEscapeToReturn tests that *T cannot be returned from functions.
func TestPointerEscapeToReturn(t *testing.T) {
	expectErrors(t, `
package main

func getPointer() *int {
	var x int = 42
	return &x
}
`, "cannot return *T")
}

// TestPointerEscapeToGlobal tests that *T cannot be assigned to global variables.
func TestPointerEscapeToGlobal(t *testing.T) {
	expectErrors(t, `
package main

var global *int

func main() {
	var x int = 42
	global = &x
}
`, "*T cannot escape")
}

// TestPointerEscapeToHeapStructField tests that *T cannot be assigned to heap struct fields.
func TestPointerEscapeToHeapStructField(t *testing.T) {
	expectErrors(t, `
package main

type Container struct {
	ptr *int
}

func main() {
	var x int = 42
	c := new(Container)
	c.ptr = &x
}
`, "*T cannot escape")
}

// TestPointerEscapeToFunctionArg tests that *T cannot be passed to functions
// expecting ref T or stored types.
func TestPointerEscapeToFunctionArg(t *testing.T) {
	expectErrors(t, `
package main

func store(p *int) {
}

var globalPtr *int

func storeGlobal(p *int) {
	globalPtr = p
}

func main() {
	var x int = 42
	storeGlobal(&x)
}
`, "*T cannot")
}

// TestPointerNoEscapeToLocalVar tests that *T can be assigned to local variables.
func TestPointerNoEscapeToLocalVar(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	var x int = 42
	var p *int = &x
	println(*p)
}
`)
}

// TestPointerNoEscapeToStackStructField tests that *T can be assigned to stack struct fields.
func TestPointerNoEscapeToStackStructField(t *testing.T) {
	expectNoErrors(t, `
package main

type Container struct {
	ptr *int
}

func main() {
	var x int = 42
	var c Container
	c.ptr = &x
}
`)
}

// TestRefToPointerForbidden tests that ref T cannot be converted to *T.
// This ensures GC-managed references cannot become stack pointers.
func TestRefCannotConvertToPointer(t *testing.T) {
	// Note: This tests that ref T and *T are distinct types
	expectErrors(t, `
package main

type Node struct {
	value int
}

func main() {
	n := new(Node)
	var p *Node = n
}
`, "cannot")
}

// TestPointerToPointerLocal tests that *T can point to local with nested pointers.
func TestPointerToPointerLocal(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	var x int = 42
	var p1 *int = &x
	var p2 **int = &p1
}
`)
}

// TestNilPointerAssignment tests that nil can be assigned to *T.
func TestNilPointerAssignment(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	var p *int = nil
}
`)
}

// TestNilRefAssignment tests that nil can be assigned to ref T.
func TestNilRefAssignment(t *testing.T) {
	expectNoErrors(t, `
package main

type Node struct {
	value int
}

func main() {
	var n ref Node = nil
}
`)
}

// TestRefCanEscapeToReturn tests that ref T CAN be returned (heap-allocated).
func TestRefCanEscapeToReturn(t *testing.T) {
	expectNoErrors(t, `
package main

type Node struct {
	value int
}

func makeNode() ref Node {
	return new(Node)
}
`)
}

// TestRefCanEscapeToGlobal tests that ref T CAN be stored in globals.
func TestRefCanEscapeToGlobal(t *testing.T) {
	expectNoErrors(t, `
package main

type Node struct {
	value int
}

var globalNode ref Node

func main() {
	globalNode = new(Node)
}
`)
}

// TestRefCanEscapeToStructField tests that ref T CAN be stored in struct fields.
func TestRefCanEscapeToStructField(t *testing.T) {
	expectNoErrors(t, `
package main

type Node struct {
	value int
	next ref Node
}

func setNext(n1 ref Node, n2 ref Node) {
	n1.next = n2
}
`)
}

// TestAddressOfGlobalOK tests that &global produces ref T not *T.
// Globals are not stack-local so they're safe.
func TestAddressOfGlobalVar(t *testing.T) {
	expectNoErrors(t, `
package main

var global int = 42

func main() {
	var p *int = &global
}
`)
}

// TestPointerDereference tests that *p works for pointer types.
func TestPointerDereference(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	var x int = 42
	var p *int = &x
	y := *p
}
`)
}

// TestRefDereference tests that *r works for ref types (field access).
func TestRefFieldAccess(t *testing.T) {
	expectNoErrors(t, `
package main

type Point struct {
	x int
	y int
}

func main() {
	p := new(Point)
	p.x = 10
	p.y = 20
	sum := p.x + p.y
}
`)
}

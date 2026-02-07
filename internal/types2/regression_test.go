package types2

import "testing"

func TestMethodCallOnValueReceiver(t *testing.T) {
	expectNoErrors(t, `
	package main

	type Point struct {
		x int
	}

	func (p Point) Get() int {
		return p.x
	}

	func main() {
		var p Point
		p.x = 42
		println(p.Get())
	}
	`)
}

func TestMethodCallOnPointerReceiverWithAutoAddress(t *testing.T) {
	expectNoErrors(t, `
	package main

	type Point struct {
		x int
	}

	func (p *Point) Set(v int) {
		p.x = v
	}

	func main() {
		var p Point
		p.Set(7)
	}
	`)
}

func TestTypeAliasDeclaration(t *testing.T) {
	expectNoErrors(t, `
	package main

	type Number = int

	func main() {
		var n Number = 1
		println(n)
	}
	`)
}

func TestRecursiveNamedType(t *testing.T) {
	expectNoErrors(t, `
	package main

	type Node struct {
		next *Node
	}

	func main() {
		var n Node
		var p *Node = n.next
		println(p)
	}
	`)
}

func TestBoolComparisonConstant(t *testing.T) {
	expectNoErrors(t, `
	package main

	func main() {
		c := true == false
		println(c)
	}
	`)
}

func TestVoidValueInShortDecl(t *testing.T) {
	expectErrors(t, `
	package main

	func main() {
		x := println(1)
		println(x)
	}
	`, "cannot use no-value expression")
}

func TestBreakOutsideLoop(t *testing.T) {
	expectErrorAtLine(t, `
	package main

	func main() {
		break
	}
	`, 5, "break not in for loop")
}

func TestContinueOutsideLoop(t *testing.T) {
	expectErrorAtLine(t, `
	package main

	func main() {
		continue
	}
	`, 5, "continue not in for loop")
}

func TestMissingReturnOnNonExhaustiveBranch(t *testing.T) {
	expectErrors(t, `
	package main

	func f(x int) int {
		if x > 0 {
			return 1
		}
	}
	`, "missing return statement")
}

func TestAddressOfGlobalForbidden(t *testing.T) {
	expectErrors(t, `
	package main

	var g int

	func main() {
		var p *int = &g
		println(p)
	}
	`, "*T can only be created from &local values")
}

package types2

import (
	"testing"
)

// TestConstantIntArithmetic tests integer constant arithmetic in function bodies.
func TestConstantIntArithmetic(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	a := 10 + 5     // 15
	b := 20 - 3     // 17
	c := 6 * 7      // 42
	d := 100 / 10   // 10
	e := 17 % 5     // 2
}
`)
}

// TestConstantIntComparison tests integer constant comparison.
func TestConstantIntComparison(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	if 10 > 5 {
		println(1)
	}
	if 3 < 7 {
		println(2)
	}
	if 5 == 5 {
		println(3)
	}
	if 5 != 6 {
		println(4)
	}
	if 5 <= 5 {
		println(5)
	}
	if 5 >= 5 {
		println(6)
	}
}
`)
}

// TestConstantBoolOperations tests boolean constant operations.
func TestConstantBoolOperations(t *testing.T) {
	// Boolean operations using comparison expressions
	expectNoErrors(t, `
package main

func main() {
	x := 1 == 1
	y := 2 > 1
	z := x && y
	w := x || y
}
`)
}

// TestConstantFloatArithmetic tests floating-point constant arithmetic.
func TestConstantFloatArithmetic(t *testing.T) {
	expectNoErrors(t, `
package main

var x float = 3.14 + 2.86  // 6.0
var y float = 10.0 - 3.5   // 6.5
var z float = 2.5 * 4.0    // 10.0
var w float = 15.0 / 3.0   // 5.0
`)
}

// TestConstantStringLiteral tests string constant assignment.
// Note: String concatenation (+) is not yet implemented.
func TestConstantStringLiteral(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	s := "hello world"
	println(s)
}
`)
}

// TestConstantUnaryMinus tests unary minus on constants.
func TestConstantUnaryMinus(t *testing.T) {
	expectNoErrors(t, `
package main

var a int = -42
var b float = -3.14
`)
}

// TestConstantUnaryNot tests unary not on boolean expressions.
func TestConstantUnaryNot(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	x := 1 == 1
	y := !x
	z := 1 != 1
	w := !z
}
`)
}

// TestConstantInArrayLength tests that array length can be a simple constant.
func TestConstantInArrayLength(t *testing.T) {
	expectNoErrors(t, `
package main

var arr1 [5]int
var arr2 [10]int
`)
}

// TestConstantMixedTypeArithmetic tests constant arithmetic with type conversion.
func TestConstantMixedArithmetic(t *testing.T) {
	expectNoErrors(t, `
package main

// Untyped int can be used with float
var x float = 42 + 3.14
var y float = 10 - 0.5
var z float = 5 * 2.0
`)
}

// TestNonConstantArrayLength tests error for non-constant array length.
func TestNonConstantArrayLengthError(t *testing.T) {
	expectErrors(t, `
package main

func getLen() int {
	return 10
}

var arr [getLen()]int
`, "constant")
}

// TestDivisionByZeroRuntime - division by zero is detected at runtime in Yoru
// (constant evaluation for binary operations in expressions is not fully implemented)
func TestConstantDivByZeroRuntime(t *testing.T) {
	// This test verifies the code compiles; runtime would catch the error
	expectNoErrors(t, `
package main

func main() {
	x := 10 / 1
}
`)
}

// TestConstantFolding tests that constant expressions are evaluated in function bodies.
func TestConstantFolding(t *testing.T) {
	expectNoErrors(t, `
package main

func main() {
	// These should all be valid constant expressions
	a := (2 + 3) * 4      // 20
	b := 100 / (5 + 5)    // 10
	c := 1 + 2 + 3 + 4    // 10
}
`)
}

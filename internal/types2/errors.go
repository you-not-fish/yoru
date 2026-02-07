// Package types2 implements type checking for the Yoru programming language.
package types2

import (
	"fmt"

	"github.com/you-not-fish/yoru/internal/syntax"
)

// TypeError represents a type checking error.
type TypeError struct {
	Pos syntax.Pos
	Msg string
}

// Error implements the error interface.
func (e *TypeError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Msg)
}

// ErrorHandler is a function called for each type error.
type ErrorHandler func(pos syntax.Pos, msg string)

// errorf reports a type checking error at the given position.
func (c *Checker) errorf(pos syntax.Pos, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)

	if c.errors == 0 {
		c.first = &TypeError{Pos: pos, Msg: msg}
	}
	c.errors++

	if c.conf.Error != nil {
		c.conf.Error(pos, msg)
	}
}

// error reports a type checking error at the current position.
func (c *Checker) error(msg string) {
	c.errorf(c.pos, "%s", msg)
}

// invalidAST reports an invalid AST error.
func (c *Checker) invalidAST(pos syntax.Pos, format string, args ...interface{}) {
	c.errorf(pos, "invalid AST: "+format, args...)
}

// invalidOp reports an invalid operation error.
func (c *Checker) invalidOp(x *operand, format string, args ...interface{}) {
	c.errorf(x.pos, "invalid operation: "+format, args...)
}

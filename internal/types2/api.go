package types2

import (
	"go/constant"

	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
)

// Config specifies the configuration for type checking.
type Config struct {
	// Error is called for each type error.
	// If nil, errors are silently ignored.
	Error ErrorHandler

	// Sizes provides type size and alignment information.
	// If nil, DefaultSizes is used.
	Sizes *types.Sizes
}

// Info holds the results of type checking.
type Info struct {
	// Types maps expressions to their type and value information.
	Types map[syntax.Expr]TypeAndValue

	// Defs maps defining identifiers to their declared objects.
	// For variables declared with var or :=, this maps the Name to the Var.
	// For type declarations, this maps the Name to the TypeName.
	// For function declarations, this maps the Name to the FuncObj.
	Defs map[*syntax.Name]types.Object

	// Uses maps referencing identifiers to their referenced objects.
	// For each Name that references a previously declared object,
	// this maps the Name to that object.
	Uses map[*syntax.Name]types.Object

	// Scopes maps AST nodes to their scopes.
	// This includes File, FuncDecl, BlockStmt, IfStmt, and ForStmt.
	Scopes map[syntax.Node]*types.Scope
}

// TypeAndValue holds the type and value information for an expression.
type TypeAndValue struct {
	Type  types.Type     // expression type
	Value constant.Value // constant value (nil if not constant)
	mode  operandMode    // operand mode
}

// IsVoid reports whether the expression has no value (void function call).
func (tv TypeAndValue) IsVoid() bool {
	return tv.mode == novalue
}

// IsBuiltin reports whether the expression is a built-in function.
func (tv TypeAndValue) IsBuiltin() bool {
	return tv.mode == builtin
}

// IsType reports whether the expression is a type expression.
func (tv TypeAndValue) IsType() bool {
	return tv.mode == typexpr
}

// IsConstant reports whether the expression is a constant.
func (tv TypeAndValue) IsConstant() bool {
	return tv.mode == constant_
}

// IsAddressable reports whether the expression is addressable (variable).
func (tv TypeAndValue) IsAddressable() bool {
	return tv.mode == variable
}

// IsValue reports whether the expression has a value.
func (tv TypeAndValue) IsValue() bool {
	return tv.mode == constant_ || tv.mode == variable || tv.mode == value
}

// Check type-checks a parsed file.
// It returns the package for the file and the first error encountered, if any.
func Check(filename string, file *syntax.File, conf *Config, info *Info) (*types.Package, error) {
	if conf == nil {
		conf = &Config{}
	}
	if conf.Sizes == nil {
		conf.Sizes = types.DefaultSizes
	}

	// Initialize info maps if not provided
	if info != nil {
		if info.Types == nil {
			info.Types = make(map[syntax.Expr]TypeAndValue)
		}
		if info.Defs == nil {
			info.Defs = make(map[*syntax.Name]types.Object)
		}
		if info.Uses == nil {
			info.Uses = make(map[*syntax.Name]types.Object)
		}
		if info.Scopes == nil {
			info.Scopes = make(map[syntax.Node]*types.Scope)
		}
	}

	c := &Checker{
		conf:      conf,
		info:      info,
		funcDecls: make(map[*syntax.FuncDecl]*types.FuncObj),
	}

	c.checkFile(file)

	if c.errors > 0 {
		return c.pkg, c.first
	}
	return c.pkg, nil
}

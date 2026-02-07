// Package syntax implements lexical and syntactic analysis for the Yoru programming language.
package syntax

// ----------------------------------------------------------------------------
// Interfaces
//
// There are 3 main classes of nodes: Expressions, Statements, and Declarations.
// All nodes implement the Node interface. Expression, Statement, and Declaration
// nodes further implement their respective interfaces.

// Node is the interface implemented by all AST nodes.
type Node interface {
	Pos() Pos // position of first character belonging to the node
	End() Pos // position of first character immediately after the node
	aNode()   // marker method to restrict implementations to this package
}

// Expr is the interface for all expression nodes.
type Expr interface {
	Node
	aExpr()
}

// Stmt is the interface for all statement nodes.
type Stmt interface {
	Node
	aStmt()
}

// Decl is the interface for all declaration nodes.
type Decl interface {
	Node
	aDecl()
}

// ----------------------------------------------------------------------------
// Base node types

// node is the base struct embedded in all AST nodes.
type node struct {
	pos Pos
}

func (n *node) Pos() Pos { return n.pos }
func (n *node) End() Pos { return n.pos } // default: return start position
func (n *node) aNode()   {}

// expr is embedded in all expression nodes.
type expr struct{ node }

func (*expr) aExpr() {}

// stmt is embedded in all statement nodes.
type stmt struct{ node }

func (*stmt) aStmt() {}

// decl is embedded in all declaration nodes.
type decl struct{ node }

func (*decl) aDecl() {}

// ----------------------------------------------------------------------------
// Files and Declarations

// File represents a complete source file.
type File struct {
	node
	PkgName *Name         // package name
	Imports []*ImportDecl // import declarations
	Decls   []Decl        // top-level declarations
}

// ImportDecl represents an import declaration: import "path"
type ImportDecl struct {
	decl
	Path *BasicLit // import path (StringLit)
}

// TypeDecl represents a type declaration.
// type Name Type (definition) or type Name = Type (alias)
type TypeDecl struct {
	decl
	Name  *Name // type name
	Alias bool  // true for type alias (type T = U)
	Type  Expr  // the type expression
}

// VarDecl represents a variable declaration: var Name Type = Value
type VarDecl struct {
	decl
	Name  *Name // variable name
	Type  Expr  // explicit type (nil if inferred)
	Value Expr  // initial value (nil if none)
}

// FuncDecl represents a function or method declaration.
// func (Recv) Name(Params) Result { Body }
type FuncDecl struct {
	decl
	Recv   *Field     // receiver (nil for functions)
	Name   *Name      // function name
	Params []*Field   // parameter list
	Result Expr       // return type (nil for void)
	Body   *BlockStmt // function body
}

// Field represents a named field in a struct, parameter list, or receiver.
type Field struct {
	node
	Name *Name // field name (nil for anonymous fields, not used in Yoru)
	Type Expr  // field type
}

// ----------------------------------------------------------------------------
// Expressions

// Name represents an identifier.
type Name struct {
	expr
	Value string // identifier string
}

// BasicLit represents a literal value (int, float, string).
type BasicLit struct {
	expr
	Value string  // literal text (decoded for strings)
	Kind  LitKind // IntLit, FloatLit, StringLit
}

// Operation represents a unary or binary operation.
// For unary operations, Y is nil.
// For binary operations, both X and Y are set.
type Operation struct {
	expr
	Op Token // operator token
	X  Expr  // left operand (or only operand for unary)
	Y  Expr  // right operand (nil for unary)
}

// CallExpr represents a function call: Fun(Args...)
type CallExpr struct {
	expr
	Fun  Expr   // function expression
	Args []Expr // argument list
}

// IndexExpr represents an index expression: X[Index]
type IndexExpr struct {
	expr
	X     Expr // indexed expression (array or pointer)
	Index Expr // index expression
}

// SelectorExpr represents a selector expression: X.Sel
type SelectorExpr struct {
	expr
	X   Expr  // receiver expression
	Sel *Name // field/method name
}

// ParenExpr represents a parenthesized expression: (X)
type ParenExpr struct {
	expr
	X Expr // inner expression
}

// NewExpr represents heap allocation: new(Type)
type NewExpr struct {
	expr
	Type Expr // type to allocate
}

// CompositeLit represents a composite literal: Type{Elems...}
// Used for struct literals.
type CompositeLit struct {
	expr
	Type  Expr   // type (required in Yoru)
	Elems []Expr // elements (can be KeyValueExpr)
}

// KeyValueExpr represents a key:value pair in composite literals.
type KeyValueExpr struct {
	expr
	Key   Expr // field name
	Value Expr // field value
}

// ----------------------------------------------------------------------------
// Type Expressions

// ArrayType represents an array type: [Len]Elem
type ArrayType struct {
	expr
	Len  Expr // length expression (must be constant)
	Elem Expr // element type
}

// PointerType represents a pointer type: *Base
type PointerType struct {
	expr
	Base Expr // base type
}

// RefType represents a GC-managed reference type: ref Base
type RefType struct {
	expr
	Base Expr // base type
}

// StructType represents a struct type: struct { Fields... }
type StructType struct {
	expr
	Fields []*Field // field declarations
}

// ----------------------------------------------------------------------------
// Statements

// EmptyStmt represents an empty statement (just a semicolon).
type EmptyStmt struct {
	stmt
}

// ExprStmt represents an expression used as a statement.
type ExprStmt struct {
	stmt
	X Expr // expression
}

// AssignStmt represents an assignment: LHS = RHS or LHS := RHS
type AssignStmt struct {
	stmt
	Op  Token  // _Assign or _Define
	LHS []Expr // left-hand side expressions
	RHS []Expr // right-hand side expressions
}

// BlockStmt represents a block statement: { Stmts... }
type BlockStmt struct {
	stmt
	Stmts  []Stmt // statements
	Rbrace Pos    // position of closing brace
}

// IfStmt represents an if statement: if Cond Then [else Else]
type IfStmt struct {
	stmt
	Cond Expr       // condition expression
	Then *BlockStmt // then branch
	Else Stmt       // else branch (nil, *IfStmt, or *BlockStmt)
}

// ForStmt represents a for statement: for Cond { Body }
// Note: Yoru only supports "for cond {}" form (no init/post, no range).
type ForStmt struct {
	stmt
	Cond Expr       // condition (nil only when recovering from syntax errors)
	Body *BlockStmt // loop body
}

// ReturnStmt represents a return statement: return [Result]
type ReturnStmt struct {
	stmt
	Result Expr // return value (nil for bare return)
}

// BranchStmt represents a break or continue statement.
type BranchStmt struct {
	stmt
	Tok Token // _Break or _Continue
}

// DeclStmt wraps a declaration as a statement.
// Used for variable declarations inside function bodies.
type DeclStmt struct {
	stmt
	Decl Decl // the wrapped declaration
}

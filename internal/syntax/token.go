// Package syntax implements lexical analysis for the Yoru programming language.
package syntax

import "fmt"

// Token represents the type of a lexical token.
type Token uint

const (
	// Special tokens
	_EOF   Token = iota // end of file
	_Error              // lexical error

	// Literals
	_Name    // identifier: foo, bar, Rectangle
	_Literal // literal value (used with LitKind)

	// Operators (ordered by precedence, low to high)
	// Assignment
	_Assign // =
	_Define // :=

	// Logical operators
	_OrOr   // ||
	_AndAnd // &&

	// Comparison operators
	_Eql // ==
	_Neq // !=
	_Lss // <
	_Leq // <=
	_Gtr // >
	_Geq // >=

	// Arithmetic operators (additive)
	_Add // +
	_Sub // -
	_Or  // |
	_Xor // ^

	// Arithmetic operators (multiplicative)
	_Mul // *
	_Div // /
	_Rem // %
	_And // &
	_Shl // <<
	_Shr // >>

	// Unary operators
	_Not // !

	// Delimiters
	_Lparen // (
	_Rparen // )
	_Lbrack // [
	_Rbrack // ]
	_Lbrace // {
	_Rbrace // }
	_Comma  // ,
	_Semi   // ;
	_Colon  // :
	_Dot    // .

	// Keywords
	_Break
	_Continue
	_Else
	_For
	_Func
	_If
	_Import
	_New
	_Package
	_Panic
	_Ref
	_Return
	_Struct
	_Type
	_Var

	tokenCount
)

// tokenNames maps tokens to their string representation.
var tokenNames = [...]string{
	_EOF:   "EOF",
	_Error: "ERROR",

	_Name:    "NAME",
	_Literal: "LITERAL",

	_Assign: "=",
	_Define: ":=",

	_OrOr:   "||",
	_AndAnd: "&&",

	_Eql: "==",
	_Neq: "!=",
	_Lss: "<",
	_Leq: "<=",
	_Gtr: ">",
	_Geq: ">=",

	_Add: "+",
	_Sub: "-",
	_Or:  "|",
	_Xor: "^",

	_Mul: "*",
	_Div: "/",
	_Rem: "%",
	_And: "&",
	_Shl: "<<",
	_Shr: ">>",

	_Not: "!",

	_Lparen: "(",
	_Rparen: ")",
	_Lbrack: "[",
	_Rbrack: "]",
	_Lbrace: "{",
	_Rbrace: "}",
	_Comma:  ",",
	_Semi:   ";",
	_Colon:  ":",
	_Dot:    ".",

	_Break:    "break",
	_Continue: "continue",
	_Else:     "else",
	_For:      "for",
	_Func:     "func",
	_If:       "if",
	_Import:   "import",
	_New:      "new",
	_Package:  "package",
	_Panic:    "panic",
	_Ref:      "ref",
	_Return:   "return",
	_Struct:   "struct",
	_Type:     "type",
	_Var:      "var",
}

// String returns the string representation of the token.
func (t Token) String() string {
	if t < tokenCount {
		return tokenNames[t]
	}
	return fmt.Sprintf("token(%d)", t)
}

// Precedence returns the operator precedence for binary operators.
// Returns 0 for non-operators.
// Precedence levels (higher = binds tighter):
//
//	1: ||
//	2: &&
//	3: == != < <= > >=
//	4: + - | ^
//	5: * / % & << >>
func (t Token) Precedence() int {
	switch t {
	case _OrOr:
		return 1
	case _AndAnd:
		return 2
	case _Eql, _Neq, _Lss, _Leq, _Gtr, _Geq:
		return 3
	case _Add, _Sub, _Or, _Xor:
		return 4
	case _Mul, _Div, _Rem, _And, _Shl, _Shr:
		return 5
	}
	return 0
}

// IsKeyword reports whether t is a keyword token.
func (t Token) IsKeyword() bool {
	return t >= _Break && t <= _Var
}

// IsLiteral reports whether t is a literal token (_Name or _Literal).
func (t Token) IsLiteral() bool {
	return t == _Literal
}

// IsOperator reports whether t is an operator token.
func (t Token) IsOperator() bool {
	return t >= _Assign && t <= _Not
}

// IsEOF reports whether t is the EOF token.
func (t Token) IsEOF() bool {
	return t == _EOF
}

// IsDefine reports whether t is the define operator (:=).
func (t Token) IsDefine() bool {
	return t == _Define
}

// IsAssign reports whether t is the assignment operator (=).
func (t Token) IsAssign() bool {
	return t == _Assign
}

// Exported operator tokens for type checker access
const (
	Not Token = _Not // !
	Sub Token = _Sub // -
	And Token = _And // &
	Mul Token = _Mul // *
)

// LitKind represents the kind of a literal token.
type LitKind uint8

const (
	IntLit    LitKind = iota // 123, 0x1F, 0o77, 0b1010
	FloatLit                 // 3.14, 1e10, 2.5e-3
	StringLit                // "hello", "line\n"
)

// litKindNames maps literal kinds to their string representation.
var litKindNames = [...]string{
	IntLit:    "int",
	FloatLit:  "float",
	StringLit: "string",
}

// String returns the string representation of the literal kind.
func (k LitKind) String() string {
	if k <= StringLit {
		return litKindNames[k]
	}
	return fmt.Sprintf("LitKind(%d)", k)
}

// keywords maps keyword strings to their token type.
// Note: Pre-declared identifiers (int, float, bool, string, true, false, nil, println)
// are NOT keywords - they are scanned as _Name and bound in the Universe during Phase 3.
var keywords = map[string]Token{
	"break":    _Break,
	"continue": _Continue,
	"else":     _Else,
	"for":      _For,
	"func":     _Func,
	"if":       _If,
	"import":   _Import,
	"new":      _New,
	"package":  _Package,
	"panic":    _Panic,
	"ref":      _Ref,
	"return":   _Return,
	"struct":   _Struct,
	"type":     _Type,
	"var":      _Var,
}

// LookupKeyword returns the token for the given identifier string.
// If the identifier is a keyword, returns the keyword token.
// Otherwise, returns _Name.
func LookupKeyword(ident string) Token {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return _Name
}

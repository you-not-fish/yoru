package syntax

import (
	"strings"
	"testing"
)

func TestTokenString(t *testing.T) {
	tests := []struct {
		tok  Token
		want string
	}{
		// Special tokens
		{_EOF, "EOF"},
		{_Error, "ERROR"},

		// Literals
		{_Name, "NAME"},
		{_Literal, "LITERAL"},

		// Operators
		{_Assign, "="},
		{_Define, ":="},
		{_OrOr, "||"},
		{_AndAnd, "&&"},
		{_Eql, "=="},
		{_Neq, "!="},
		{_Lss, "<"},
		{_Leq, "<="},
		{_Gtr, ">"},
		{_Geq, ">="},
		{_Add, "+"},
		{_Sub, "-"},
		{_Or, "|"},
		{_Xor, "^"},
		{_Mul, "*"},
		{_Div, "/"},
		{_Rem, "%"},
		{_And, "&"},
		{_Shl, "<<"},
		{_Shr, ">>"},
		{_Not, "!"},

		// Delimiters
		{_Lparen, "("},
		{_Rparen, ")"},
		{_Lbrack, "["},
		{_Rbrack, "]"},
		{_Lbrace, "{"},
		{_Rbrace, "}"},
		{_Comma, ","},
		{_Semi, ";"},
		{_Colon, ":"},
		{_Dot, "."},

		// Keywords
		{_Break, "break"},
		{_Continue, "continue"},
		{_Else, "else"},
		{_For, "for"},
		{_Func, "func"},
		{_If, "if"},
		{_Import, "import"},
		{_New, "new"},
		{_Package, "package"},
		{_Panic, "panic"},
		{_Ref, "ref"},
		{_Return, "return"},
		{_Struct, "struct"},
		{_Type, "type"},
		{_Var, "var"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.tok.String(); got != tt.want {
				t.Errorf("Token(%d).String() = %q, want %q", tt.tok, got, tt.want)
			}
		})
	}
}

func TestTokenStringUnknown(t *testing.T) {
	tok := Token(999)
	got := tok.String()
	if !strings.HasPrefix(got, "token(") {
		t.Errorf("unknown token string = %q, want prefix 'token('", got)
	}
}

func TestTokenPrecedence(t *testing.T) {
	tests := []struct {
		tok  Token
		want int
	}{
		// Non-operators have precedence 0
		{_EOF, 0},
		{_Name, 0},
		{_Literal, 0},
		{_Assign, 0},
		{_Define, 0},
		{_Lparen, 0},

		// Precedence 1: ||
		{_OrOr, 1},

		// Precedence 2: &&
		{_AndAnd, 2},

		// Precedence 3: comparison
		{_Eql, 3},
		{_Neq, 3},
		{_Lss, 3},
		{_Leq, 3},
		{_Gtr, 3},
		{_Geq, 3},

		// Precedence 4: additive
		{_Add, 4},
		{_Sub, 4},
		{_Or, 4},
		{_Xor, 4},

		// Precedence 5: multiplicative
		{_Mul, 5},
		{_Div, 5},
		{_Rem, 5},
		{_And, 5},
		{_Shl, 5},
		{_Shr, 5},
	}

	for _, tt := range tests {
		t.Run(tt.tok.String(), func(t *testing.T) {
			if got := tt.tok.Precedence(); got != tt.want {
				t.Errorf("Token(%v).Precedence() = %d, want %d", tt.tok, got, tt.want)
			}
		})
	}
}

func TestTokenIsKeyword(t *testing.T) {
	keywords := []Token{
		_Break, _Continue, _Else, _For, _Func, _If, _Import,
		_New, _Package, _Panic, _Ref, _Return, _Struct, _Type, _Var,
	}

	nonKeywords := []Token{
		_EOF, _Error, _Name, _Literal, _Assign, _Define,
		_Add, _Sub, _Lparen, _Rparen,
	}

	for _, tok := range keywords {
		if !tok.IsKeyword() {
			t.Errorf("%v.IsKeyword() = false, want true", tok)
		}
	}

	for _, tok := range nonKeywords {
		if tok.IsKeyword() {
			t.Errorf("%v.IsKeyword() = true, want false", tok)
		}
	}
}

func TestTokenIsLiteral(t *testing.T) {
	if !_Literal.IsLiteral() {
		t.Error("_Literal.IsLiteral() = false, want true")
	}

	nonLiterals := []Token{_EOF, _Name, _Assign, _Func}
	for _, tok := range nonLiterals {
		if tok.IsLiteral() {
			t.Errorf("%v.IsLiteral() = true, want false", tok)
		}
	}
}

func TestTokenIsOperator(t *testing.T) {
	operators := []Token{
		_Assign, _Define, _OrOr, _AndAnd,
		_Eql, _Neq, _Lss, _Leq, _Gtr, _Geq,
		_Add, _Sub, _Or, _Xor,
		_Mul, _Div, _Rem, _And, _Shl, _Shr,
		_Not,
	}

	nonOperators := []Token{
		_EOF, _Error, _Name, _Literal,
		_Lparen, _Rparen, _Lbrack, _Rbrack,
		_Func, _If, _For,
	}

	for _, tok := range operators {
		if !tok.IsOperator() {
			t.Errorf("%v.IsOperator() = false, want true", tok)
		}
	}

	for _, tok := range nonOperators {
		if tok.IsOperator() {
			t.Errorf("%v.IsOperator() = true, want false", tok)
		}
	}
}

func TestTokenIsEOF(t *testing.T) {
	if !_EOF.IsEOF() {
		t.Error("_EOF.IsEOF() = false, want true")
	}

	nonEOF := []Token{_Error, _Name, _Literal, _Func}
	for _, tok := range nonEOF {
		if tok.IsEOF() {
			t.Errorf("%v.IsEOF() = true, want false", tok)
		}
	}
}

func TestLitKindString(t *testing.T) {
	tests := []struct {
		kind LitKind
		want string
	}{
		{IntLit, "int"},
		{FloatLit, "float"},
		{StringLit, "string"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("LitKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestLitKindStringUnknown(t *testing.T) {
	kind := LitKind(99)
	got := kind.String()
	if !strings.HasPrefix(got, "LitKind(") {
		t.Errorf("unknown LitKind string = %q, want prefix 'LitKind('", got)
	}
}

func TestLookupKeyword(t *testing.T) {
	// Test all keywords
	keywordTests := []struct {
		ident string
		want  Token
	}{
		{"break", _Break},
		{"continue", _Continue},
		{"else", _Else},
		{"for", _For},
		{"func", _Func},
		{"if", _If},
		{"import", _Import},
		{"new", _New},
		{"package", _Package},
		{"panic", _Panic},
		{"ref", _Ref},
		{"return", _Return},
		{"struct", _Struct},
		{"type", _Type},
		{"var", _Var},
	}

	for _, tt := range keywordTests {
		t.Run(tt.ident, func(t *testing.T) {
			if got := LookupKeyword(tt.ident); got != tt.want {
				t.Errorf("LookupKeyword(%q) = %v, want %v", tt.ident, got, tt.want)
			}
		})
	}
}

func TestLookupKeywordNonKeyword(t *testing.T) {
	// Pre-declared identifiers should NOT be keywords
	// They should return _Name
	nonKeywords := []string{
		"int", "float", "bool", "string",
		"true", "false", "nil", "println",
		"foo", "bar", "Rectangle", "_underscore",
	}

	for _, ident := range nonKeywords {
		t.Run(ident, func(t *testing.T) {
			if got := LookupKeyword(ident); got != _Name {
				t.Errorf("LookupKeyword(%q) = %v, want _Name", ident, got)
			}
		})
	}
}

func TestKeywordCount(t *testing.T) {
	// Verify we have exactly 15 keywords
	expectedCount := 15
	count := 0
	for tok := _Break; tok <= _Var; tok++ {
		count++
	}
	if count != expectedCount {
		t.Errorf("keyword count = %d, want %d", count, expectedCount)
	}

	// Also verify the map has the same count
	if len(keywords) != expectedCount {
		t.Errorf("keywords map size = %d, want %d", len(keywords), expectedCount)
	}
}

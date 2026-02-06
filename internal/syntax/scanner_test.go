package syntax

import (
	"strings"
	"testing"
)

func TestScanTokens(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		tokens []Token
		lits   []string
	}{
		// Identifiers (ASI inserts ; at EOF for _Name)
		{"ident", "foo", []Token{_Name, _Semi}, []string{"foo", "EOF"}},
		{"ident_underscore", "_bar", []Token{_Name, _Semi}, []string{"_bar", "EOF"}},
		{"ident_mixed", "foo123", []Token{_Name, _Semi}, []string{"foo123", "EOF"}},
		{"ident_caps", "FooBar", []Token{_Name, _Semi}, []string{"FooBar", "EOF"}},

		// Pre-declared identifiers (should be _Name, not keywords)
		{"predecl_int", "int", []Token{_Name, _Semi}, []string{"int", "EOF"}},
		{"predecl_float", "float", []Token{_Name, _Semi}, []string{"float", "EOF"}},
		{"predecl_bool", "bool", []Token{_Name, _Semi}, []string{"bool", "EOF"}},
		{"predecl_string", "string", []Token{_Name, _Semi}, []string{"string", "EOF"}},
		{"predecl_true", "true", []Token{_Name, _Semi}, []string{"true", "EOF"}},
		{"predecl_false", "false", []Token{_Name, _Semi}, []string{"false", "EOF"}},
		{"predecl_nil", "nil", []Token{_Name, _Semi}, []string{"nil", "EOF"}},
		{"predecl_println", "println", []Token{_Name, _Semi}, []string{"println", "EOF"}},

		// Integer literals (ASI inserts ; at EOF for _Literal)
		{"int_dec", "123", []Token{_Literal, _Semi}, []string{"123", "EOF"}},
		{"int_zero", "0", []Token{_Literal, _Semi}, []string{"0", "EOF"}},
		{"int_hex_lower", "0x1f", []Token{_Literal, _Semi}, []string{"0x1f", "EOF"}},
		{"int_hex_upper", "0X1F", []Token{_Literal, _Semi}, []string{"0X1F", "EOF"}},
		{"int_hex_mixed", "0xDeAdBeEf", []Token{_Literal, _Semi}, []string{"0xDeAdBeEf", "EOF"}},
		{"int_oct_lower", "0o77", []Token{_Literal, _Semi}, []string{"0o77", "EOF"}},
		{"int_oct_upper", "0O77", []Token{_Literal, _Semi}, []string{"0O77", "EOF"}},
		{"int_bin_lower", "0b1010", []Token{_Literal, _Semi}, []string{"0b1010", "EOF"}},
		{"int_bin_upper", "0B1010", []Token{_Literal, _Semi}, []string{"0B1010", "EOF"}},
		{"int_leading_zero", "007", []Token{_Literal, _Semi}, []string{"007", "EOF"}},

		// Float literals
		{"float_simple", "3.14", []Token{_Literal, _Semi}, []string{"3.14", "EOF"}},
		{"float_no_frac", "3.", []Token{_Literal, _Semi}, []string{"3.", "EOF"}},
		{"float_exp_lower", "1e10", []Token{_Literal, _Semi}, []string{"1e10", "EOF"}},
		{"float_exp_upper", "1E10", []Token{_Literal, _Semi}, []string{"1E10", "EOF"}},
		{"float_exp_pos", "1e+10", []Token{_Literal, _Semi}, []string{"1e+10", "EOF"}},
		{"float_exp_neg", "2.5e-3", []Token{_Literal, _Semi}, []string{"2.5e-3", "EOF"}},
		{"float_zero_exp", "0e0", []Token{_Literal, _Semi}, []string{"0e0", "EOF"}},

		// String literals (decoded content)
		{"string_simple", `"hello"`, []Token{_Literal, _Semi}, []string{"hello", "EOF"}},
		{"string_empty", `""`, []Token{_Literal, _Semi}, []string{"", "EOF"}},
		{"string_escape_n", `"a\nb"`, []Token{_Literal, _Semi}, []string{"a\nb", "EOF"}},
		{"string_escape_t", `"a\tb"`, []Token{_Literal, _Semi}, []string{"a\tb", "EOF"}},
		{"string_escape_r", `"a\rb"`, []Token{_Literal, _Semi}, []string{"a\rb", "EOF"}},
		{"string_escape_backslash", `"a\\b"`, []Token{_Literal, _Semi}, []string{"a\\b", "EOF"}},
		{"string_escape_quote", `"a\"b"`, []Token{_Literal, _Semi}, []string{"a\"b", "EOF"}},
		{"string_escape_zero", `"a\0b"`, []Token{_Literal, _Semi}, []string{"a\x00b", "EOF"}},
		{"string_escape_hex", `"\x41\x42"`, []Token{_Literal, _Semi}, []string{"AB", "EOF"}},

		// Single-char operators (no ASI for most operators)
		{"op_add", "+", []Token{_Add}, []string{"+"}},
		{"op_sub", "-", []Token{_Sub}, []string{"-"}},
		{"op_mul", "*", []Token{_Mul}, []string{"*"}},
		{"op_div", "/", []Token{_Div}, []string{"/"}},
		{"op_rem", "%", []Token{_Rem}, []string{"%"}},
		{"op_and", "&", []Token{_And}, []string{"&"}},
		{"op_or", "|", []Token{_Or}, []string{"|"}},
		{"op_xor", "^", []Token{_Xor}, []string{"^"}},
		{"op_not", "!", []Token{_Not}, []string{"!"}},
		{"op_lss", "<", []Token{_Lss}, []string{"<"}},
		{"op_gtr", ">", []Token{_Gtr}, []string{">"}},
		{"op_assign", "=", []Token{_Assign}, []string{"="}},
		{"op_colon", ":", []Token{_Colon}, []string{":"}},

		// Two-char operators
		{"op_andand", "&&", []Token{_AndAnd}, []string{"&&"}},
		{"op_oror", "||", []Token{_OrOr}, []string{"||"}},
		{"op_eql", "==", []Token{_Eql}, []string{"=="}},
		{"op_neq", "!=", []Token{_Neq}, []string{"!="}},
		{"op_leq", "<=", []Token{_Leq}, []string{"<="}},
		{"op_geq", ">=", []Token{_Geq}, []string{">="}},
		{"op_shl", "<<", []Token{_Shl}, []string{"<<"}},
		{"op_shr", ">>", []Token{_Shr}, []string{">>"}},
		{"op_define", ":=", []Token{_Define}, []string{":="}},

		// Delimiters (ASI for ), ], })
		{"delim_lparen", "(", []Token{_Lparen}, []string{"("}},
		{"delim_rparen", ")", []Token{_Rparen, _Semi}, []string{")", "EOF"}},
		{"delim_lbrack", "[", []Token{_Lbrack}, []string{"["}},
		{"delim_rbrack", "]", []Token{_Rbrack, _Semi}, []string{"]", "EOF"}},
		{"delim_lbrace", "{", []Token{_Lbrace}, []string{"{"}},
		{"delim_rbrace", "}", []Token{_Rbrace, _Semi}, []string{"}", "EOF"}},
		{"delim_comma", ",", []Token{_Comma}, []string{","}},
		{"delim_semi", ";", []Token{_Semi}, []string{";"}},
		{"delim_dot", ".", []Token{_Dot}, []string{"."}},

		// Keywords (ASI for break, continue, return)
		{"kw_break", "break", []Token{_Break, _Semi}, []string{"break", "EOF"}},
		{"kw_continue", "continue", []Token{_Continue, _Semi}, []string{"continue", "EOF"}},
		{"kw_else", "else", []Token{_Else}, []string{"else"}},
		{"kw_for", "for", []Token{_For}, []string{"for"}},
		{"kw_func", "func", []Token{_Func}, []string{"func"}},
		{"kw_if", "if", []Token{_If}, []string{"if"}},
		{"kw_import", "import", []Token{_Import}, []string{"import"}},
		{"kw_new", "new", []Token{_New}, []string{"new"}},
		{"kw_package", "package", []Token{_Package}, []string{"package"}},
		{"kw_panic", "panic", []Token{_Panic}, []string{"panic"}},
		{"kw_ref", "ref", []Token{_Ref}, []string{"ref"}},
		{"kw_return", "return", []Token{_Return, _Semi}, []string{"return", "EOF"}},
		{"kw_struct", "struct", []Token{_Struct}, []string{"struct"}},
		{"kw_type", "type", []Token{_Type}, []string{"type"}},
		{"kw_var", "var", []Token{_Var}, []string{"var"}},

		// Compound expressions (last token triggers ASI)
		{"expr_add", "1 + 2", []Token{_Literal, _Add, _Literal, _Semi}, []string{"1", "+", "2", "EOF"}},
		{"expr_call", "foo()", []Token{_Name, _Lparen, _Rparen, _Semi}, []string{"foo", "(", ")", "EOF"}},
		{"expr_index", "arr[0]", []Token{_Name, _Lbrack, _Literal, _Rbrack, _Semi}, []string{"arr", "[", "0", "]", "EOF"}},
		{"expr_selector", "p.x", []Token{_Name, _Dot, _Name, _Semi}, []string{"p", ".", "x", "EOF"}},
		{"expr_compare", "a == b", []Token{_Name, _Eql, _Name, _Semi}, []string{"a", "==", "b", "EOF"}},
		{"expr_logical", "a && b || c", []Token{_Name, _AndAnd, _Name, _OrOr, _Name, _Semi}, []string{"a", "&&", "b", "||", "c", "EOF"}},
		{"expr_assign", "x := 1", []Token{_Name, _Define, _Literal, _Semi}, []string{"x", ":=", "1", "EOF"}},

		// Comments
		{"comment_skip", "a // comment\nb", []Token{_Name, _Semi, _Name, _Semi}, []string{"a", "newline", "b", "EOF"}},
		{"comment_eof", "a // comment", []Token{_Name, _Semi}, []string{"a", "EOF"}},

		// Whitespace handling
		{"whitespace_spaces", "  a  ", []Token{_Name, _Semi}, []string{"a", "EOF"}},
		{"whitespace_tabs", "\ta\t", []Token{_Name, _Semi}, []string{"a", "EOF"}},
		{"whitespace_mixed", " \t a \t ", []Token{_Name, _Semi}, []string{"a", "EOF"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScanner("test", strings.NewReader(tt.src), nil)
			for i, wantTok := range tt.tokens {
				s.Next()
				if s.Token() != wantTok {
					t.Errorf("token %d: got %v, want %v", i, s.Token(), wantTok)
				}
				if tt.lits != nil && tt.lits[i] != "" {
					if s.Literal() != tt.lits[i] {
						t.Errorf("literal %d: got %q, want %q", i, s.Literal(), tt.lits[i])
					}
				}
			}
			s.Next()
			if !s.Token().IsEOF() {
				t.Errorf("expected EOF, got %v %q", s.Token(), s.Literal())
			}
		})
	}
}

func TestScanLitKind(t *testing.T) {
	tests := []struct {
		src  string
		kind LitKind
	}{
		{"123", IntLit},
		{"0x1F", IntLit},
		{"0o77", IntLit},
		{"0b1010", IntLit},
		{"3.14", FloatLit},
		{"1e10", FloatLit},
		{"2.5e-3", FloatLit},
		{`"hello"`, StringLit},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			s := NewScanner("test", strings.NewReader(tt.src), nil)
			s.Next()
			if s.Token() != _Literal {
				t.Fatalf("expected _Literal, got %v", s.Token())
			}
			if s.LitKind() != tt.kind {
				t.Errorf("LitKind = %v, want %v", s.LitKind(), tt.kind)
			}
		})
	}
}

func TestASI(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		tokens []Token
		lits   []string
	}{
		// Identifier followed by newline
		{
			"ident_newline",
			"foo\nbar",
			[]Token{_Name, _Semi, _Name},
			[]string{"foo", "newline", "bar"},
		},
		// Literal followed by newline
		{
			"literal_newline",
			"123\n456",
			[]Token{_Literal, _Semi, _Literal},
			[]string{"123", "newline", "456"},
		},
		// return followed by newline
		{
			"return_newline",
			"return\n1",
			[]Token{_Return, _Semi, _Literal},
			[]string{"return", "newline", "1"},
		},
		// break followed by newline
		{
			"break_newline",
			"break\nfoo",
			[]Token{_Break, _Semi, _Name},
			[]string{"break", "newline", "foo"},
		},
		// continue followed by newline
		{
			"continue_newline",
			"continue\nfoo",
			[]Token{_Continue, _Semi, _Name},
			[]string{"continue", "newline", "foo"},
		},
		// ) followed by newline
		{
			"rparen_newline",
			"foo()\nbar",
			[]Token{_Name, _Lparen, _Rparen, _Semi, _Name},
			[]string{"foo", "(", ")", "newline", "bar"},
		},
		// ] followed by newline
		{
			"rbrack_newline",
			"arr[0]\nbar",
			[]Token{_Name, _Lbrack, _Literal, _Rbrack, _Semi, _Name},
			[]string{"arr", "[", "0", "]", "newline", "bar"},
		},
		// } followed by newline
		{
			"rbrace_newline",
			"{\n}\nfoo",
			[]Token{_Lbrace, _Rbrace, _Semi, _Name},
			[]string{"{", "}", "newline", "foo"},
		},
		// + followed by newline (no ASI)
		{
			"add_newline",
			"1 +\n2",
			[]Token{_Literal, _Add, _Literal},
			[]string{"1", "+", "2"},
		},
		// = followed by newline (no ASI)
		{
			"assign_newline",
			"x =\n1",
			[]Token{_Name, _Assign, _Literal},
			[]string{"x", "=", "1"},
		},
		// , followed by newline (no ASI)
		{
			"comma_newline",
			"a,\nb",
			[]Token{_Name, _Comma, _Name},
			[]string{"a", ",", "b"},
		},
		// ASI at EOF
		{
			"ident_eof",
			"foo",
			[]Token{_Name, _Semi},
			[]string{"foo", "EOF"},
		},
		// Multiple newlines
		{
			"multiple_newlines",
			"foo\n\n\nbar",
			[]Token{_Name, _Semi, _Name},
			[]string{"foo", "newline", "bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScanner("test", strings.NewReader(tt.src), nil)
			for i, wantTok := range tt.tokens {
				s.Next()
				if s.Token() != wantTok {
					t.Errorf("token %d: got %v, want %v", i, s.Token(), wantTok)
				}
				if tt.lits[i] != "" {
					if s.Literal() != tt.lits[i] {
						t.Errorf("literal %d: got %q, want %q", i, s.Literal(), tt.lits[i])
					}
				}
			}
		})
	}
}

func TestASIDisabled(t *testing.T) {
	src := "foo\nbar"
	s := NewScanner("test", strings.NewReader(src), nil)
	s.SetASIEnabled(false)

	// Without ASI, newlines are just skipped
	s.Next()
	if s.Token() != _Name || s.Literal() != "foo" {
		t.Errorf("got %v %q, want NAME foo", s.Token(), s.Literal())
	}

	s.Next()
	if s.Token() != _Name || s.Literal() != "bar" {
		t.Errorf("got %v %q, want NAME bar", s.Token(), s.Literal())
	}

	s.Next()
	if !s.Token().IsEOF() {
		t.Errorf("expected EOF, got %v", s.Token())
	}
}

func TestPosition(t *testing.T) {
	src := `package main

func foo() {
    x := 123
}`

	expected := []struct {
		tok  Token
		line uint32
		col  uint32
	}{
		{_Package, 1, 1},
		{_Name, 1, 9},      // main
		{_Semi, 1, 13},     // ASI at newline
		{_Func, 3, 1},      // after blank line
		{_Name, 3, 6},      // foo
		{_Lparen, 3, 9},    // (
		{_Rparen, 3, 10},   // )
		{_Lbrace, 3, 12},   // {
		{_Name, 4, 5},      // x
		{_Define, 4, 7},    // :=
		{_Literal, 4, 10},  // 123
		{_Semi, 4, 13},     // ASI
		{_Rbrace, 5, 1},    // }
		{_Semi, 5, 2},      // ASI at EOF
	}

	s := NewScanner("test.yoru", strings.NewReader(src), nil)
	for i, exp := range expected {
		s.Next()
		pos := s.Pos()
		if s.Token() != exp.tok {
			t.Errorf("token %d: got %v, want %v", i, s.Token(), exp.tok)
		}
		if pos.Line() != exp.line || pos.Col() != exp.col {
			t.Errorf("token %d (%v): pos = %d:%d, want %d:%d",
				i, s.Token(), pos.Line(), pos.Col(), exp.line, exp.col)
		}
	}
}

func TestScanErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string
	}{
		{"unterminated_string", `"hello`, "string not terminated"},
		{"bad_escape", `"\q"`, "unknown escape sequence"},
		{"bad_hex_escape", `"\xGG"`, "invalid hex escape"},
		{"bad_hex_literal", "0xGG", "invalid hex digit"},
		{"bad_octal_literal", "0o99", "invalid octal digit"},
		{"bad_binary_literal", "0b123", "invalid binary digit"},
		{"empty_exponent", "1e", "exponent has no digits"},
		{"bad_char", "@", "unexpected character"},
		{"bad_char_hash", "#", "unexpected character"},
		{"bad_char_dollar", "$", "unexpected character"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errMsg string
			errh := func(line, col uint32, msg string) {
				if errMsg == "" { // capture first error only
					errMsg = msg
				}
			}
			s := NewScanner("test", strings.NewReader(tt.src), errh)
			for {
				s.Next()
				if s.Token().IsEOF() {
					break
				}
			}
			if errMsg == "" {
				t.Errorf("expected error containing %q, got no error", tt.wantErr)
			} else if !strings.Contains(errMsg, tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, errMsg)
			}
		})
	}
}

func TestCompleteProgram(t *testing.T) {
	src := `package main

type Point struct {
    x int
    y float
}

func add(a, b int) int {
    return a + b
}

func main() {
    var p Point
    p.x = 10
    p.y = 3.14

    if p.x > 0 {
        println("positive")
    }

    var i int = 0
    for i < 10 {
        i = i + 1
    }

    result := add(1, 2)
    println(result)
}
`

	s := NewScanner("test.yoru", strings.NewReader(src), nil)
	tokenCount := 0
	for {
		s.Next()
		tokenCount++
		if s.Token().IsEOF() {
			break
		}
		if tokenCount > 1000 {
			t.Fatal("too many tokens, possible infinite loop")
		}
	}

	// Just verify it doesn't crash and produces a reasonable number of tokens
	if tokenCount < 50 {
		t.Errorf("expected at least 50 tokens, got %d", tokenCount)
	}
}

func TestCommentsInCode(t *testing.T) {
	src := `// This is a comment
package main // another comment

// Comment before func
func foo() { // inline comment
    x := 1 // assign
    // standalone comment
    return x // return
}
`

	expected := []Token{
		_Package, _Name, _Semi,
		_Func, _Name, _Lparen, _Rparen, _Lbrace,
		_Name, _Define, _Literal, _Semi,
		_Return, _Name, _Semi,
		_Rbrace, _Semi,
	}

	s := NewScanner("test.yoru", strings.NewReader(src), nil)
	for i, wantTok := range expected {
		s.Next()
		if s.Token() != wantTok {
			t.Errorf("token %d: got %v, want %v", i, s.Token(), wantTok)
		}
	}
}

func FuzzScanner(f *testing.F) {
	// Seed corpus
	seeds := []string{
		"package main",
		"func foo() { return 123 }",
		`var s string = "hello\nworld"`,
		"x := 0x1F + 0b1010",
		"if a && b || c { }",
		"for i < 10 { i = i + 1 }",
		"type Point struct { x int }",
		"p.x = 10",
		"arr[0] = 1",
		"// comment\nfoo",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		errh := func(line, col uint32, msg string) {
			// Errors are acceptable, we just don't want panics
		}
		s := NewScanner("fuzz", strings.NewReader(src), errh)
		for i := 0; i < 10000; i++ { // Prevent infinite loops
			s.Next()
			if s.Token().IsEOF() {
				break
			}
		}
	})
}

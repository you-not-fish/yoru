package syntax

import (
	"fmt"
	"io"
	"strings"
)

// Scanner performs lexical analysis on Yoru source code.
type Scanner struct {
	source // embedded character reader

	// Current token info
	tok    Token   // token type
	lit    string  // token literal (identifier name, number, string content)
	kind   LitKind // literal kind (only valid when tok == _Literal)
	tokPos Pos     // token start position

	// ASI (Automatic Semicolon Insertion) state
	nlsemi bool // whether to insert semicolon at newline

	// Configuration
	asiEnabled bool // whether ASI is enabled (default true, can be disabled with -no-asi)

	// Literal accumulation
	litBuf strings.Builder
}

// NewScanner creates a new Scanner for the given source.
// The errh function is called for each lexical error; if nil, errors are silently ignored.
func NewScanner(filename string, src io.Reader, errh func(line, col uint32, msg string)) *Scanner {
	s := &Scanner{
		source:     *newSource(filename, src, errh),
		asiEnabled: true, // ASI enabled by default
	}
	return s
}

// SetASIEnabled enables or disables automatic semicolon insertion.
func (s *Scanner) SetASIEnabled(enabled bool) {
	s.asiEnabled = enabled
}

// Next advances to the next token.
func (s *Scanner) Next() {
	// 1. Check if we need to insert semicolon at newline/EOF
	nlsemi := s.nlsemi
	s.nlsemi = false

redo:
	// 2. Skip whitespace (not including '\n')
	s.skipWhitespace()

	// 3. ASI: insert semicolon before newline or EOF if needed
	if s.asiEnabled && nlsemi && (s.ch == '\n' || s.ch < 0) {
		s.tokPos = s.pos()
		s.tok = _Semi
		if s.ch == '\n' {
			s.lit = "newline"
			s.nextch()
		} else {
			s.lit = "EOF"
		}
		return
	}

	// 4. Skip newlines when not inserting semicolon
	if s.ch == '\n' {
		s.nextch()
		goto redo
	}

	// 5. Record token start position
	s.tokPos = s.pos()

	// 6. Scan token based on current character
	switch {
	case s.ch < 0:
		s.tok = _EOF
		s.lit = ""

	case isLetter(s.ch):
		s.scanIdent()

	case isDigit(s.ch):
		s.scanNumber()

	case s.ch == '"':
		s.scanString()

	case isOperatorStart(s.ch):
		if s.scanOperator() {
			// scanOperator returned true, meaning we skipped a comment
			goto redo
		}

	default:
		s.error(fmt.Sprintf("unexpected character %q", s.ch))
		s.nextch()
		goto redo
	}

	// 7. Set nlsemi flag for next token
	s.nlsemi = s.shouldInsertSemi()
}

// Token returns the current token type.
func (s *Scanner) Token() Token {
	return s.tok
}

// Literal returns the current token's literal value.
func (s *Scanner) Literal() string {
	return s.lit
}

// LitKind returns the current literal's kind (only valid when Token() == _Literal).
func (s *Scanner) LitKind() LitKind {
	return s.kind
}

// Pos returns the current token's start position.
func (s *Scanner) Pos() Pos {
	return s.tokPos
}

// skipWhitespace skips space, tab, and carriage return.
// Note: newline is NOT skipped here because it may trigger ASI.
func (s *Scanner) skipWhitespace() {
	for isWhitespace(s.ch) {
		s.nextch()
	}
}

// shouldInsertSemi reports whether a semicolon should be inserted
// after the current token when followed by a newline.
func (s *Scanner) shouldInsertSemi() bool {
	switch s.tok {
	case _Name, _Literal:
		return true
	case _Break, _Continue, _Return:
		return true
	case _Rparen, _Rbrack, _Rbrace:
		return true
	}
	return false
}

// startLit begins accumulating a literal.
func (s *Scanner) startLit() {
	s.litBuf.Reset()
	s.litBuf.WriteRune(s.ch)
}

// continueLit adds the current character to the literal being accumulated.
func (s *Scanner) continueLit() {
	s.litBuf.WriteRune(s.ch)
}

// stopLit ends literal accumulation and returns the accumulated string.
func (s *Scanner) stopLit() string {
	return s.litBuf.String()
}

// scanIdent scans an identifier or keyword.
func (s *Scanner) scanIdent() {
	s.startLit()
	s.nextch()

	for isLetter(s.ch) || isDigit(s.ch) {
		s.continueLit()
		s.nextch()
	}

	s.lit = s.stopLit()

	// Check if it's a keyword
	s.tok = LookupKeyword(s.lit)
}

// scanNumber scans a number literal (integer or float).
func (s *Scanner) scanNumber() {
	s.litBuf.Reset()
	s.kind = IntLit

	if s.ch == '0' {
		s.litBuf.WriteRune(s.ch)
		s.nextch()
		switch lower(s.ch) {
		case 'x':
			// Hexadecimal: 0x or 0X
			s.litBuf.WriteRune(s.ch)
			s.nextch()
			s.scanHexDigits()
		case 'o':
			// Octal: 0o or 0O
			s.litBuf.WriteRune(s.ch)
			s.nextch()
			s.scanOctalDigits()
		case 'b':
			// Binary: 0b or 0B
			s.litBuf.WriteRune(s.ch)
			s.nextch()
			s.scanBinaryDigits()
		default:
			// Decimal starting with 0, or just 0
			if isDigit(s.ch) {
				// Leading zeros in decimal are allowed (e.g., 007)
				s.scanDecimalDigits()
			}
			// Check for float
			if s.ch == '.' || lower(s.ch) == 'e' {
				s.scanFraction()
			}
		}
	} else {
		// Decimal number - scan all digits including first
		s.scanDecimalDigits()
		// Check for float
		if s.ch == '.' || lower(s.ch) == 'e' {
			s.scanFraction()
		}
	}

	s.lit = s.litBuf.String()
	s.tok = _Literal
}

// scanDecimalDigits scans decimal digits.
func (s *Scanner) scanDecimalDigits() {
	for isDigit(s.ch) {
		s.continueLit()
		s.nextch()
	}
}

// scanHexDigits scans hexadecimal digits.
func (s *Scanner) scanHexDigits() {
	if !isHexDigit(s.ch) {
		s.error("invalid hex digit")
		return
	}
	for isHexDigit(s.ch) {
		s.continueLit()
		s.nextch()
	}
}

// scanOctalDigits scans octal digits.
func (s *Scanner) scanOctalDigits() {
	if !isOctalDigit(s.ch) {
		s.error("invalid octal digit")
		return
	}
	for isOctalDigit(s.ch) {
		s.continueLit()
		s.nextch()
	}
}

// scanBinaryDigits scans binary digits.
func (s *Scanner) scanBinaryDigits() {
	if !isBinaryDigit(s.ch) {
		s.error("invalid binary digit")
		return
	}
	for isBinaryDigit(s.ch) {
		s.continueLit()
		s.nextch()
	}
	// Check for invalid trailing digits (e.g., 0b123)
	if isDigit(s.ch) {
		s.error("invalid binary digit")
	}
}

// scanFraction scans the fractional part of a float (. and/or exponent).
func (s *Scanner) scanFraction() {
	// Decimal point
	if s.ch == '.' {
		s.kind = FloatLit
		s.continueLit()
		s.nextch()
		s.scanDecimalDigits()
	}

	// Exponent
	if lower(s.ch) == 'e' {
		s.kind = FloatLit
		s.continueLit()
		s.nextch()

		// Optional sign
		if s.ch == '+' || s.ch == '-' {
			s.continueLit()
			s.nextch()
		}

		if !isDigit(s.ch) {
			s.error("exponent has no digits")
			return
		}
		s.scanDecimalDigits()
	}
}

// scanString scans a string literal.
// The resulting literal is the decoded string content (escape sequences are interpreted).
func (s *Scanner) scanString() {
	s.nextch() // skip opening "
	var b strings.Builder

	for {
		switch {
		case s.ch == '"':
			s.nextch()
			s.lit = b.String()
			s.tok = _Literal
			s.kind = StringLit
			return

		case s.ch == '\\':
			if r, ok := s.scanEscape(); ok {
				b.WriteRune(r)
			}

		case s.ch == '\n' || s.ch < 0:
			s.error("string not terminated")
			s.lit = b.String()
			s.tok = _Literal
			s.kind = StringLit
			return

		default:
			b.WriteRune(s.ch)
			s.nextch()
		}
	}
}

// scanEscape scans an escape sequence and returns the decoded rune.
func (s *Scanner) scanEscape() (rune, bool) {
	s.nextch() // skip \

	switch s.ch {
	case 'n':
		s.nextch()
		return '\n', true
	case 't':
		s.nextch()
		return '\t', true
	case 'r':
		s.nextch()
		return '\r', true
	case '\\':
		s.nextch()
		return '\\', true
	case '"':
		s.nextch()
		return '"', true
	case '0':
		s.nextch()
		return 0, true
	case 'x':
		s.nextch()
		return s.scanHexEscape()
	default:
		s.error(fmt.Sprintf("unknown escape sequence: \\%c", s.ch))
		s.nextch()
		return 0, false
	}
}

// scanHexEscape scans a \xNN escape sequence.
func (s *Scanner) scanHexEscape() (rune, bool) {
	var val rune
	for i := 0; i < 2; i++ {
		if !isHexDigit(s.ch) {
			s.error("invalid hex escape")
			return 0, false
		}
		val = val*16 + hexValue(s.ch)
		s.nextch()
	}
	return val, true
}

// hexValue returns the numeric value of a hex digit.
func hexValue(r rune) rune {
	switch {
	case '0' <= r && r <= '9':
		return r - '0'
	case 'a' <= lower(r) && lower(r) <= 'f':
		return lower(r) - 'a' + 10
	}
	return 0
}

// scanOperator scans an operator or delimiter.
// Returns true if a comment was skipped (caller should rescan).
func (s *Scanner) scanOperator() bool {
	ch := s.ch
	s.nextch()

	switch ch {
	case '+':
		s.tok = _Add
		s.lit = "+"
	case '-':
		s.tok = _Sub
		s.lit = "-"
	case '*':
		s.tok = _Mul
		s.lit = "*"
	case '/':
		if s.ch == '/' {
			// Line comment
			s.skipLineComment()
			return true
		}
		s.tok = _Div
		s.lit = "/"
	case '%':
		s.tok = _Rem
		s.lit = "%"
	case '&':
		if s.ch == '&' {
			s.nextch()
			s.tok = _AndAnd
			s.lit = "&&"
		} else {
			s.tok = _And
			s.lit = "&"
		}
	case '|':
		if s.ch == '|' {
			s.nextch()
			s.tok = _OrOr
			s.lit = "||"
		} else {
			s.tok = _Or
			s.lit = "|"
		}
	case '^':
		s.tok = _Xor
		s.lit = "^"
	case '<':
		switch s.ch {
		case '=':
			s.nextch()
			s.tok = _Leq
			s.lit = "<="
		case '<':
			s.nextch()
			s.tok = _Shl
			s.lit = "<<"
		default:
			s.tok = _Lss
			s.lit = "<"
		}
	case '>':
		switch s.ch {
		case '=':
			s.nextch()
			s.tok = _Geq
			s.lit = ">="
		case '>':
			s.nextch()
			s.tok = _Shr
			s.lit = ">>"
		default:
			s.tok = _Gtr
			s.lit = ">"
		}
	case '=':
		if s.ch == '=' {
			s.nextch()
			s.tok = _Eql
			s.lit = "=="
		} else {
			s.tok = _Assign
			s.lit = "="
		}
	case '!':
		if s.ch == '=' {
			s.nextch()
			s.tok = _Neq
			s.lit = "!="
		} else {
			s.tok = _Not
			s.lit = "!"
		}
	case ':':
		if s.ch == '=' {
			s.nextch()
			s.tok = _Define
			s.lit = ":="
		} else {
			s.tok = _Colon
			s.lit = ":"
		}
	case '(':
		s.tok = _Lparen
		s.lit = "("
	case ')':
		s.tok = _Rparen
		s.lit = ")"
	case '[':
		s.tok = _Lbrack
		s.lit = "["
	case ']':
		s.tok = _Rbrack
		s.lit = "]"
	case '{':
		s.tok = _Lbrace
		s.lit = "{"
	case '}':
		s.tok = _Rbrace
		s.lit = "}"
	case ',':
		s.tok = _Comma
		s.lit = ","
	case ';':
		s.tok = _Semi
		s.lit = ";"
	case '.':
		s.tok = _Dot
		s.lit = "."
	}

	return false
}

// skipLineComment skips a line comment (from // to end of line).
func (s *Scanner) skipLineComment() {
	// Already consumed the second /
	s.nextch()
	for s.ch != '\n' && s.ch >= 0 {
		s.nextch()
	}
}

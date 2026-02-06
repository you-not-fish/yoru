package syntax

import (
	"strings"
	"testing"
)

func TestSourceBasic(t *testing.T) {
	src := newSource("test", strings.NewReader("abc"), nil)

	// First character should be 'a'
	if src.ch != 'a' {
		t.Errorf("initial ch = %q, want 'a'", src.ch)
	}
	if src.line != 1 || src.col != 1 {
		t.Errorf("initial pos = %d:%d, want 1:1", src.line, src.col)
	}

	// Next character 'b'
	src.nextch()
	if src.ch != 'b' {
		t.Errorf("ch = %q, want 'b'", src.ch)
	}
	if src.line != 1 || src.col != 2 {
		t.Errorf("pos = %d:%d, want 1:2", src.line, src.col)
	}

	// Next character 'c'
	src.nextch()
	if src.ch != 'c' {
		t.Errorf("ch = %q, want 'c'", src.ch)
	}
	if src.line != 1 || src.col != 3 {
		t.Errorf("pos = %d:%d, want 1:3", src.line, src.col)
	}

	// EOF
	src.nextch()
	if src.ch != -1 {
		t.Errorf("ch = %d, want -1 (EOF)", src.ch)
	}
}

func TestSourceNewline(t *testing.T) {
	src := newSource("test", strings.NewReader("a\nb\nc"), nil)

	// 'a' at 1:1
	if src.ch != 'a' || src.line != 1 || src.col != 1 {
		t.Errorf("got ch=%q pos=%d:%d, want ch='a' pos=1:1", src.ch, src.line, src.col)
	}

	// '\n' at 1:2
	src.nextch()
	if src.ch != '\n' || src.line != 1 || src.col != 2 {
		t.Errorf("got ch=%q pos=%d:%d, want ch='\\n' pos=1:2", src.ch, src.line, src.col)
	}

	// 'b' at 2:1 (after newline)
	src.nextch()
	if src.ch != 'b' || src.line != 2 || src.col != 1 {
		t.Errorf("got ch=%q pos=%d:%d, want ch='b' pos=2:1", src.ch, src.line, src.col)
	}

	// '\n' at 2:2
	src.nextch()
	if src.ch != '\n' || src.line != 2 || src.col != 2 {
		t.Errorf("got ch=%q pos=%d:%d, want ch='\\n' pos=2:2", src.ch, src.line, src.col)
	}

	// 'c' at 3:1
	src.nextch()
	if src.ch != 'c' || src.line != 3 || src.col != 1 {
		t.Errorf("got ch=%q pos=%d:%d, want ch='c' pos=3:1", src.ch, src.line, src.col)
	}
}

func TestSourceUTF8(t *testing.T) {
	// Test multi-byte UTF-8 characters
	src := newSource("test", strings.NewReader("a中b"), nil)

	// 'a' at col 1
	if src.ch != 'a' {
		t.Errorf("ch = %q, want 'a'", src.ch)
	}

	// '中' (3 bytes in UTF-8) at col 2
	src.nextch()
	if src.ch != '中' {
		t.Errorf("ch = %q, want '中'", src.ch)
	}
	if src.col != 2 {
		t.Errorf("col = %d, want 2", src.col)
	}

	// 'b' at col 3 (column is character count, not byte offset)
	src.nextch()
	if src.ch != 'b' {
		t.Errorf("ch = %q, want 'b'", src.ch)
	}
	if src.col != 3 {
		t.Errorf("col = %d, want 3", src.col)
	}
}

func TestSourceEmpty(t *testing.T) {
	src := newSource("test", strings.NewReader(""), nil)

	// Empty source should immediately be at EOF
	if src.ch != -1 {
		t.Errorf("ch = %d, want -1 (EOF)", src.ch)
	}
}

func TestSourcePos(t *testing.T) {
	src := newSource("test.yoru", strings.NewReader("ab"), nil)

	pos := src.pos()
	if pos.Line() != 1 || pos.Col() != 1 || pos.Filename() != "test.yoru" {
		t.Errorf("pos = %v, want test.yoru:1:1", pos)
	}

	src.nextch()
	pos = src.pos()
	if pos.Line() != 1 || pos.Col() != 2 {
		t.Errorf("pos = %v, want 1:2", pos)
	}
}

func TestSourceError(t *testing.T) {
	var errMsg string
	var errLine, errCol uint32

	errh := func(line, col uint32, msg string) {
		errLine = line
		errCol = col
		errMsg = msg
	}

	src := newSource("test", strings.NewReader("a"), errh)
	src.error("test error")

	if errMsg != "test error" {
		t.Errorf("error message = %q, want %q", errMsg, "test error")
	}
	if errLine != 1 || errCol != 1 {
		t.Errorf("error pos = %d:%d, want 1:1", errLine, errCol)
	}
}

func TestSourceErrorNilHandler(t *testing.T) {
	// Should not panic with nil error handler
	src := newSource("test", strings.NewReader("a"), nil)
	src.error("test error") // Should not panic
}

// Test character classification helpers

func TestIsLetter(t *testing.T) {
	letters := []rune{'a', 'z', 'A', 'Z', '_'}
	for _, r := range letters {
		if !isLetter(r) {
			t.Errorf("isLetter(%q) = false, want true", r)
		}
	}

	nonLetters := []rune{'0', '9', ' ', '\n', '+', '中'}
	for _, r := range nonLetters {
		if isLetter(r) {
			t.Errorf("isLetter(%q) = true, want false", r)
		}
	}
}

func TestIsDigit(t *testing.T) {
	digits := []rune{'0', '1', '5', '9'}
	for _, r := range digits {
		if !isDigit(r) {
			t.Errorf("isDigit(%q) = false, want true", r)
		}
	}

	nonDigits := []rune{'a', 'Z', ' ', '+'}
	for _, r := range nonDigits {
		if isDigit(r) {
			t.Errorf("isDigit(%q) = true, want false", r)
		}
	}
}

func TestIsHexDigit(t *testing.T) {
	hexDigits := []rune{'0', '9', 'a', 'f', 'A', 'F'}
	for _, r := range hexDigits {
		if !isHexDigit(r) {
			t.Errorf("isHexDigit(%q) = false, want true", r)
		}
	}

	nonHexDigits := []rune{'g', 'G', 'z', ' ', '+'}
	for _, r := range nonHexDigits {
		if isHexDigit(r) {
			t.Errorf("isHexDigit(%q) = true, want false", r)
		}
	}
}

func TestIsOctalDigit(t *testing.T) {
	octalDigits := []rune{'0', '1', '7'}
	for _, r := range octalDigits {
		if !isOctalDigit(r) {
			t.Errorf("isOctalDigit(%q) = false, want true", r)
		}
	}

	nonOctalDigits := []rune{'8', '9', 'a'}
	for _, r := range nonOctalDigits {
		if isOctalDigit(r) {
			t.Errorf("isOctalDigit(%q) = true, want false", r)
		}
	}
}

func TestIsBinaryDigit(t *testing.T) {
	if !isBinaryDigit('0') || !isBinaryDigit('1') {
		t.Error("isBinaryDigit should return true for '0' and '1'")
	}

	nonBinary := []rune{'2', '9', 'a'}
	for _, r := range nonBinary {
		if isBinaryDigit(r) {
			t.Errorf("isBinaryDigit(%q) = true, want false", r)
		}
	}
}

func TestLower(t *testing.T) {
	tests := []struct {
		input rune
		want  rune
	}{
		{'A', 'a'},
		{'Z', 'z'},
		{'a', 'a'},
		{'z', 'z'},
		{'0', '0'}, // digits unchanged
	}

	for _, tt := range tests {
		if got := lower(tt.input); got != tt.want {
			t.Errorf("lower(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsWhitespace(t *testing.T) {
	whitespace := []rune{' ', '\t', '\r'}
	for _, r := range whitespace {
		if !isWhitespace(r) {
			t.Errorf("isWhitespace(%q) = false, want true", r)
		}
	}

	// Note: '\n' is NOT whitespace (it may trigger ASI)
	nonWhitespace := []rune{'\n', 'a', '0'}
	for _, r := range nonWhitespace {
		if isWhitespace(r) {
			t.Errorf("isWhitespace(%q) = true, want false", r)
		}
	}
}

func TestIsOperatorStart(t *testing.T) {
	operators := []rune{'+', '-', '*', '/', '%', '&', '|', '^', '<', '>', '=', '!', ':',
		'(', ')', '[', ']', '{', '}', ',', ';', '.'}
	for _, r := range operators {
		if !isOperatorStart(r) {
			t.Errorf("isOperatorStart(%q) = false, want true", r)
		}
	}

	nonOperators := []rune{'a', '0', ' ', '\n', '@', '#', '$'}
	for _, r := range nonOperators {
		if isOperatorStart(r) {
			t.Errorf("isOperatorStart(%q) = true, want false", r)
		}
	}
}

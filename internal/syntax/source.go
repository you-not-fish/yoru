package syntax

import (
	"io"
	"unicode/utf8"
)

// source is a character reader with position tracking.
// It reads UTF-8 encoded source files and provides character-by-character access.
type source struct {
	// Input
	buf []byte // source buffer (entire file read into memory)

	// Position tracking
	filename string // source file name
	line     uint32 // current line number (1-based)
	col      uint32 // current column number (1-based, byte offset)

	// Current state
	ch   rune // current character, -1 for EOF
	offs int  // current byte offset in buf

	// Error handling
	errh func(line, col uint32, msg string)
}

// newSource creates a new source from an io.Reader.
// The entire content is read into memory.
// The errh function is called for each error; if nil, errors are silently ignored.
func newSource(filename string, src io.Reader, errh func(line, col uint32, msg string)) *source {
	s := &source{
		filename: filename,
		line:     1,
		col:      0, // Will be incremented to 1 by first nextch()
		ch:       -1, // Sentinel: -1 means "before first char", prevents position update
		errh:     errh,
	}

	// Read entire source into buffer
	var err error
	s.buf, err = io.ReadAll(src)
	if err != nil {
		s.error("error reading source file: " + err.Error())
		s.ch = -1
		return s
	}

	// Initialize first character
	s.nextch()
	return s
}

// nextch reads the next character from the source and updates position.
// Sets s.ch to -1 at EOF.
//
// Position tracking: (line, col) always refers to the position of s.ch after nextch() returns.
// Initial state: line=1, col=0, s.ch=-1
// After first nextch(): line=1, col=1, s.ch=first char
func (s *source) nextch() {
	// Update position based on previous character FIRST
	// Note: s.ch == -1 initially (sentinel), meaning "before first char"
	if s.ch == '\n' {
		s.line++
		s.col = 1
	} else {
		// Always increment col (including from initial col=0 to col=1)
		s.col++
	}

	// Then check for EOF
	if s.offs >= len(s.buf) {
		s.ch = -1
		return
	}

	// Read next rune
	r, width := utf8.DecodeRune(s.buf[s.offs:])
	if r == utf8.RuneError && width == 1 {
		s.error("invalid UTF-8 encoding")
		// Continue anyway to avoid getting stuck
	}

	s.ch = r
	s.offs += width
}

// pos returns the current position (position of current character).
func (s *source) pos() Pos {
	return NewPos(s.filename, s.line, s.col)
}

// error reports a lexical error at the current position.
func (s *source) error(msg string) {
	if s.errh != nil {
		s.errh(s.line, s.col, msg)
	}
}

// Character classification helpers

// isLetter reports whether r is a letter (a-z, A-Z, or _).
func isLetter(r rune) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '_'
}

// isDigit reports whether r is a decimal digit (0-9).
func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

// isHexDigit reports whether r is a hexadecimal digit (0-9, a-f, A-F).
func isHexDigit(r rune) bool {
	return isDigit(r) || 'a' <= lower(r) && lower(r) <= 'f'
}

// isOctalDigit reports whether r is an octal digit (0-7).
func isOctalDigit(r rune) bool {
	return '0' <= r && r <= '7'
}

// isBinaryDigit reports whether r is a binary digit (0 or 1).
func isBinaryDigit(r rune) bool {
	return r == '0' || r == '1'
}

// lower returns the lowercase version of r if r is an ASCII letter,
// otherwise returns r unchanged.
// This uses a clever bit trick: ('a' - 'A') is 0x20, and OR-ing with 0x20
// converts uppercase ASCII letters to lowercase while leaving other characters unchanged.
func lower(r rune) rune {
	return ('a' - 'A') | r
}

// isWhitespace reports whether r is a whitespace character (space, tab, or carriage return).
// Note: newline '\n' is NOT included because it may trigger ASI.
func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r'
}

// isOperatorStart reports whether r can start an operator or delimiter.
func isOperatorStart(r rune) bool {
	switch r {
	case '+', '-', '*', '/', '%', '&', '|', '^', '<', '>', '=', '!', ':',
		'(', ')', '[', ']', '{', '}', ',', ';', '.':
		return true
	}
	return false
}

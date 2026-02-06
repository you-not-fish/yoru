package syntax

import "fmt"

// Pos represents a position in a source file.
// The zero value is an invalid position.
type Pos struct {
	filename string // source file name
	line     uint32 // 1-based line number
	col      uint32 // 1-based column number (byte offset in line)
}

// NewPos creates a new Pos with the given filename, line, and column.
// Line and column numbers are 1-based.
func NewPos(filename string, line, col uint32) Pos {
	return Pos{filename: filename, line: line, col: col}
}

// String returns a string representation of the position in the format
// "filename:line:col" or "line:col" if filename is empty.
func (p Pos) String() string {
	if p.filename != "" {
		return fmt.Sprintf("%s:%d:%d", p.filename, p.line, p.col)
	}
	return fmt.Sprintf("%d:%d", p.line, p.col)
}

// IsValid reports whether the position is valid.
// A position is valid if line > 0.
func (p Pos) IsValid() bool {
	return p.line > 0
}

// Line returns the 1-based line number.
func (p Pos) Line() uint32 {
	return p.line
}

// Col returns the 1-based column number (byte offset in line).
func (p Pos) Col() uint32 {
	return p.col
}

// Filename returns the source file name.
func (p Pos) Filename() string {
	return p.filename
}

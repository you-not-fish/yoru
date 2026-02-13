package codegen

import (
	"fmt"
	"io"

	"github.com/you-not-fish/yoru/internal/ssa"
)

// emitter wraps an io.Writer with helpers for emitting LLVM IR text.
type emitter struct {
	w   io.Writer
	err error // first write error
	tmp int   // counter for anonymous temporaries (%t0, %t1, ...)
}

// emit writes a formatted line to the output (no indentation).
func (e *emitter) emit(format string, args ...interface{}) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format+"\n", args...)
}

// emitRaw writes a formatted string without a trailing newline.
func (e *emitter) emitRaw(format string, args ...interface{}) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, args...)
}

// emitLine writes a blank line.
func (e *emitter) emitLine() {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintln(e.w)
}

// emitComment writes a comment line.
func (e *emitter) emitComment(text string) {
	e.emit("; %s", text)
}

// emitLabel writes a basic block label.
func (e *emitter) emitLabel(b *ssa.Block) {
	e.emit("%s:", blockName(b))
}

// emitInst writes an indented instruction line.
func (e *emitter) emitInst(format string, args ...interface{}) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, "  "+format+"\n", args...)
}

// nextTmp returns the next anonymous temporary name (%t0, %t1, ...).
func (e *emitter) nextTmp() string {
	name := fmt.Sprintf("%%t%d", e.tmp)
	e.tmp++
	return name
}

// valueName returns the LLVM local name for an SSA value: %vN.
func valueName(v *ssa.Value) string {
	return fmt.Sprintf("%%v%d", v.ID)
}

// blockName returns the LLVM label for an SSA block.
// Block 0 is "entry", others are "bN".
func blockName(b *ssa.Block) string {
	if b.ID == 0 {
		return "entry"
	}
	return fmt.Sprintf("b%d", b.ID)
}

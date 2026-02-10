package ssa

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/you-not-fish/yoru/internal/types"
)

// Fprint writes the SSA representation of a function to w.
//
// Format:
//
//	func name(params) result:
//	  b0: (entry)
//	    v1 = Arg <int> {n}
//	    v2 = Const64 <int> [42]
//	    v3 = Add64 <int> v1 v2
//	    Return v3
func Fprint(w io.Writer, f *Func) {
	// Function header
	fmt.Fprintf(w, "func %s", f.Name)
	if f.Sig != nil {
		fmt.Fprintf(w, "(")
		for i := 0; i < f.Sig.NumParams(); i++ {
			if i > 0 {
				fmt.Fprintf(w, ", ")
			}
			p := f.Sig.Param(i)
			fmt.Fprintf(w, "%s %s", p.Name(), p.Type())
		}
		fmt.Fprintf(w, ")")
		if f.Sig.Result() != nil {
			fmt.Fprintf(w, " %s", f.Sig.Result())
		}
	}
	fmt.Fprintf(w, ":\n")

	// Blocks
	for _, b := range f.Blocks {
		fprintBlock(w, b, f)
	}
}

// fprintBlock writes a single block to w.
func fprintBlock(w io.Writer, b *Block, f *Func) {
	// Block header
	label := ""
	if b == f.Entry {
		label = " (entry)"
	}

	// Show predecessor list for non-entry blocks
	predsStr := ""
	if len(b.Preds) > 0 {
		preds := make([]string, len(b.Preds))
		for i, p := range b.Preds {
			preds[i] = p.String()
		}
		predsStr = " <- " + strings.Join(preds, " ")
	}

	fmt.Fprintf(w, "  %s:%s%s\n", b, label, predsStr)

	// Values
	for _, v := range b.Values {
		fmt.Fprintf(w, "    %s\n", formatValue(v))
	}

	// Terminator
	fmt.Fprintf(w, "    %s\n", formatTerminator(b))
}

// formatValue formats a value as a string.
func formatValue(v *Value) string {
	var sb strings.Builder

	// For void ops, don't print "vN = "
	if v.Op.IsVoid() {
		sb.WriteString(v.Op.String())
	} else {
		fmt.Fprintf(&sb, "v%d = %s", v.ID, v.Op)
	}

	// Type
	if v.Type != nil {
		fmt.Fprintf(&sb, " <%s>", v.Type)
	}

	// AuxInt (always show for const ops and specific ops, otherwise only if non-zero)
	switch v.Op {
	case OpConst64, OpConstBool:
		fmt.Fprintf(&sb, " [%d]", v.AuxInt)
	case OpZero, OpStructFieldPtr:
		fmt.Fprintf(&sb, " [%d]", v.AuxInt)
	default:
		// Show AuxInt for other ops only if non-zero
		if v.AuxInt != 0 && v.Op != OpConstFloat {
			fmt.Fprintf(&sb, " [%d]", v.AuxInt)
		}
	}

	// AuxFloat
	switch v.Op {
	case OpConstFloat:
		fmt.Fprintf(&sb, " [%g]", v.AuxFloat)
	}

	// Aux
	if v.Aux != nil {
		fmt.Fprintf(&sb, " {%s}", formatAux(v.Aux))
	}

	// Arguments
	for _, arg := range v.Args {
		fmt.Fprintf(&sb, " v%d", arg.ID)
	}

	return sb.String()
}

// formatTerminator formats a block terminator.
func formatTerminator(b *Block) string {
	switch b.Kind {
	case BlockPlain:
		if len(b.Succs) > 0 {
			return fmt.Sprintf("Plain -> %s", b.Succs[0])
		}
		return "Plain"
	case BlockIf:
		if len(b.Controls) > 0 && len(b.Succs) >= 2 {
			return fmt.Sprintf("If v%d -> %s %s", b.Controls[0].ID, b.Succs[0], b.Succs[1])
		}
		return "If (malformed)"
	case BlockReturn:
		if len(b.Controls) > 0 && b.Controls[0] != nil {
			return fmt.Sprintf("Return v%d", b.Controls[0].ID)
		}
		return "Return"
	case BlockExit:
		return "Exit"
	default:
		return "???"
	}
}

// Sprint returns the SSA representation of a function as a string.
func Sprint(f *Func) string {
	var sb strings.Builder
	Fprint(&sb, f)
	return sb.String()
}

// formatAux formats an Aux value for display.
func formatAux(aux interface{}) string {
	switch a := aux.(type) {
	case *types.FuncObj:
		return a.Name()
	case types.Type:
		return a.String()
	case string:
		return a
	default:
		return fmt.Sprintf("%v", aux)
	}
}

// Print writes the SSA representation of a function to stdout.
func Print(f *Func) {
	Fprint(os.Stdout, f)
}

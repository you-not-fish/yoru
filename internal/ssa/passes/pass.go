package passes

import (
	"fmt"
	"os"

	"github.com/you-not-fish/yoru/internal/ssa"
)

// Pass describes a single SSA optimization pass.
type Pass struct {
	Name string
	Fn   func(f *ssa.Func)
}

// Config controls pass execution behavior.
type Config struct {
	DumpBefore string // dump SSA before this pass ("*" for all)
	DumpAfter  string // dump SSA after this pass ("*" for all)
	Verify     bool   // verify SSA before/after each pass
	DumpFunc   string // restrict dumps to this function name
}

// Run executes the given passes on f in order.
func Run(f *ssa.Func, passes []Pass, cfg Config) error {
	for _, p := range passes {
		if shouldDump(cfg.DumpBefore, p.Name) && matchFunc(cfg.DumpFunc, f.Name) {
			fmt.Fprintf(os.Stderr, "--- before %s (%s) ---\n", p.Name, f.Name)
			ssa.Fprint(os.Stderr, f)
			fmt.Fprintln(os.Stderr)
		}

		if cfg.Verify {
			if err := ssa.Verify(f); err != nil {
				return fmt.Errorf("verify before %s: %w", p.Name, err)
			}
		}

		p.Fn(f)

		if cfg.Verify {
			if err := ssa.Verify(f); err != nil {
				return fmt.Errorf("verify after %s: %w", p.Name, err)
			}
		}

		if shouldDump(cfg.DumpAfter, p.Name) && matchFunc(cfg.DumpFunc, f.Name) {
			fmt.Fprintf(os.Stderr, "--- after %s (%s) ---\n", p.Name, f.Name)
			ssa.Fprint(os.Stderr, f)
			fmt.Fprintln(os.Stderr)
		}
	}
	return nil
}

func shouldDump(pattern, name string) bool {
	return pattern == "*" || pattern == name
}

func matchFunc(filter, name string) bool {
	return filter == "" || filter == name
}

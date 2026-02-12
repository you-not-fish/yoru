package passes

import (
	"testing"

	"github.com/you-not-fish/yoru/internal/ssa"
	"github.com/you-not-fish/yoru/internal/types"
)

func TestRunEmpty(t *testing.T) {
	f := ssa.NewFunc("f", types.NewFunc(nil, nil, nil))
	f.Entry.Kind = ssa.BlockReturn

	err := Run(f, nil, Config{})
	if err != nil {
		t.Fatalf("Run with no passes: %v", err)
	}
}

func TestRunSinglePass(t *testing.T) {
	f := ssa.NewFunc("f", types.NewFunc(nil, nil, nil))
	f.Entry.Kind = ssa.BlockReturn

	called := false
	passes := []Pass{
		{Name: "test", Fn: func(fn *ssa.Func) { called = true }},
	}

	err := Run(f, passes, Config{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !called {
		t.Error("pass was not called")
	}
}

func TestRunWithVerify(t *testing.T) {
	f := ssa.NewFunc("f", types.NewFunc(nil, nil, nil))
	f.Entry.Kind = ssa.BlockReturn

	passes := []Pass{
		{Name: "noop", Fn: func(fn *ssa.Func) {}},
	}

	err := Run(f, passes, Config{Verify: true})
	if err != nil {
		t.Fatalf("Run with verify: %v", err)
	}
}

func TestRunMultiplePasses(t *testing.T) {
	f := ssa.NewFunc("f", types.NewFunc(nil, nil, nil))
	f.Entry.Kind = ssa.BlockReturn

	var order []string
	passes := []Pass{
		{Name: "first", Fn: func(fn *ssa.Func) { order = append(order, "first") }},
		{Name: "second", Fn: func(fn *ssa.Func) { order = append(order, "second") }},
	}

	err := Run(f, passes, Config{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("pass order = %v, want [first second]", order)
	}
}

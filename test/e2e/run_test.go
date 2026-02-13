package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/you-not-fish/yoru/internal/codegen"
	"github.com/you-not-fish/yoru/internal/ssa"
	"github.com/you-not-fish/yoru/internal/ssa/passes"
	"github.com/you-not-fish/yoru/internal/syntax"
	"github.com/you-not-fish/yoru/internal/types"
	"github.com/you-not-fish/yoru/internal/types2"
)

// TestE2E runs end-to-end tests for all .yoru files in testdata/.
// Each test:
//  1. Runs the full pipeline: parse → typecheck → SSA → mem2reg → codegen
//  2. Writes the LLVM IR to a temp .ll file
//  3. Compiles with clang, linking against the runtime
//  4. Runs the binary and captures stdout
//  5. Compares output against the .golden file
func TestE2E(t *testing.T) {
	// Find all .yoru files in testdata.
	testFiles, err := filepath.Glob("testdata/*.yoru")
	if err != nil {
		t.Fatal(err)
	}
	if len(testFiles) == 0 {
		t.Fatal("no .yoru test files found in testdata/")
	}

	// Check that clang is available.
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not found, skipping E2E tests")
	}

	// Find the runtime source. Walk up from the test directory.
	runtimeC := findRuntime(t)

	for _, testFile := range testFiles {
		name := strings.TrimSuffix(filepath.Base(testFile), ".yoru")
		t.Run(name, func(t *testing.T) {
			runE2ETest(t, testFile, runtimeC)
		})
	}
}

// runE2ETest runs a single end-to-end test.
func runE2ETest(t *testing.T, yoruFile, runtimeC string) {
	t.Helper()

	// Read expected output from .golden file.
	goldenFile := strings.TrimSuffix(yoruFile, ".yoru") + ".golden"
	expected, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("reading golden file: %v", err)
	}

	// Create temp directory for build artifacts.
	tmpDir := t.TempDir()
	llFile := filepath.Join(tmpDir, "output.ll")
	binFile := filepath.Join(tmpDir, "output")

	// Step 1: Compile .yoru → .ll (in-process).
	compileTo(t, yoruFile, llFile)

	// Step 2: Link with clang.
	cmd := exec.Command("clang",
		"-target", "arm64-apple-macosx26.0.0",
		llFile, runtimeC,
		"-o", binFile,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clang failed:\n%s\n%v", out, err)
	}

	// Step 3: Run binary and capture stdout.
	cmd = exec.Command(binFile)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("binary execution failed: %v", err)
	}

	// Step 4: Compare output.
	got := string(out)
	want := string(expected)
	if got != want {
		t.Errorf("output mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

// compileTo runs the full compilation pipeline in-process and writes LLVM IR to llFile.
func compileTo(t *testing.T, yoruFile, llFile string) {
	t.Helper()

	// Parse.
	f, err := os.Open(yoruFile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var parseErrs []string
	parseErrh := func(pos syntax.Pos, msg string) {
		parseErrs = append(parseErrs, pos.String()+": "+msg)
	}
	p := syntax.NewParser(yoruFile, f, parseErrh)
	ast := p.Parse()
	if len(parseErrs) > 0 {
		t.Fatalf("parse errors:\n%s", strings.Join(parseErrs, "\n"))
	}

	// Type check.
	var typeErrs []string
	typeErrh := func(pos syntax.Pos, msg string) {
		typeErrs = append(typeErrs, pos.String()+": "+msg)
	}
	conf := &types2.Config{
		Error: typeErrh,
		Sizes: types.DefaultSizes,
	}
	info := &types2.Info{
		Types:  make(map[syntax.Expr]types2.TypeAndValue),
		Defs:   make(map[*syntax.Name]types.Object),
		Uses:   make(map[*syntax.Name]types.Object),
		Scopes: make(map[syntax.Node]*types.Scope),
	}
	_, _ = types2.Check(yoruFile, ast, conf, info)
	if len(typeErrs) > 0 {
		t.Fatalf("type errors:\n%s", strings.Join(typeErrs, "\n"))
	}

	// Build SSA.
	funcs := ssa.BuildFile(ast, info, types.DefaultSizes)

	// Run passes.
	pipeline := []passes.Pass{
		{Name: "mem2reg", Fn: passes.Mem2Reg},
	}
	for _, fn := range funcs {
		ssa.ComputeDom(fn)
		if err := passes.Run(fn, pipeline, passes.Config{}); err != nil {
			t.Fatalf("pass pipeline failed for %s: %v", fn.Name, err)
		}
	}

	// Generate LLVM IR.
	out, err := os.Create(llFile)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer out.Close()

	if err := codegen.Generate(out, funcs, types.DefaultSizes); err != nil {
		t.Fatalf("codegen: %v", err)
	}
}

// findRuntime locates the runtime/runtime.c file relative to the test directory.
func findRuntime(t *testing.T) string {
	t.Helper()

	// Try relative paths from the test file's location.
	candidates := []string{
		"../../runtime/runtime.c",
		"../../../runtime/runtime.c",
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}

	t.Fatal("cannot find runtime/runtime.c")
	return ""
}

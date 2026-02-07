package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEmitTypedASTIncludesNestedExprTypes(t *testing.T) {
	src := `package main

type Point struct {
	x int
}

func (p Point) Get() int {
	return p.x
}

func main() {
	var p Point
	p.x = 3
	println(p.Get() + 1)
}
`
	filename := writeTempYoruFile(t, src)
	code, out, errOut := captureOutput(t, func() int {
		return runEmitTypedAST(filename)
	})

	if code != 0 {
		t.Fatalf("runEmitTypedAST exit=%d\nstderr:\n%s\nstdout:\n%s", code, errOut, out)
	}
	if errOut != "" {
		t.Fatalf("unexpected stderr:\n%s", errOut)
	}
	if !strings.Contains(out, `CallExpr (void) [Fun=Name "println"`) {
		t.Fatalf("typed AST missing println call details:\n%s", out)
	}
	if !strings.Contains(out, `Operation + (int) [X=CallExpr (int)`) {
		t.Fatalf("typed AST missing nested binary/call type info:\n%s", out)
	}
	if !strings.Contains(out, `CallExpr (int) [Fun=SelectorExpr .Get`) {
		t.Fatalf("typed AST missing method call type info:\n%s", out)
	}
}

func TestRunEmitLayoutOutputsStructLayout(t *testing.T) {
	src := `package main

type Pair struct {
	a int
	b int
}
`
	filename := writeTempYoruFile(t, src)
	code, out, errOut := captureOutput(t, func() int {
		return runEmitLayout(filename)
	})

	if code != 0 {
		t.Fatalf("runEmitLayout exit=%d\nstderr:\n%s\nstdout:\n%s", code, errOut, out)
	}
	if errOut != "" {
		t.Fatalf("unexpected stderr:\n%s", errOut)
	}
	if !strings.Contains(out, "=== Struct Layouts ===") {
		t.Fatalf("layout output missing header:\n%s", out)
	}
	if !strings.Contains(out, "type Pair struct {") {
		t.Fatalf("layout output missing struct block:\n%s", out)
	}
	if !strings.Contains(out, "a          int") {
		t.Fatalf("layout output missing field a details:\n%s", out)
	}
	if !strings.Contains(out, "size: 16, align: 8") {
		t.Fatalf("layout output missing struct size/align:\n%s", out)
	}
}

func writeTempYoruFile(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	filename := filepath.Join(dir, "input.yoru")
	if err := os.WriteFile(filename, []byte(src), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return filename
}

func captureOutput(t *testing.T, fn func() int) (code int, stdout string, stderr string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}

	os.Stdout = wOut
	os.Stderr = wErr

	code = fn()

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	_ = rOut.Close()
	_ = rErr.Close()

	return code, string(outBytes), string(errBytes)
}

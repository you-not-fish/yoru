// Package main implements the Yoru compiler entry point.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Compiler flags
var (
	emitTokens   = flag.Bool("emit-tokens", false, "Output token stream")
	emitAST      = flag.Bool("emit-ast", false, "Output AST")
	emitTypedAST = flag.Bool("emit-typed-ast", false, "Output typed AST")
	emitSSA      = flag.Bool("emit-ssa", false, "Output SSA")
	emitLL       = flag.Bool("emit-ll", false, "Output LLVM IR")
	emitLayout   = flag.Bool("emit-layout", false, "Output struct layouts")
	output       = flag.String("o", "", "Output file")
	doctor       = flag.Bool("doctor", false, "Check toolchain")
	version      = flag.Bool("version", false, "Print version")
	trace        = flag.Bool("trace", false, "Output timing trace")
	dumpFunc     = flag.String("dump-func", "", "Only dump specific function")
	ssaVerify    = flag.Bool("ssa-verify", false, "Verify SSA after each pass")
	gcStats      = flag.Bool("gc-stats", false, "Print GC statistics")
	gcVerbose    = flag.Bool("gc-verbose", false, "Verbose GC output")
	gcStress     = flag.Bool("gc-stress", false, "Trigger GC on every allocation")
)

// Version information
const Version = "0.1.0-dev"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Yoru Compiler %s\n\n", Version)
		fmt.Fprintf(os.Stderr, "Usage: yoruc [options] <file.yoru>\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *version {
		fmt.Printf("yoruc version %s\n", Version)
		fmt.Printf("go version %s\n", runtime.Version())
		os.Exit(0)
	}

	if *doctor {
		os.Exit(runDoctor())
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: no input file")
		fmt.Fprintln(os.Stderr, "usage: yoruc [options] <file.yoru>")
		os.Exit(1)
	}

	// TODO: Implement compilation pipeline
	fmt.Fprintf(os.Stderr, "yoruc: compilation not yet implemented\n")
	fmt.Fprintf(os.Stderr, "input file: %s\n", args[0])
	os.Exit(1)
}

// runDoctor checks the toolchain and returns an exit code.
func runDoctor() int {
	fmt.Println("Yoru Toolchain Doctor")
	fmt.Println("=====================")
	fmt.Println()

	allOk := true

	// Check Go version
	goVersion := runtime.Version()
	fmt.Printf("Go:      %s", goVersion)
	if checkGoVersion(goVersion) {
		fmt.Println(" ✓")
	} else {
		fmt.Println(" ✗ (need 1.21+)")
		allOk = false
	}

	// Check clang (required)
	clangVersion, clangOk := checkTool("clang", "--version")
	fmt.Printf("clang:   %s", clangVersion)
	if clangOk {
		fmt.Println(" ✓")
	} else {
		fmt.Println(" ✗ (not found)")
		allOk = false
	}

	// Check opt (optional)
	optVersion, optOk := checkTool("opt", "--version")
	fmt.Printf("opt:     %s", optVersion)
	if optOk {
		fmt.Println(" ✓")
	} else {
		fmt.Println(" (optional, not found)")
	}

	// Check llvm-as (optional)
	llvmAsVersion, llvmAsOk := checkTool("llvm-as", "--version")
	fmt.Printf("llvm-as: %s", llvmAsVersion)
	if llvmAsOk {
		fmt.Println(" ✓")
	} else {
		fmt.Println(" (optional, not found)")
	}

	fmt.Println()
	if allOk {
		fmt.Println("All required tools available!")
		return 0
	}

	fmt.Println("Some required tools are missing.")
	fmt.Println("See docs/toolchain.md for installation instructions.")
	return 1
}

// checkGoVersion returns true if the Go version is 1.21 or higher.
func checkGoVersion(v string) bool {
	// Extract version number (e.g., "go1.23.3" -> "1.23")
	if !strings.HasPrefix(v, "go") {
		return false
	}
	v = strings.TrimPrefix(v, "go")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}

	// Check major.minor version
	major := parts[0]
	minor := parts[1]

	// Go 1.21+ is required
	if major == "1" {
		// Parse minor version
		var minorNum int
		fmt.Sscanf(minor, "%d", &minorNum)
		return minorNum >= 21
	}

	// Go 2.x or higher is fine
	return major >= "2"
}

// checkTool runs a tool with the given arguments and returns the first line of output.
func checkTool(name string, args ...string) (string, bool) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}

	// Extract first line
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		line := strings.TrimSpace(lines[0])
		// Truncate long lines
		if len(line) > 60 {
			line = line[:57] + "..."
		}
		return line, true
	}
	return "", false
}

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Yoru is an educational compiler for a simplified Go-like language, implemented in Go. The primary goal is to deeply understand compiler development, particularly Go compiler design principles. The project implements a full compilation pipeline from lexical analysis to LLVM IR code generation, with plans for GC support.

**Key Design Philosophy:**
- Simplified Go-like syntax (single return values, simple `for` loops, no slices/maps)
- Distinguishes `*T` (stack-only pointers) from `ref T` (GC-managed heap references)
- Manual implementation of all compiler phases (no parser generators)
- Uses LLVM as the backend via `llir/llvm` (pure Go, no CGO)

## Build and Development Commands

### Building
```bash
make build              # Build the yoruc compiler to build/yoruc
make                    # Same as 'make build'
go build -v ./cmd/yoruc # Direct Go build
```

### Testing
```bash
make test              # Run all Go tests
go test ./...          # Same as above
go test ./internal/syntax -v  # Test specific package with verbose output
go test -run TestASI   # Run specific test by name
go test -fuzz=FuzzScanner -fuzztime=5m  # Fuzz testing (5 minutes)
```

### Smoke Testing
```bash
make smoke             # Link and run runtime test (verifies runtime linkage)
make smoke-gc-verbose  # Same with GC tracing enabled
make layout-test       # Verify C/compiler struct layout consistency
```

### Toolchain Management
```bash
make doctor            # Check required tools (clang, opt, llvm-as)
./build/yoruc -doctor  # Run toolchain check after build
make deps              # Update Go dependencies
```

### Using the Compiler
```bash
./build/yoruc -emit-tokens file.yoru    # Output token stream
./build/yoruc -emit-tokens -no-asi file.yoru  # Disable automatic semicolon insertion
./build/yoruc -version                  # Show version
```

## Architecture

### Compilation Pipeline (5-Stage)

```
.yoru source → [Lexer/Parser] → AST
                     ↓
            [Sema/Types] → Typed AST (with desugaring)
                     ↓
            [SSA Gen] → SSA/MIR (middle IR)
                     ↓
            [SSA Passes] → Optimized SSA (DCE, CSE, ConstProp)
                     ↓
            [Codegen] → LLVM IR → clang link with runtime → executable
```

**Critical Design Decision:** Optimizations are done at the SSA level only. AST/Typed AST only perform desugaring to avoid maintaining two optimization systems.

### Directory Structure

```
cmd/yoruc/          # Compiler entry point, CLI flags
internal/syntax/    # Lexer, Parser, AST nodes (Phase 1-2)
internal/types/     # Type system, Universe (Phase 3)
internal/types2/    # Type checker (Phase 3)
internal/ssa/       # SSA IR, optimization passes (Phase 4)
internal/codegen/   # LLVM IR generation (Phase 5)
internal/rtabi/     # ABI constants shared with runtime (CRITICAL)
runtime/            # C runtime (malloc, GC, print, panic)
test/               # E2E tests, ABI tests
docs/               # Design docs, phase plans
```

### Phase Status

- **Phase 0** (✅ Complete): Toolchain setup, ABI definition, runtime linkage
- **Phase 1** (✅ Complete): Lexer with ASI support, position tracking, 288 test cases
- **Phase 2** (🔜 Next): Parser, AST construction
- **Phase 3-8**: Type checking, SSA, codegen, GC (see `docs/yoru-compiler-design.md`)

### Runtime ABI (Critical Contract)

**Platform Lock (Phase 0):** `arm64-apple-macosx26.0.0`

The compiler and runtime must agree on:
- **Target triple & DataLayout** (defined in `internal/rtabi/types.go`)
- **Object header layout** (16 bytes: TypeDesc* + next_mark)
- **TypeDesc structure** (size + num_ptrs + offsets pointer)
- **Type sizes/alignments** (int=8, float=8, bool=1, ptr=8, string=16)

**Golden Rule:** All layout constants in `internal/rtabi/types.go` MUST match `runtime/runtime.h`. Layout consistency is verified by `make layout-test`.

### Type System Design

**Two-Pointer System (UAF Prevention):**
- `*T`: Stack-only pointers, created by `&local`, cannot escape to heap/globals/returns
- `ref T`: GC-managed heap references, created by `new(T)`, can be nil
- **Hard rule:** `ref T → *T` conversion is forbidden (compile error)

**ASI (Automatic Semicolon Insertion):**
- Enabled by default, insertions after: `NAME`, `LITERAL`, `break`, `continue`, `return`, `)`, `]`, `}`
- Disable with `-no-asi` flag for testing/debugging

### Lexer Implementation Details (Phase 1)

**Scanner architecture:**
- `source` struct: UTF-8 character reader with 1-based line:col tracking
- `Scanner` embeds `source`, maintains current token state
- ASI logic: `nlsemi` flag tracks whether to insert `;` before newline/EOF
- Position is tracked BEFORE reading the character (fixed in initial implementation)

**Key implementation details:**
- Number scanning: Handles `0x`, `0o`, `0b` prefixes; validates trailing digits
- String scanning: Returns decoded content (escape sequences interpreted)
- Binary digit validation: `0b123` correctly errors on `2` and `3`
- Comment handling: `//` line comments skipped by `scanOperator()`

**Pre-declared identifiers** (`int`, `float`, `bool`, `string`, `true`, `false`, `nil`, `println`) are scanned as `_Name` tokens, not keywords. They are bound in the Universe during Phase 3.

### Testing Strategy

**Unit tests:** `internal/syntax/*_test.go` - 288 test cases covering tokens, ASI, positions, errors
**Golden tests:** Compare output against `.golden` files (for AST, SSA dumps)
**Fuzz tests:** `FuzzScanner` runs continuously to find edge cases
**Smoke tests:** `test/runtime_test.ll` linked with `runtime/runtime.c` verifies ABI
**Layout tests:** `test/abi/layout_basic.c` ensures C/compiler agreement on struct layouts

### Common Gotchas

1. **Position tracking:** The `nextch()` function updates position BEFORE checking EOF, not after
2. **Number scanning:** Don't accumulate the first digit twice (use `litBuf.Reset()`)
3. **Binary literals:** Must check for invalid trailing digits like `0b129`
4. **ASI at EOF:** Tests must expect `_Semi` token with literal "EOF" before `_EOF`
5. **Layout consistency:** Changes to `rtabi/types.go` MUST be reflected in `runtime/runtime.h`

### Documentation

- `docs/yoru-compiler-design.md`: Complete language spec and implementation roadmap
- `docs/runtime-abi.md`: ABI specification (object headers, type descriptors, calling convention)
- `docs/phase-1-lexer-design.md`: Detailed Phase 1 design and implementation notes
- `docs/toolchain.md`: External dependencies and version requirements

### Key References

The implementation heavily references Go compiler source code:
- Lexer: `cmd/compile/internal/syntax/scanner.go`
- Parser: `cmd/compile/internal/syntax/parser.go`
- Types: `cmd/compile/internal/types2/`
- SSA: `cmd/compile/internal/ssa/`

## Development Workflow

1. **Before changes:** Run `make test` to ensure baseline passes
2. **During development:** Use `-emit-tokens` to debug lexer output
3. **Before commit:** Run `make test && make smoke && make layout-test`
4. **For new features:** Add test cases before implementation (TDD recommended)
5. **When changing ABI:** Update BOTH `rtabi/types.go` AND `runtime/runtime.h`, then run `make layout-test`

## Current Focus (Phase 1 Complete)

Phase 1 (Lexer) is complete with all verification passing:
- ✅ 288 test cases pass
- ✅ Fuzz testing stable (220k iterations, 10s)
- ✅ `-emit-tokens` CLI integration complete
- ✅ ASI correctly implemented with `-no-asi` toggle

Next priority is Phase 2 (Parser):
- Define AST node structures in `internal/syntax/nodes.go`
- Implement recursive descent parser in `internal/syntax/parser.go`
- Add `-emit-ast` support to CLI
- Create golden test files for AST output

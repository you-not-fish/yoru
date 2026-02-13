# Yoru Compiler Makefile
#
# Usage:
#   make          - Build the compiler
#   make test     - Run Go tests
#   make smoke    - Run smoke test (runtime linkage)
#   make doctor   - Check toolchain
#   make clean    - Remove build artifacts

.PHONY: all build test e2e smoke layout-test doctor clean deps help

# Configuration
GO := go
GOFLAGS := -v
TARGET := arm64-apple-macosx26.0.0

# Directories
BUILD_DIR := build
RUNTIME_DIR := runtime
TEST_DIR := test

# Targets
YORUC := $(BUILD_DIR)/yoruc

# Default target
all: build

# Build compiler
build: $(YORUC)

$(YORUC): cmd/yoruc/main.go internal/rtabi/*.go internal/codegen/*.go
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $@ ./cmd/yoruc

# Run all Go tests
test: build
	$(GO) test ./...

# E2E tests: compile .yoru → .ll → binary, compare output against golden files
e2e: build
	$(GO) test ./test/e2e/ -v

# Smoke test: verify runtime linkage
smoke: build
	@echo "=== Smoke Test ==="
	@mkdir -p $(BUILD_DIR)
	clang -target $(TARGET) \
		$(TEST_DIR)/runtime_test.ll \
		$(RUNTIME_DIR)/runtime.c \
		-o $(BUILD_DIR)/runtime_test
	$(BUILD_DIR)/runtime_test
	@echo ""
	@echo "=== Smoke Test PASSED ==="

# Layout consistency test: compare C and compiler layouts
layout-test:
	@echo "=== Layout Consistency Test ==="
	@mkdir -p $(BUILD_DIR)
	clang -target $(TARGET) \
		$(TEST_DIR)/abi/layout_basic.c \
		-o $(BUILD_DIR)/layout_basic
	$(BUILD_DIR)/layout_basic > $(BUILD_DIR)/layout_basic.out
	diff -u $(TEST_DIR)/abi/layout_basic.golden $(BUILD_DIR)/layout_basic.out
	@echo "=== Layout Test PASSED ==="

# Run doctor to check toolchain
doctor: build
	$(YORUC) -doctor

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

# Install/update Go dependencies
deps:
	$(GO) mod tidy

# Verify LLVM IR (requires opt)
verify-ll:
	@if [ -f "$(BUILD_DIR)/output.ll" ]; then \
		opt -verify $(BUILD_DIR)/output.ll -o /dev/null && echo "LLVM IR verified"; \
	else \
		echo "No output.ll found"; \
	fi

# Run with verbose GC
smoke-gc-verbose:
	@echo "=== Smoke Test (GC Verbose) ==="
	@mkdir -p $(BUILD_DIR)
	clang -target $(TARGET) \
		$(TEST_DIR)/runtime_test.ll \
		$(RUNTIME_DIR)/runtime.c \
		-o $(BUILD_DIR)/runtime_test
	YORU_GC_VERBOSE=1 $(BUILD_DIR)/runtime_test
	@echo ""
	@echo "=== Smoke Test PASSED ==="

# Help
help:
	@echo "Yoru Compiler Build System"
	@echo ""
	@echo "Targets:"
	@echo "  all             Build everything (default)"
	@echo "  build           Build the compiler"
	@echo "  test            Run Go tests"
	@echo "  e2e             Run end-to-end codegen tests"
	@echo "  smoke           Run smoke test (runtime linkage)"
	@echo "  smoke-gc-verbose Run smoke test with GC tracing"
	@echo "  layout-test     Run layout consistency test"
	@echo "  doctor          Check toolchain"
	@echo "  clean           Remove build artifacts"
	@echo "  deps            Update Go dependencies"
	@echo "  verify-ll       Verify LLVM IR (requires opt)"
	@echo ""
	@echo "Configuration:"
	@echo "  TARGET=$(TARGET)"

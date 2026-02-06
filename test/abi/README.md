# Layout Consistency Test

This directory provides a C reference for struct layout under the locked
DataLayout. It should match the compiler's `-emit-layout` output.

Run (host toolchain):

```bash
clang -target arm64-apple-macosx26.0.0 test/abi/layout_basic.c -o /tmp/layout_basic
/tmp/layout_basic > /tmp/layout_basic.out

diff -u test/abi/layout_basic.golden /tmp/layout_basic.out
```

Compiler side:

```bash
yoruc -emit-layout test/types/testdata/layout_basic.yoru > /tmp/layout_basic.out

diff -u test/abi/layout_basic.golden /tmp/layout_basic.out
```

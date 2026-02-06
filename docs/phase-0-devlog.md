# Phase 0 开发记录（工具链 & ABI）

本文档记录 Phase 0 的开发内容与取舍，覆盖实际实现、测试与限制。内容以仓库当前状态为准。

## 目标与范围

Phase 0 的核心目标是锁定 ABI 与工具链，跑通最小的 LLVM IR + runtime 链接，并建立后续阶段的约束基础。

具体目标（来自设计文档的 Phase 0 要求）：
- 定义 ABI 文档（`docs/runtime-abi.md`），锁定 `target triple` 与 `DataLayout`。
- 实现最小 C runtime，并能与 LLVM IR 链接运行。
- 提供 toolchain 检查工具（`yoruc -doctor`）及文档。
- 提供 smoke 测试与 CI 基础。

## 已完成的实现内容

### ABI 与平台锁定
- 锁定平台：`arm64-apple-macosx26.0.0`。
- 锁定 DataLayout：`e-m:o-i64:64-i128:128-n32:64-S128-Fn32`。
- ABI 规范明确：对象头、TypeDesc、string、ref 表示、运行时函数签名等。
- 共享 ABI 常量：`internal/rtabi/types.go` 与 `runtime/runtime.h` 保持一致。

相关文件：
- `docs/runtime-abi.md`
- `internal/rtabi/types.go`
- `runtime/runtime.h`

### 最小运行时（C）
- 提供基础 I/O：`rt_print_i64`/`rt_print_f64`/`rt_print_bool`/`rt_print_string`/`rt_println`。
- 提供错误处理：`rt_panic`/`rt_panic_string`。
- 提供分配接口：`rt_alloc`（返回数据区指针，带对象头）。
- 提供 GC 基础实现（mark-sweep + LLVM shadow stack 遍历）。

相关文件：
- `runtime/runtime.h`
- `runtime/runtime.c`

### 手写 LLVM IR Smoke Test
- 提供 `test/runtime_test.ll`，验证：
  - 运行时函数链接可用
  - 字符串与基本打印可用
  - `rt_alloc` 分配可用
  - `rt_collect` 可调用

相关文件：
- `test/runtime_test.ll`
- `Makefile`（`make smoke`）

### 工具链检查与文档
- `yoruc -doctor` 检查：Go/clang 必需，opt/llvm-as 可选。
- `docs/toolchain.md` 记录安装与版本需求。

相关文件：
- `cmd/yoruc/main.go`
- `docs/toolchain.md`

### CI 基础
- GitHub Actions（macOS）跑通 build/doctor/smoke/layout-test/go test。
- 输出 toolchain 版本信息，便于复现。

相关文件：
- `.github/workflows/ci.yml`

## 关键取舍与理由

### 锁定单一平台
- 取舍：仅支持 macOS arm64（Apple Silicon）。
- 理由：避免前端布局与后端 DataLayout 不一致导致的字段偏移错误；降低早期复杂度。

### 运行时使用 C
- 取舍：runtime 用 C 实现，而非 Go。
- 理由：与 clang/LLVM 链接路径清晰，ABI 可控，符合后续生成 LLVM IR 的方向。

### Shadow Stack 与 GC 的提前实现
- 取舍：虽然 Phase 0 仅要求“最小 runtime”，当前 runtime 已包含 mark-sweep 与 shadow-stack 扫描。
- 理由：为后续 Phase 6/7 提前建立 ABI 与数据结构；避免后期推翻。
- 风险与补救：由于编译器尚未生成 gcroot，本阶段禁止自动触发 GC（见“修复与安全措施”）。

## 修复与安全措施（本轮）

### 1) 修复 shadow-stack root 扫描语义
- 问题：`entry->roots[i]` 是“root slot 地址”，不能当作对象指针直接标记。
- 修复：先把 `roots[i]` 当作 `void**` 解引用，再标记真实对象指针。
- 影响：避免将栈槽地址误当对象导致崩溃或误标记。

相关修改：
- `runtime/runtime.c`
- `runtime/runtime.h`（注释澄清 root slot 语义）

### 2) 禁止 Phase 0 自动 GC
- 问题：编译器尚未生成 `llvm.gcroot`，自动 GC 可能回收仍在使用的对象。
- 修复：默认关闭自动 GC，只在显式启用时触发。
- 新增环境变量：
  - `YORU_GC_ENABLE=1` 启用自动 GC
  - `YORU_GC_STRESS=1` 每次分配触发 GC
- 说明：显式调用 `rt_collect()` 仍可执行，用于测试。

相关修改：
- `runtime/runtime.c`

### 3) 文档路径纠正
- 问题：设计文档中仍引用 `runtime/rt.h` 与 `runtime/rt.c`，但实际文件为 `runtime/runtime.h`、`runtime/runtime.c`。
- 修复：更新文档路径与示例命令。

相关修改：
- `docs/yoru-compiler-design.md`

## 使用到的关键技术

- LLVM IR（手写 `.ll`）：验证 ABI + runtime 链接路径。
- DataLayout/Target Triple 固定：保证前端布局与后端一致。
- Shadow-stack GC（LLVM 规范结构）：为后续 GC 集成做准备。
- Makefile 统一入口：`make build`/`make smoke`/`make layout-test`。
- CI（GitHub Actions）：固定环境检查与 smoke 测试。

## 测试与覆盖情况

现有测试（已提供，但不一定在本次修改中执行）：
- `make smoke`：链接 `test/runtime_test.ll` + runtime 并运行。
- `make layout-test`：用 clang 输出 struct 布局并与 golden 比对。
- `make doctor`：工具链存在性验证。
- CI：macOS 上执行 build/doctor/smoke/layout-test/go test。

覆盖现状与不足：
- 已覆盖：运行时链接、基本函数调用、分配、布局一致性（仅 C 端）。
- 未覆盖：编译器输出的结构体布局（尚未实现布局计算）。
- 未覆盖：GC root 生成（Phase 6 才实现），因此当前不应依赖自动 GC 正确性。

## 当前限制与已知缺口

- 编译器主流程尚未实现（`yoruc` 目前只支持 `-doctor`）。
- 布局一致性测试仅验证 C 端输出，不含编译器侧布局。
- 自动 GC 默认关闭，直到生成 `llvm.gcroot`。

## 后续建议（进入 Phase 1+ 前）

- 在类型系统/布局实现后，补齐“编译器布局 vs clang”一致性测试。
- 在 Phase 6 加入 `llvm.gcroot` 生成前，不要启用 `YORU_GC_ENABLE`。
- 将 `rt_collect` 的调用路径与 `gc_stress` 测试挂到编译器参数上。


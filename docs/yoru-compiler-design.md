# Yoru 编程语言编译器设计文档

## 概述

Yoru（日语"夜"的意思）是一门用 Go 实现的编程语言，主要目的是深入学习编译器开发，特别是理解 Go 编译器的设计理念。

## 1. 语言设计

### 1.1 设计哲学

Yoru 是一门**简化版的 Go-like 语言**，保留足够的复杂度来学习编译器核心概念，同时避免过度复杂。

### 1.2 核心特性（简化版）

#### 基本类型（4 种）

```yoru
int      // 64位整数（唯一整数类型）
float    // 64位浮点（唯一浮点类型）
bool     // 布尔
string   // 字符串（先只支持字面量 + println）
```

#### 复合类型（3 种）

```yoru
[N]T     // 数组（编译期固定大小）
*T       // 指针（非托管，只允许 &local 产生）
struct   // 结构体
```

> **注意**：`interface` 作为后期扩展特性，参见 Phase 8+。

#### 引用类型（GC 托管）

```yoru
ref T    // GC 托管引用，可为 nil（通过 new(T) 产生）
```

> **设计决策**：区分 `*T`（裸指针）和 `ref T`（托管引用），便于 GC 实现。
> - `*T`：只能通过 `&local` 产生，不参与 GC
> - `ref T`：通过 `new(T)` 产生，GC 托管

**⚠️ ref/ptr 硬规则（防止 UAF）**

| 规则 | 说明 |
|------|------|
| 禁止 `ref T` → `*T` 转换 | 包括显式转换、隐式取地址、字段地址传播；否则会 UAF |
| 选择器自动解引用 | `p.width` 在 `p: ref Rectangle` 上合法（等价于 `(*p).width`） |
| `nil` 是零值 | `nil` 同时是 `ref T` 和 `*T` 的零值；Typed AST 需标注 nil 的具体类型 |

**⚠️ `*T` 生命周期/逃逸规则（强制）**

> 目标：把 `*T` 约束为**栈内短生命周期指针**，把长期存活需求统一交给 `ref T`，避免 UAF。

- `*T` 只能指向**当前函数栈帧**中的对象（局部变量、参数的栈副本）。
- `*T` **禁止逃逸**：不得返回、不得写入全局、不得写入任何 `ref` 持有的对象或其字段/数组元素。
- `*T` 不能被保存到**生命周期更长**的位置（例如：返回值、全局变量、heap 对象字段）。
- 任何无法证明“短生命周期”的 `*T` 用法直接报错；若要长期存活，必须改用 `ref T = new(T)`。

示例（应报错）：

```yoru
var g *int

func bad() *int {
    var x int
    g = &x        // error: *T 逃逸到全局
    return &x     // error: *T 逃逸到返回值
}

func bad2() {
    var x int
    var r ref Box = new(Box)
    r.ptr = &x    // error: *T 逃逸到 heap 对象
}
```

#### 控制流（极简）

```yoru
if/else           // 条件分支
for cond { }      // 唯一循环形式（无 init/post，无 range）
return            // 返回
```

#### 函数和方法

```yoru
func name(params) returnType { body }  // 函数（单返回值）
func (recv T) name() { }               // 方法
```

> **注意**：多返回值作为后期扩展特性，参见 Phase 8+。

**方法集与调用规则（简化）**

- 方法接收者允许 `T` 或 `*T`。
- 选择器调用（`x.M()`）支持**自动解引用/取地址**，但不产生 `ref T → *T` 的类型转换：
  - `x: T` 调用 `(*T)` 方法时，要求 `x` 可寻址，编译器隐式降为 `(&x).M()`。
  - `x: *T` 可调用 `T` 方法（隐式解引用）。
  - `x: ref T` 可调用 `T`/`*T` 方法（隐式解引用），但 Typed AST 不把它当成 `*T` 值。
- **不支持** method value / method expression（如 `T.M`、`x.M` 作为一等函数）以简化实现。

#### 变量声明

```yoru
var x T = value   // 显式类型
x := value        // 类型推断
```

#### 其他

```yoru
type T = U        // 类型别名
type T struct {}  // 类型定义
package/import    // 包管理（可解析但暂不做语义化）
new(T)            // 分配并返回 ref T
```

#### 包与编译单元（学习版约束）

- Phase 0-3：**单文件编译**；`package` 必须是 `main`。
- `import` 仅做语法解析，语义阶段给出 **"unsupported import"** 提示（不解析包，不做符号加载）。
- Phase 4+：可扩展为“同目录多文件同包编译”，但**不支持初始化顺序**与**跨包依赖**（后期再做）。

#### 省略的特性

| 特性 | 替代方案 |
|------|----------|
| uint, int8, int16... | 统一用 int |
| float32 | 统一用 float |
| slice []T | 用 [N]T 数组 + 指针 |
| for init; cond; post | 用 for cond {} + 外部变量 |
| for range | 用 for + 索引 |
| switch | 用 if/else 链 |
| rune | 用 int |
| interface | 后期扩展（Phase 8+） |
| 多返回值 | 后期扩展（Phase 8+） |

### 1.3 刻意省略的特性

| 特性 | 省略原因 |
|------|----------|
| `defer` | 需要 defer 链表、栈指针追踪、open-coded defer 优化 |
| `recover` | 必须在 defer 中调用，与 defer 紧密耦合 |
| `map` 类型 | 需要复杂的运行时哈希表 |
| `slice` 类型 | 需要运行时支持（len, cap, append, 扩容） |
| `switch` | if/else 链可替代 |
| 多种整数/浮点类型 | 简化类型系统，避免类型转换复杂度 |
| `for range` | 需要迭代器协议 |
| 泛型 | 类型参数化显著增加复杂度 |
| 可变参数函数 | 后期可添加 |
| `interface` | 需要 itab/类型信息/方法集/动态派发 ABI，复杂度高 |

### 1.4 错误处理机制

Yoru 采用**panic + 返回值**的简化错误处理模型：

**panic 行为：**
- `panic(msg)` 导致程序立即终止并打印错误消息
- 不支持 `defer` 和 `recover`（避免复杂的栈展开机制）
- 不要求栈追踪（后期可加）

```yoru
func divide(a, b int) int {
    if b == 0 {
        panic("division by zero")  // 程序终止，打印消息
    }
    return a / b
}
```

**常规错误处理模式（使用返回值）：**

```yoru
type Result struct {
    value int
    ok    bool
}

func divide_safe(a, b int) Result {
    if b == 0 {
        return Result{0, false}
    }
    return Result{a / b, true}
}

func main() {
    r := divide_safe(10, 0)
    if !r.ok {
        println("error: division by zero")
        return
    }
    println(r.value)
}
```

**设计理由：**
1. defer/panic/recover 紧密耦合，recover 必须在 defer 中调用才有效
2. defer 需要复杂的运行时支持（defer 链表、栈指针追踪、open-coded defer 优化）
3. 专注编译器核心学习，错误处理可后期扩展

### 1.5 示例程序

```yoru
package main

// 结构体
type Rectangle struct {
    width  int
    height int
}

// 方法
func (r Rectangle) Area() int {
    return r.width * r.height
}

// 函数
func sum(arr *[5]int) int {
    var total int = 0
    var i int = 0
    for i < 5 {          // 极简 for：只有条件
        total = total + arr[i]
        i = i + 1
    }
    return total
}

func main() {
    // 变量声明
    var r Rectangle
    r.width = 10
    r.height = 5

    // 方法调用
    println(r.Area())  // 50

    // 数组和指针
    var arr [5]int
    arr[0] = 1
    arr[1] = 2
    arr[2] = 3
    arr[3] = 4
    arr[4] = 5
    println(sum(&arr))  // 15

    // 条件分支
    if r.width > r.height {
        println("wider")
    } else {
        println("taller")
    }

    // ref 使用
    var p ref Rectangle = new(Rectangle)
    p.width = 20
    println(p.width)
}
```

---

## 2. 编译器架构

### 2.1 总体架构（5 阶段流水线）

```
源代码 (.yoru 文件)
        │
        ▼
  [1. Lexer/Parser] ──────────> AST
        │
        ▼
  [2. Sema/Types] ────────────> Typed AST（含 desugar）
        │
        ▼
  [3. SSA 生成] ──────────────> SSA/MIR（中端主 IR）
        │
        ▼
  [4. SSA Passes] ────────────> 优化后的 SSA
   - DCE (死代码消除)              │
   - CSE (公共子表达式消除)          │
   - ConstProp (常量传播)           │
   - (可选) Escape Analysis（Phase 8，用于减少 heap） ▼
                             [5. Codegen] ────> LLVM IR
                                  │
                                  ▼
                             clang 链接 runtime
                                  │
                                  ▼
                              可执行文件
```

> **设计决策**：优化集中在 SSA 层，AST/Typed AST 只做 desugar，避免两套优化体系。

### 2.2 目录结构

```
yoru/
├── cmd/
│   └── yoruc/              # 编译器入口
│       └── main.go
├── internal/
│   ├── syntax/             # 词法分析、语法分析、AST 节点
│   │   ├── token.go        # Token 定义
│   │   ├── scanner.go      # 词法分析器实现
│   │   ├── source.go       # 源文件读取器
│   │   ├── pos.go          # 位置追踪
│   │   ├── nodes.go        # AST 节点定义
│   │   ├── parser.go       # 语法分析器实现
│   │   └── walk.go         # AST 遍历工具
│   ├── types/              # 类型系统
│   │   ├── type.go         # 类型表示
│   │   ├── universe.go     # 预声明类型
│   │   └── scope.go        # 作用域管理
│   ├── types2/             # 类型检查器（参照 Go 设计）
│   │   ├── check.go        # 主类型检查
│   │   ├── expr.go         # 表达式检查
│   │   ├── stmt.go         # 语句检查
│   │   └── decl.go         # 声明检查
│   ├── ssa/                # SSA 形式（中端主 IR）
│   │   ├── value.go        # SSA 值
│   │   ├── block.go        # 基本块
│   │   ├── func.go         # SSA 函数
│   │   ├── op.go           # SSA 操作
│   │   ├── verify.go       # SSA 验证器（必须有）
│   │   └── passes/         # 优化 Pass
│   │       ├── deadcode.go
│   │       ├── cse.go
│   │       ├── constprop.go
│   │       └── escape.go        # Phase 8（可选，用于减少 heap）
│   ├── codegen/            # 代码生成
│   │   └── llvm.go         # SSA → LLVM IR 转换
│   └── rtabi/              # 运行时 ABI（编译器与 runtime 共享协议）
│       ├── types.go        # TypeDesc 布局常量
│       └── funcs.go        # runtime 函数签名
├── runtime/                # C 语言运行时（与 clang 链接）
│   ├── runtime.h           # ABI 定义（ObjectHeader, TypeDesc）
│   └── runtime.c           # 分配器、print、panic、GC
├── test/                   # 测试用例
├── examples/               # 示例程序
└── docs/                   # 文档
    └── runtime-abi.md      # Runtime ABI 规范（必须先写）
```

---

## 3. 技术决策

### 3.1 后端方案：混合策略

**推荐：先用 LLVM (llir/llvm)，后可选添加原生后端**

| 方案 | 优点 | 缺点 | 学习价值 |
|------|------|------|----------|
| **llir/llvm** | 纯 Go，无 CGO<br>立即获得优化<br>跨平台<br>专注前端/中端 | 较少代码生成控制<br>依赖 LLVM 概念 | 前端/中端学习价值高 |
| **原生后端** | 完全控制<br>理解寄存器分配<br>匹配 Go 方案 | 复杂度更高<br>平台相关 | 非常高但耗时 |

**学习路径：**
1. **Phase 0-5**：使用 `llir/llvm` 快速出结果
2. **Phase 6-7**：实现运行时（GC 等）
3. **Phase 8+**：实现原生 x86-64 后端（进阶目标）

### 3.2 关键库决策

| 组件 | 推荐 | 理由 |
|------|------|------|
| **词法分析器** | 手动实现 | 学习核心；Go 的 scanner.go 是极佳参考 |
| **语法分析器** | 手动递归下降 | Go 使用此方案；最适合学习 |
| **类型检查器** | 手动实现 | 编译器核心技能 |
| **SSA** | 手动实现 | Go 的 SSA 包是黄金标准 |
| **后端** | llir/llvm（后原生） | 平衡学习和进度 |
| **运行时** | C 语言实现 | 与 clang 链接最稳定 |

### 3.3 不使用外部解析器生成器

避免使用 ANTLR 或 yacc 的原因：
1. 手动实现学得更多
2. Go 编译器使用手写解析器
3. 手动解析器的错误信息更好
4. 对 AST 结构有更多控制

### 3.4 目标平台与内存布局（必须先定稿）

业界推荐做法：**前端布局 = 后端 DataLayout**，否则字段偏移、GC offsets 会错。

- Phase 0 固定 `target triple` 与 `DataLayout`（学习期先锁定一种平台，如 x86-64 SysV），写入 `docs/runtime-abi.md`。
- 代码生成把同一份 `DataLayout` 写进 LLVM Module；类型/结构体布局从该布局规则推导（不再“手算”一套）。
- `runtime/runtime.h` 中 `ObjectHeader/TypeDesc` 与编译器生成保持一致。
- 加一致性测试：对比 clang/LLVM 的 `offsetof/sizeof` 与编译器计算结果（golden）。

---

## 4. 实现阶段

### Phase 0：工具链 & ABI 定稿（第 0-1 周）⭐ 先做

```
任务：
- [ ] 编写 docs/runtime-abi.md：
    - rt_alloc(size, typedesc)、rt_collect()、rt_panic(msg)、rt_print_*
    - ObjectHeader、TypeDesc 内存布局（字段顺序、对齐）
- [ ] 在 docs/runtime-abi.md 明确 target triple / DataLayout（学习期先锁定一种平台）
- [ ] 编译器/运行时共享同一份布局规则（禁止“前端手算一套，后端又一套”）
- [ ] 编写 docs/toolchain.md：外部依赖版本要求（clang, opt, llvm-as）
- [ ] 在 runtime/ 写最小 C runtime：rt_print_i64 + rt_panic
- [ ] 手写最小 .ll（或用 llir 生成），调用 rt_print_i64，用 clang 链接跑通
- [ ] 实现 yoruc doctor：探测 clang/opt/llvm-as，缺失时给 warning 并降级

测试/验收：
- [ ] make smoke：生成可执行文件并输出固定结果
- [ ] CI 上能跑（Linux/macOS 至少一个）
- [ ] CI 打印 toolchain 版本摘要（便于复现）
```

### Phase 1：词法分析器（第 2-5 周）

```
任务：
- [ ] Token 类型定义
- [ ] 带位置追踪的源文件读取器（行:列）
- [ ] 基本 token 扫描（标识符、数字、运算符）
- [ ] 关键字识别
- [ ] 字符串字面量处理（含转义序列）
- [ ] 自动分号插入（ASI）默认开启：用 `-no-asi` 关闭

可测试/可观测：
- [ ] `-emit-tokens` 输出 token+pos
- [ ] scanner 单元测试覆盖：
    - 字符串转义非法输入（必须）
    - 注释处理（行尾注释）
- [ ] Fuzz：go test -fuzz=FuzzScanner

验收：
- [ ] 100+ token 测试用例通过
- [ ] fuzz 跑 5-10 分钟不崩
- [ ] 位置信息准确

参考：Go 源码 cmd/compile/internal/syntax/scanner.go
```

### Phase 2：语法分析器（第 6-9 周）

```
任务：
- [ ] AST 节点定义
- [ ] package 和 import 解析（可解析但不语义化）
- [ ] 类型声明：struct（先不做 interface）
- [ ] 函数声明 + 语句块
- [ ] 表达式 Pratt/precedence
- [ ] 语句：if/else、for cond {}、return、赋值、块语句
- [ ] 语法错误恢复：至少做到"一个文件报多个错"

可测试/可观测：
- [ ] `-emit-ast`（建议 JSON + 可读文本两种）
- [ ] golden tests：testdata/*.yoru → *.ast.golden
- [ ] parser fuzz：go test -fuzz=FuzzParse

验收：
- [ ] 示例程序能 parse 成 AST
- [ ] 20 个语法错误用例，错误位置稳定且不 panic

参考：Go 源码 cmd/compile/internal/syntax/parser.go
```

### Phase 3：类型系统 & 语义分析（第 10-13 周）

```
任务：
- [ ] Universe（预声明类型：int, float, bool, string）
- [ ] 预声明函数：println, new, panic
- [ ] 名称解析、作用域（Scope）
- [ ] 类型检查（赋值/调用/字段选择/索引）
- [ ] := 类型推断（先局部推断即可）
- [ ] struct 字段布局（offset/align 计算并打印）
- [ ] ref T 与 *T 类型区分
- [ ] `*T` 非逃逸检查（禁止返回/全局/写入 heap 对象等）
- [ ] 方法集与隐式解引用/取地址规则（见 1.2 方法规则）
- [ ] ⚠️ 禁止 ref T → *T 转换（编译错误，防止 UAF）

可测试/可观测：
- [ ] `-emit-typed-ast`：把每个 Expr/Ident 的类型打印出来
- [ ] `-emit-layout`：打印每个 struct 的 size/align/field offsets
- [ ] 错误测试：error_*.yoru 对比诊断（建议 golden）
- [ ] 专门测试 ref → * 禁止规则
- [ ] 专门测试 `*T` 逃逸检查（返回/全局/写入 heap 字段）

验收：
- [ ] 端到端：println(1+2)、if、for、函数调用可通过类型检查
- [ ] 200+ 类型错误用例
- [ ] ref → * 转换被正确拒绝
- [ ] `*T` 逃逸用例被正确拒绝

参考：Go 源码 cmd/compile/internal/types2/
```

### Phase 4：SSA 生成（第 14-19 周）

> **实现策略**：Alloca-first approach —— 先生成 alloca/load/store 形式，再由 mem2reg 提升为 SSA φ 形式。
> 分为 4 个子阶段：4A（基础设施）→ 4B（AST→SSA lowering）→ 4C（支配树 & mem2reg）→ 4D（优化 passes）。

#### Phase 4A：SSA 基础设施

```
任务：
- [ ] SSA 数据结构：Func/Block/Value（value.go, block.go, func.go）
- [ ] Op 枚举 + OpInfo 表，含 IsPure 标记（op.go）
- [ ] SSA 文本格式打印器（print.go）
- [ ] 基础验证器：nil type、terminator、phi args、边一致性（verify.go）
- [ ] CLI：接入 -emit-ssa 占位（实际 pipeline 在 4B 接入）

可测试/可观测：
- [ ] 手动构建 SSA 函数并打印
- [ ] 验证器能捕获 7+ 种结构错误
- [ ] OpInfo 的 IsPure 分类正确

验收：
- [ ] go test ./internal/ssa/... 全绿
- [ ] 打印格式稳定（golden test）
- [ ] 验证器错误测试全部通过
```

#### Phase 4B：AST→SSA Lowering（alloca 形式）

```
任务：
- [ ] Typed AST → alloca-based SSA 转换（build.go）
- [ ] 表达式 lowering：字面量、二元/一元运算、调用、选择器、索引、new、复合字面量
- [ ] 语句 lowering：赋值、变量声明、if/else（CFG 分裂）、for（回边）、return、break/continue
- [ ] 方法调用 lowering、内建函数处理
- [ ] 端到端：parse → typecheck → SSA → print

可测试/可观测：
- [ ] `-emit-ssa`：完整 pipeline 输出 alloca 形式 SSA
- [ ] `-dump-func=<name>`：只 dump 某个函数
- [ ] golden：*.ssa.golden

验收：
- [ ] fib/循环/函数调用能生成 SSA 且 verify 通过
- [ ] 所有表达式/语句类型都有 lowering 覆盖
```

#### Phase 4C：支配树 & mem2reg

```
任务：
- [ ] Cooper 支配算法（dom.go）
- [ ] 支配边界计算
- [ ] mem2reg pass：提升 alloca 为 SSA 寄存器、插入 phi、重命名变量（passes/mem2reg.go）
- [ ] 带支配检查的完整验证器
- [ ] Pass 基础设施：-dump-before / -dump-after

可测试/可观测：
- [ ] `-ssa-verify`：每次 pass 前后都 verify
- [ ] golden：mem2reg 前后对比

验收：
- [ ] mem2reg 后 alloca 数量显著减少
- [ ] phi 参数数量等于 preds 数量
- [ ] 支配关系一致性检查通过
```

#### Phase 4D：优化 Passes & SROA

```
任务：
- [ ] DCE（passes/deadcode.go）
- [ ] Block-local CSE + IsPure 硬性过滤（passes/cse.go）
- [ ] 常量传播/折叠（passes/constprop.go）
- [ ] 复合值 SROA（passes/sroa.go）
- [ ] `-ssa-stats`：每个函数输出 block 数、value 数、phi 数、call 数

⚠️ SSA Pass 纯度约束：
- Pure 值优化：只对无副作用的 Value 做（整型算术、比较、phi、cast）
- Effect 指令：call/store/panic/print/alloc 等视为有副作用，不可 CSE、不可随意移动、不可删除
- 在 CSE/ConstProp 里用 IsPure() 硬性过滤
- CSE 必须满足**支配关系**；v1 仅做 basic block 内的 CSE（最安全）

可测试/可观测：
- [ ] golden：优化前后对比
- [ ] `-ssa-stats` 统计正确

验收：
- [ ] DCE 能消除未使用的纯值
- [ ] CSE 不会合并副作用指令
- [ ] 常量折叠正确
- [ ] SROA 能拆分简单 struct

参考：Go 源码 cmd/compile/internal/ssa/
```

### Phase 5：LLVM IR Codegen（第 20-27 周）

> **注意**：此阶段先不含 GC，但 `new(T)` 仍走 `rt_alloc(size, typedesc)`（runtime 只做 malloc 不 collect）。
> 这样 Phase 6/7 不需要改动分配路径的大结构，只需加 roots + GC。

> **关键设计决策**：直接生成文本 LLVM IR，不使用 `llir/llvm` 库。
> 理由：教育价值（理解每条指令）、零依赖、完全控制格式、与 `test/runtime_test.ll` 一致、便于调试。
> 使用 opaque pointers (`ptr`)，遵循 LLVM 15+。Bool 在 SSA 流中为 `i1`，仅在 runtime 函数边界用 `i8`（zext/trunc）。

#### Phase 5A：最小可执行程序（常量 + 算术 + Print）

```
目标：第一个 Yoru 程序编译、链接、运行。println(42) → 输出 42。

SSA Ops（18 个）：
- 常量：OpConst64, OpConstFloat, OpConstBool, OpConstString, OpConstNil
- 整数算术：OpAdd64, OpSub64, OpMul64, OpDiv64, OpMod64, OpNeg64
- 浮点算术：OpAddF64, OpSubF64, OpMulF64, OpDivF64, OpNegF64
- 内建：OpPrintln, OpPanic

Block 类型：BlockPlain, BlockReturn, BlockExit

基础设施：
- [ ] internal/codegen/ 包：文本 IR 发射器
- [ ] 类型映射（types.Type → LLVM 类型字符串）
- [ ] 模块头部（target triple, data layout from rtabi）
- [ ] runtime 函数声明（from rtabi.RuntimeFunctions()）
- [ ] 字符串全局常量（OpConstString）
- [ ] CLI：yoruc -emit-ll 标志
- [ ] E2E 测试框架

验收：
- [ ] println(42) → 输出 "42"
- [ ] println("Hello, Yoru!") → 输出 "Hello, Yoru!"
- [ ] println(1+2*3) → 输出 "7"
- [ ] println(3.14) → 输出 "3.14"
```

#### Phase 5B：控制流 + 函数

```
目标：Fibonacci 里程碑 — 带参数的函数、if/else、for 循环、递归调用。

SSA Ops（新增 18 个，累计 36 个）：
- 整数比较：OpEq64, OpNeq64, OpLt64, OpLeq64, OpGt64, OpGeq64
- 浮点比较：OpEqF64..OpGeqF64
- 指针比较：OpEqPtr, OpNeqPtr
- 布尔：OpNot, OpAndBool, OpOrBool
- 转换：OpIntToFloat, OpFloatToInt
- SSA：OpPhi, OpCopy, OpArg
- 调用：OpStaticCall

Block 类型：BlockIf（新增）

验收：
- [ ] fibonacci(10) = 55
- [ ] if/else 分支
- [ ] for 循环
- [ ] 多函数调用
- [ ] 布尔打印
```

#### Phase 5C：内存 + 聚合类型 + 堆分配

```
目标：struct、array、new(T)、方法调用 — 完成 Phase 5 全部范围。

SSA Ops（新增 7 个，总计 43 个）：
- 内存：OpAlloca, OpLoad, OpStore, OpZero
- Struct/Array：OpStructFieldPtr, OpArrayIndexPtr
- 地址：OpAddr
- 堆分配：OpNewAlloc
- Nil 检查：OpNilCheck
- 字符串：OpStringLen, OpStringPtr
- 调用：OpCall（间接调用）

基础设施：TypeDesc 生成、命名 struct 类型、GEP 模式、llvm.memset、rt_bounds_check

验收：
- [ ] struct 字段读写
- [ ] new(T) 分配成功
- [ ] 数组索引
- [ ] 方法调用
- [ ] 完整示例程序
```

### Phase 6：接入 shadow-stack roots（第 24-27 周）

> **关键阶段**：开始"正确 GC"主线。

```
任务：
- [ ] 对每个函数：设置 fn.GC = "shadow-stack"（llir 支持该字段）
- [ ] 在 entry block：为每个 ref 局部/临时值创建 root slot（alloca），并插 llvm.gcroot
- [ ] 栈上复合值含 ref 的根覆盖：依赖前一阶段标量化；无法标量化的复合值强制 heap
- [ ] ⚠️ 所有 gcroot 必须在第一个 basic block（LLVM 硬规则）
- [ ] ⚠️ 覆盖 h(f(), g()) 中间值场景：f() 返回的 ref 必须立即存入 root slot
- [ ] ⚠️ GC 引用在每个 call 前必须先 store 到 gcroot alloca，call 后再 reload（防止 call 触发 GC 丢 root）
- [ ] 实现保守策略：只要是 ref 类型，就强制落栈到 root slot（store/load）

llvm.gcroot 使用要点（来自 LLVM 文档）：
; 1. 函数必须标记 gc "shadow-stack"
define void @foo() gc "shadow-stack" {
entry:
  ; 2. root slot 必须在 entry block
  %root = alloca i8*
  ; 3. 显式初始化为 null
  store i8* null, i8** %root
  ; 4. 声明为 gcroot
  call void @llvm.gcroot(i8** %root, i8* null)
  
  ; 5. call 前 store，call 后 reload
  store i8* %ref_val, i8** %root
  call void @some_func()  ; 可能触发 GC
  %ref_val2 = load i8*, i8** %root
}

可测试/可观测：
- [ ] `-gc-stress`：让 rt_alloc 每次都触发 rt_collect
- [ ] 新增 e2e：专门覆盖 h(f(), g())

验收：
- [ ] -gc-stress 下跑 1e5 次分配不崩
- [ ] 所有 e2e 在 -O0 下稳定
```

### Phase 7：mark-sweep GC（第 28-31 周）

```
任务：
- [ ] runtime 堆对象链表 + header（mark bit、size、TypeDesc*）
- [ ] TypeDesc 生成：每个可分配类型一个全局 TypeDesc
- [ ] GC 实现：
    - 遍历 shadow stack root chain（LLVM 维护的 llvm_gc_root_chain）
    - mark：按 offsets 扫描对象内部 ref 字段
    - sweep：free 未标记对象，清 mark 位
- [ ] GC 仅扫描 shadow-stack roots + heap（不做保守栈扫描，依赖 root slots 全覆盖）
- [ ] rt_alloc：达到阈值触发 collect（先简单：按累计分配字节触发）

TypeDesc 结构（推荐：固定头 + 指针指向 offsets 表）：
typedef struct {
    size_t size;              // 对象大小
    size_t num_ptrs;          // ref 字段数量
    const uint32_t* offsets;  // 指向 offsets 表（更容易做 LLVM 全局常量）
} TypeDesc;

// 编译器生成示例：
// @Offsets_Point = private constant [2 x i32] [i32 0, i32 8]
// @TypeDesc_Point = constant { i64, i64, i32* } { i64 16, i64 2, i32* @Offsets_Point }

⚠️ 为何不用 flexible array member：
- flexible array 做 LLVM 全局常量时没有固定大小，对齐和生成都很麻烦
- 固定头 + 指针方案：C/LLVM 都容易对齐，GC 扫描时用 offsets[i] 遍历即可

Runtime 断言（debug build）：
- [ ] 扫描 root 时检查指针对齐
- [ ] 检查指针是否落在 heap 区间（可选）
- [ ] mark 时检查对象 header magic（防止野指针当对象）

可测试/可观测（必须补齐）：
- [ ] `-gc-stats`：打印 pause time、live bytes、freed bytes、root count
- [ ] `-gc-verbose`：每次 GC 打印触发原因
- [ ] `-gc-verify`：GC 完成后检查一致性（未标记对象已不在链表或已 free）
- [ ] 3 类 GC 回归测试：
    - 环引用（mark-sweep 应能回收）
    - 深链 + 断头（验证传播和回收）
    - 中间值 + 触发 GC（h(f(), g()) 模式）

验收：
- [ ] GC 压测：循环分配 + 丢弃引用，内存不会无限上涨
- [ ] 深链断头后，GC 的 freed 数明显上升
- [ ] -gc-verify 不报一致性错误
```

### Phase 8：扩展特性（第 32 周+）按里程碑启用

> 每个特性都有独立验收与回滚开关。

```
建议顺序：
1. 多返回值：tuple lowering → LLVM struct return（加 e2e）
2. interface（动态派发）：先只做 var i I = &T{} + i.M()（不做 type assert）
3. 逃逸分析 v1：只为"减少 heap 分配"服务（安全规则已在 Phase 3 的非逃逸检查中保证）
4. 内联：只 inline leaf 小函数（先不碰递归/巨大函数）
5. 原生 x86-64 后端（进阶目标）
```

---

## 5. 关键算法和数据结构

### 5.1 词法分析算法

**自动分号插入 (ASI)**

```go
// 来自 Go 的 scanner - 分号插入规则
// 在这些 token 后遇到换行时插入分号：
// - 标识符
// - 字面量（int, float, string）
// - 关键字：break, continue, return
// - 分隔符：), ], }

func (s *scanner) next() {
    nlsemi := s.nlsemi
    s.nlsemi = false

    // ... 扫描 token ...

    if s.ch == '\n' && nlsemi {
        s.tok = _Semi
        s.lit = "newline"
        return
    }
}
```

### 5.2 语法分析算法

**Pratt 解析（表达式优先级）**

```go
// 运算符优先级表（来自 Go 的 tokens.go）
const (
    _ = iota
    precOrOr    // ||
    precAndAnd  // &&
    precCmp     // == != < <= > >=
    precAdd     // + - | ^
    precMul     // * / % & &^ << >>
)

func (p *parser) binaryExpr(prec int) Expr {
    x := p.unaryExpr()
    for {
        op, oprec := p.tokPrec()
        if oprec < prec {
            return x
        }
        p.next()
        y := p.binaryExpr(oprec + 1) // 右结合
        x = &BinaryExpr{Op: op, X: x, Y: y}
    }
}
```

### 5.3 类型检查算法

**类型统一**

```go
func (check *Checker) identical(x, y *Type) bool {
    if x == y {
        return true
    }
    if x.Kind() != y.Kind() {
        return false
    }
    switch x.Kind() {
    case ARRAY:
        return x.NumElem() == y.NumElem() &&
               check.identical(x.Elem(), y.Elem())
    case PTR, REF:
        return check.identical(x.Elem(), y.Elem())
    case STRUCT:
        return check.identicalFields(x.Fields(), y.Fields())
    // ...
    }
}
```

### 5.4 SSA 算法

**支配关系计算**（用于 phi 节点插入）

```go
// 算法来自 Cooper 等人的 "A Simple, Fast Dominance Algorithm"
func computeDominators(fn *Func) {
    // 初始化
    for _, b := range fn.Blocks {
        b.idom = nil
    }
    fn.Entry.idom = fn.Entry

    changed := true
    for changed {
        changed = false
        for _, b := range fn.Blocks {
            if b == fn.Entry {
                continue
            }
            newIdom := firstPred(b)
            for _, p := range b.Preds[1:] {
                if p.idom != nil {
                    newIdom = intersect(p, newIdom)
                }
            }
            if b.idom != newIdom {
                b.idom = newIdom
                changed = true
            }
        }
    }
}
```

**Phi 节点插入**

```go
func insertPhis(fn *Func) {
    // 对于每个变量 v：
    // 1. 找到所有定义 v 的块
    // 2. 计算这些块的支配边界
    // 3. 在边界块插入 phi 节点

    for _, v := range fn.vars {
        defBlocks := v.definingBlocks()
        frontier := dominanceFrontier(defBlocks)

        for _, b := range frontier {
            phi := b.NewValue(OpPhi, v.Type)
            // Phi 参数在重命名阶段填充
        }
    }
}
```

### 5.5 优化算法

**死代码消除**

```go
func deadcode(f *Func) {
    // 标记阶段 - 标记所有可达值
    reachable := make(map[*Value]bool)

    // 从控制值和内存操作开始
    var worklist []*Value
    for _, b := range f.Blocks {
        for _, c := range b.Controls {
            worklist = append(worklist, c)
        }
    }

    for len(worklist) > 0 {
        v := worklist[len(worklist)-1]
        worklist = worklist[:len(worklist)-1]

        if reachable[v] {
            continue
        }
        reachable[v] = true

        for _, arg := range v.Args {
            worklist = append(worklist, arg)
        }
    }

    // 清除阶段 - 移除不可达值
    for _, b := range f.Blocks {
        i := 0
        for _, v := range b.Values {
            if reachable[v] {
                b.Values[i] = v
                i++
            }
        }
        b.Values = b.Values[:i]
    }
}
```

**公共子表达式消除 (CSE)**

```go
func cse(f *Func) {
    // v1：仅做 basic block 内 CSE，避免支配关系问题
    // 跨块 CSE 需要基于支配树/GVN，未实现前禁止跨块替换

    type valueKey struct {
        op   Op
        typ  *Type
        aux  interface{}
        args string // 参数 ID 的规范字符串
    }

    for _, b := range f.Blocks {
        equiv := make(map[valueKey]*Value)
        for _, v := range b.Values {
            if !v.IsPure() {
                continue
            }
            key := makeKey(v)
            if existing := equiv[key]; existing != nil {
                // 用 existing 替换 v
                v.replaceWith(existing)
            } else {
                equiv[key] = v
            }
        }
    }
}
```

### 5.6 逃逸分析算法

基于 Go 的逃逸分析：

> 说明：该逃逸分析用于**优化减少 heap 分配**，并不承担 UAF 安全；安全由 Phase 3 的 `*T` 非逃逸检查保证。

```go
// 逃逸分析使用有向带权图
// 顶点：分配点（变量、new()）
// 边：赋值（解引用次数作为权重）

type location struct {
    n        Node        // 分配
    escapes  bool        // 必须逃逸到堆？
    paramEsc leaks       // 参数逃逸摘要
}

type edge struct {
    dst    *location
    src    *location
    derefs int          // 解引用次数
}

func escapeAnalysis(fn *Func) {
    // 1. 构建图
    b := &batch{}
    for _, n := range fn.Dcl {
        b.newLoc(n)
    }

    // 2. 遍历函数，添加边
    b.walkFunc(fn)

    // 3. 传播逃逸信息
    b.walkAll()

    // 4. 标记逃逸的分配
    for _, loc := range b.allLocs {
        if loc.escapes {
            loc.n.SetEsc(EscHeap)
        } else {
            loc.n.SetEsc(EscNone)
        }
    }
}
```

### 5.7 关键数据结构

**符号表（作用域）**

```go
type Scope struct {
    parent   *Scope
    children []*Scope
    elems    map[string]Object
}

func (s *Scope) Lookup(name string) Object {
    for scope := s; scope != nil; scope = scope.parent {
        if obj := scope.elems[name]; obj != nil {
            return obj
        }
    }
    return nil
}
```

**控制流图**

```go
type CFG struct {
    entry  *Block
    exit   *Block
    blocks []*Block
}

type Block struct {
    id     int
    succs  []*Block
    preds  []*Block
    instrs []Instr
}
```

**支配树**

```go
type DomTree struct {
    idom     map[*Block]*Block  // 直接支配者
    children map[*Block][]*Block
    df       map[*Block][]*Block // 支配边界
}
```

---

## 6. 测试策略

### 6.1 测试分类

| 类别 | 描述 | 示例 |
|------|------|------|
| 单元测试 | 测试单个组件 | Scanner token 输出 |
| 集成测试 | 测试组件交互 | Parser + Type Checker |
| 端到端测试 | 编译并运行程序 | 完整编译管道 |
| Golden 测试 | 与预期输出比较 | AST dump，SSA 输出 |
| 错误测试 | 验证错误消息 | 语法/类型错误 |
| Fuzz 测试 | 随机输入测试 | scanner/parser fuzz |

### 6.2 测试文件组织

```
internal/
└── syntax/
    ├── scanner_test.go      # 词法分析单元测试
    ├── parser_test.go       # 语法分析单元测试
    └── testdata/
        ├── tokens.golden    # 预期 token 输出
        ├── parse_*.yoru     # 解析测试用例
        └── fuzz/
            ├── FuzzScanner/
            └── FuzzParse/

test/
├── types/
│   ├── check_test.go        # 类型检查测试
│   └── testdata/
│       ├── valid_*.yoru     # 有效程序
│       └── error_*.yoru     # 带类型错误的程序
├── ssa/
│   ├── passes_test.go       # 优化测试
│   ├── verify_test.go       # SSA 验证器测试
│   └── testdata/
│       └── opt_*.yoru       # 优化测试用例
├── codegen/
│   └── gen_test.go          # 代码生成测试
├── gc/
│   ├── stress_test.go       # GC 压测
│   └── testdata/
│       ├── alloc_pressure.yoru
│       ├── linked_list.yoru
│       └── intermediate_value.yoru
└── e2e/
    ├── run_test.go          # 端到端测试
    └── testdata/
        ├── hello.yoru       # 测试程序
        ├── hello.golden     # 预期输出
        └── ...
```

### 6.3 测试实现模式

**Scanner 测试**

```go
func TestScanner(t *testing.T) {
    tests := []struct {
        src    string
        tokens []Token
    }{
        {"123", []Token{_Literal}},
        {"foo", []Token{_Name}},
        {"func", []Token{_Func}},
        {"1 + 2", []Token{_Literal, _Add, _Literal}},
    }

    for _, tt := range tests {
        s := NewScanner("test", strings.NewReader(tt.src), nil)
        for _, want := range tt.tokens {
            s.Next()
            if s.Token() != want {
                t.Errorf("got %v, want %v", s.Token(), want)
            }
        }
    }
}
```

**Golden 文件测试**

```go
func TestParseGolden(t *testing.T) {
    files, _ := filepath.Glob("testdata/*.yoru")
    for _, f := range files {
        t.Run(f, func(t *testing.T) {
            ast := Parse(f)
            got := ast.String()

            golden := strings.TrimSuffix(f, ".yoru") + ".golden"
            want, _ := os.ReadFile(golden)

            if got != string(want) {
                t.Errorf("mismatch:\ngot:\n%s\nwant:\n%s", got, want)
            }
        })
    }
}
```

### 6.4 GC 压测用例（关键）

```yoru
// 1. 高分配率、短生命周期
func test_alloc_pressure() {
    var i int = 0
    for i < 100000 {
        _ = new(Point)  // 立即成为垃圾
        i = i + 1
    }
    // 验收：内存不会无限增长
}

// 2. 链式引用（验证标记传播）
func test_linked_list() {
    var head ref Node = new(Node)
    var curr ref Node = head
    var i int = 0
    for i < 1000 {
        curr.next = new(Node)
        curr = curr.next
        i = i + 1
    }
    // 验收：head 存活时整个链表存活
    head = nil
    // 验收：GC 后链表被回收
}

// 3. ⚠️ 中间值 + 触发 GC（最关键）
func test_root_preservation() {
    // f() 返回 ref，g() 触发 GC
    // 必须保证 f() 的返回值不被回收
    var result ref Point = combine(make_point(), trigger_gc())
    println(result.x)  // 必须正确输出
}
```

---

## 7. 构建与调试

### 7.1 编译器开关（从第 1 天就做）

```bash
yoruc [options] <file.yoru>

# 中间产物输出（调试必备）
-emit-tokens      # 输出 token 流
-no-asi           # 关闭自动分号插入（默认开启）
-emit-ast         # 输出 AST（JSON 或文本）
-emit-typed-ast   # 输出带类型的 AST
-emit-ssa         # 输出 SSA
-emit-ll          # 输出 LLVM IR（最常用）

# 调试与观测
-trace            # JSONL 格式输出各阶段耗时、文件数、函数数
-dump-func=<name> # 只 dump 某个函数（避免大项目刷屏）
-print-passes     # 列出所有 SSA passes
-dump-before=<p>  # 在 pass p 之前 dump SSA
-dump-after=<p>   # 在 pass p 之后 dump SSA
-deterministic    # 禁止 map 随机遍历导致输出变化
-ssa-verify       # 每次 pass 前后都验证 SSA（debug 时强制开启）

# 优化级别
-O0               # 无优化（默认，调试用）
-O1               # 基础优化

# GC 调试（必须有）
-gc-stats         # 打印 GC 统计：次数、回收对象数、heap size、root count
-gc-verbose       # 打印每次 GC 详情（触发原因）
-gc-stress        # 每次分配都触发 GC（专门抓 root 丢失）
-heap-dump        # 打印前 N 个对象的 TypeID/size/marked

# 输出
-o <file>         # 输出文件名
```

### 7.2 构建流程

```bash
# 方式 1：一步到位
yoruc foo.yoru -o foo

# 方式 2：分步（调试用）
yoruc -emit-ll foo.yoru -o foo.ll
clang foo.ll runtime/runtime.c -O0 -g -o foo

# 方式 3：看 LLVM 优化效果
clang foo.ll runtime/runtime.c -O2 -o foo

# 方式 4：只验证 LLVM IR 合法性
opt -verify foo.ll
# 或
llvm-as foo.ll
```

### 7.3 测试命令

```bash
# 运行所有测试
go test ./...

# 运行特定类型测试
go test ./internal/syntax/...
go test ./test/e2e/...

# Fuzz 测试（跑 5-10 分钟）
go test -fuzz=FuzzScanner ./internal/syntax/
go test -fuzz=FuzzParse ./internal/syntax/

# 更新 golden 文件（改动后）
go test ./... -update-golden

# GC 压测（带统计）
yoruc -gc-stats test/gc/pressure.yoru -o /tmp/gc_test && /tmp/gc_test

# GC stress 测试
yoruc -gc-stress test/gc/intermediate_value.yoru -o /tmp/gc_stress && /tmp/gc_stress

# runtime Debug 构建（开发期推荐）
clang foo.ll runtime/runtime.c -fsanitize=address,undefined -g -o foo
```

---

## 8. 各阶段验收标准（Definition of Done）

### 通用验收标准（每个 Phase 都必须满足）

1. `go test ./...` 全绿
2. 至少一个 e2e 测试：编译 → `opt -verify` → 链接 → 运行
3. 所有新 pass 都有 `-dump-before/-dump-after` 支持
4. 开启 `-gc-stress` 时，GC 相关用例稳定（不崩、不误回收）
5. SSA/LL dump 使用稳定 ID（避免 golden test 抖动）
6. 每个阶段都有可读 dump（tokens/ast/ssa/ll 至少一个）

### Phase 0：工具链 & ABI
- [ ] docs/runtime-abi.md 定义完成
- [ ] runtime/runtime.h 定义 ObjectHeader、TypeDesc
- [ ] target triple / DataLayout 固定且一致性测试通过
- [ ] 示例：手写 .ll 文件，能链接 runtime.c 跑通

### Phase 1：词法分析器
- [ ] yoruc -emit-tokens 输出正确
- [ ] 100+ token 测试用例通过
- [ ] 自动分号插入正确（换行场景）
- [ ] 位置信息准确（行:列）
- [ ] fuzz 跑 5-10 分钟不崩

### Phase 2：语法分析器
- [ ] yoruc -emit-ast 输出正确
- [ ] 所有语法结构有 golden test
- [ ] 语法错误有清晰诊断（指出位置）
- [ ] 能解析：函数、struct、if、for cond {}、表达式
- [ ] 20 个语法错误用例

### Phase 3：类型系统
- [ ] yoruc -emit-typed-ast 输出正确
- [ ] 类型检查错误有清晰诊断
- [ ] 测试用例：未定义变量/函数、类型不匹配、重复定义、方法解析
- [ ] := 类型推断正确
- [ ] 200+ 类型错误用例

### Phase 4：SSA 生成
- [ ] yoruc -emit-ssa 输出正确
- [ ] SSA 验证器能抓住结构错误
- [ ] -dump-func 支持只 dump 某函数
- [ ] fib/循环/函数调用能生成 SSA

### Phase 5A：最小可执行程序
- [ ] yoruc -emit-ll 输出合法 LLVM IR
- [ ] println(42) → 输出 "42"
- [ ] println("Hello, Yoru!") → 输出 "Hello, Yoru!"
- [ ] println(1+2*3) → 输出 "7"
- [ ] E2E 测试框架建立

### Phase 5B：控制流 + 函数
- [ ] fibonacci(10) = 55
- [ ] if/else、for 循环
- [ ] 多函数调用

### Phase 5C：内存 + 聚合类型
- [ ] struct 字段读写、new(T) 分配
- [ ] 数组索引、方法调用

### Phase 6：GC Roots
- [ ] llvm.gcroot 在 entry block
- [ ] 中间值 h(f(), g()) 正确处理
- [ ] -gc-stress 下跑 1e5 次分配不崩

### Phase 7：GC 完整
- [ ] rt_alloc 能分配对象
- [ ] GC 压测：循环分配 10 万对象不崩溃
- [ ] GC 统计：-gc-stats 输出合理
- [ ] 链式对象：断开头引用后被回收
- [ ] ⚠️ 中间值测试：h(f(), g()) 模式不崩溃

### Phase 8：扩展特性
- [ ] 多返回值：tuple lowering 工作
- [ ] interface：基础动态派发工作
- [ ] 逃逸分析：有基准证明减少了 heap 分配
- [ ] 内联：leaf 小函数内联工作

---

## 9. 里程碑和成功标准

### 里程碑 1："Hello World" 编译（第 9 周）

```yoru
package main

func main() {
    println("Hello, Yoru!")
}
```

- 词法分析正确生成 token
- 语法分析产生有效 AST
- 基本类型检查通过
- 生成 LLVM IR
- 可执行文件运行

### 里程碑 2：算术和控制流（第 13 周）

```yoru
package main

func fibonacci(n int) int {
    if n <= 1 {
        return n
    }
    return fibonacci(n-1) + fibonacci(n-2)
}

func main() {
    println(fibonacci(10))  // 55
}
```

### 里程碑 3：用户定义类型（第 19 周）

```yoru
package main

type Point struct {
    x, y int
}

func (p *Point) Add(other Point) {
    p.x = p.x + other.x
    p.y = p.y + other.y
}

func main() {
    var p Point
    p.x = 1
    p.y = 2
    var other Point
    other.x = 10
    other.y = 20
    (&p).Add(other)
    println(p.x)  // 11
    println(p.y)  // 22
}
```

### 里程碑 4：GC 正确性（第 31 周）

- shadow-stack roots 正确插入
- mark-sweep GC 工作
- 中间值不被误回收
- 循环分配 10 万对象内存不爆

### 里程碑 5：完整语言（第 36 周+）

- 所有核心特性工作
- 带 GC 的运行时
- 多包支持
- 标准库基础（print，字符串操作）
- 可选：interface、多返回值

---

## 10. 参考资源

### 10.1 主要参考（Go 编译器）

| 组件 | Go 源码路径 | 关键文件 |
|------|-------------|----------|
| 词法分析 | `cmd/compile/internal/syntax/` | scanner.go, tokens.go |
| 语法分析 | `cmd/compile/internal/syntax/` | parser.go, nodes.go |
| 类型系统 | `cmd/compile/internal/types/` | type.go, universe.go |
| 类型检查 | `cmd/compile/internal/types2/` | check.go, expr.go |
| SSA | `cmd/compile/internal/ssa/` | value.go, block.go, compile.go |
| 逃逸分析 | `cmd/compile/internal/escape/` | escape.go, graph.go |
| 运行时 | `runtime/` | malloc.go, mgc.go |

### 10.2 LLVM GC 文档

- [LLVM GC 概念](https://llvm.org/docs/GCRoot.html)
- [Shadow Stack 策略](https://llvm.org/docs/GCRoot.html#the-shadow-stack-strategy)
- `llvm.gcroot` 必须在 entry block
- runtime 需遍历 `llvm_gc_root_chain`

### 10.3 推荐书籍

1. **《编译原理》**（龙书）- Aho, Lam, Sethi, Ullman
2. **《Engineering a Compiler》** - Cooper, Torczon
3. **《Modern Compiler Implementation》** - Appel

### 10.4 Go 相关资源

1. Go 编译器 README：`/usr/local/go/src/cmd/compile/README.md`
2. SSA README：`/usr/local/go/src/cmd/compile/internal/ssa/README.md`
3. Go 博客：编译器内部文章
4. GopherCon 关于 Go 内部的演讲

---

## 11. 快速开始命令

```bash
# 创建目录结构
mkdir -p cmd/yoruc internal/{syntax,types,types2,ssa,codegen,rtabi} runtime test examples docs

# 初始化关键文件
touch cmd/yoruc/main.go
touch internal/syntax/{token,scanner,source,pos,nodes,parser,walk}.go
touch internal/types/{type,universe,scope}.go
touch internal/ssa/{value,block,func,op,verify}.go
touch internal/rtabi/{types,funcs}.go
touch runtime/{runtime.h,runtime.c}
touch docs/runtime-abi.md

# 添加 llir/llvm 依赖
go get github.com/llir/llvm

# 运行测试
go test ./...
```

---

## 附录：技术决策总结

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 解析方法 | 手写递归下降 | 学习价值高，匹配 Go |
| 后端 | llir/llvm（后原生） | 平衡学习和结果 |
| Runtime 语言 | C | 与 clang 链接最稳定 |
| GC 策略 | shadow-stack + mark-sweep | 易上手，LLVM 支持 |
| 类型系统 | 静态，强类型 | 类似 Go |
| 错误处理 | panic + 返回值 | 简化运行时 |
| for 循环 | 只有 for cond {} | 保持简洁一致 |
| interface | 后期扩展 | 避免早期复杂度 |
| 多返回值 | 后期扩展 | 避免 ABI 复杂度 |
| 泛型 | 不支持 | 避免类型参数化复杂度 |

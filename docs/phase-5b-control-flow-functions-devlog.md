# Phase 5B 开发记录（控制流 + 函数）

本文档记录 Phase 5B 的开发内容、技术原理、设计决策与踩坑经验。

## 目标与范围

Phase 5B 是 LLVM IR 代码生成的第二个子阶段，目标是验证**控制流**和**函数调用**的完整编译流水线——从 `.yoru` 源码到可执行二进制，通过 E2E 测试覆盖。Phase 5A 已完成最小可执行程序（常量 + 算术 + println），5B 在此基础上验证分支、循环、递归等控制流特性。

**里程碑**：`fibonacci(10) = 55` —— 递归函数 + 条件分支 + 算术运算的经典组合。

具体目标：
- 验证 `if/else` 条件分支的完整流水线（解析 → 类型检查 → SSA → mem2reg → LLVM IR → 执行）
- 验证 `for` 循环（含 `break`）与循环变量的 phi 节点正确性
- 验证多函数定义、递归调用、嵌套调用
- 验证布尔运算（比较、NOT、短路 AND/OR）的端到端行为
- 验证嵌套控制流（循环套循环、循环内分支）
- 新增 9 个 E2E 测试（共 18 个），修复发现的 parser bug

**核心策略**：Phase 5B 的所有 SSA 基础设施（比较 Op、布尔 Op、BlockIf、StaticCall、Phi、Arg 以及对应的 codegen）在 Phase 4A-4C 和 Phase 5A 中**已经实现**。5B 的工作是编写 E2E 测试来验证完整流水线，并修复测试过程中发现的 bug。

---

## 已完成的实现内容

### 文件清单

**新建文件（18 个）**：

```
test/e2e/testdata/
├── fibonacci.yoru          # 递归函数（里程碑测试）
├── fibonacci.golden
├── if_else.yoru            # if/else 分支 + else-if 链
├── if_else.golden
├── for_loop.yoru           # for 循环 + 循环变量
├── for_loop.golden
├── multi_func.yoru         # 多函数 + 嵌套调用
├── multi_func.golden
├── bool_ops.yoru           # 比较 + 布尔 NOT/AND/OR
├── bool_ops.golden
├── func_void.yoru          # void 函数 + 字符串参数
├── func_void.golden
├── break_continue.yoru     # break + if inside for
├── break_continue.golden
├── nested_loops.yoru       # 嵌套循环 + 嵌套条件
├── nested_loops.golden
├── comparison_int.yoru     # >= 比较 + 一元取负 + 多返回路径
├── comparison_int.golden
```

**修改文件（1 个）**：

```
internal/syntax/
├── parser.go               # 修复 composite literal 歧义（noBrace 机制）
```

### 1. E2E 测试用例

#### fibonacci.yoru（里程碑）

```yoru
func fibonacci(n int) int {
    if n <= 1 {
        return n
    }
    return fibonacci(n-1) + fibonacci(n-2)
}

func main() {
    println(fibonacci(10))
}
```

期望输出：`55`

**覆盖特性**：递归函数调用（`StaticCall`）、整数参数与返回值（`Arg`/`BlockReturn`）、条件分支（`BlockIf`）、比较运算（`Leq64`）、算术（`Sub64`/`Add64`）。

这个测试是整个 Phase 5B 的关键里程碑——它同时验证了函数定义/调用、控制流、SSA phi 节点、LLVM IR 生成和 runtime 链接的正确性。

#### if_else.yoru

```yoru
func main() {
    var x int = 10
    if x > 5 {
        println("greater")
    } else {
        println("smaller")
    }
    if x == 10 {
        println("ten")
    }
    if x < 0 {
        println("negative")
    } else {
        if x < 20 {
            println("small positive")
        } else {
            println("large positive")
        }
    }
}
```

期望输出：`greater\nten\nsmall positive\n`

**覆盖特性**：if/else 分支、无 else 的 if、嵌套 else-if 链（`IfStmt` 的 `Else` 嵌套 `IfStmt`）、变量声明与比较。

#### for_loop.yoru

```yoru
func main() {
    var sum int = 0
    var i int = 1
    for i <= 10 {
        sum = sum + i
        i = i + 1
    }
    println(sum)
    println(i)
}
```

期望输出：`55\n11\n`

**覆盖特性**：for 循环、循环变量跨迭代的 phi 节点（mem2reg 提升后生成 `OpPhi`）、循环条件比较（`Leq64`）、变量多次赋值。

这个测试对 phi 节点生成至关重要——`sum` 和 `i` 在循环体内被修改，mem2reg 必须在循环头块插入 phi 来合并 entry 和 loop body 两条路径的值。

#### multi_func.yoru

```yoru
func add(a int, b int) int {
    return a + b
}

func square(x int) int {
    return x * x
}

func main() {
    println(add(3, 4))
    println(square(5))
    println(add(square(2), square(3)))
}
```

期望输出：`7\n25\n13\n`

**覆盖特性**：多函数定义、函数作为参数嵌套调用（`add(square(2), square(3))`）、整数参数传递与返回值使用。

#### bool_ops.yoru

```yoru
func main() {
    println(1 < 2)
    println(3 > 5)
    println(10 == 10)
    println(10 != 10)
    println(!true)
    println(!false)
    println(true && true)
    println(true && false)
    println(false || true)
    println(false || false)
}
```

期望输出：`true\nfalse\ntrue\nfalse\nfalse\ntrue\ntrue\nfalse\ntrue\nfalse\n`

**覆盖特性**：所有整数比较运算（`Lt64`/`Gt64`/`Eq64`/`Neq64`）、布尔 NOT（`OpNot`）、短路 AND/OR（展开为 CFG + `OpPhi`）、布尔值的 `i1 → i8` 转换和 runtime 打印。

#### func_void.yoru

```yoru
func greet(name string) {
    println("hello", name)
}

func main() {
    greet("world")
    greet("yoru")
}
```

期望输出：`hello world\nhello yoru\n`

**覆盖特性**：void 函数（无返回值的 `StaticCall`）、字符串参数传递（`{ ptr, i64 }` 结构体）、多参数 `println` 空格分隔。

#### break_continue.yoru

```yoru
func main() {
    var i int = 0
    var sum int = 0
    for i < 100 {
        i = i + 1
        if i > 10 {
            break
        }
        sum = sum + i
    }
    println(sum)
    println(i)
}
```

期望输出：`55\n11\n`

**覆盖特性**：`break` 语句（跳转到循环出口块）、循环内的条件分支（`if` inside `for`）、`break` 后的循环变量状态。

#### nested_loops.yoru

```yoru
func main() {
    var i int = 0
    var count int = 0
    for i < 5 {
        var j int = 0
        for j < 5 {
            if i == j {
                count = count + 1
            }
            j = j + 1
        }
        i = i + 1
    }
    println(count)
}
```

期望输出：`5\n`

**覆盖特性**：嵌套 for 循环（内外两层 `BlockIf`）、循环内条件（三层嵌套块结构）、内层变量作用域（`j` 在外层每次迭代重新声明）。

#### comparison_int.yoru

```yoru
func max(a int, b int) int {
    if a >= b {
        return a
    }
    return b
}

func abs(x int) int {
    if x < 0 {
        return -x
    }
    return x
}

func main() {
    println(max(3, 7))
    println(max(10, 2))
    println(abs(-42))
    println(abs(15))
}
```

期望输出：`7\n10\n42\n15\n`

**覆盖特性**：`>=` 比较（`Geq64`）、一元取负（`Neg64`）、多个返回路径（条件 return + fallthrough return）。此测试触发了 parser 的 composite literal 歧义 bug（见踩坑点）。

### 2. Parser 修复：composite literal 歧义（`parser.go`）

在 `Parser` 结构体中新增 `noBrace` 字段：

```go
type Parser struct {
    // ...
    noBrace bool // suppress Name{ composite literal in if/for conditions
}
```

在 `ifStmt()` 和 `forStmt()` 中解析条件表达式前设置标志：

```go
func (p *Parser) ifStmt() Stmt {
    // ...
    p.want(_If)
    old := p.noBrace
    p.noBrace = true
    s.Cond = p.expr()
    p.noBrace = old
    s.Then = p.blockStmt()
    // ...
}
```

在 `operand()` 中检查标志，条件禁止 composite literal 解析：

```go
case _Name:
    n := &Name{Value: p.lit}
    n.pos = p.pos
    p.next()
    // Suppressed in if/for conditions where { starts a block body.
    if !p.noBrace && p.tok == _Lbrace {
        return p.compositeLit(n)
    }
    return n
```

---

## 技术原理

### 1. 控制流的完整编译流水线

以 `if x > 5 { println("greater") } else { println("smaller") }` 为例，追踪从源码到机器码的完整路径：

**阶段 1：解析（Parser）**

```
IfStmt {
    Cond: Operation{Op: ">", X: Name{x}, Y: BasicLit{5}}
    Then: BlockStmt{[ExprStmt{CallExpr{println, ["greater"]}}]}
    Else: BlockStmt{[ExprStmt{CallExpr{println, ["smaller"]}}]}
}
```

Parser 的 `ifStmt()` 调用 `p.expr()` 解析条件，`p.blockStmt()` 解析 then/else 体。

**阶段 2：类型检查（types2）**

类型检查器验证：
- `x` 是 `int` 类型（通过 `info.Uses[x]` 查找）
- `5` 是 `UntypedInt`，与 `int` 兼容
- `>` 运算对 `(int, int)` 合法，结果类型 `bool`
- `println("greater")` 参数类型 `string` 合法

**阶段 3：SSA 生成（build.go）**

```
b0: (entry)
    v0 = Alloca <*int> {x}
    v1 = Const64 <int> [10]
    Store v0 v1
    v3 = Load <int> v0
    v4 = Const64 <int> [5]
    v5 = Gt64 <bool> v3 v4
    If v5 -> b1 b3

b1: <- b0                          // then
    v6 = ConstString <string> {"greater"}
    Println v6
    Plain -> b2

b3: <- b0                          // else
    v8 = ConstString <string> {"smaller"}
    Println v8
    Plain -> b2

b2: <- b1 b3                       // merge
    ...
```

`ifStmt()` 将当前块设为 `BlockIf`，创建 bThen、bElse、bDone 三个新块。条件值作为 Controls[0]，then/else 体分别在对应块中构建，最后汇合到 bDone。

**阶段 4：mem2reg**

消除 `x` 的 alloca/load/store：

```
b0: (entry)
    v5 = Gt64 <bool> 10 5          // 常量直接内联
    If v5 -> b1 b3
```

**阶段 5：LLVM IR 生成（codegen）**

```llvm
%v5 = icmp sgt i64 10, 5
br i1 %v5, label %b1, label %b3
b1:
  ; ... print "greater" ...
  br label %b2
b3:
  ; ... print "smaller" ...
  br label %b2
b2:
  ; ... continue ...
```

`BlockIf` 生成 `br i1` 指令，条件值必须是 `i1` 类型。`BlockPlain` 生成无条件 `br label`。

### 2. For 循环与 Phi 节点

for 循环是 phi 节点的核心用武之地。以 `for_loop.yoru` 为例：

```yoru
var sum int = 0
var i int = 1
for i <= 10 {
    sum = sum + i
    i = i + 1
}
```

**SSA 生成**（alloca 形式）：

```
b0: (entry)
    v_sum = Alloca <*int> {sum}
    Store v_sum [0]
    v_i = Alloca <*int> {i}
    Store v_i [1]
    Plain -> b1

b1: (header)                          // 循环头
    v3 = Load <int> v_i
    v4 = Leq64 <bool> v3 [10]
    If v4 -> b2 b3

b2: (body)                            // 循环体
    v5 = Load <int> v_sum
    v6 = Load <int> v_i
    v7 = Add64 <int> v5 v6
    Store v_sum v7                    // sum = sum + i
    v8 = Load <int> v_i
    v9 = Add64 <int> v8 [1]
    Store v_i v9                      // i = i + 1
    Plain -> b1                       // 回边

b3: (exit)
    ...
```

**mem2reg 后**（phi 形式）：

```
b0: (entry)
    Plain -> b1

b1: (header)
    v_i   = Phi <int> [1, %entry], [v9, %b2]
    v_sum = Phi <int> [0, %entry], [v7, %b2]
    v4 = Leq64 <bool> v_i [10]
    If v4 -> b2 b3

b2: (body)
    v7 = Add64 <int> v_sum v_i
    v9 = Add64 <int> v_i [1]
    Plain -> b1

b3: (exit)
    ...
```

**关键不变量**：Phi 的 Args 顺序必须严格匹配所在块的 `Preds` 顺序。`b1.Preds = [b0, b2]`，所以 phi 的第一个参数来自 entry（初始值），第二个参数来自 b2（循环体更新后的值）。

**生成的 LLVM IR**：

```llvm
b1:
  %v22 = phi i64 [ 1, %entry ], [ %v15, %b2 ]
  %v21 = phi i64 [ 0, %entry ], [ %v11, %b2 ]
  %v8 = icmp sle i64 %v22, 10
  br i1 %v8, label %b2, label %b3
b2:
  %v11 = add i64 %v21, %v22
  %v15 = add i64 %v22, 1
  br label %b1
```

LLVM 的 phi 指令要求列出所有前驱块及对应的值。codegen 的 `lowerPhi` 遍历 `v.Args` 和 `v.Block.Preds`，一一对应生成 `[ value, %pred ]` 对。

### 3. 递归函数调用的编译

`fibonacci(n)` 的递归调用涉及多个关键环节：

**函数定义** → LLVM `define`：

```llvm
define i64 @fibonacci(i64 %n) {
```

codegen 的 `lowerFunc` 从 `fn.Sig` 读取参数列表和返回类型，生成 LLVM 函数定义。`main` 函数被重命名为 `yoru_main`（因为 C runtime 提供了真正的 `main`，调用 `rt_init()` → `yoru_main()` → `rt_shutdown()`）。

**参数使用** → LLVM 参数名：

mem2reg 后，函数参数的 `OpArg` 直接映射为 LLVM 参数寄存器 `%n`。codegen 的 `operand()` 检测到 `OpArg` 时返回 `%<name>` 而非 `%vN`，利用了 `Aux` 中存储的参数名。

**递归调用** → LLVM `call`：

```llvm
%v9 = sub i64 %n, 1
%v10 = call i64 @fibonacci(i64 %v9)
```

`lowerStaticCall` 从 `v.Aux.(*types.FuncObj)` 获取被调用函数的名称和签名，逐参数生成 `type %val` 格式的实参列表。

**多返回路径**：

```llvm
b1:                     ; n <= 1
  ret i64 %n
b2:                     ; n > 1
  ; ... recursive calls ...
  ret i64 %v15
```

每个 `BlockReturn` 生成一条 `ret` 指令。LLVM IR 允许函数内有多个 `ret`，不需要合并到单一出口。

### 4. 短路求值的 CFG 展开

`true && false` 不能简单地对两边求值再合并——右操作数在左操作数为 false 时不应被求值。AST→SSA 阶段将短路求值展开为 CFG：

**`a && b` 的 CFG 结构**：

```
当前块:
    left = expr(a)
    If left -> bRight bShort

bShort:                             // 短路：a 为 false，直接返回 false
    shortVal = ConstBool [0]
    Plain -> bMerge

bRight:                             // a 为 true，需要继续求值 b
    right = expr(b)
    Plain -> bMerge

bMerge:
    result = Phi(shortVal, right)   // preds: [bShort, bRight]
```

**生成的 LLVM IR**（以 `true && false` 为例）：

```llvm
br i1 true, label %b1, label %b2
b2:                                 ; short-circuit: false
  br label %b3
b1:                                 ; evaluate right operand
  br label %b3
b3:                                 ; merge
  %v7 = phi i1 [ false, %b2 ], [ false, %b1 ]
```

### 5. 布尔值的 i1/i8 边界转换

Yoru 在 SSA 层面统一使用 `i1` 表示布尔值，但 C runtime 的 `rt_print_bool(i8)` 接受 `i8` 参数。codegen 在函数调用边界插入转换指令：

```llvm
; println(1 < 2) 的生成过程:
%t0 = zext i1 true to i8           ; i1 → i8 扩展
call void @rt_print_bool(i8 %t0)   ; 传递 i8 给 runtime
call void @rt_println()
```

`zext`（zero-extend）将 1 位值扩展为 8 位——`true`(i1) → `1`(i8)，`false`(i1) → `0`(i8)。这个转换仅在调用 runtime 函数时需要，SSA 内部的布尔运算始终使用 `i1`。

---

## 关键设计决策与理由

### 1. E2E 测试驱动而非单元测试

**决策**：Phase 5B 以 E2E 测试为主要验证手段，而非为 codegen 编写单独的单元测试。

**理由**：
- 控制流和函数调用涉及编译流水线的**所有阶段**（Parser → TypeChecker → SSA → mem2reg → Codegen → Clang → Runtime），任何一个环节的 bug 都可能导致最终输出错误。E2E 测试是唯一能同时验证所有环节的手段。
- 单独测试 codegen 只能验证"给定 SSA 输入，LLVM IR 输出正确"，无法发现上游阶段的问题（如 parser 的 composite literal 歧义）。
- Phase 5A 已经建立了完善的 E2E 测试框架（`compileTo` → `clang link` → 执行对比），5B 复用此基础设施。

### 2. 测试用例的选择原则

**决策**：9 个测试用例覆盖了特性的不同组合，而非逐个特性单独测试。

**理由**：

| 测试 | 核心特性 | 组合验证 |
|------|---------|---------|
| fibonacci | 递归 + 条件 + 算术 | 函数调用栈 + BlockIf + 返回值 |
| if_else | 分支 + 嵌套 else-if | 多个 BlockIf 的正确连接 |
| for_loop | 循环 + 变量迭代 | Phi 节点的前驱顺序 |
| multi_func | 多函数 + 嵌套调用 | StaticCall 参数传递 |
| bool_ops | 比较 + 短路 | CFG 展开 + i1/i8 转换 |
| func_void | void 函数 + string 参数 | void StaticCall + { ptr, i64 } 传递 |
| break_continue | break + 循环内分支 | 循环出口块跳转 |
| nested_loops | 嵌套循环 + 条件 | 多层 BlockIf + 多组 Phi |
| comparison_int | >= 比较 + 一元负 | Geq64 + Neg64 + 多返回路径 |

每个测试覆盖多个特性组合，确保特性之间的交互不出问题。例如 `fibonacci` 不仅测试递归调用，还隐式测试了返回值在调用者中被用作算术操作数的场景。

### 3. noBrace 机制解决 composite literal 歧义

**决策**：在 Parser 中添加 `noBrace` 上下文标志，在 if/for 条件解析时禁止 `Name{` 被解释为 composite literal。

**替代方案考量**：

| 方案 | 优点 | 缺点 |
|------|------|------|
| 完全删除 composite literal 支持 | 最简单 | 阻断 Phase 5C 的 struct literal 支持 |
| 要求 if/for 条件加括号 | 无需改 parser | 语法与 Go 不同，体验差 |
| noBrace 上下文标志 | 精确、向前兼容 | 新增 parser 状态 |
| 使用 Go 的解决方案（限制更复杂） | 完全兼容 Go | 实现复杂 |

**选择 noBrace 的理由**：这是最小改动——仅新增一个 bool 字段、在两处设置/恢复、在一处检查。它精确地解决了 if/for 条件中的歧义，同时保留了其他位置（赋值右侧、函数参数等）的 composite literal 支持，不影响 Phase 5C。

Go 编译器也面临同样的歧义（`if T{} == T{} {}`），通过更复杂的上下文追踪来解决。Yoru 的 noBrace 方案是等效的简化版。

---

## 踩坑点

### 1. Composite Literal 歧义导致 `>=`/`==` 条件解析失败

**问题**：`comparison_int.yoru` 和 `nested_loops.yoru` 两个测试出现 parse error。

**表现**：

```
comparison_int.yoru:5:3: expected operand
comparison_int.yoru:5:3: expected }
comparison_int.yoru:5:10: expected {
```

```
nested_loops.yoru:10:11: expected }
nested_loops.yoru:11:4: expected {
nested_loops.yoru:18:1: expected }
```

**原因分析**：

以 `if a >= b {` 为例，追踪解析过程：

1. Parser 进入 `ifStmt()`，调用 `p.expr()` 解析条件
2. `binaryExpr(0)` → `operand()` 解析出 `a` (Name)
3. 回到 `binaryExpr(0)`，`>=` 的优先级为 3 > 0，进入循环
4. 消费 `>=`，调用 `binaryExpr(3)` 解析右操作数
5. 在内层 `binaryExpr(3)` → `operand()` 中，解析出 `b` (Name)
6. **BUG**：`operand()` 接下来检查 `p.tok == _Lbrace`——发现是 `{`（if 体的开头）
7. `operand()` 将 `b{...}` 当作 composite literal 解析，消费了 `{`
8. 内部尝试将 `return a` 解析为 composite literal 的元素——失败，报 "expected operand"

**根本原因**：`operand()` 中的 composite literal 检测没有考虑上下文。在 if/for 条件中，`Name` 后面的 `{` 是块体开头，不是 composite literal。

同样的问题也影响 `if i == j {`（`nested_loops.yoru`）——解析 `j` 后看到 `{`，误入 composite literal 路径。

**有趣的是**，这个 bug 不影响 `if x > 5 {` 这样的表达式。原因是：解析 `5` 时，`5` 是 `_Literal` token，不是 `_Name`，composite literal 检测只针对 `_Name` token。所以只有当比较运算符右侧是**标识符**（变量名、`true`/`false` 等）时才会触发此 bug。

**修复**：见上文"Parser 修复"部分——在 if/for 条件解析时设置 `noBrace = true`。

**教训**：

1. Go 语法中 composite literal 与块体 `{` 的歧义是经典问题，任何 Go-like 语言的 parser 都需要处理。
2. 仅靠现有的 parser 单元测试无法发现此 bug——之前的测试用例恰好没有在 if/for 条件中使用 `Name op Name {` 模式（大部分用的是 `x > 0` 或 `x < 10`，右侧是数字字面量）。
3. E2E 测试的价值在此凸显——它在完整流水线中暴露了 parser 层面的 bug。

### 2. Phase 4C 的记录已经提到此 bug（但未修复）

Phase 4C 的开发记录中，踩坑点 #4 已经记载了这个歧义问题，当时的解决方案是"在测试中避免触发"：

> **修复**：在测试中避免在 if 条件中使用裸标识符 + `{` 的组合，改用比较表达式 `if x > 0 { ... }`

Phase 5B 通过引入 `noBrace` 机制正式修复了此问题，不再需要回避特定的语法模式。这使得 `if a >= b {}`、`if i == j {}`、`for cond {}`（其中 cond 以标识符结尾）等模式都能正确解析。

---

## 测试结果

### E2E 测试（18 个全部通过）

| 测试 | 期望输出 | 状态 |
|------|---------|------|
| arith_float | `6.28` | PASS（5A） |
| arith_int | `10\n3\n28\n3\n1` | PASS（5A） |
| arithmetic | `7` | PASS（5A） |
| bool_ops | `true\nfalse\ntrue\nfalse\nfalse\ntrue\ntrue\nfalse\ntrue\nfalse` | PASS |
| break_continue | `55\n11` | PASS |
| comparison_int | `7\n10\n42\n15` | PASS |
| fibonacci | `55` | PASS |
| for_loop | `55\n11` | PASS |
| func_void | `hello world\nhello yoru` | PASS |
| if_else | `greater\nten\nsmall positive` | PASS |
| multi_func | `7\n25\n13` | PASS |
| nested_loops | `5` | PASS |
| print_bool | `true\nfalse` | PASS（5A） |
| print_float | `3.14` | PASS（5A） |
| print_int | `42` | PASS（5A） |
| print_mixed | `42 hello 3.14 true` | PASS（5A） |
| print_multi | `1 2 3` | PASS（5A） |
| print_string | `Hello, Yoru!` | PASS（5A） |

### 全量测试套件

```
go test ./...        → all packages PASS
make smoke           → runtime linkage PASS
make layout-test     → ABI consistency PASS
```

---

## 验收标准核对

设计文档（`yoru-compiler-design.md` Phase 5B）列出的验收标准：

| 验收项 | 对应测试 | 状态 |
|--------|---------|------|
| fibonacci(10) = 55 | fibonacci.yoru | ✅ |
| if/else 分支 | if_else.yoru | ✅ |
| for 循环 | for_loop.yoru, break_continue.yoru | ✅ |
| 多函数调用 | multi_func.yoru, func_void.yoru | ✅ |
| 布尔打印 | bool_ops.yoru | ✅ |

---

## 与前阶段的关系

| 阶段 | 提供的基础 | Phase 5B 的使用 |
|------|-----------|----------------|
| Phase 4A | Op 枚举（含比较/布尔/调用 Op） | E2E 测试验证这些 Op 的端到端行为 |
| Phase 4B | 语句/表达式降低（if/for/call → SSA） | E2E 测试验证降低逻辑的正确性 |
| Phase 4C | 支配树 + mem2reg（alloca → phi） | E2E 测试验证循环中 phi 节点的正确性 |
| Phase 5A | codegen 基础设施 + E2E 测试框架 | 复用 codegen 和测试框架 |

Phase 5B 本身几乎没有新增编译器代码——唯一的改动是 parser 的 `noBrace` 修复。这验证了一个重要观点：**当基础设施设计正确时，新特性的验证主要是编写测试。** Phase 4A-4C 和 Phase 5A 的实现已经覆盖了控制流和函数调用所需的所有编译器逻辑。

---

## 后续计划（Phase 5C）

Phase 5B 完成了控制流和函数调用的端到端验证。Phase 5C 将进入内存和聚合类型领域：

- struct 字段读写（`OpStructFieldPtr` + `OpLoad`/`OpStore`）
- 数组索引（`OpArrayIndexPtr`）
- `new(T)` 堆分配（`OpNewAlloc` → `rt_alloc`）
- 方法调用（receiver 作为第一个参数的 `OpStaticCall`）
- composite literal（`Point{x: 1, y: 2}`）

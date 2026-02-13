# Phase 5C 开发记录（内存 + 聚合类型 + 堆分配）

本文档记录 Phase 5C 的开发内容、技术原理、设计决策与踩坑经验。

## 目标与范围

Phase 5C 是 LLVM IR 代码生成的第三个（也是最后一个）子阶段，目标是验证**聚合类型**（struct/array）、**指针/引用**（`*T`/`ref T`）、**堆分配**（`new(T)`）和**方法调用**的完整编译流水线。Phase 5A 完成了常量+算术+println，Phase 5B 完成了控制流+函数调用，5C 在此基础上验证 Yoru 类型系统中最核心的内存操作。

**里程碑**：`full_example.yoru` —— 集 struct 定义、value receiver 方法、数组循环、`ref T` 堆分配于一体的综合测试。

具体目标：
- 验证 struct 字段读写（`p.x = 10`、`println(p.x)`）的完整流水线
- 验证 composite literal（`Point{x: 1, y: 2}` 和 `Point{3, 4}`）
- 验证数组声明、索引赋值、循环遍历
- 验证 `*T` 栈指针的取地址（`&arr`）和解引用（`p[i]`）
- 验证 `ref T` 堆引用的 `new(T)` 分配和字段访问
- 验证 value receiver 和 pointer receiver 方法调用（含自动取地址、自动解引用）
- 为 `ref T` 解引用操作插入 nil check
- 新增 8 个 E2E 测试（共 26 个），修复发现的编译器 bug

**核心发现**：Phase 5C 的所有 SSA Op（`OpAlloca`、`OpLoad`、`OpStore`、`OpZero`、`OpStructFieldPtr`、`OpArrayIndexPtr`、`OpAddr`、`OpNewAlloc`、`OpNilCheck` 等）及其对应的 codegen 降低逻辑，在 Phase 4B 和 5A/5B 中**已经实现完毕**。5C 的主要工作是：（1）修复 SSA builder 中缺失的 nil check 插入和自动解引用逻辑；（2）修复 codegen 字符串预收集遗漏；（3）编写 E2E 测试验证端到端行为。

---

## 已完成的实现内容

### 文件清单

**新建文件（16 个）**：

```
test/e2e/testdata/
├── struct_field.yoru         # struct 声明 + 字段读写
├── struct_field.golden
├── struct_literal.yoru       # composite literal（keyed + positional）
├── struct_literal.golden
├── array_index.yoru          # 数组声明 + 索引 + 循环求和
├── array_index.golden
├── pointer_arg.yoru          # &arr + 通过指针索引 + 指针写入
├── pointer_arg.golden
├── ref_heap.yoru             # new(T) + ref T 字段读写
├── ref_heap.golden
├── method_value.yoru         # value receiver 方法调用
├── method_value.golden
├── method_pointer.yoru       # pointer receiver + 自动取地址
├── method_pointer.golden
├── full_example.yoru         # 综合里程碑测试
├── full_example.golden
```

**修改文件（4 个）**：

```
internal/ssa/build_expr.go    # nil check 插入 + 自动解引用
internal/ssa/op.go            # OpNilCheck 标记为 IsVoid
internal/ssa/ssa_test.go      # 更新 TestOpIsVoid 预期
internal/codegen/codegen.go   # collectStrings() 预扫描 nil check 字符串
docs/yoru-compiler-design.md  # Phase 5C 验收标准更新
```

### 1. SSA Builder 修复

#### 1a. Nil Check 插入（6 个插入点）

新增两个辅助函数：

```go
func (b *builder) nilCheck(ptr *Value) {
    b.fn.NewValue(b.b, OpNilCheck, nil, ptr)
}

func isRef(t types.Type) bool {
    _, ok := t.Underlying().(*types.Ref)
    return ok
}
```

在以下 6 个位置插入 nil check：

| 位置 | 函数 | 触发条件 | 说明 |
|------|------|---------|------|
| 1 | `selectorExpr()` | X 是 `ref T` | `p.x` 读字段前检查 p 非 nil |
| 2 | `indexExpr()` | X 是 `ref [N]T` | `p[i]` 读元素前检查 p 非 nil |
| 3 | `unaryExpr()` `*` | X 是 `ref T` | `*p` 解引用前检查 p 非 nil |
| 4 | `addr()` SelectorExpr | X 是 `ref T` | `&p.x` 取字段地址前检查 p 非 nil |
| 5 | `addr()` IndexExpr | X 是 `ref [N]T` | `&p[i]` 取元素地址前检查 p 非 nil |
| 6 | `methodCallExpr()` | receiver 是 `ref T` | `p.Method()` 调用前检查 p 非 nil |

**设计原则**：仅对 `ref T` 插入 nil check，不对 `*T` 插入。原因是 `*T` 只能由 `&local` 创建（逃逸分析禁止 `*T` 跨函数传递），所以 `*T` 在语义上不可能为 nil。

#### 1b. 方法调用自动解引用

在 `methodCallExpr()` 中处理 receiver 类型不匹配的三种情况：

```go
if isPointerOrRef(recvParamType) && !isPointerOrRef(recvExprType) {
    // Case 1: value → pointer: auto-address (已有)
    recv = b.addr(sel.X)
} else if !isPointerOrRef(recvParamType) && isPointerOrRef(recvExprType) {
    // Case 2: pointer/ref → value: auto-dereference (新增)
    if isRef(recvExprType) {
        b.nilCheck(recv)
    }
    recv = b.fn.NewValue(b.b, OpLoad, recvParamType, recv)
} else if isRef(recvExprType) {
    // Case 3: ref → ref/pointer method: nil check only (新增)
    b.nilCheck(recv)
}
```

Case 2 的典型场景：`ref Rectangle` 调用 `func (r Rectangle) Area() int`，需要先 load 出 Rectangle 值再传给方法。

### 2. OpNilCheck 标记为 IsVoid

```go
// 修改前
OpNilCheck: {Name: "NilCheck"},

// 修改后
OpNilCheck: {Name: "NilCheck", IsVoid: true},
```

`OpNilCheck` 不产生结果值（它要么通过要么 panic），必须标记为 void，否则 SSA 验证器会报 "non-void value has nil Type"。

### 3. Codegen collectStrings() 修复

`lowerNilCheck()` 内部调用 `g.stringIndex("nil pointer dereference")` 来获取全局字符串常量的索引。但 `collectStrings()` 在 codegen 开始前只预扫描 `OpConstString` 和 `OpPrintln`，不会扫描 `OpNilCheck`。如果字符串未预收集，全局常量会在函数定义之后才发射，导致 LLVM IR 格式错误。

```go
// 新增扫描
if v.Op == ssa.OpNilCheck {
    g.stringIndex("nil pointer dereference")
}
if v.Op == ssa.OpPanic && len(v.Args) == 0 {
    g.stringIndex("panic")
}
```

### 4. E2E 测试用例

#### struct_field.yoru

```yoru
type Point struct {
    x int
    y int
}

func main() {
    var p Point
    p.x = 10
    p.y = 20
    println(p.x)
    println(p.y)
    println(p.x + p.y)
}
```

期望输出：`10\n20\n30\n`

**覆盖特性**：struct 类型声明、`OpAlloca` 为 struct 分配栈空间、`OpZero` 清零、`OpStructFieldPtr` + `OpStore` 写字段、`OpStructFieldPtr` + `OpLoad` 读字段、字段值参与算术运算。

#### struct_literal.yoru

```yoru
type Point struct {
    x int
    y int
}

func main() {
    var p Point = Point{x: 1, y: 2}
    println(p.x)
    println(p.y)
    var q Point = Point{3, 4}
    println(q.x)
    println(q.y)
}
```

期望输出：`1\n2\n3\n4\n`

**覆盖特性**：keyed composite literal（`Point{x: 1, y: 2}`）、positional composite literal（`Point{3, 4}`）。两者在 SSA 层面的生成方式相同：先 `OpZero` 清零目标内存，再逐字段 `OpStructFieldPtr` + `OpStore`。

#### array_index.yoru

```yoru
func main() {
    var arr [5]int
    arr[0] = 10
    arr[1] = 20
    arr[2] = 30
    arr[3] = 40
    arr[4] = 50
    println(arr[0])
    println(arr[2])
    println(arr[4])
    var sum int = 0
    var i int = 0
    for i < 5 {
        sum = sum + arr[i]
        i = i + 1
    }
    println(sum)
}
```

期望输出：`10\n30\n50\n150\n`

**覆盖特性**：数组类型声明 `[5]int`、`OpAlloca` 为数组分配栈空间（40 字节）、`OpArrayIndexPtr` + `OpStore` 写元素、`OpArrayIndexPtr` + `OpLoad` 读元素、循环中动态索引的数组访问。

#### pointer_arg.yoru

```yoru
func main() {
    var arr [5]int
    arr[0] = 1
    arr[1] = 2
    arr[2] = 3
    arr[3] = 4
    arr[4] = 5
    var p *[5]int = &arr
    var sum int = 0
    var i int = 0
    for i < 5 {
        sum = sum + p[i]
        i = i + 1
    }
    println(sum)
    p[2] = 99
    println(arr[2])
}
```

期望输出：`15\n99\n`

**覆盖特性**：`OpAddr`（`&arr` 取数组地址）、通过 `*[5]int` 指针索引数组（`p[i]`）、通过指针修改原数组（`p[2] = 99` 反映在 `arr[2]`）。

**注意**：最初的设计想把 `*[5]int` 作为函数参数传递（`func sum(arr *[5]int) int`），但 Yoru 的逃逸检查器禁止 `*T` 跨函数传递（v1 保守规则），所以改为同函数内操作。详见踩坑点 2。

#### ref_heap.yoru

```yoru
type Point struct {
    x int
    y int
}

func main() {
    var p ref Point = new(Point)
    p.x = 20
    p.y = 30
    println(p.x)
    println(p.y)
    println(p.x + p.y)
}
```

期望输出：`20\n30\n50\n`

**覆盖特性**：`OpNewAlloc` 堆分配（调用 `rt_alloc(size, null)`）、通过 `ref T` 写字段（隐式解引用 + nil check）、通过 `ref T` 读字段、`ref T` 的字段值参与算术。

这是 `ref T` 和 `*T` 区别的第一个端到端验证：`new(Point)` 返回 `ref Point`（堆引用，GC 管理），而不是 `*Point`（栈指针）。

#### method_value.yoru

```yoru
type Rectangle struct {
    width int
    height int
}

func (r Rectangle) Area() int {
    return r.width * r.height
}

func main() {
    var r Rectangle
    r.width = 5
    r.height = 3
    println(r.Area())
}
```

期望输出：`15\n`

**覆盖特性**：value receiver 方法定义（`func (r Rectangle) Area()`）、方法调用（`r.Area()`）。Value receiver 在 SSA 层面被展开为第一个参数是 receiver 的 `OpStaticCall`。

#### method_pointer.yoru

```yoru
type Point struct {
    x int
    y int
}

func (p *Point) Scale(factor int) {
    p.x = p.x * factor
    p.y = p.y * factor
}

func main() {
    var p Point
    p.x = 3
    p.y = 4
    p.Scale(10)
    println(p.x)
    println(p.y)
}
```

期望输出：`30\n40\n`

**覆盖特性**：pointer receiver 方法定义（`func (p *Point) Scale()`）、自动取地址（`p.Scale(10)` 中 `p` 是 value 类型，自动变为 `(&p).Scale(10)`）、通过指针 receiver 修改原始 struct（Scale 修改 `p.x` 和 `p.y`，效果反映到调用者）。

#### full_example.yoru（里程碑）

```yoru
type Rectangle struct {
    width int
    height int
}

func (r Rectangle) Area() int {
    return r.width * r.height
}

func sumTo(n int) int {
    var sum int = 0
    var i int = 1
    for i <= n {
        sum = sum + i
        i = i + 1
    }
    return sum
}

func main() {
    var r Rectangle
    r.width = 6
    r.height = 7
    println(r.Area())

    var arr [5]int
    arr[0] = 10
    arr[1] = 20
    arr[2] = 30
    arr[3] = 40
    arr[4] = 50
    var sum int = 0
    var i int = 0
    for i < 5 {
        sum = sum + arr[i]
        i = i + 1
    }
    println(sum)

    var p ref Rectangle = new(Rectangle)
    p.width = 100
    p.height = 200
    println(p.Area())

    if r.Area() > 40 {
        println(1)
    } else {
        println(0)
    }

    println(sumTo(10))
}
```

期望输出：`42\n150\n20000\n1\n55\n`

**覆盖特性**：这是 Phase 5C 的综合里程碑测试，同时验证：
- struct 定义 + value receiver 方法（`Rectangle.Area()`）
- 数组声明 + 循环遍历求和
- `ref T` 堆分配 + 方法调用（`p.Area()` 其中 `p: ref Rectangle` 调用 value receiver，触发自动解引用 + nil check）
- 条件分支（`if r.Area() > 40`）
- 多函数调用（`sumTo(10)`）

特别值得注意的是 `p.Area()` 这行：`p` 是 `ref Rectangle`，`Area` 是 value receiver 方法。SSA builder 需要：（1）nil check `p`；（2）`OpLoad` 把 `ref Rectangle` 解引用为 `Rectangle` 值；（3）将值作为第一个参数传给 `OpStaticCall`。

---

## 技术原理

### 1. Struct 字段访问的 LLVM IR 生成

以 `p.x = 10`（其中 `p: Point`，`Point{x int; y int}`）为例，追踪完整的编译路径：

**SSA 生成**：

```
v0 = Alloca <*Point> {p}        // 栈上分配 Point（16 字节）
v1 = Zero v0 [16]               // 清零
v2 = StructFieldPtr <*int> [0] v0  // &p.x（字段索引 0）
v3 = Const64 <int> [10]
Store v2 v3                     // *(&p.x) = 10
```

`OpStructFieldPtr` 接收一个 struct 指针和字段索引（通过 `AuxInt`），返回字段的指针。这是 LLVM GEP（GetElementPointer）指令的直接映射。

**LLVM IR**：

```llvm
%p = alloca { i64, i64 }
call void @llvm.memset.p0.i64(ptr %p, i8 0, i64 16, i1 false)
%v2 = getelementptr { i64, i64 }, ptr %p, i32 0, i32 0
store i64 10, ptr %v2
```

GEP 的两个索引参数：第一个 `i32 0` 表示"不偏移基指针"（因为 `%p` 已经是 struct 指针），第二个 `i32 0` 表示第 0 个字段。

### 2. 数组索引的 GEP 模式

以 `arr[i]`（其中 `arr: [5]int`）为例：

**SSA**：

```
v_idx = ...                        // 动态索引 i
v_elem = ArrayIndexPtr <*int> v_arr v_idx  // &arr[i]
v_val = Load <int> v_elem          // arr[i]
```

**LLVM IR**：

```llvm
%v_elem = getelementptr [5 x i64], ptr %arr, i64 0, i64 %i
%v_val = load i64, ptr %v_elem
```

与 struct 字段访问类似，GEP 的第一个索引 `i64 0` 表示不偏移基指针，第二个索引 `i64 %i` 是数组元素索引。关键区别是数组索引可以是动态值（变量），而 struct 字段索引必须是编译期常量。

### 3. `new(T)` 堆分配

`new(Point)` 的编译路径：

**SSA**：

```
v = NewAlloc <ref Point> {Point}   // Aux 保存元素类型
```

**Codegen（lower.go）**：

```go
func (g *generator) lowerNewAlloc(v *ssa.Value) {
    elemType := v.Aux.(types.Type)
    size := g.sizes.Sizeof(elemType)
    // TypeDesc 暂时传 null（Phase 6/7 才接入 GC）
    g.e.emitInst("%s = call ptr @rt_alloc(i64 %d, ptr null)", valueName(v), size)
}
```

**LLVM IR**：

```llvm
%v1 = call ptr @rt_alloc(i64 16, ptr null)
```

`rt_alloc` 是 runtime 提供的堆分配函数，参数为分配大小和 TypeDesc 指针。当前阶段 TypeDesc 传 null——在没有 GC 的情况下这是安全的，因为 `rt_alloc` 只需要 size 来分配内存。Phase 6/7 接入 GC 时才需要生成真正的 TypeDesc。

### 4. Nil Check 的 LLVM IR 模式

`OpNilCheck` 生成一个条件分支：

```llvm
; nil check for ref T pointer
%cmp = icmp eq ptr %p, null
br i1 %cmp, label %nilchk.fail.42, label %nilchk.ok.42

nilchk.fail.42:
  ; 构造 "nil pointer dereference" 字符串
  %t0 = insertvalue { ptr, i64 } undef, ptr @.str.0, 0
  %t1 = insertvalue { ptr, i64 } %t0, i64 23, 1
  call void @rt_panic_string({ ptr, i64 } %t1)
  unreachable

nilchk.ok.42:
  ; 继续正常执行...
```

**设计要点**：
- 使用 `icmp eq ptr %p, null` 检测 nil
- 失败路径调用 `rt_panic_string` 并标记 `unreachable`（告诉 LLVM 该路径不返回）
- 成功路径用新 label 继续，后续指令在 `nilchk.ok.N` 块中发射
- 字符串 `"nil pointer dereference"` 作为全局常量预先收集（见 collectStrings 修复）

### 5. 方法调用的 Receiver 传递

Yoru 的方法调用在 SSA 层面被降低为普通的 `OpStaticCall`，receiver 作为第一个参数。以三种场景说明：

**场景 A：Value receiver，Value caller**

```yoru
var r Rectangle
r.Area()  // r: Rectangle, Area recv: Rectangle
```

SSA：直接将 `r` 的值（通过 `OpLoad` 从 alloca 加载）作为第一个参数。

**场景 B：Pointer receiver，Value caller（自动取地址）**

```yoru
var p Point
p.Scale(10)  // p: Point, Scale recv: *Point
```

SSA：`p` 是 value 但方法需要 `*Point`，builder 自动插入 `OpAddr`（取地址）：

```
v_addr = Addr v_p_alloca        // &p
v_factor = Const64 [10]
StaticCall {Point.Scale} v_addr v_factor
```

**场景 C：Ref caller，Value receiver（自动解引用 + nil check）**

```yoru
var p ref Rectangle = new(Rectangle)
p.Area()  // p: ref Rectangle, Area recv: Rectangle
```

SSA：`p` 是 `ref Rectangle` 但方法需要 `Rectangle` 值，builder 自动插入 nil check + `OpLoad`：

```
v_p = Load <ref Rectangle> v_p_alloca
NilCheck v_p                    // panic if nil
v_recv = Load <Rectangle> v_p  // 解引用得到 Rectangle 值
StaticCall {Rectangle.Area} v_recv
```

### 6. Composite Literal 的 SSA 生成

`Point{x: 1, y: 2}` 在 SSA 层面不是一条指令，而是分解为一系列操作：

```
v0 = Alloca <*Point> {tmp}     // 临时 alloca
Zero v0 [16]                   // 清零
v1 = StructFieldPtr <*int> [0] v0  // &tmp.x
Store v1 [1]                   // tmp.x = 1
v2 = StructFieldPtr <*int> [1] v0  // &tmp.y
Store v2 [2]                   // tmp.y = 2
v3 = Load <Point> v0           // 加载完整 struct 值
```

keyed literal（`Point{x: 1, y: 2}`）和 positional literal（`Point{3, 4}`）在 SSA 生成后完全等价——类型检查器在 Phase 3 已经将 positional 映射到对应的字段索引。

---

## 关键设计决策与理由

### 1. 仅对 ref T 插入 nil check，不对 *T 插入

**决策**：nil check 只在 `isRef(t)` 为 true 时插入，`*T` 不做检查。

**理由**：Yoru 的类型系统通过逃逸分析保证 `*T` 不可能为 nil：
- `*T` 只能由 `&local` 创建（`OpAddr` 总是指向有效的栈 alloca）
- `*T` 不能作为函数参数传递、不能存入全局变量、不能从函数返回
- `*T` 不能由 `nil` 赋值（类型检查器禁止）

因此 `*T` 在语义上是"保证非 nil 的栈指针"，不需要运行时检查。这与 `ref T`（可以是 `nil`，来自 `new(T)` 或零值）形成对照。

### 2. OpNilCheck 标记为 IsVoid

**决策**：将 `OpNilCheck` 从 non-void 改为 void。

**替代方案**：Go 编译器的 `OpNilCheck` 返回一个与输入相同的指针值（用于后续使用），这样可以让 nil check 参与值传播。

**选择 IsVoid 的理由**：
- Yoru 的 nil check 仅作为 side effect（panic or continue），后续代码直接使用原始指针
- 如果 `OpNilCheck` 返回值，则每次使用都需要将 nil check 的结果传递下去，增加 SSA 图的复杂度
- IsVoid 使得 nil check 在 SSA 层面更像 `OpStore`（纯 side effect），符合"check and continue"的语义
- SSA 验证器要求：如果 `IsVoid() == false`，则 `Type` 不能为 nil，而我们创建 nil check 时传入 `nil` 类型

### 3. pointer_arg 测试改为同函数内操作

**决策**：最初设计为 `func sum(arr *[5]int) int` 的跨函数指针传递，改为同一函数内的 `var p *[5]int = &arr` + 循环遍历。

**理由**：Yoru 的 v1 逃逸检查器（`checkCallArgEscape()`）保守地禁止 `*T` 作为任何非 builtin 函数的参数。这是正确的设计——`*T` 是栈指针，传递给函数可能导致函数持有一个即将失效的栈帧引用。测试改为验证同函数内的指针操作，同样能覆盖 `OpAddr` 和通过指针的数组索引。

### 4. TypeDesc 推迟到 Phase 6/7

**决策**：`new(T)` 调用 `rt_alloc(size, null)` 时传入 null 作为 TypeDesc。

**理由**：TypeDesc 的唯一消费者是 GC（用于遍历对象内的指针字段）。Phase 5C 没有 GC，所以 null TypeDesc 是安全的。将 TypeDesc 生成推迟到 Phase 6/7（GC 接入阶段）避免了过早优化。

### 5. collectStrings 预扫描策略

**决策**：在 `collectStrings()` 中显式扫描 `OpNilCheck` 和 `OpPanic`，而非改为惰性分配。

**替代方案**：让 `stringIndex()` 在任何时刻被调用时都能正确插入全局字符串（惰性方式）。

**选择预扫描的理由**：codegen 的架构是先发射所有全局声明（字符串常量、类型定义），再发射函数体。如果改为惰性分配，字符串全局变量可能在函数体中间才被发射，破坏 LLVM IR 的模块结构。预扫描保证所有可能用到的字符串在模块顶部统一声明。

---

## 踩坑点

### 1. OpNilCheck 的 IsVoid 标志遗漏

**问题**：添加 nil check 插入后，SSA 单元测试 `TestBuildRefFieldAccess` 立即失败。

**错误信息**：

```
SSA verification failed:
  func f, b0, v4 (NilCheck): non-void value has nil Type
```

**原因分析**：

`nilCheck()` 辅助函数创建 `OpNilCheck` 时传入 `nil` 作为 Type：

```go
b.fn.NewValue(b.b, OpNilCheck, nil, ptr)
```

SSA 验证器（`verify.go`）检查：如果一个 Op 不是 void（`!v.Op.IsVoid()`），那么它必须有非 nil 的 Type。`OpNilCheck` 在 `opInfoTable` 中没有 `IsVoid: true` 标记，所以验证器认为它应该有类型。

**修复**：在 `opInfoTable` 中将 `OpNilCheck` 标记为 `IsVoid: true`，同时更新 `TestOpIsVoid` 测试将 `OpNilCheck` 从 nonVoidOps 移到 voidOps。

**教训**：新增 SSA 值创建逻辑时，必须确认 Op 的 void/non-void 属性与传入的 Type 一致。验证器在这方面做了严格检查，是发现问题的第一道防线。

### 2. *T 逃逸检查器阻止跨函数指针传递

**问题**：`pointer_arg.yoru` 最初设计为包含一个 `func sum(arr *[5]int) int` 函数，在 `main` 中调用 `sum(&arr)`。类型检查阶段报错。

**错误信息**：

```
testdata/pointer_arg.yoru:20:19: *T cannot be passed to function (may escape);
use ref T for heap data
```

**原因分析**：

Yoru 的逃逸检查器（`internal/types2/escape.go` 的 `checkCallArgEscape()`）实现了 v1 保守规则：**禁止将 `*T` 传递给任何非 builtin 函数**。这不区分函数签名是否接受 `*T`——即使函数参数类型就是 `*[5]int`，在调用处传入 `&arr` 也会被拒绝。

```go
func (c *Checker) checkCallArgEscape(e *syntax.CallExpr, args []*operand) {
    // ...
    for i, arg := range args {
        if !types.IsPointer(arg.typ) {
            continue
        }
        if !isBuiltin {
            c.errorf(e.Args[i].Pos(),
                "*T cannot be passed to function (may escape); use ref T for heap data")
        }
    }
}
```

这是一个有意为之的保守设计——避免栈指针逃逸到被调用函数的栈帧（如果被调用函数将指针存入全局变量或返回，就会导致 UAF）。

**修复**：将 `pointer_arg.yoru` 改为在同一函数内操作：`var p *[5]int = &arr`，然后通过 `p[i]` 遍历和修改。同样的调整也适用于 `full_example.yoru`——改为函数内数组循环，而非跨函数传递。

**教训**：`*T` 在 Yoru 中是真正的"局部栈指针"——不只是 Go 的 `*T`（Go 允许指针逃逸到堆），而是严格限制在当前函数作用域内。跨函数传递数据应使用 `ref T`（堆引用）。

### 3. collectStrings 遗漏导致 LLVM IR 结构错误（潜在问题）

**问题**：如果不修复 `collectStrings()`，`OpNilCheck` 使用的 `"nil pointer dereference"` 字符串不会在模块顶部预声明。

**潜在后果**：`lowerNilCheck()` 调用 `g.stringIndex("nil pointer dereference")` 时会动态创建全局字符串常量。但此时 codegen 已经在函数体内部，新创建的全局声明会插入到函数定义之后——LLVM IR 要求全局声明在函数定义之前。

**实际影响**：在当前测试中这个 bug 可能不总是触发（取决于 `stringIndex` 的缓存和发射时序），但它是一个时间炸弹。

**修复**：在 `collectStrings()` 中显式扫描 `OpNilCheck` 和 `OpPanic`，确保相关字符串在模块顶部统一声明。

---

## 测试结果

### E2E 测试（26 个全部通过）

| 测试 | 期望输出 | 状态 |
|------|---------|------|
| arith_float | `6.28` | PASS（5A） |
| arith_int | `10\n3\n28\n3\n1` | PASS（5A） |
| arithmetic | `7` | PASS（5A） |
| array_index | `10\n30\n50\n150` | PASS |
| bool_ops | `true\nfalse\ntrue\n...` | PASS（5B） |
| break_continue | `55\n11` | PASS（5B） |
| comparison_int | `7\n10\n42\n15` | PASS（5B） |
| fibonacci | `55` | PASS（5B） |
| for_loop | `55\n11` | PASS（5B） |
| full_example | `42\n150\n20000\n1\n55` | PASS |
| func_void | `hello world\nhello yoru` | PASS（5B） |
| if_else | `greater\nten\nsmall positive` | PASS（5B） |
| method_pointer | `30\n40` | PASS |
| method_value | `15` | PASS |
| multi_func | `7\n25\n13` | PASS（5B） |
| nested_loops | `5` | PASS（5B） |
| pointer_arg | `15\n99` | PASS |
| print_bool | `true\nfalse` | PASS（5A） |
| print_float | `3.14` | PASS（5A） |
| print_int | `42` | PASS（5A） |
| print_mixed | `42 hello 3.14 true` | PASS（5A） |
| print_multi | `1 2 3` | PASS（5A） |
| print_string | `Hello, Yoru!` | PASS（5A） |
| ref_heap | `20\n30\n50` | PASS |
| struct_field | `10\n20\n30` | PASS |
| struct_literal | `1\n2\n3\n4` | PASS |

### 全量测试套件

```
go test ./...        → all packages PASS
```

---

## 验收标准核对

设计文档（`yoru-compiler-design.md` Phase 5C）列出的验收标准：

| 验收项 | 对应测试 | 状态 |
|--------|---------|------|
| struct 字段读写 + println | struct_field.yoru | ✅ |
| composite literal（keyed + positional） | struct_literal.yoru | ✅ |
| 数组声明、赋值、循环求和 | array_index.yoru | ✅ |
| 指针取地址 + 通过指针索引 | pointer_arg.yoru | ✅ |
| new(T) + ref T 字段读写 + OpNilCheck | ref_heap.yoru | ✅ |
| value receiver 方法调用 | method_value.yoru | ✅ |
| pointer receiver + 自动取地址 | method_pointer.yoru | ✅ |
| 完整示例（struct+method+array+ref+if/else） | full_example.yoru | ✅ |

---

## 与前阶段的关系

| 阶段 | 提供的基础 | Phase 5C 的使用 |
|------|-----------|----------------|
| Phase 4A | Op 枚举（含内存/聚合/地址 Op） | E2E 测试验证这些 Op 的端到端行为 |
| Phase 4B | SSA builder（struct/array/method/new 的降低逻辑） | 补全 nil check 插入和自动解引用 |
| Phase 4C | mem2reg（alloca → phi） | composite literal 和数组操作中的 alloca 被提升 |
| Phase 5A | codegen 基础设施 + 类型映射（llvmType、GEP 生成） | 复用所有 codegen 基础设施 |
| Phase 5B | 控制流 + 函数调用的端到端验证 | 复用 E2E 测试框架和流水线 |

Phase 5C 的核心改动集中在 SSA builder（`build_expr.go`），新增约 30 行代码。与 Phase 5B 类似，绝大部分编译器基础设施在之前的阶段已经实现。这再次验证了"基础设施先行"策略的有效性——Phase 4A-4C 实现了完整的 SSA Op 和降低逻辑，Phase 5 系列主要通过 E2E 测试来发现和修复遗漏。

---

## Phase 5 总结

Phase 5 共三个子阶段，覆盖了从最简单的 `println(42)` 到 struct + method + ref + array 的完整编译流水线：

| 子阶段 | E2E 测试数 | 核心内容 |
|--------|-----------|---------|
| 5A | 9 | 常量 + 算术 + println + 类型转换 |
| 5B | 9 | 控制流 + 函数调用 + 递归 |
| 5C | 8 | struct + array + pointer + ref + method + new(T) |
| **总计** | **26** | **Yoru 语言核心特性全覆盖** |

Phase 5 完成后，Yoru 编译器已经能够编译和执行包含所有核心语言特性的程序。剩余的 Phase 6-8 将聚焦于 GC（shadow-stack roots、标记-清除、压缩）——不再新增语言特性，而是为现有的 `ref T` 提供正确的内存管理。

---

## 后续计划（Phase 6）

Phase 6 将接入 shadow-stack GC roots，为 `ref T` 提供真正的垃圾回收支持：

- 为每个函数设置 `gc "shadow-stack"` 属性
- 为每个 `ref` 类型局部变量/临时值创建 root slot + `llvm.gcroot`
- 生成 TypeDesc（包含类型大小、指针字段偏移）
- 在每次 call 前后保存/恢复 ref 值到 root slot
- `new(T)` 传入正确的 TypeDesc（替代当前的 null）

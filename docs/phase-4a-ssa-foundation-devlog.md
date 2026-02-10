# Phase 4A 开发记录（SSA 基础设施）

本文档记录 Phase 4A 的开发内容、设计决策、技术原理与踩坑经验。

## 目标与范围

Phase 4A 是 SSA 生成的第一个子阶段，目标是建立 SSA IR 的**数据结构基础设施**，不涉及实际的 AST→SSA 转换（那是 Phase 4B 的工作）。

具体目标：
- 定义 SSA 核心数据结构：`Value`、`Block`、`Func`
- 定义 Op 枚举与 OpInfo 元数据表（含 IsPure/IsVoid 标记）
- 实现文本格式 SSA 打印器
- 实现结构验证器（11 项检查）
- 在 CLI 中接入 `-emit-ssa` 占位
- 全面测试覆盖（21 个测试用例）

### 设计文档变更

将原设计文档中的 Phase 4（单一大阶段）拆分为 4 个子阶段：

| 子阶段 | 内容 | 前置依赖 |
|--------|------|----------|
| 4A | SSA 数据结构 + 打印 + 验证 | Phase 3 |
| 4B | AST→SSA Lowering（alloca 形式） | 4A |
| 4C | 支配树 + mem2reg | 4B |
| 4D | 优化 Passes（DCE/CSE/ConstProp/SROA） | 4C |

**核心策略决定**：采用 **alloca-first approach**（先生成 alloca/load/store，后由 mem2reg 提升为真正的 SSA φ 形式），而非直接构建 SSA。这是因为：
1. alloca 形式生成简单——每个变量声明生成一个 alloca，读取/赋值生成 load/store，无需在构建时计算支配关系或插入 φ 节点。
2. mem2reg 是成熟的标准算法，可以机械地将 alloca 提升为 SSA 寄存器。
3. Go 编译器也采用类似思路（虽然细节不同）：先生成未优化的中间表示，再通过 passes 优化。

---

## 已完成的实现内容

### 文件清单

```
internal/ssa/
├── op.go        # Op 枚举 + OpInfo 表（232 行）
├── value.go     # Value 结构（95 行）
├── block.go     # Block 结构 + BlockKind（100 行）
├── func.go      # Func 结构 + 工厂方法（75 行）
├── print.go     # SSA 文本打印器（135 行）
├── verify.go    # 结构验证器（135 行）
└── ssa_test.go  # 测试（345 行，21 个测试）
```

### 1. Op 枚举与 OpInfo 表 (`op.go`)

**设计思路**：参考 Go 编译器 `cmd/compile/internal/ssa/op.go`，但大幅简化。Go 编译器有数千个 Op（因为它同时覆盖所有架构），Yoru 只定义语言级别的 45 个 Op。

Op 分类如下：

| 类别 | Op | 数量 | IsPure | 说明 |
|------|----|------|--------|------|
| 常量 | Const64, ConstFloat, ConstBool, ConstString, ConstNil | 5 | ✓ | 常量没有副作用 |
| 整数运算 | Add64, Sub64, Mul64, Div64, Mod64, Neg64 | 6 | ✓ | 纯值运算 |
| 浮点运算 | AddF64, SubF64, MulF64, DivF64, NegF64 | 5 | ✓ | 纯值运算 |
| 整数比较 | Eq64, Neq64, Lt64, Leq64, Gt64, Geq64 | 6 | ✓ | 纯值运算 |
| 浮点比较 | EqF64, NeqF64, LtF64, LeqF64, GtF64, GeqF64 | 6 | ✓ | 纯值运算 |
| 指针比较 | EqPtr, NeqPtr | 2 | ✓ | 纯值运算 |
| 布尔运算 | Not, AndBool, OrBool | 3 | ✓ | 短路求值在 AST→SSA 时已用 CFG 展开 |
| 内存操作 | Alloca, Load, Store, Zero | 4 | ✗ | 有副作用；Store/Zero 是 void |
| 复合访问 | StructFieldPtr, ArrayIndexPtr | 2 | ✓ | 本质是指针算术，无副作用 |
| 类型转换 | IntToFloat, FloatToInt | 2 | ✓ | 纯值运算 |
| 函数调用 | StaticCall, Call | 2 | ✗ | 调用可能有任意副作用 |
| 堆分配 | NewAlloc | 1 | ✗ | 调用 runtime，有副作用 |
| SSA 专用 | Phi, Copy, Arg | 3 | ✓ | SSA 形式的基础构件 |
| 取地址 | Addr | 1 | ✓ | 计算 alloca 的地址 |
| 内建函数 | Println, Panic | 2 | ✗ | I/O + 程序终止；void |
| Nil 检查 | NilCheck | 1 | ✗ | 可能 panic |
| 字符串 | StringLen, StringPtr | 2 | ✓ | string 的 {ptr, len} 解构 |

**OpInfo 表的实现方式**：使用固定大小数组 `[opCount]OpInfo`，以 Op 值直接索引，O(1) 查询。`opCount` 是 iota 枚举的最后一个哨兵值，这样新增 Op 时编译器会自动扩展数组大小。

**IsPure 的语义**：

IsPure 标记是后续优化 passes 的关键守卫：
- **Pure 值**：没有副作用，可以被 CSE 合并、被 DCE 删除、被自由移动。
- **Impure 值**：有副作用（内存写入、I/O、可能 panic），不可 CSE、不可 DCE、不可移动。

注意一些细微决策：
- `OpLoad` 标记为 **impure**：虽然"读内存"看似无副作用，但 load 依赖内存状态——同一地址的两次 load 可能因中间的 store 而结果不同。在没有 memory SSA 的前提下，将 load 标记为 impure 是最安全的选择。
- `OpDiv64` 标记为 **pure**：Go 的整数除以零会 panic，但在 SSA 层面我们不区分——如果需要除零检查，应在 lowering 时显式插入。这与 Go 编译器的做法一致。
- `OpNilCheck` 标记为 **impure**：可能触发 panic，因此不能被 DCE 消除。
- `OpAlloca` 标记为 **impure**：它分配栈空间，是内存副作用操作。

**IsVoid 的语义**：

标记为 void 的 Op（Store, Zero, Println, Panic）不产生值。打印器不会为它们输出 `vN = ...` 格式，验证器不要求它们有 Type。

### 2. Value 结构 (`value.go`)

```go
type Value struct {
    ID       ID             // 函数内唯一标识
    Op       Op             // 操作码
    Type     types.Type     // 结果类型（void op 为 nil）
    Args     []*Value       // 输入操作数
    Block    *Block         // 所属基本块
    AuxInt   int64          // 辅助整数（常量值、字段索引等）
    AuxFloat float64        // 辅助浮点数
    Aux      interface{}    // 辅助数据（字符串、*types.FuncObj 等）
    Uses     int32          // 引用计数（供 DCE 使用）
    Pos      syntax.Pos     // 源码位置
}
```

**三个 Aux 字段的设计理由**：

参考 Go 编译器 SSA，Value 需要携带与 Op 相关的附加数据。Go 用 `AuxInt int64` 和 `Aux Aux`（接口）。Yoru 额外增加了 `AuxFloat float64`，因为 Yoru 只有一种浮点类型（64 位），将浮点常量直接存储在字段中比装箱到 `interface{}` 更高效、类型安全。

各 Aux 字段的典型用途：

| 字段 | 用途 |
|------|------|
| `AuxInt` | OpConst64 的整数值、OpConstBool 的 0/1、OpArg 的参数索引、OpStructFieldPtr 的字段索引、OpZero 的字节数 |
| `AuxFloat` | OpConstFloat 的浮点值 |
| `Aux` | OpConstString 的字符串值、OpStaticCall 的 `*types.FuncObj`、OpArg 的参数名、OpAlloca 的变量名 |

**引用计数（Uses）**：

`AddArg`、`SetArgs`、`ReplaceArg` 会自动维护 `Uses` 计数。这为 DCE pass 提供了 O(1) 判断——`Uses == 0 && IsPure()` 的值可以安全删除。

### 3. Block 结构 (`block.go`)

```go
type BlockKind int
const (
    BlockInvalid BlockKind = iota
    BlockPlain      // 无条件跳转 → Succs[0]
    BlockIf         // 条件分支 → Succs[0](then) / Succs[1](else)
    BlockReturn     // 函数返回
    BlockExit       // 程序终止（panic）
)
```

**设计决策：terminator-as-Kind**

与 LLVM IR 不同（terminator 是 block 最后一条指令），Yoru SSA 将 terminator 信息编码在 Block 的 Kind + Controls + Succs 中。这是 Go 编译器的做法——`Block.Kind` 描述如何终止，`Controls` 持有条件值或返回值，`Succs` 持有后继块列表。

好处：
- 不需要遍历 Values 来找 terminator。
- 终止信息与普通 Values 分离，简化遍历逻辑。
- 块的控制流结构一目了然。

**Succs/Preds 双向维护**：

`AddSucc` 同时更新 `b.Succs` 和 `succ.Preds`，确保 CFG 边的双向一致性。验证器会检查这一不变量。

**支配树字段预留**：

`Idom` 和 `Dominees` 字段已定义但 Phase 4A 不使用。这些将在 Phase 4C（支配树 + mem2reg）中填充。

### 4. Func 结构 (`func.go`)

```go
type Func struct {
    Name        string
    Sig         *types.Func     // 类型检查器提供的签名
    Blocks      []*Block        // Blocks[0] = entry
    Entry       *Block
    nextValueID ID
    nextBlockID ID
}
```

**工厂方法设计**：

- `NewFunc(name, sig)` 自动创建 entry block，保证 `Blocks[0] == Entry` 不变量。
- `NewBlock(kind)` 分配递增 ID，追加到 `Blocks` 切片。
- `NewValue(block, op, typ, args...)` 分配递增 ID，追加到指定 block 的 `Values` 切片，并通过 `AddArg` 维护引用计数。
- `NewValuePos` 是带源码位置的变体。

ID 分配简单使用递增计数器，保证 `v0, v1, v2...` 和 `b0, b1, b2...` 的稳定输出（对 golden test 很重要）。

### 5. 文本打印器 (`print.go`)

输出格式设计参考 Go 编译器的 SSA dump 和 LLVM IR 的可读性：

```
func fibonacci(n int) int:
  b0: (entry)
    v0 = Arg <int> {n}
    v1 = Const64 <int> [1]
    v2 = Leq64 <bool> v0 v1
    If v2 -> b1 b2
  b1: <- b0
    Return v0
  b2: <- b0
    v3 = Sub64 <int> v0 v1
    v4 = StaticCall <int> {fibonacci} v3
    v5 = Const64 <int> [2]
    v6 = Sub64 <int> v0 v5
    v7 = StaticCall <int> {fibonacci} v6
    v8 = Add64 <int> v4 v7
    Return v8
```

格式规则：
- 函数头：`func name(params) result:`
- 块头：`  bN: (entry)` 或 `  bN: <- pred1 pred2`（显示前驱列表）
- 值：`    vN = Op <type> [auxint] {aux} arg1 arg2`
- void 值：`    Op <type> args`（不输出 `vN =`）
- 终止器独占一行：`    If vN -> bX bY` / `    Return vN` / `    Plain -> bX`

**AuxInt 显示逻辑**：

常量 Op（Const64, ConstBool）始终显示 `[value]`（即使为 0，因为 `[0]` 是有意义的）。其他 Op 仅当 AuxInt 非零时显示，避免噪音。这在实现中遇到了一个 bug（见踩坑点）。

### 6. 结构验证器 (`verify.go`)

验证器执行 11 项结构检查，全部失败后汇总为一条错误信息返回（而非第一个错就停止），方便一次性修复多个问题。

| # | 检查项 | 出错含义 |
|---|--------|----------|
| 1 | Entry block 无前驱 | 入口块不应有从其他块来的边 |
| 2 | 每个 Block 的 Kind 有效 | BlockInvalid 表示未初始化 |
| 3 | Block.Func 指针正确 | 块必须属于此函数 |
| 4 | Value.Block 指针正确 | 值必须属于其所在块 |
| 5 | Non-void 值有非 nil Type | 产生值的 Op 必须有类型 |
| 6 | Args 非 nil | nil 参数是编程错误 |
| 7 | Phi 参数数 == 前驱数 | SSA 不变量 |
| 8 | 终止器结构匹配 Kind | Plain 需 1 后继，If 需 1 控制+2 后继，Return/Exit 需 0 后继 |
| 9 | Succs/Preds 双向一致 | b→succ 则 succ 的 Preds 包含 b，反之亦然 |
| 10 | 控制值非 nil（void return 除外） | 终止器引用的值必须存在 |
| 11 | 所有引用的值在函数内 | 不允许引用函数外的 Value |

---

## 关键设计决策与理由

### 采用 alloca-first 而非直接构建 SSA

**决策**：Phase 4B 将生成 alloca/load/store 形式，Phase 4C 再由 mem2reg 提升为 SSA。

**理由**：
- 直接构建 SSA 需要在构建过程中同时追踪变量的"当前定义"和"待填充的 φ 节点"，控制流复杂（特别是 for 循环的回边）时容易出错。
- alloca-first 将变量的定义-使用关系完全委托给内存语义——每次写变量就 store，每次读变量就 load——构建过程可以忽略 SSA 形式，大幅降低 4B 的复杂度。
- mem2reg 是经典算法（Cytron et al. 1991），实现稳定，且只需在 4C 一次性完成。
- LLVM 本身的 clang 前端也使用同样策略。

### Terminator 不作为 Value

**决策**：跳转/返回信息编码在 Block.Kind + Controls 中，而非作为普通 Value。

**理由**：Go 编译器的做法。将终止器与普通值分离，使得"遍历块内所有计算"不需要跳过终止器，"获取终止器信息"也不需要扫描 Values 列表。

### 三个 Aux 字段

**决策**：分离 AuxInt（int64）、AuxFloat（float64）、Aux（interface{}）。

**理由**：Yoru 只有 int（64 位）和 float（64 位）两种数值类型。将最常用的整数辅助值和浮点辅助值分开存储，避免装箱/拆箱开销。Aux 接口用于不常见的辅助数据（函数对象、字符串等）。

### OpLoad 标记为 impure

**决策**：虽然 Load 不修改内存，但标记为 impure。

**理由**：在没有 memory SSA（显式追踪内存状态的 SSA 形式）的情况下，两次对同一地址的 load 可能因中间的 store 而得到不同结果。如果将 load 标记为 pure 并允许 CSE，可能会合并两次语义不同的 load。这是安全性选择——宁可少优化，不可错误优化。

如果未来引入 memory SSA（将 store 的结果作为 load 的输入参数），load 可以按内存版本做 CSE，届时可放宽此限制。

### OpDiv64 标记为 pure

**决策**：整数除法可能因除以零而 panic，但仍标记为 pure。

**理由**：在 SSA 层面，div-by-zero 是"未定义行为"——如果需要检查，应在 lowering 时显式插入条件检查。这与 Go 编译器的做法一致。标记为 pure 允许 CSE 合并相同的除法运算。

---

## 踩坑点

### 1. AuxInt 的打印逻辑

**问题**：OpArg 使用 AuxInt 存储参数索引（0, 1, 2...），初版打印器仅对 Const64/ConstBool 等特定 Op 显示 `[auxint]`，导致 `v1 = Arg <int> {y}` 丢失了参数索引 `[1]`。

**表现**：`TestPrintFormat` golden test 失败：

```
got:    v1 = Arg <int> {y}
want:   v1 = Arg <int> [1] {y}
```

**修复**：改为对所有 Op，当 AuxInt 非零时都显示 `[auxint]`。仅对常量 Op（Const64/ConstBool）和特定 Op（Zero/StructFieldPtr）始终显示（因为值为 0 也有意义）。

**教训**：AuxInt 的打印策略应该是"非零即显示"，因为不同 Op 对 AuxInt 的语义不同，无法穷举所有需要显示的 Op。对于 0 值有意义的 Op（如常量），单独处理。

### 2. syntax.Pos 不是整数类型

**问题**：测试中写 `types.NewVar(0, "x", ...)` 试图传递零位置，但 `NewVar` 的第一个参数是 `syntax.Pos`（结构体），不是整数。Go 编译器中 Pos 是 `uint`，但 Yoru 的 Pos 是带三个字段的结构体。

**表现**：编译错误。

**修复**：声明 `var nopos syntax.Pos`（零值结构体）用于测试中的占位位置。

**教训**：在不同包之间工作时，不要假设类型的底层表示。始终查看 API 签名。

### 3. 未使用变量导致编译失败

**问题**：`TestVerifyNoTerminator` 中 `entry := f.Entry` 的 `entry` 变量未使用（只是为了建立函数，实际不需要操作 entry）。Go 的"未使用变量"编译错误很严格。

**修复**：改为 `_ = f.Entry`。

---

## 技术原理补充

### SSA 形式简述

SSA（Static Single Assignment）是编译器中端的主流中间表示。核心不变量：**每个变量只被赋值一次**。

例如，源码：
```
x = 1
x = x + 2
y = x
```

在 SSA 中表示为：
```
v1 = Const64 [1]
v2 = Add64 v1 [2]     // 不是"修改 x"，而是"定义新值 v2"
v3 = Copy v2           // y 引用 v2
```

当控制流合并时（if/else 两条路径修改了同一变量），用 φ（phi）函数选择：
```
  b1: v1 = Const64 [1]    // if 分支
  b2: v2 = Const64 [2]    // else 分支
  b3: v3 = Phi v1 v2      // 合并：来自 b1 取 v1，来自 b2 取 v2
```

SSA 的好处：
1. 每个值只有一个定义点，def-use 链天然可用。
2. 优化 pass 更容易——CSE 只需比较 Op+Args，DCE 只需看 Uses==0。
3. 寄存器分配更简单——SSA 的值天然不会冲突。

### alloca-first approach 详解

在 alloca-first 策略中，4B 阶段会这样转换源码：

源码：
```yoru
func foo(n int) int {
    var x int = n + 1
    if n > 0 {
        x = x * 2
    }
    return x
}
```

alloca 形式 SSA（4B 输出）：
```
func foo(n int) int:
  b0: (entry)
    v0 = Arg <int> {n}
    v1 = Alloca <*int> {x}       // 为 x 分配栈空间
    v2 = Const64 <int> [1]
    v3 = Add64 <int> v0 v2
    Store v1 v3                   // x = n + 1
    v4 = Const64 <int> [0]
    v5 = Gt64 <bool> v0 v4
    If v5 -> b1 b2
  b1:
    v6 = Load <int> v1            // 读 x
    v7 = Const64 <int> [2]
    v8 = Mul64 <int> v6 v7
    Store v1 v8                   // x = x * 2
    Plain -> b2
  b2:
    v9 = Load <int> v1            // 读 x
    Return v9
```

mem2reg 后（4C 输出）：
```
func foo(n int) int:
  b0: (entry)
    v0 = Arg <int> {n}
    v2 = Const64 <int> [1]
    v3 = Add64 <int> v0 v2
    v4 = Const64 <int> [0]
    v5 = Gt64 <bool> v0 v4
    If v5 -> b1 b2
  b1:
    v7 = Const64 <int> [2]
    v8 = Mul64 <int> v3 v7        // 直接用 v3，不需要 load
    Plain -> b2
  b2:
    v9 = Phi <int> v8 v3          // 合并 b1(v8) 和 b0(v3) 的值
    Return v9
```

alloca/load/store 全部消失，变量 x 被提升为 SSA 寄存器。

### OpInfo 表的数组实现

使用固定大小数组而非 map 的原因：
1. Op 是连续的 iota 枚举，天然适合数组索引。
2. 查询 O(1)，无哈希开销。
3. `[opCount]OpInfo` 的大小由编译器根据 opCount 自动确定。如果新增 Op 后忘记在表中添加对应条目，该条目的值为零值 `OpInfo{}`，Name 为空字符串——容易在测试中发现。

### 验证器的"收集所有错误"策略

验证器不在第一个错误处返回，而是收集所有错误后一次性报告。原因：
1. SSA 构建的 bug 往往导致多个关联错误（例如边不一致会同时触发 Succs 和 Preds 两个方向的检查）。
2. 一次性看到所有问题，比逐个修复再重跑更高效。
3. 错误信息格式化为带缩进的多行文本，便于阅读。

---

## 测试覆盖情况

共 21 个测试，覆盖 4 个维度：

### 构建与结构测试
| 测试 | 验证内容 |
|------|----------|
| `TestManualConstruct` | 手动构建 `add(x, y int) int` 函数并检查结构 |
| `TestNewFuncCreatesEntry` | NewFunc 自动创建 entry block |
| `TestFuncNewBlock` | Block ID 递增分配 |
| `TestValueUseCount` | AddArg 正确维护 Uses 计数 |
| `TestTypesIntegration` | 与 types 包的接口兼容性 |

### 打印格式测试
| 测试 | 验证内容 |
|------|----------|
| `TestPrintFormat` | golden test：add 函数的完整输出 |
| `TestPrintIfBlock` | If 终止器格式 |
| `TestPrintPhiBlock` | Phi 节点格式 |
| `TestValueString` | Value 短格式和长格式 |

### 验证器错误测试
| 测试 | 触发的检查项 |
|------|-------------|
| `TestVerifyNilType` | non-void 值的 nil Type |
| `TestVerifyNoTerminator` | plain block 无后继 |
| `TestVerifyPhiArgCount` | phi 参数数与前驱数不匹配 |
| `TestVerifyInconsistentEdges` | Preds 有但 Succs 无对应边 |
| `TestVerifyEntryNoPreds` | entry block 有前驱 |
| `TestVerifyBlockInvalidKind` | BlockInvalid |
| `TestVerifyValueBlockMismatch` | Value.Block 指针错误 |
| `TestVerifyNilArg` | nil 参数 |
| `TestVerifyValid` | 合法函数通过验证 |

### Op 元数据测试
| 测试 | 验证内容 |
|------|----------|
| `TestOpIsPure` | 所有 pure/impure Op 的分类正确性 |
| `TestOpIsVoid` | 所有 void/non-void Op 的分类正确性 |
| `TestOpString` | Op 名称字符串 |
| `TestBlockKindString` | BlockKind 名称字符串 |

---

## CLI 变更

在 `cmd/yoruc/main.go` 中接入 `-emit-ssa` 占位处理：

```go
if *emitSSA {
    fmt.Fprintf(os.Stderr, "yoruc: SSA generation not yet implemented (Phase 4B)\n")
    os.Exit(1)
}
```

实际的 parse → typecheck → SSA → print pipeline 将在 Phase 4B 接入。`-emit-ssa` flag 本身在之前的 Phase 就已经定义（`flag.Bool`），但没有对应的处理分支。

---

## 与 Go 编译器的对比

| 方面 | Go 编译器 | Yoru |
|------|----------|------|
| Op 数量 | ~1000+（含全架构） | 45（语言级别） |
| Op 定义 | 代码生成（`gen/*.rules`） | 手写 iota 枚举 |
| Value.Aux | `Aux` 接口 | AuxInt + AuxFloat + Aux |
| Block terminator | Block.Kind + Controls | 相同 |
| ID 类型 | `ID int32` | `ID int32` |
| 验证器 | `checkFunc`（非常详细） | 11 项基础检查 |
| 打印格式 | 自定义文本 | 类似但简化 |

---

## 后续计划（Phase 4B）

Phase 4A 提供了 SSA 的"空容器"——可以手动构建、打印和验证，但还不能从源码自动生成。Phase 4B 需要：

1. 实现 `build.go`：遍历 Typed AST，为每个函数生成 alloca 形式 SSA。
2. 表达式 lowering：将 `BinaryExpr`、`CallExpr` 等 AST 节点映射到对应的 Op。
3. 语句 lowering：将 `IfStmt` 映射为 CFG 分裂（BlockIf + 两个后继），`ForStmt` 映射为回边。
4. 接入 CLI pipeline：`-emit-ssa` 真正输出从源码生成的 SSA。

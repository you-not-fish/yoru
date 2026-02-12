# Phase 4C 开发记录（支配树 + mem2reg）

本文档记录 Phase 4C 的开发内容、技术原理、设计决策与踩坑经验。

## 目标与范围

Phase 4C 是 SSA 生成的第三个子阶段，目标是将 Phase 4B 输出的 **alloca 形式 SSA** 转换为**真正的 SSA phi 形式**。具体来说：

1. 计算支配树（Dominance Tree）和支配边界（Dominance Frontier）
2. 实现 Pass 基础设施（Pass Runner）
3. 实现 mem2reg Pass：消除冗余的 alloca/load/store，在合并点插入 `OpPhi` 节点
4. 增强验证器支持支配性检查
5. CLI 集成 Pass Pipeline

**核心转换效果**：

```
// Phase 4B 输出（alloca 形式）        →  Phase 4C 输出（phi 形式）
b0: (entry)                              b0: (entry)
  v0 = Alloca <*int> {x}                   v1 = Const64 <int> [1]
  v1 = Const64 <int> [1]                   If v7 -> b1 b2
  Store v0 v1
  If v7 -> b1 b2                         b1: <- b0
                                            v8 = Const64 <int> [2]
b1: <- b0                                  Plain -> b2
  v8 = Const64 <int> [2]
  Store v0 v8                            b2: <- b0 b1
  Plain -> b2                              v9 = Phi <int> v8 v1   // 真正的 SSA！
                                            Return v9
b2: <- b0 b1
  v9 = Load <int> v0
  Return v9
```

---

## 已完成的实现内容

### 文件清单

**新建文件（6 个）**：

```
internal/ssa/
├── dom.go                 # 支配树算法（143 行）
├── dom_test.go            # 支配树测试（233 行，7 个测试）
internal/ssa/passes/
├── pass.go                # Pass 基础设施（59 行）
├── pass_test.go           # Pass Runner 测试（60 行，4 个测试）
├── mem2reg.go             # mem2reg Pass（402 行）
├── mem2reg_test.go        # mem2reg 测试（505 行，15+12 个测试）
```

**修改文件（3 个）**：

```
internal/ssa/
├── func.go                # 新增 NewValueAtFront, ReplaceUses
├── verify.go              # 新增 VerifyDom
cmd/yoruc/
├── main.go                # 新增 -dump-before/-dump-after, Pass Pipeline
```

---

## 技术原理

### 1. 支配关系（Dominance）

**定义**：在 CFG 中，节点 A **支配（dominates）** 节点 B，当且仅当从 entry 到 B 的**每条路径**都经过 A。直觉上，A 支配 B 意味着"想要执行 B，就必须先执行 A"。

**直接支配者（Immediate Dominator, Idom）**：B 的直接支配者是 B 的所有严格支配者中，最接近 B 的那个。每个非 entry 节点恰好有一个 Idom，由此形成一棵树——**支配树（Dominator Tree）**。

```
CFG:                    支配树:
  b0                      b0
  ├→ b1 ──┐              ├── b1
  └→ b2 ──┘              ├── b2
      b3                  └── b3
```

在这个菱形 CFG 中，`b3` 的两条到达路径分别经过 `b1` 和 `b2`，但只有 `b0` 出现在所有路径上，所以 `b3.Idom = b0`。

**支配边界（Dominance Frontier）**：节点 A 的支配边界 `DF(A)` 是这样的节点集合：A 支配 B 的某个前驱，但不严格支配 B 本身。直觉上，DF(A) 是"A 的支配范围的边界"——在这些节点上，A 的值需要通过 φ 节点与其他路径合并。

```
菱形 CFG 中:
  DF(b1) = {b3}    // b1 支配 b1（b3 的前驱），但不严格支配 b3
  DF(b2) = {b3}    // 同理

循环 CFG (b0→b1→b2→b1, b1→b3) 中:
  DF(b2) = {b1}    // b2 支配 b2（b1 的前驱），但不严格支配 b1（回边）
```

### 2. Cooper 支配算法

我们使用 Cooper, Harvey, Kennedy 的 "A Simple, Fast Dominance Algorithm"（2001），这是一个简洁高效的不动点迭代算法：

**步骤**：
1. 对 CFG 做 DFS，计算**逆后序（Reverse Post-Order, RPO）**
2. 初始化：`entry.Idom = entry`（哨兵值），其他块 Idom = nil
3. 迭代直到收敛：对 RPO 中的每个非 entry 块 b：
   - 从 b 的前驱中选取第一个已有 Idom 的作为 `newIdom`
   - 对其余已处理的前驱 p：`newIdom = intersect(p, newIdom)`
   - 若 `b.Idom != newIdom`：更新，标记 changed
4. 修复：`entry.Idom = nil`
5. 从 Idom 关系构建 Dominees 列表

**intersect 函数** 是核心——两个"finger"沿 Idom 链向上走，用 RPO 编号决定哪个 finger 前进，直到相遇：

```go
intersect(b1, b2):
    while b1 != b2:
        while rpoNum[b1] > rpoNum[b2]:  // b1 在 RPO 中更靠后
            b1 = b1.Idom                 // 向上走
        while rpoNum[b2] > rpoNum[b1]:
            b2 = b2.Idom
    return b1  // 最近公共支配者
```

**为什么用 RPO 编号**：RPO 的性质保证如果 A 支配 B，则 A 在 RPO 中出现在 B 之前（即 `rpoNum[A] < rpoNum[B]`）。这使得 `intersect` 可以通过比较数字来决定方向——编号大的需要向上走（更靠近 entry）。

**收敛性**：算法通常在 2-3 次迭代内收敛。对于可归约（reducible）CFG（没有不可归约循环），保证只需 2 次。Yoru 的 for 循环只产生可归约 CFG。

**支配边界计算**：标准算法——对每个有 2+ 前驱的块 b（即 join point），从每个前驱沿 Idom 链向上走到 `b.Idom`，路途中的每个块都将 b 加入其 DF：

```go
for each block b with len(b.Preds) >= 2:
    for each pred p of b:
        runner = p
        while runner != nil && runner != b.Idom:
            DF[runner] += {b}
            runner = runner.Idom
```

### 3. mem2reg 算法

mem2reg（memory to register promotion）是将 alloca/load/store 形式转换为 SSA phi 形式的标准算法。分为 5 个步骤：

#### 3a. 识别可提升的 Alloca

并非所有 alloca 都能提升。一个 alloca 是 **promotable** 的，当且仅当它的每个 use 都是以下之一：
- `OpLoad`：alloca 作为 `Args[0]`（从中读取）
- `OpStore`：alloca 作为 `Args[0]`（向其写入，注意不能是 `Args[1]`）
- `OpZero`：alloca 作为 `Args[0]`（零初始化）

**不可提升**的情况：
- `OpAddr`：地址被取走（可能逃逸）
- `OpStructFieldPtr` / `OpArrayIndexPtr`：部分访问（需要 SROA，Phase 4D）
- `OpStore` 的 `Args[1]`：alloca 被当作值存储到其他地方（地址逃逸）

```go
// 可提升：只有 load/store
v0 = Alloca <*int> {x}
Store v0 v1          // OK: alloca 是 Args[0]（目标地址）
v2 = Load <int> v0   // OK: alloca 是 Args[0]（源地址）

// 不可提升：有 StructFieldPtr 使用
v0 = Alloca <*Point> {p}
v1 = StructFieldPtr v0 [0]  // 非 load/store 使用，不可提升
```

#### 3b. 找到定义块

对每个可提升的 alloca，收集所有包含 `OpStore` 或 `OpZero`（以该 alloca 为 `Args[0]`）的块——这些块是该变量的"定义点"。

#### 3c. 插入 Phi 节点（迭代支配边界）

经典的 **Iterated Dominance Frontier (IDF)** 算法：

```
对每个可提升的 alloca:
    worklist = 该 alloca 的定义块集合
    while worklist 非空:
        取出块 b
        for each d in DF[b]:
            if d 还没有 phi:
                在 d 的开头插入 phi（类型 = alloca 的元素类型）
                phi.Args = make([]*Value, len(d.Preds))  // 预分配，暂为 nil
                将 d 加入 worklist（phi 本身也是定义，需要迭代传播）
```

**为什么用迭代版本**：考虑这样的情况——变量在 `b1` 被赋值，`b3` 是 `b1` 的支配边界，但 `b3` 本身也是另一条路径的汇合点。`b3` 处的 phi 是一个新定义，可能在 `b3` 的支配边界 `b5` 上也需要 phi。不迭代就会遗漏这些间接需要的 phi。

#### 3d. 重命名（支配树前序遍历）

这是 mem2reg 中最核心的步骤——沿支配树前序遍历，维护每个 alloca 的"到达定义（reaching definition）栈"：

```
对每个 alloca 的栈初始化为 [zeroConst]  // 零值常量

visit(block b):
    // 1. 处理本块的 phi——phi 本身就是新定义
    for each phi in phiMap[b]:
        push phi onto stack[alloca]

    // 2. 顺序处理本块的值
    for each value v in b:
        if v is Load(alloca):
            用栈顶替换 v 的所有使用
            标记 v 为 dead
        if v is Store(alloca, val):
            push val onto stack[alloca]
            标记 v 为 dead
        if v is Zero(alloca):
            push zeroConst onto stack[alloca]
            标记 v 为 dead

    // 3. 填充后继块的 phi 参数
    for each successor s of b:
        predIdx = b 在 s.Preds 中的索引
        for each phi in phiMap[s]:
            phi.Args[predIdx] = stack[alloca].top()

    // 4. 递归到支配子树
    for each child in b.Dominees:
        visit(child)

    // 5. 弹出本块推入的定义（恢复栈）
    pop definitions pushed in this block
```

**为什么用支配树前序遍历**：支配树的性质保证——如果 A 支配 B，则 A 中的定义在 B 处一定可见。前序遍历意味着先访问支配者再访问被支配者，所以在处理 B 时，A 中推入的定义还在栈上，可以直接使用。

**为什么要"弹出"**：当从 `visit(b)` 返回后，b 中的定义对 b 的兄弟节点不可见（兄弟节点不被 b 支配）。弹出恢复了父节点的栈状态。

#### 3e. 清理

1. **移除死值**：标记为 dead 的 load/store/zero 被从各块的 Values 列表中删除，同时递减其 args 的 Uses 计数
2. **移除空 alloca**：Uses 降为 0 的 promotable alloca 被删除
3. **消除平凡 phi**：如果 phi 的所有参数都相同（或是自引用），则用该参数替换 phi 的所有使用。循环直到不动点

### 4. Pass 基础设施

```go
type Pass struct {
    Name string
    Fn   func(f *ssa.Func)
}

type Config struct {
    DumpBefore string   // 在此 pass 前 dump SSA（"*" = 所有）
    DumpAfter  string   // 在此 pass 后 dump SSA
    Verify     bool     // 每个 pass 前后验证 SSA
    DumpFunc   string   // 只 dump 指定函数
}

func Run(f *Func, passes []Pass, cfg Config) error
```

Runner 对每个 pass：
1. 若 `DumpBefore` 匹配，向 stderr 打印 SSA
2. 若 `Verify`，在 pass 前调用 `ssa.Verify`
3. 执行 pass
4. 若 `Verify`，在 pass 后调用 `ssa.Verify`
5. 若 `DumpAfter` 匹配，向 stderr 打印 SSA

### 5. 支配性验证器（VerifyDom）

在原有 11 项结构检查基础上新增 5 项支配性检查：

1. **Entry Idom 为 nil**
2. **所有可达非 entry 块的 Idom 非 nil 且不等于自身**
3. **非 phi 值的每个 arg，其定义块必须支配使用块**（同块内要求定义在使用之前）
4. **phi 的每个 arg[i]，其定义块必须支配 Preds[i]**
5. **控制值的定义块必须支配其所在块**

`dominates(a, b)` 通过沿 Idom 链向上走实现，复杂度 O(depth)，对 Yoru 的函数规模足够。

---

## 关键设计决策与理由

### 1. 使用 Cooper 算法而非 Lengauer-Tarjan

**决策**：选择 Cooper 等人 2001 年的简单不动点迭代算法。

**理由**：Lengauer-Tarjan（1979）理论复杂度更优（接近线性），但实现复杂（需要 semidominator、eval-link 等数据结构），且对小型 CFG 常数因子更大。Cooper 算法实现简洁（~80 行代码），对 Yoru 的函数规模（通常 <100 块）性能完全足够。Go 编译器也在 `cmd/compile/internal/ssa/dom.go` 中使用类似的简单算法。

### 2. Phi 参数预分配为 nil

**决策**：插入 phi 时，`phi.Args = make([]*Value, len(b.Preds))`，元素初始为 nil，**不触碰 use count**。

**理由**：phi 的参数要在 rename 阶段才能确定（需要知道每条路径上的到达定义）。如果在插入时就设置参数并增加 use count，rename 阶段还要先 undo 再 redo，增加复杂度。使用 nil 预分配，rename 阶段直接 `phi.Args[predIdx] = val; val.Uses++`，简洁且正确。

### 3. 零值常量作为初始到达定义

**决策**：对每个 promotable alloca，在 entry block 生成一个该类型的零值常量（`Const64[0]`、`ConstBool[0]`、`ConstString{""}`、`ConstNil` 等），作为 rename 栈的初始元素。

**理由**：如果变量声明后没有被赋值就被读取（`var x int; return x`），到达定义栈上只有初始元素。零值常量正确地表达了 Yoru 的零值语义。这些常量在没有被使用时（比如变量在所有路径上都被赋值了），其 Uses 保持 0，后续 DCE 可以清理。

### 4. Pass 放在独立包 `passes/`

**决策**：mem2reg 放在 `internal/ssa/passes/` 包，而不是直接放在 `internal/ssa/`。

**理由**：
- **职责分离**：`ssa` 包定义 IR 数据结构和基本操作，`passes` 包包含变换逻辑。后续 DCE、CSE、ConstProp 等 pass 也放在 `passes/` 下。
- **避免循环依赖**：pass 依赖 `ssa` 包的类型，反过来 `ssa` 包不需要知道 pass 的存在。
- **测试隔离**：`passes` 包的测试可以自由导入 `ssa`，而 `ssa` 包的测试不会意外依赖 pass 逻辑。

### 5. 平凡 Phi 消除在 cleanup 阶段

**决策**：rename 完成后，额外运行一轮 trivial phi elimination。

**理由**：IDF 算法可能插入一些不必要的 phi。例如变量在 if 的 then 分支赋值，但 else 分支不赋值——merge 处插入的 phi 可能两个参数相同（来自 else 分支的到达定义和初始值可能相同）。消除平凡 phi（所有非自引用参数都是同一个值）可以减少后续 pass 的工作量。

### 6. ComputeDom 在 Mem2Reg 内部调用

**决策**：`Mem2Reg` 函数内部调用 `ComputeDom`，确保支配树是最新的。

**理由**：pass 应该是自包含的——调用者不需要记住"先调用 ComputeDom 再调用 Mem2Reg"。虽然 CLI 层也调用了 ComputeDom（因为 VerifyDom 需要），重复调用 ComputeDom 开销可以忽略不计。

---

## 踩坑点

### 1. Phi Args 的 Use Count 管理

**问题**：phi 节点在插入时 Args 为 nil，在 rename 时逐个填充。必须精确管理 use count，否则验证器报错或后续 DCE 误删。

**表现**：初版实现在填充 phi args 时忘记增加 use count，导致到达定义的 Uses 偏低，cleanup 阶段错误地删除了还在被 phi 引用的值。

**修复**：在 rename 阶段填充 phi 参数时显式增加 use count：
```go
phi.Args[predIdx] = val
val.Uses++  // 关键！
```

**教训**：SSA 中所有值之间的引用关系必须通过 use count 精确跟踪。任何新建引用（包括 phi args）都必须对应 `Uses++`，任何断开引用都必须 `Uses--`。

### 2. NewValueAtFront 的切片操作

**问题**：在块的 Values 列表开头插入 phi 节点时，需要将现有元素后移。

**表现**：初版使用 `append + copy` 模式，但顺序错误导致覆盖数据。

**修复**：正确的 prepend 模式是先 append 一个 nil 扩展切片，再 copy 后移，最后设置第一个元素：
```go
b.Values = append(b.Values, nil)      // 扩展
copy(b.Values[1:], b.Values)          // 后移
b.Values[0] = v                       // 插入
```

**教训**：Go 切片的 prepend 操作不如 append 直观。`copy(dst, src)` 在 dst 和 src 重叠时行为正确（类似 memmove），但必须保证先扩展再 copy。

### 3. ReplaceUses 必须同时处理 Controls

**问题**：`ReplaceUses(old, new)` 初版只扫描了 `v.Args`，忘记处理 `b.Controls`。

**表现**：mem2reg 将一个 Load 替换为 Arg 值后，return block 的 Controls 仍然引用旧的 Load，验证器报告"control value not found in function"。

**修复**：`ReplaceUses` 同时扫描所有块的 Controls：
```go
for i, c := range b.Controls {
    if c == old {
        old.Uses--
        b.Controls[i] = new
        new.Uses++
    }
}
```

**教训**：SSA 中值的"使用者"不只是其他值的 Args——还有块终止器的 Controls（条件分支的条件值、return 的返回值）。任何全局替换操作必须覆盖两者。

### 4. Yoru 解析器的 `Name{` 歧义

**问题**：编写测试时，`if c { ... }` 模式导致 parse error。

**表现**：
```
test.yoru:4:5: expected }
test.yoru:5:2: expected {
```

**原因分析**：Yoru 的解析器在 `operand()` 中，遇到 `_Name` token 后会检查下一个 token 是否是 `_Lbrace`（`{`）——如果是，就当作复合字面量（composite literal）`T{...}` 来解析。

所以 `if c {` 被解析为：
1. `if` 关键字 → 开始 if 语句
2. `c` → Name token，检查下一个是 `{` → 尝试解析 `c{...}` 复合字面量
3. `}` 在 `c{` 之后出现 → 被当作空复合字面量 `c{}`
4. 但 `c` 不是类型名 → 后续解析全部错位

相同问题也影响 `if c == true {`，因为 `true` 在 Yoru 中是 `_Name` token（预声明标识符，不是关键字），所以 `true{` 也被误认为复合字面量。

**修复**：在测试中避免在 if 条件中使用裸标识符 + `{` 的组合，改用比较表达式：
```yoru
// 错误: 触发复合字面量歧义
if c { ... }
if c == true { ... }

// 正确: 比较运算符不会触发歧义（> 不是 {）
if x > 0 { ... }

// 正确: 括号隔离了 Name 和 {
if (c) { ... }
```

**教训**：Go 编译器也有这个歧义（`if T{} == T{} {}`），通过在特定上下文中禁止 composite literal 来解决。Yoru 目前没有实现这个上下文感知，所以使用括号或避免裸标识符条件是必要的 workaround。这是解析器层面已知的局限性，不影响语言的表达能力。

### 5. `*T` 指针不能作为函数参数

**问题**：测试"地址被取走的变量不应被 mem2reg 提升"时，尝试将 `*int` 指针传递给函数，触发 Yoru 的类型检查错误。

**表现**：
```
type errors:
  test.yoru:7:11: *T cannot be passed to function (may escape); use ref T for heap data
```

**原因分析**：Yoru 的两指针系统规定 `*T` 是栈指针，不能逃逸。将 `*T` 传递给函数意味着指针可能在被调用方中存活更久，违反栈指针的生命周期约束。

**修复**：改用手动构建 SSA 来测试此场景——直接创建带有 `OpAddr` 使用的 alloca，绕过语言层面的限制：
```go
alloca := f.NewValue(entry, ssa.OpAlloca, types.NewPointer(types.Typ[types.Int]))
addr := f.NewValue(entry, ssa.OpAddr, types.NewPointer(types.Typ[types.Int]), alloca)
// addr 的存在使 alloca 不可提升
```

**教训**：Yoru 的 `*T`/`ref T` 设计在测试中会产生意想不到的约束。对于需要测试低层 SSA 行为的场景，手动构建 SSA（绕过 parse → typecheck 流水线）比写 Yoru 源代码更灵活。

### 6. 零值常量的残留

**问题**：mem2reg 为每个 promotable alloca 在 entry block 生成零值常量，但如果变量在所有路径上都被赋值了，零值常量就是多余的。

**表现**：
```
b0: (entry)
  v1 = Const64 <int> [0]       // mem2reg 生成的零值
  v14 = Const64 <int> [0]      // 另一个 alloca 的零值
  ...                           // 这些值从未被使用
```

**当前状态**：这不是 bug——零值常量的 Uses 为 0，后续 DCE pass 会清理它们。Phase 4C 的职责仅是生成正确的 phi 形式 SSA，清理冗余值是 Phase 4D（优化 passes）的工作。

**教训**：每个 pass 只做一件事。mem2reg 不需要兼顾清理——保持职责单一可以减少 bug 并方便测试。

---

## 实现细节

### 1. Func 辅助方法 (`func.go`)

**`NewValueAtFront(b, op, typ, args...)`**：

与 `NewValue` 功能相同，但将新值**前置**到 `b.Values` 而非追加。用于在块开头插入 phi 节点——phi 必须在块的其他值之前，因为它们从前驱块获取值，不依赖本块内的其他计算。

**`ReplaceUses(old, new)`**：

全函数扫描，将所有对 `old` 的引用替换为 `new`。用于 rename 阶段将 Load 替换为到达定义，以及 trivial phi elimination。通过 `ReplaceArg(i, new)` 调整 Args 的 use count，手动调整 Controls 的 use count。

### 2. 支配树实现 (`dom.go`)

**RPO 计算**：DFS 从 entry 开始，后序追加到 order 切片，最后反转得到 RPO。不可达块不会被访问，自动排除。

**Cooper 算法**：entry 的 Idom 初始化为自身（哨兵值，用于 intersect 终止条件），最后修复为 nil。`intersect` 利用 RPO 编号的单调性高效找到最近公共支配者。

**支配边界**：只扫描有 2+ 前驱的块（join point），从每个前驱向上走 Idom 链直到被支配者的 Idom，途中每个块都将该 join point 加入其 DF。使用 `appendUnique` 避免重复。

### 3. mem2reg 各子步骤

**可提升性判断**：遍历所有值的所有 args，检查 alloca 被用在什么位置。关键区分：`OpStore` 的 `Args[0]` 是目标地址（OK），`Args[1]` 是值（地址逃逸，NOT OK）。

**IDF 计算**：经典 worklist 算法，确保 phi 的插入也会触发新的 DF 传播。

**Rename**：用 `pushCounts` map 记录本块推入了多少个定义，退出时精确弹出。这比用 snapshot/restore 更简洁——不需要复制整个栈。

**Dead value 移除**：两阶段——先移除标记为 dead 的 load/store/zero 并递减 use count，再移除 Uses==0 的 alloca。两阶段顺序很重要：必须先移除 dead values（它们引用了 alloca），alloca 的 use count 才会降到 0。

### 4. 验证器 (`verify.go`)

VerifyDom 先调用 Verify（结构完整性），再检查支配性。`dominates(a, b)` 的实现是从 b 沿 Idom 链向上走到 nil（entry 的 Idom），途中如果碰到 a 就返回 true。

对 phi 值的特殊处理：phi 的 `arg[i]` 不需要支配 phi 所在块，但必须支配 `Preds[i]`。这是因为 phi 的语义是"在从 pred[i] 进入时取 arg[i] 的值"，所以 arg[i] 只需要在 pred[i] 处可用。

---

## 测试覆盖

### 支配树测试（7 个）

| 测试 | CFG 结构 | 验证内容 |
|------|----------|---------|
| `TestDomSingleBlock` | 单块 | entry.Idom = nil |
| `TestDomLinearChain` | b0→b1→b2 | b1.Idom=b0, b2.Idom=b1 |
| `TestDomDiamond` | b0→{b1,b2}→b3 | b3.Idom=b0, DF(b1)=DF(b2)={b3} |
| `TestDomLoop` | b0→b1→b2→b1, b1→b3 | b2.Idom=b1, DF(b2)={b1} |
| `TestRPOOrdering` | 菱形 | entry 在 RPO 首位，merge 在末位 |
| `TestDomComplex` | 嵌套菱形 | 二层嵌套的 Idom 和 DF |
| `TestDomUnreachable` | entry + 孤立块 | 不可达块 Idom = nil |

### Pass Runner 测试（4 个）

| 测试 | 验证内容 |
|------|---------|
| `TestRunEmpty` | 空 pass 列表不报错 |
| `TestRunSinglePass` | 单个 pass 被调用 |
| `TestRunWithVerify` | 开启 verify 模式不报错 |
| `TestRunMultiplePasses` | 多个 pass 按顺序执行 |

### mem2reg 测试（15 个 + 12 个子测试）

| 测试 | 源码模式 | 验证内容 |
|------|----------|---------|
| `SimpleReturn` | `var x = 42; return x` | 0 alloca, 0 load, 0 store |
| `Parameter` | `func f(x int) int { return x }` | 0 alloca, 1 OpArg |
| `Reassignment` | `x=1; x=2; return x` | 返回 Const64[2] |
| `DiamondPhi` | if/else 赋不同值 | >=1 phi, 0 alloca |
| `LoopPhi` | for 循环变量 | >=1 phi, 0 alloca |
| `NonPromotableStruct` | struct 字段访问 | alloca 保留 |
| `NonPromotableAddr` | OpAddr 使用 | alloca 保留 |
| `ShortCircuit` | `a && b` | 现有 phi 保留，param alloca 消除 |
| `ZeroInit` | `var x int; return x` | 返回 Const64[0] |
| `MultipleVars` | 2 个变量的 phi | >=2 phi |
| `AllocaCount` | 前后对比 | alloca 数量减少到 0 |
| `BoolVar` | `var x bool` | ConstBool[0] |
| `StringVar` | `var s string = "hello"` | 0 alloca |
| `FloatVar` | `var x float = 3.14` | 0 alloca |
| `ExistingBuildTests` | 12 个已有语法模式 | 全部 verify 通过 |
| `PassRunner` | 通过 Run() 执行 mem2reg | 0 alloca, verify 通过 |

---

## CLI 使用

```bash
# 输出 mem2reg 后的 SSA
./build/yoruc -emit-ssa file.yoru

# 对比 mem2reg 前后
./build/yoruc -emit-ssa -dump-before=mem2reg -dump-after=mem2reg file.yoru

# 所有 pass 前后都 dump
./build/yoruc -emit-ssa -dump-before="*" -dump-after="*" file.yoru

# 开启验证 + 只看特定函数
./build/yoruc -emit-ssa -ssa-verify -dump-func=f file.yoru
```

---

## 后续工作

Phase 4C 完成后，SSA 已经是真正的 phi 形式。下一步 Phase 4D 将实现优化 passes：

| Pass | 功能 | 依赖 |
|------|------|------|
| DCE | 死代码消除 | use count |
| ConstProp | 常量传播 | phi + 算术 |
| CSE | 公共子表达式消除 | IsPure |
| SROA | 标量替代聚合 | struct/array alloca 分解 |

当前 mem2reg 留下的零值常量残留将由 DCE 自然清理。

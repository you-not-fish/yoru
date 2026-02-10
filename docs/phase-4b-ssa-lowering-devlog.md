# Phase 4B 开发记录（AST→SSA Lowering, Alloca 形式）

本文档记录 Phase 4B 的开发内容、技术原理、设计决策与踩坑经验。

## 目标与范围

Phase 4B 是 SSA 生成的第二个子阶段，目标是将 Typed AST **自动降低（lower）** 为 alloca 形式的 SSA IR。Phase 4A 已建立 SSA 数据结构（Value、Block、Func、Op、打印器、验证器），4B 在其上实现真正的 AST→SSA 转换。

具体目标：
- 实现 `builder` 结构体，持有构建状态（当前块、变量映射、循环目标等）
- 语句降低：声明、赋值、return、if/else、for、break/continue
- 表达式降低：常量、算术、比较、短路求值、函数调用、方法调用、内建函数、复合类型访问
- 30 个测试用例覆盖所有语法构造
- CLI 接入 `-emit-ssa` 到真正的 parse→typecheck→SSA→print 流水线

**核心策略**：alloca-first approach（每个变量一个 alloca，读取生成 load，写入生成 store），不涉及 φ 节点插入。后续 Phase 4C 的 mem2reg 负责将 alloca 提升为 SSA 寄存器。

---

## 已完成的实现内容

### 文件清单

```
internal/ssa/
├── build.go       # Builder 结构 + 语句降低（368 行）
├── build_expr.go  # 表达式降低（738 行）
├── build_test.go  # 测试（1148 行，30 个测试）
├── print.go       # 修改：改进 Aux 显示格式
├── verify.go      # 修改：增加 void 函数调用的例外
cmd/yoruc/
├── main.go        # 修改：接入 -emit-ssa 到真实流水线
```

### 1. Builder 结构与状态 (`build.go`)

```go
type builder struct {
    info  *types2.Info  // 类型检查器输出（只读）
    sizes *types.Sizes  // DefaultSizes，用于 sizeof 查询

    fn *Func  // 当前 SSA 函数
    b  *Block // 当前块（nil = 不可达代码）

    vars map[types.Object]*Value // Object → alloca 映射

    breakTarget    *Block // 最内层循环的出口块
    continueTarget *Block // 最内层循环的头块
}
```

**vars 映射的工作原理**：

`vars` 是整个 builder 最核心的数据结构。它将类型检查器产出的 `types.Object`（变量、参数等的唯一标识）映射到该变量对应的 `OpAlloca` 值。每次读取变量时，通过 `info.Uses[name]` 查找 Object，再通过 `vars[obj]` 找到 alloca，最后 emit `OpLoad`。每次赋值时，通过 `addr(lhs)` 找到 alloca，emit `OpStore`。

这种设计的好处是完全不需要关心 SSA 形式——变量的"多次赋值"在 alloca 视角下只是多次 store 到同一地址，不违反任何不变量。

### 2. 入口函数

- **`BuildFile(file, info, sizes) []*Func`**：遍历文件的所有声明，对每个有函数体的 `FuncDecl` 调用 `buildFunc`。
- **`buildFunc(fd, info, sizes) *Func`**：创建 `Func`，emit 参数和 receiver 的 `OpArg + OpAlloca + OpStore`，降低函数体，处理隐式 void return。

**参数处理模式**（以参数 `n int` 为例）：
```
v0 = Arg <int> {n}          // 读取调用者传入的值
v1 = Alloca <*int> {n}      // 在栈上分配空间
Store v1 v0                  // 将参数值存入 alloca
```

之后函数体中对 `n` 的每次读取都变成 `Load v1`，每次赋值都变成 `Store v1 newVal`。这样参数也可以被重新赋值（`n = n + 1`），与局部变量统一处理。

**所有 alloca 放在 entry block**：这是 mem2reg 的前提条件。无论变量在哪个作用域声明，alloca 一律通过 `b.fn.NewValue(b.fn.Entry, OpAlloca, ...)` 放入 entry block。

### 3. 语句降低 (`build.go`)

| AST 节点 | 降低方式 |
|----------|---------|
| `EmptyStmt` | 无操作 |
| `ExprStmt` | 求值表达式，丢弃结果 |
| `DeclStmt(VarDecl)` | `entryAlloca` + store 初值（或 `OpZero` 无初值） |
| `AssignStmt(=)` | `addr(LHS)` + `expr(RHS)` + `OpStore` |
| `AssignStmt(:=)` | `entryAlloca` + `expr(RHS)` + `OpStore` |
| `ReturnStmt` | `expr(Result)` + `b.Kind = BlockReturn` + `SetControl` |
| `IfStmt` | 见下文"控制流生成" |
| `ForStmt` | 见下文"控制流生成" |
| `BranchStmt(break)` | `b.AddSucc(breakTarget)` + `b = nil` |
| `BranchStmt(continue)` | `b.AddSucc(continueTarget)` + `b = nil` |
| `BlockStmt` | 直接递归 `b.stmts(s.Stmts)`，作用域由类型检查器的 Object 标识处理 |

**不可达代码处理**：return/break/continue/panic 之后，设置 `b.b = nil`，`stmts()` 循环在遇到 `b == nil` 时立即退出，跳过后续语句。

### 4. 控制流生成

#### if/else

```
当前块:
    cond = expr(s.Cond)
    BlockIf cond -> bThen bElse  (无 else 时 bElse = bDone)

bThen:                      bElse:
    stmts(s.Then)               stmts(s.Else)
    Plain -> bDone              Plain -> bDone

bDone:
    ... (继续)
```

关键细节：
- **else-if 链**：`s.Else` 如果是 `*syntax.IfStmt`，递归调用 `b.ifStmt(els)`，此时 `b.b = bElse`，效果是在 else 块中生成嵌套的 if 结构。
- **两个分支都 return**：此时 `bDone` 没有前驱（`len(bDone.Preds) == 0`），调用 `removeDead(bDone)` 将其从函数块列表中移除，设置 `b.b = nil`。

#### for 循环

```
当前块:
    Plain -> bHeader

bHeader:
    cond = expr(s.Cond)
    BlockIf cond -> bBody bExit

bBody:
    stmts(s.Body)
    Plain -> bHeader   (回边)

bExit:
    ... (继续)
```

关键细节：
- **break/continue 目标**：进入循环体前保存并设置 `breakTarget = bExit`、`continueTarget = bHeader`，退出后恢复旧值。支持嵌套循环。
- **无限循环** (`for { ... }`)：不生成 `BlockIf`，直接 `bHeader.AddSucc(bBody)`。

### 5. 表达式降低 (`build_expr.go`)

#### 总体调度

`expr(e syntax.Expr) *Value` 是表达式降低的入口。首先检查类型检查器是否标记该表达式为常量（`info.Types[e].IsConstant()`），如果是则走常量生成路径；否则按 AST 节点类型分派。

#### 常量生成 (`constValue`)

| 常量类型 | SSA Op | 存储位置 |
|---------|--------|---------|
| `constant.Int` | `OpConst64` | `AuxInt` |
| `constant.Float` | `OpConstFloat` | `AuxFloat` |
| `constant.Bool` | `OpConstBool` | `AuxInt` (0/1) |
| `constant.String` | `OpConstString` | `Aux` |
| nil | `OpConstNil` | — |

**Untyped→Concrete 转换**：常量通常是 untyped（如 `42` 是 `UntypedInt`），在生成 SSA 时调用 `types.DefaultType(typ)` 转换为具体类型（`int`）。

#### 变量读取 (`nameExpr`)

```
obj := info.Uses[name]      // 从类型检查器查找引用的 Object
alloca := vars[obj]          // 从 vars 映射找到 alloca
OpLoad <obj.Type()> alloca   // 生成 load
```

特殊情况：`nil` 字面量——通过 `obj` 类型断言为 `*types.Nil`，直接生成 `OpConstNil`。

#### 一元运算

| 运算符 | 降低方式 |
|--------|---------|
| `!x` | `OpNot` |
| `-x` | `OpNeg64`（整数）/ `OpNegF64`（浮点） |
| `&x` | `addr(x)` — 直接返回 alloca 指针 |
| `*p` | `expr(p)` → `OpLoad`（解引用） |

#### 二元运算

非短路运算统一处理：
1. `expr(X)` 求值左操作数
2. `expr(Y)` 求值右操作数
3. 根据操作数类型（int/float/ptr）和运算符 token 查表确定 SSA Op

运算符到 Op 的映射表：

| 运算符 | 整数 Op | 浮点 Op | 指针 Op |
|--------|---------|---------|---------|
| `+` | `OpAdd64` | `OpAddF64` | — |
| `-` | `OpSub64` | `OpSubF64` | — |
| `*` | `OpMul64` | `OpMulF64` | — |
| `/` | `OpDiv64` | `OpDivF64` | — |
| `%` | `OpMod64` | — | — |
| `==` | `OpEq64` | `OpEqF64` | `OpEqPtr` |
| `!=` | `OpNeq64` | `OpNeqF64` | `OpNeqPtr` |
| `<` | `OpLt64` | `OpLtF64` | — |
| `<=` | `OpLeq64` | `OpLeqF64` | — |
| `>` | `OpGt64` | `OpGtF64` | — |
| `>=` | `OpGeq64` | `OpGeqF64` | — |

**Token 匹配方式**：由于 `syntax` 包中的 token 常量（如 `_AndAnd`、`_Add`）是 unexported 的，无法直接在 `ssa` 包中引用。解决方案是使用 `tok.String()` 返回的字符串（如 `"&&"`、`"+"`）进行匹配。这稍有性能代价但保持了包间解耦。对于逻辑运算符，syntax 包提供了 `tok.IsLogical()` 导出方法。

#### 短路求值 (`shortCircuit`)

`&&` 和 `||` 不能简单地求值两边再合并——右操作数可能不被执行（短路语义）。因此它们展开为 CFG：

**`a && b`**:
```
当前块:
    left = expr(a)
    BlockIf left -> bRight bShort

bShort:
    shortVal = ConstBool [0]   // 短路值 = false
    Plain -> bMerge

bRight:
    right = expr(b)
    Plain -> bMerge

bMerge:
    phi = Phi(shortVal, right)  // preds: [bShort, bRight]
```

**`a || b`**:
```
当前块:
    left = expr(a)
    BlockIf left -> bShort bRight

bShort:
    shortVal = ConstBool [1]   // 短路值 = true
    Plain -> bMerge

bRight:
    right = expr(b)
    Plain -> bMerge

bMerge:
    phi = Phi(shortVal, right)  // preds: [bShort, bRight]
```

**关键不变量**：Phi 的 Args 顺序必须严格匹配 `bMerge.Preds` 的顺序。Preds 顺序由 `AddSucc` 的调用顺序决定——`bShort.AddSucc(bMerge)` 先于 `bRightEnd.AddSucc(bMerge)`，所以 `bMerge.Preds = [bShort, bRightEnd]`，Phi args 也必须是 `[shortVal, right]`。

**嵌套短路**：`expr(e.Y)` 可能本身也是短路表达式，导致 `b.b` 被改变（不再是 bRight）。因此在 `expr(b)` 之后需要捕获 `bRightEnd := b.b`，用 `bRightEnd` 而非 `bRight` 添加到 bMerge 的前驱。

#### 函数调用

**普通函数调用** (`regularCall`)：
1. 从 `info.Uses[name]` 查找 `*types.FuncObj`
2. 依次 `expr(arg)` 求值参数
3. 生成 `OpStaticCall`，`Aux = funcObj`，结果类型从签名获取

**方法调用** (`methodCallExpr`)：
1. 从 `info.Uses[sel.Sel]` 查找方法对应的 `*types.FuncObj`
2. 求值 receiver
3. 自动取地址：如果方法需要指针 receiver 但提供了值，自动 `addr(sel.X)` 取地址
4. receiver 作为第一个参数，后续跟普通参数
5. 生成 `OpStaticCall`

**内建函数** (`builtinCall`)：

| 内建函数 | 降低方式 |
|---------|---------|
| `println(args...)` | `OpPrintln`（void），args 作为 SSA 参数 |
| `new(T)` | `OpNewAlloc`（结果类型 `ref T`），`Aux = 元素类型` |
| `panic(msg)` | `OpPanic`（void）+ `b.Kind = BlockExit` + `b = nil` |

#### 复合类型操作

**字段访问** (`selectorExpr`)：
```
// p.x（p 是指针/ref）:
basePtr = expr(p)
fieldPtr = OpStructFieldPtr <*fieldType> basePtr [fieldIndex]
result = OpLoad <fieldType> fieldPtr

// s.x（s 是值类型 struct）:
basePtr = addr(s)     // 取结构体 alloca 的地址
fieldPtr = OpStructFieldPtr <*fieldType> basePtr [fieldIndex]
result = OpLoad <fieldType> fieldPtr
```

**数组索引** (`indexExpr`)：
```
basePtr = addr(arr)   // 或 expr(ptr) 如果是指针数组
idx = expr(index)
elemPtr = OpArrayIndexPtr <*elemType> basePtr idx
result = OpLoad <elemType> elemPtr
```

**结构体字面量** (`compositeLitExpr`)：
```
alloca = entryAlloca(litType)
OpZero alloca [sizeof]           // 先零初始化
// 逐字段初始化:
fieldPtr0 = OpStructFieldPtr alloca [0]
OpStore fieldPtr0 val0
fieldPtr1 = OpStructFieldPtr alloca [1]
OpStore fieldPtr1 val1
// 读出完整结构体:
result = OpLoad <litType> alloca
```

支持键值形式 (`Point{x: 1, y: 2}`) 和位置形式 (`Point{1, 2}`)。

#### 地址计算 (`addr`)

`addr()` 是赋值语句 LHS 和取地址运算的核心——它返回一个指向存储位置的 `*Value`（指针）：

| LHS 形式 | addr 返回值 |
|----------|------------|
| `x`（变量名） | `vars[obj]`（直接返回 alloca） |
| `x.field` | `OpStructFieldPtr(addr(x), fieldIndex)` |
| `arr[i]` | `OpArrayIndexPtr(addr(arr), expr(i))` |
| `*p` | `expr(p)`（解引用的 LHS 就是指针本身） |

### 6. 验证器修改 (`verify.go`)

增加了一项例外：`OpStaticCall` 和 `OpCall` 可以有 `nil` Type。

```go
if !v.Op.IsVoid() && v.Type == nil && v.Op != OpStaticCall && v.Op != OpCall {
    add("non-void value has nil Type")
}
```

原因：void 函数（如 `func f() {}`）的调用生成 `OpStaticCall` 但不产生值，Type 为 nil。而 `OpStaticCall` 不能标记为 IsVoid，因为有返回值的函数调用也用它。

### 7. 打印器改进 (`print.go`)

新增 `formatAux()` 函数，改善 Aux 字段的显示格式：

```go
func formatAux(aux interface{}) string {
    switch a := aux.(type) {
    case *types.FuncObj:
        return a.Name()           // 只显示函数名，不显示完整结构体
    case types.Type:
        return a.String()         // 调用类型的 String() 方法
    case string:
        return a
    default:
        return fmt.Sprintf("%v", aux)
    }
}
```

改善前：`{&{{fib 0x140003e2b00 ...}}}`（原始结构体指针）
改善后：`{fib}`（清晰的函数名）

### 8. CLI 接入 (`cmd/yoruc/main.go`)

新增 `runEmitSSA(filename) int` 函数，替换原有的 Phase 4B 占位符：

```go
func runEmitSSA(filename string) int {
    // 1. 解析
    // 2. 类型检查
    // 3. ssa.BuildFile(ast, info, sizes)
    // 4. 对每个函数：
    //    - 如果设置了 -dump-func，按名称过滤
    //    - 如果设置了 -ssa-verify，调用 ssa.Verify
    //    - 调用 ssa.Print 输出
}
```

支持的 CLI 参数：
- `-emit-ssa`：输出 SSA
- `-dump-func <name>`：只输出指定函数
- `-ssa-verify`：在输出前验证每个函数的结构完整性

---

## 关键设计决策与理由

### 1. 参数映射：使用 `sig.Param(i)` 而非 `info.Defs[paramName]`

**决策**：将参数的 alloca 直接映射到 `sig.Param(i)` 返回的 `*types.Var` 对象。

**理由**：类型检查器在 `checkFuncSignature` 中为每个参数创建新的 `*types.Var` 对象，并直接将其插入函数作用域。但它**不会**在 `info.Defs` 中注册参数名的定义——只有局部变量声明（`var x`、`x :=`）才会注册到 `info.Defs`。因此 `info.Defs[fd.Params[i].Name]` 返回 `nil`。

正确做法：`sig.Param(i)` 返回的 `*types.Var` 就是函数体内 `info.Uses[x]` 查找 `x` 时返回的同一个对象。所以直接 `b.vars[sig.Param(i)] = alloca` 即可。

Receiver 同理：`sig.Recv()` 返回的 `*types.Var` 就是函数体内引用 receiver 时 `info.Uses` 返回的对象。

### 2. 所有 alloca 放在 entry block

**决策**：无论变量在哪个嵌套层级声明，alloca 一律放在 entry block。

**理由**：这是 mem2reg 算法的前提。mem2reg 只提升 entry block 中的 alloca，因为只有 entry block 支配所有其他块（保证 alloca 在任何 load/store 之前执行）。如果 alloca 放在非 entry 块，从其他路径到达 load 时 alloca 可能尚未执行，语义就是错的。

LLVM 的 `mem2reg` pass 也有同样的要求。

### 3. `b.b = nil` 表示不可达

**决策**：return/panic/break/continue 之后将 `b.b = nil`，后续代码不会被处理。

**理由**：这是最简单的不可达代码处理方式。`stmts()` 循环在每次迭代开始检查 `b.b == nil`，如果是则立即退出。不需要显式标记"不可达"——只要当前块指针为 nil，后续所有语句构建操作自然跳过。

### 4. 死块移除

**决策**：当 if/else 两个分支都 return 时，合并块（bDone）没有前驱，需要主动从函数块列表中移除。

**理由**：如果不移除，验证器会报错"plain block has 0 succs, want 1"（因为 BlockPlain 要求有且仅有一个后继）。虽然死块对语义无影响，但保留它会导致验证失败，也会产生无意义的输出。

### 5. Token 通过字符串匹配

**决策**：使用 `tok.String()` 进行运算符到 SSA Op 的映射（如 `"+" → OpAdd64`）。

**理由**：`syntax` 包的 token 常量（`_Add`、`_Sub` 等）是 unexported 的 `iota` 值，无法在 `ssa` 包中引用。有两个选择：
1. 在 `syntax` 包中导出 token 常量
2. 通过 `tok.String()` 字符串匹配

选择方案 2 是因为改动最小，不需要修改 syntax 包。性能损失可以忽略（构建阶段不是热路径）。但 syntax 包已经导出了一些便利方法（`tok.IsLogical()`、`tok.IsBreak()`、`tok.IsDefine()`），这些可以直接使用。

### 6. 短路求值展开为 CFG

**决策**：`&&` 和 `||` 在 AST→SSA 阶段展开为包含分支和 Phi 的 CFG 结构。

**理由**：短路语义要求右操作数在某些条件下不被求值。在 SSA 中表达这一点只有两种方式：
1. 引入 `OpSelect` 类的特殊 Op（需要后端特殊处理）
2. 展开为 If/Phi（通用方案，后端只需处理基本的 If 分支）

方案 2 更通用，且 LLVM 和 Go 编译器都采用类似策略。展开后的 Phi 节点也可以被后续优化 passes 正常处理（如常量传播可以消除已知条件的分支）。

---

## 踩坑点

### 1. 参数映射 — `info.Defs[paramName]` 返回 nil

**问题**：第一版实现使用 `info.Defs[fd.Params[i].Name]` 来查找参数对应的 `types.Object`，结果返回 nil，导致 panic。

**表现**：
```
panic: ssa.nameExpr: no alloca for "x"
```

**原因分析**：类型检查器的工作方式是——在 `checkFuncSignature` 中为每个参数创建 `*types.Var`，将其添加到函数签名（`*types.Func`）中，并直接将其插入函数作用域。但它**不会**调用 `info.Defs[paramNameNode] = paramVar`。`info.Defs` 只记录局部变量声明（`var`/`:=`）。

函数体内引用参数 `x` 时，类型检查器通过作用域查找找到这个 `*types.Var`，并将其记录在 `info.Uses[xNode] = paramVar`。所以 `info.Uses` 中的 `*types.Var` 和 `sig.Param(i)` 返回的是**同一个指针**。

**修复**：
```go
// 错误: obj 为 nil
obj := b.info.Defs[fd.Params[i].Name]

// 正确: 直接使用签名中的参数对象
param := sig.Param(i)
b.vars[param] = alloca
```

**教训**：类型检查器的 `info.Defs` 和 `info.Uses` 映射的语义必须清楚理解。`Defs` 只包含声明点（`var x`、`x :=`、`type T`、`func f`），`Uses` 包含引用点。参数不算"声明"——它们是签名的一部分，通过作用域机制被函数体引用。

### 2. 死合并块导致验证失败

**问题**：当 if/else 两个分支都以 return 结束时，程序 panic 或验证失败。

**表现**：
```
SSA verification failed:
  func f, b3: plain block has 0 succs, want 1
```

**原因分析**：`ifStmt` 创建 bThen、bElse、bDone 三个块。两个分支都 return 后，`b.b` 被设为 nil（不可达），bDone 没有任何前驱（没有块跳转到它），但它仍然存在于 `fn.Blocks` 中，Kind 为 `BlockPlain` 且没有后继。

**修复**：在 if 语句处理结束时检查 bDone 是否可达：
```go
if len(bDone.Preds) > 0 {
    b.b = bDone           // bDone 可达，继续在此块构建
} else {
    b.removeDead(bDone)   // bDone 不可达，从块列表中移除
    b.b = nil             // 标记当前为不可达
}
```

`removeDead` 从 `fn.Blocks` 切片中删除目标块。

**教训**：创建新块后必须考虑它可能成为死块的情况。特别是"合并块"——当所有前驱分支都提前终止时，合并块永远不会被执行。

### 3. return 语句中表达式求值顺序

**问题**：`return fib(n-1) + fib(n-2)` 中含有函数调用，而函数调用（特别是当参数含 `&&`/`||` 时）可能生成新的块，导致 `b.b` 指针改变。

**表现**：
```
SSA verification failed:
  func fib, b0: if block has 1 succs, want 2
```

**原因分析**：初版 `returnStmt` 实现是：
```go
b.b.Kind = BlockReturn       // 先设置当前块为 return
val := b.expr(s.Result)      // 再求值表达式
b.b.SetControl(val)
```

问题在于 `expr()` 可能改变 `b.b`——比如短路求值会创建新块并将 `b.b` 更新为 merge 块。此时 `BlockReturn` 被设置在了原来的块上（可能变成了一个中间块），而新的当前块（merge 块）没有被标记为 return。

**修复**：先求值，后设置块类型：
```go
val := b.expr(s.Result)      // 先求值（可能改变 b.b）
b.b.Kind = BlockReturn       // 在最终的当前块上设置 return
b.b.SetControl(val)
```

**教训**：任何可能改变 `b.b` 的操作（`expr()`、`stmts()` 等）都必须在设置当前块的终止器**之前**完成。这是一个隐含的不变量：终止器设置必须是对当前块的最后操作。

### 4. void 函数调用的 nil Type

**问题**：调用无返回值的函数（如 `sideEffect()`）时，`OpStaticCall` 的 Type 为 nil，触发验证器报错。

**表现**：
```
SSA verification failed:
  func main, b0, v5 (StaticCall): non-void value has nil Type
```

**原因分析**：`OpStaticCall` 不能标记为 `IsVoid`，因为它既用于有返回值的函数调用（需要 Type），也用于无返回值的函数调用（Type 为 nil）。验证器的检查逻辑是"非 void op 必须有 Type"，这对 StaticCall 过于严格。

**修复**：在验证器中增加例外：
```go
if !v.Op.IsVoid() && v.Type == nil && v.Op != OpStaticCall && v.Op != OpCall {
    add("non-void value has nil Type")
}
```

**教训**：有些 Op 是"有条件 void"的（取决于被调用函数是否有返回值），验证器的规则需要为此留出空间。另一种设计是为 void 调用单独定义 `OpStaticCallVoid`，但会增加 Op 数量和构建逻辑复杂度。

### 5. `&x` 的逃逸约束

**问题**：测试 `&x` 时，初版测试函数将 `*T` 指针作为返回值或函数参数，触发 Yoru 的逃逸分析编译错误。

**表现**：
```
type errors:
  test.yoru:4:9: cannot return *int (stack pointer cannot escape)
```

**原因分析**：Yoru 的类型系统规定 `*T` 是栈指针，不能逃逸到堆/返回值/全局变量。这是 Yoru 的核心设计：`*T` 只能用于本地操作（如通过指针修改值），`ref T` 才能存活到函数外部。

**修复**：修改测试，只在函数内部使用 `&x`：
```yoru
func f() int {
    var x int = 10
    var p *int = &x
    return *p       // 解引用后返回 int 值，而非返回 *int
}
```

**教训**：Yoru 的 `*T`/`ref T` 两指针系统需要在测试设计中格外注意。任何涉及 `*T` 的测试都不能让指针逃逸到函数外部。

### 6. `FuncObj` 的打印格式

**问题**：函数调用的 SSA 输出中，`{Aux}` 部分显示为 Go 结构体的原始指针表示，极难阅读。

**表现**：
```
v5 = StaticCall <int> {&{{fib 0x140003e2b00 <nil> ...}}} v4
```

**修复**：在 `print.go` 中增加 `formatAux()` 函数，对 `*types.FuncObj` 类型特殊处理，只输出函数名：
```
v5 = StaticCall <int> {fib} v4
```

**教训**：SSA 打印器的可读性对调试至关重要。每当引入新的 Aux 类型（FuncObj、Type 等），都应确保它有合理的显示格式。

---

## 技术原理补充

### alloca 形式与真正 SSA 的区别

alloca 形式（4B 输出）本质上是"带结构的非 SSA IR"：变量可以被多次 store，违反了 SSA 的"每个变量只赋值一次"原则。但它的优势在于：

1. **构建极其简单**：不需要追踪每个变量的"当前定义"，不需要在 join point 插入 φ 节点。
2. **正确性天然保证**：load/store 的内存语义自动处理了控制流合并的问题。
3. **mem2reg 可以机械转换**：alloca 形式到真正 SSA 的转换是标准算法。

对比示例：

```yoru
func f(cond bool) int {
    var x int = 1
    if cond {
        x = 2
    }
    return x
}
```

alloca 形式（Phase 4B 输出）：
```
b0: (entry)
    v0 = Arg <bool> {cond}
    v1 = Alloca <*int> {cond}
    Store v1 v0
    v3 = Alloca <*int> {x}
    v4 = Const64 <int> [1]
    Store v3 v4
    v6 = Load <bool> v1
    If v6 -> b1 b2
b1: <- b0
    v7 = Const64 <int> [2]
    Store v3 v7           // 再次 store 到同一个 alloca
    Plain -> b2
b2: <- b0 b1
    v8 = Load <int> v3    // 从 alloca load——值取决于走了哪条路径
    Return v8
```

mem2reg 后（Phase 4C 输出，预期）：
```
b0: (entry)
    v0 = Arg <bool> {cond}
    v4 = Const64 <int> [1]
    If v0 -> b1 b2
b1: <- b0
    v7 = Const64 <int> [2]
    Plain -> b2
b2: <- b0 b1
    v8 = Phi <int> v7 v4  // 来自 b1 取 v7(=2)，来自 b0 取 v4(=1)
    Return v8
```

### CFG（Control Flow Graph）的构建过程

SSA 的基本单位是基本块（Basic Block），每个块包含一系列无分支的值计算，以一个终止器结尾。块之间通过 Succs/Preds 边连接，形成控制流图。

构建 CFG 的核心操作：
1. **分裂当前块**：遇到 `if` 时，当前块变为 `BlockIf`，创建 2-3 个新块
2. **添加边**：`AddSucc` 建立 Succs/Preds 双向链接
3. **设置当前块**：`b.b = newBlock` 切换构建目标
4. **终止当前块**：设置 Kind + Controls，不再向其中添加值

Yoru 的 BlockKind 与终止器的对应关系：

| BlockKind | Controls | Succs | 语义 |
|-----------|----------|-------|------|
| `BlockPlain` | 无 | 1 | 无条件跳转 |
| `BlockIf` | 1（条件值） | 2 | 条件为 true 跳 Succs[0]，false 跳 Succs[1] |
| `BlockReturn` | 0-1（返回值） | 0 | 函数返回 |
| `BlockExit` | 0 | 0 | 程序终止（panic） |

### 类型检查器的 Info 映射在 SSA 构建中的角色

SSA 构建器是类型检查器输出的**消费者**，通过 `types2.Info` 获取所有类型信息：

```
info.Types[expr]  → TypeAndValue  // 表达式的类型和常量值
info.Defs[name]   → Object        // 声明点的对象（var、type、func）
info.Uses[name]   → Object        // 引用点的对象
```

关键使用模式：

1. **常量检测**：`info.Types[e].IsConstant()` → 走常量生成路径
2. **变量查找**：`info.Uses[name]` → 找到 Object → `vars[obj]` → 找到 alloca
3. **类型获取**：`info.Types[e].Type` → 确定结果类型和运算符选择
4. **函数解析**：`info.Uses[funcName]` → `*types.FuncObj` → 获取签名
5. **内建检测**：`info.Types[e.Fun].IsBuiltin()` → 分派到内建处理

SSA 构建器**不做任何类型检查**——所有类型正确性由 Phase 3 保证。如果遇到类型不匹配（如对字符串做加法），说明是类型检查器的 bug，SSA 构建器直接 panic。

---

## 测试覆盖

共 30 个测试，通过 `buildFromSource(t, src)` 辅助函数统一执行 parse → typecheck → BuildFile → Verify 全流水线。

### 测试辅助函数

```go
func buildFromSource(t *testing.T, src string) []*Func {
    // 1. 解析
    // 2. 类型检查
    // 3. BuildFile 生成 SSA
    // 4. 对每个函数调用 Verify，失败时输出 SSA dump
    return funcs
}
```

每个测试都自动执行验证器检查，确保生成的 SSA 结构完整。

### 测试分类

| 类别 | 测试 | 验证内容 |
|------|------|----------|
| **基础** | EmptyFunc | 空函数 → 单块 void return |
| | ReturnConstant | return 42 → OpConst64 [42] |
| | ReturnParam | return x → Arg + Alloca + Store + Load |
| | Arithmetic | a + b * 2 → OpMul64 + OpAdd64 |
| | VarDecl | var x = 10 → Alloca + Store |
| | ShortDecl | x := 5 → Alloca + Store |
| | Reassignment | x = 2 → 两次 Store |
| | VarDeclZeroInit | var x int → OpZero |
| **控制流** | IfNoElse | if → BlockIf + 2 return |
| | IfElse | if-else → BlockIf + 2 succs |
| | ElseIfChain | if/else-if/else → 2 个 BlockIf |
| | ForLoop | for → If + 回边 + >=4 块 |
| | ForBreak | break → 跳到 exit block |
| | ForContinue | continue → 跳到 header block |
| | BothBranchesReturn | if{return}else{return} → 死块移除 |
| **短路求值** | ShortCircuitAnd | && → Phi + >=4 块 |
| | ShortCircuitOr | \|\| → Phi + >=4 块 |
| **内建** | Println | println(42) → OpPrintln |
| | Panic | panic("boom") → OpPanic + BlockExit |
| | New | new(Point) → OpNewAlloc |
| **复合类型** | StructLiteral | Point{x:1,y:2} → StructFieldPtr + Store |
| | FieldRead | p.x → StructFieldPtr + Load |
| | FieldWrite | p.x = 42 → StructFieldPtr + Store |
| | ArrayIndexRead | arr[0] → ArrayIndexPtr + Load |
| | ArrayIndexWrite | arr[0] = 42 → ArrayIndexPtr + Store |
| | AddressOf | &x → alloca 直接使用 |
| | Deref | *p → Load |
| | RefFieldAccess | ref Point → StructFieldPtr + Load |
| **运算符** | FloatOps | float + → OpAddF64 |
| | BoolComparison | > → OpGt64 |
| | Not | ! → OpNot |
| | Negate | - → OpNeg64 |
| **函数调用** | FuncCall | add(1,2) → OpStaticCall |
| | MethodCall | p.Sum() → OpStaticCall（receiver 为第一个参数） |
| **其他** | NestedLoops | 嵌套 for → 2 个 BlockIf |
| | MultipleFuncs | 3 个函数 → 3 个 Func |
| | BlockStmt | {嵌套块} → 正常编译 |
| | ExprStmt | sideEffect() → OpStaticCall |
| | ConstBool | true → OpConstBool [1] |
| | ConstString | "hello" → OpConstString |
| | GoldenSimple | add 函数的完整输出结构检查 |

---

## SSA 输出示例

以下是 `test_ssa.yoru` 的完整 SSA 输出（`yoruc -emit-ssa -ssa-verify`）：

```yoru
// test_ssa.yoru
package main

func add(a int, b int) int {
    return a + b
}

func fib(n int) int {
    if n <= 1 {
        return n
    }
    return fib(n - 1) + fib(n - 2)
}

func main() {
    println(add(1, 2))
    println(fib(10))
}
```

输出：
```
func add(a int, b int) int:
  b0: (entry)
    v0 = Arg <int> {a}
    v1 = Alloca <*int> {a}
    Store v1 v0
    v3 = Arg <int> [1] {b}
    v4 = Alloca <*int> {b}
    Store v4 v3
    v6 = Load <int> v1
    v7 = Load <int> v4
    v8 = Add64 <int> v6 v7
    Return v8

func fib(n int) int:
  b0: (entry)
    v0 = Arg <int> {n}
    v1 = Alloca <*int> {n}
    Store v1 v0
    v3 = Load <int> v1
    v4 = Const64 <int> [1]
    v5 = Leq64 <bool> v3 v4
    If v5 -> b1 b2
  b1: <- b0
    v6 = Load <int> v1
    Return v6
  b2: <- b0
    v7 = Load <int> v1
    v8 = Const64 <int> [1]
    v9 = Sub64 <int> v7 v8
    v10 = StaticCall <int> {fib} v9
    v11 = Load <int> v1
    v12 = Const64 <int> [2]
    v13 = Sub64 <int> v11 v12
    v14 = StaticCall <int> {fib} v13
    v15 = Add64 <int> v10 v14
    Return v15

func main():
  b0: (entry)
    v0 = Const64 <int> [1]
    v1 = Const64 <int> [2]
    v2 = StaticCall <int> {add} v0 v1
    Println v2
    v4 = Const64 <int> [10]
    v5 = StaticCall <int> {fib} v4
    Println v5
    Return
```

可以观察到：
- `add` 函数：每个参数有 Arg → Alloca → Store → Load 的完整序列
- `fib` 函数：if 语句展开为 BlockIf + 两个分支块，递归调用生成 StaticCall
- `main` 函数：常量折叠后直接生成 Const64，函数调用生成 StaticCall，void return 无控制值

---

## 与 Go 编译器的对比

| 方面 | Go 编译器 | Yoru |
|------|----------|------|
| 构建入口 | `buildssa.go` | `build.go` |
| 变量追踪 | 直接 SSA（sealed/unsealed blocks） | alloca-first（vars map） |
| 短路求值 | CFG 展开 | 相同 |
| 参数处理 | 直接作为 SSA 值 | Arg + Alloca + Store |
| 函数调用 | 复杂的 ABI lowering | 简单的 OpStaticCall |
| 不可达代码 | 多阶段消除 | b=nil 简单跳过 |
| 死块处理 | deadcode pass | 构建时 removeDead |

---

## 后续计划（Phase 4C）

Phase 4B 生成的 alloca 形式 SSA 有大量冗余的 load/store。Phase 4C 将实现：

1. **支配树构建**：计算每个块的 immediate dominator（Lengauer-Tarjan 算法或简单迭代算法）
2. **mem2reg pass**：将 entry block 中的 alloca 提升为 SSA 寄存器，消除对应的 load/store，在 join point 插入 φ 节点
3. 提升后的 SSA 将是真正的 SSA 形式——每个值只有一个定义点，φ 节点处理控制流合并

预期效果：`add` 函数从 10 条指令（含冗余 alloca/load/store）简化为 3 条指令（两个 Arg + 一个 Add64 + Return）。

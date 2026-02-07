# Phase 2: 语法分析器设计与实现文档

## 概述

本文档详细描述 Yoru 编译器语法分析器（Parser）的设计与实现方案。语法分析器是编译器前端的第二个阶段，负责将 Token 流转换为抽象语法树（AST）。

### 目标

1. 实现完整的递归下降语法分析器
2. 使用 Pratt 算法解析表达式（支持运算符优先级）
3. 定义清晰的 AST 节点层次结构
4. 实现错误恢复机制（单文件报告多个错误）
5. 提供 `-emit-ast` 输出（文本和 JSON 格式）
6. 完善的测试覆盖（单元测试、Golden 测试、Fuzz 测试）

### 前置条件

Phase 1（词法分析器）已完成：
- 288 个测试用例通过
- ASI（自动分号插入）支持
- Fuzz 测试稳定
- Token.Precedence() 方法可用于 Pratt 解析

### 参考实现

主要参考 Go 编译器的语法分析器：
- `cmd/compile/internal/syntax/parser.go`
- `cmd/compile/internal/syntax/nodes.go`
- `cmd/compile/internal/syntax/walk.go`

---

## 1. 子阶段划分

根据任务量和依赖关系，Phase 2 划分为 4 个子阶段：

| 子阶段 | 内容 | 预计时间 | 依赖 |
|--------|------|----------|------|
| 2.1 | AST 节点定义 + 基础设施 | 1 周 | Phase 1 完成 |
| 2.2 | 声明解析（package, import, type, func, var） | 1.5 周 | 2.1 |
| 2.3 | 语句和表达式解析 | 1.5 周 | 2.2 |
| 2.4 | 错误恢复 + 测试 + CLI 集成 | 1 周 | 2.3 |

---

## 2. Phase 2.1: AST 节点定义 + 基础设施

### 2.1 核心节点接口

**文件**: `internal/syntax/nodes.go`

```go
package syntax

// Node 是所有 AST 节点实现的接口
type Node interface {
    Pos() Pos       // 节点起始位置
    End() Pos       // 节点结束位置（用于错误范围）
    aNode()         // 标记方法，限制实现
}

// node 是所有 AST 节点嵌入的基础结构体
type node struct {
    pos Pos
}

func (n *node) Pos() Pos { return n.pos }
func (n *node) End() Pos { return n.pos } // 默认实现：先返回起始位置
func (n *node) aNode()   {}
```

### 2.2 表达式节点

```go
// Expr 是表达式节点的接口
type Expr interface {
    Node
    aExpr()
}

type expr struct{ node }

func (*expr) aExpr() {}

// Name 表示标识符
type Name struct {
    expr
    Value string // 标识符字符串
}

// BasicLit 表示字面量值（int, float, string）
type BasicLit struct {
    expr
    Value string  // 字面量文本
    Kind  LitKind // IntLit, FloatLit, StringLit
}

// Operation 表示一元或二元运算
// 一元运算：Y 为 nil
// 二元运算：X 和 Y 都有值
type Operation struct {
    expr
    Op Token // 运算符 token
    X  Expr  // 左操作数（一元运算时为唯一操作数）
    Y  Expr  // 右操作数（一元运算时为 nil）
}

// CallExpr 表示函数调用：Fun(Args...)
type CallExpr struct {
    expr
    Fun  Expr   // 函数表达式
    Args []Expr // 参数列表
}

// IndexExpr 表示数组/指针索引：X[Index]
type IndexExpr struct {
    expr
    X     Expr // 被索引的表达式
    Index Expr // 索引表达式
}

// SelectorExpr 表示字段访问：X.Sel
type SelectorExpr struct {
    expr
    X   Expr  // 接收者表达式
    Sel *Name // 字段名
}

// ParenExpr 表示括号表达式：(X)
type ParenExpr struct {
    expr
    X Expr
}

// NewExpr 表示堆分配：new(Type)
type NewExpr struct {
    expr
    Type Expr // 要分配的类型
}

// CompositeLit 表示复合字面量：T{Elems...}
// 用于结构体字面量
type CompositeLit struct {
    expr
    Type  Expr   // 类型（推断时可为 nil）
    Elems []Expr // 元素（可以是 KeyValueExpr）
}

// KeyValueExpr 表示复合字面量中的 key:value
type KeyValueExpr struct {
    expr
    Key   Expr // 字段名
    Value Expr // 字段值
}
```

### 2.3 类型表达式节点

```go
// ArrayType 表示 [N]Elem
type ArrayType struct {
    expr
    Len  Expr // 长度表达式（必须是常量）
    Elem Expr // 元素类型
}

// PointerType 表示 *Base
type PointerType struct {
    expr
    Base Expr // 基础类型
}

// RefType 表示 ref Base（GC 托管引用）
type RefType struct {
    expr
    Base Expr // 基础类型
}

// StructType 表示 struct { Fields... }
type StructType struct {
    expr
    Fields []*Field // 字段列表
}

// Field 表示结构体字段
type Field struct {
    node
    Name *Name // 字段名
    Type Expr  // 字段类型
}
```

### 2.4 语句节点

```go
// Stmt 是语句节点的接口
type Stmt interface {
    Node
    aStmt()
}

type stmt struct{ node }

func (*stmt) aStmt() {}

// EmptyStmt 表示空语句（仅分号）
type EmptyStmt struct {
    stmt
}

// ExprStmt 表示作为语句使用的表达式
type ExprStmt struct {
    stmt
    X Expr
}

// AssignStmt 表示赋值：LHS = RHS 或 LHS := RHS
type AssignStmt struct {
    stmt
    Op  Token  // _Assign 或 _Define
    LHS []Expr // 左侧
    RHS []Expr // 右侧
}

// BlockStmt 表示 { Stmts... }
type BlockStmt struct {
    stmt
    Stmts  []Stmt
    Rbrace Pos // } 的位置
}

// IfStmt 表示 if Cond Then [else Else]
type IfStmt struct {
    stmt
    Cond Expr       // 条件
    Then *BlockStmt // then 分支
    Else Stmt       // else 分支（nil, *IfStmt 或 *BlockStmt）
}

// ForStmt 表示 for Cond { Body }
// 注意：Yoru 只支持 "for cond {}" 形式
type ForStmt struct {
    stmt
    Cond Expr       // 条件（仅在语法错误恢复时可能为 nil）
    Body *BlockStmt // 循环体
}

// ReturnStmt 表示 return [Result]
type ReturnStmt struct {
    stmt
    Result Expr // 返回值（裸 return 时为 nil）
}

// BranchStmt 表示 break 或 continue
type BranchStmt struct {
    stmt
    Tok Token // _Break 或 _Continue
}

// DeclStmt 将声明包装为语句
type DeclStmt struct {
    stmt
    Decl Decl
}
```

### 2.5 声明节点

```go
// Decl 是声明节点的接口
type Decl interface {
    Node
    aDecl()
}

type decl struct{ node }

func (*decl) aDecl() {}

// File 表示完整的源文件
type File struct {
    node
    PkgName *Name        // 包名
    Imports []*ImportDecl // 导入声明
    Decls   []Decl       // 顶层声明
}

// ImportDecl 表示 import "path"
type ImportDecl struct {
    decl
    Path *BasicLit // 导入路径（StringLit）
}

// TypeDecl 表示类型声明
// type Name = Type（别名）或 type Name Type（定义）
type TypeDecl struct {
    decl
    Name  *Name
    Alias bool // 类型别名时为 true
    Type  Expr // 类型
}

// VarDecl 表示 var Name Type = Value
type VarDecl struct {
    decl
    Name  *Name
    Type  Expr // 显式类型（推断时为 nil）
    Value Expr // 初始值（无初始值时为 nil）
}

// FuncDecl 表示 func (Recv) Name(Params) Result { Body }
type FuncDecl struct {
    decl
    Recv   *Field     // 接收者（函数时为 nil）
    Name   *Name      // 函数名
    Params []*Field   // 参数列表
    Result Expr       // 返回类型（void 时为 nil）
    Body   *BlockStmt // 函数体
}
```

### 2.6 Phase 2.1 验收标准

- [ ] `nodes.go`: 所有 AST 节点类型定义完整
- [ ] 所有节点实现 `Node` 接口（`Pos()` 和 `End()`）
- [ ] `parser.go` 骨架：`Parser` 结构体、`NewParser()`、`next()`、`got()`、`want()`
- [ ] 能够解析 `package main` 并返回 `*File`
- [ ] 单元测试：节点创建和基本解析

---

## 3. Phase 2.2: 声明解析

### 3.1 Parser 结构体

**文件**: `internal/syntax/parser.go`

```go
package syntax

import "io"

// Parser 对 Yoru 源代码执行语法分析
type Parser struct {
    scanner *Scanner

    // 当前 token 信息（从 scanner 缓存）
    tok Token
    lit string
    pos Pos

    // 错误处理
    errh   func(pos Pos, msg string)
    errcnt int
    first  error // 遇到的第一个错误
    abort  bool  // 达到错误上限后停止继续解析

    // 上下文追踪
    fnest int // 函数嵌套深度（0 = 顶层）
}

// NewParser 创建新的 Parser
func NewParser(filename string, src io.Reader, errh func(pos Pos, msg string)) *Parser {
    scanErrh := func(line, col uint32, msg string) {
        if errh != nil {
            errh(NewPos(filename, line, col), msg)
        }
    }

    p := &Parser{
        scanner: NewScanner(filename, src, scanErrh),
        errh:    errh,
    }
    p.next() // 预读第一个 token
    return p
}

// SetASIEnabled 将 ASI 开关透传到 Scanner
func (p *Parser) SetASIEnabled(enabled bool) {
    p.scanner.SetASIEnabled(enabled)
}
```

### 3.2 Token 导航

```go
// next 前进到下一个 token
func (p *Parser) next() {
    p.scanner.Next()
    p.tok = p.scanner.Token()
    p.lit = p.scanner.Literal()
    p.pos = p.scanner.Pos()
}

// got 报告当前 token 是否为 tok
// 如果是，消费该 token 并返回 true
func (p *Parser) got(tok Token) bool {
    if p.tok == tok {
        p.next()
        return true
    }
    return false
}

// want 如果当前 token 匹配 tok 则消费它
// 否则报告错误
func (p *Parser) want(tok Token) {
    if !p.got(tok) {
        p.syntaxError("expected " + tok.String())
        p.advance()
    }
}

// expect 类似 want 但返回消费的 token 的位置
func (p *Parser) expect(tok Token) Pos {
    pos := p.pos
    p.want(tok)
    return pos
}
```

### 3.3 文件解析（入口点）

```go
// Parse 解析完整的源文件
func (p *Parser) Parse() *File {
    f := &File{}
    f.pos = p.pos

    // 解析 package 声明
    p.want(_Package)
    f.PkgName = p.name()
    p.want(_Semi)

    // 解析 import 声明
    for !p.abort && p.tok == _Import {
        f.Imports = append(f.Imports, p.importDecl())
    }

    // 解析顶层声明
    for !p.abort && p.tok != _EOF {
        if d := p.decl(); d != nil {
            f.Decls = append(f.Decls, d)
        }
    }

    return f
}

// decl 解析顶层声明
func (p *Parser) decl() Decl {
    switch p.tok {
    case _Type:
        return p.typeDecl()
    case _Var:
        return p.varDecl()
    case _Func:
        return p.funcDecl()
    default:
        p.syntaxError("expected declaration")
        p.advance()
        return nil
    }
}
```

### 3.4 Import 声明

```go
// importDecl 解析：import "path"
func (p *Parser) importDecl() *ImportDecl {
    d := &ImportDecl{}
    d.pos = p.pos

    p.want(_Import)

    if p.tok != _Literal || p.scanner.LitKind() != StringLit {
        p.syntaxError("expected string literal for import path")
        return d
    }

    d.Path = &BasicLit{Value: p.lit, Kind: StringLit}
    d.Path.pos = p.pos
    p.next()

    p.want(_Semi)
    return d
}
```

### 3.5 Type 声明

```go
// typeDecl 解析：type Name Type 或 type Name = Type
func (p *Parser) typeDecl() *TypeDecl {
    d := &TypeDecl{}
    d.pos = p.pos

    p.want(_Type)
    d.Name = p.name()

    // 检查是否为别名（=）
    if p.got(_Assign) {
        d.Alias = true
    }

    d.Type = p.type_()
    p.want(_Semi)

    return d
}

// type_ 解析类型表达式
func (p *Parser) type_() Expr {
    switch p.tok {
    case _Name:
        return p.typeName()

    case _Mul: // *T
        return p.pointerType()

    case _Ref: // ref T
        return p.refType()

    case _Lbrack: // [N]T
        return p.arrayType()

    case _Struct:
        return p.structType()

    default:
        p.syntaxError("expected type")
        return &Name{Value: "_"}
    }
}

// structType 解析 struct { Fields... }
func (p *Parser) structType() Expr {
    st := &StructType{}
    st.pos = p.pos

    p.want(_Struct)
    p.want(_Lbrace)

    for p.tok != _Rbrace && p.tok != _EOF {
        st.Fields = append(st.Fields, p.fieldDecl())
    }

    p.want(_Rbrace)
    return st
}
```

### 3.6 Var 声明

```go
// varDecl 解析：var Name Type = Value
func (p *Parser) varDecl() *VarDecl {
    d := &VarDecl{}
    d.pos = p.pos

    p.want(_Var)
    d.Name = p.name()

    // 如果有初始化器，类型是可选的
    if p.tok != _Assign {
        d.Type = p.type_()
    }

    // 可选的初始化器
    if p.got(_Assign) {
        d.Value = p.expr()
    }

    p.want(_Semi)
    return d
}
```

### 3.7 Func 声明

```go
// funcDecl 解析：func (recv) Name(params) result { body }
func (p *Parser) funcDecl() *FuncDecl {
    d := &FuncDecl{}
    d.pos = p.pos

    p.want(_Func)

    // 可选的接收者
    if p.tok == _Lparen {
        d.Recv = p.receiver()
    }

    d.Name = p.name()
    d.Params = p.paramList()

    // 可选的返回类型
    if p.tok != _Lbrace {
        d.Result = p.type_()
    }

    p.fnest++
    d.Body = p.blockStmt()
    p.fnest--

    return d
}

// receiver 解析 (name Type)
func (p *Parser) receiver() *Field {
    f := &Field{}
    f.pos = p.pos

    p.want(_Lparen)
    f.Name = p.name()
    f.Type = p.type_()
    p.want(_Rparen)

    return f
}

// paramList 解析 (p1 T1, p2 T2, ...)
func (p *Parser) paramList() []*Field {
    p.want(_Lparen)

    var params []*Field
    if p.tok != _Rparen {
        params = p.fieldList()
    }

    p.want(_Rparen)
    return params
}
```

### 3.8 Phase 2.2 验收标准

- [ ] `package main` 解析正确
- [ ] `import "path"` 解析正确
- [ ] `type T struct { ... }` 解析正确
- [ ] `type T = U`（类型别名）解析正确
- [ ] `var x T = value` 解析正确
- [ ] `func name(params) result { }` 解析正确
- [ ] `func (recv T) name() { }`（方法）解析正确
- [ ] 所有类型表达式解析正确（`*T`, `ref T`, `[N]T`, `struct`）
- [ ] 单元测试覆盖所有声明类型

---

## 4. Phase 2.3: 语句和表达式解析

### 4.1 语句调度器

```go
// stmt 解析一条语句
func (p *Parser) stmt() Stmt {
    switch p.tok {
    case _Lbrace:
        return p.blockStmt()

    case _If:
        return p.ifStmt()

    case _For:
        return p.forStmt()

    case _Return:
        return p.returnStmt()

    case _Break, _Continue:
        return p.branchStmt()

    case _Var:
        return &DeclStmt{Decl: p.varDecl()}

    case _Semi:
        s := &EmptyStmt{}
        s.pos = p.pos
        p.next()
        return s

    default:
        return p.simpleStmt()
    }
}
```

### 4.2 简单语句

```go
// simpleStmt 解析表达式语句或赋值
func (p *Parser) simpleStmt() Stmt {
    pos := p.pos
    x := p.expr()

    switch p.tok {
    case _Assign, _Define:
        // 赋值或短声明
        return p.assignStmt(pos, x)

    default:
        // 表达式语句
        s := &ExprStmt{X: x}
        s.pos = pos
        p.want(_Semi)
        return s
    }
}

// assignStmt 解析 LHS op RHS，其中 op 是 = 或 :=
func (p *Parser) assignStmt(pos Pos, lhs Expr) Stmt {
    s := &AssignStmt{Op: p.tok, LHS: []Expr{lhs}}
    s.pos = pos

    p.next() // 消费 = 或 :=

    s.RHS = []Expr{p.expr()}
    p.want(_Semi)

    return s
}
```

### 4.3 控制流语句

```go
// ifStmt 解析：if cond { then } [else { else }]
func (p *Parser) ifStmt() Stmt {
    s := &IfStmt{}
    s.pos = p.pos

    p.want(_If)
    s.Cond = p.expr()
    s.Then = p.blockStmt()

    if p.got(_Else) {
        if p.tok == _If {
            s.Else = p.ifStmt() // else if
        } else {
            s.Else = p.blockStmt() // else
        }
    }

    return s
}

// forStmt 解析：for cond { body }
func (p *Parser) forStmt() Stmt {
    s := &ForStmt{}
    s.pos = p.pos

    p.want(_For)

    // Yoru 只支持 for cond { ... }，不支持 bare for
    if p.tok == _Lbrace {
        p.syntaxError("expected for condition")
    } else {
        s.Cond = p.expr()
    }

    s.Body = p.blockStmt()
    return s
}

// returnStmt 解析：return [expr]
func (p *Parser) returnStmt() Stmt {
    s := &ReturnStmt{}
    s.pos = p.pos

    p.want(_Return)

    // 可选的返回值（检查语句终止符）
    if p.tok != _Semi && p.tok != _Rbrace && p.tok != _EOF {
        s.Result = p.expr()
    }

    p.want(_Semi)
    return s
}

// branchStmt 解析：break 或 continue
func (p *Parser) branchStmt() Stmt {
    s := &BranchStmt{Tok: p.tok}
    s.pos = p.pos
    p.next()
    p.want(_Semi)
    return s
}
```

### 4.4 Pratt 算法：表达式解析

**优先级爬升算法**

```go
// expr 解析一个表达式
func (p *Parser) expr() Expr {
    return p.binaryExpr(0)
}

// binaryExpr 使用最小优先级 prec 解析二元表达式
// 实现 Pratt 解析 / 优先级爬升
func (p *Parser) binaryExpr(prec int) Expr {
    x := p.unaryExpr()

    for {
        // 检查当前 token 是否为具有足够优先级的二元运算符
        oprec := p.tok.Precedence()
        if oprec <= prec {
            return x
        }

        // 创建运算节点
        op := &Operation{Op: p.tok, X: x}
        op.pos = p.pos

        p.next() // 消费运算符

        // 用更高优先级解析右操作数（左结合）
        op.Y = p.binaryExpr(oprec)
        x = op
    }
}

// 优先级来自 token.go：
// 1: ||
// 2: &&
// 3: == != < <= > >=
// 4: + - | ^
// 5: * / % & << >>
```

### 4.5 一元表达式解析

```go
// unaryExpr 解析一元表达式
func (p *Parser) unaryExpr() Expr {
    switch p.tok {
    case _Not: // !
        op := &Operation{Op: p.tok}
        op.pos = p.pos
        p.next()
        op.X = p.unaryExpr()
        return op

    case _Sub: // -（取负）
        op := &Operation{Op: p.tok}
        op.pos = p.pos
        p.next()
        op.X = p.unaryExpr()
        return op

    case _Mul: // *（解引用）
        op := &Operation{Op: p.tok}
        op.pos = p.pos
        p.next()
        op.X = p.unaryExpr()
        return op

    case _And: // &（取地址）
        op := &Operation{Op: p.tok}
        op.pos = p.pos
        p.next()
        op.X = p.unaryExpr()
        return op

    default:
        return p.primaryExpr()
    }
}
```

### 4.6 主表达式解析

```go
// primaryExpr 解析主表达式和后缀操作
func (p *Parser) primaryExpr() Expr {
    x := p.operand()

    // 解析后缀操作：调用、索引、选择器
    for {
        switch p.tok {
        case _Lparen: // 函数调用
            x = p.callExpr(x)

        case _Lbrack: // 索引表达式
            x = p.indexExpr(x)

        case _Dot: // 选择器表达式
            x = p.selectorExpr(x)

        default:
            return x
        }
    }
}

// operand 解析操作数（主表达式的基础）
func (p *Parser) operand() Expr {
    switch p.tok {
    case _Name:
        n := &Name{Value: p.lit}
        n.pos = p.pos
        p.next()
        if p.tok == _Lbrace {
            return p.compositeLit(n) // T{...}
        }
        return n

    case _Panic:
        // panic 在词法上是关键字，但在语法上按内建函数处理
        n := &Name{Value: "panic"}
        n.pos = p.pos
        p.next()
        return n

    case _Literal:
        lit := &BasicLit{Value: p.lit, Kind: p.scanner.LitKind()}
        lit.pos = p.pos
        p.next()
        return lit

    case _Lparen: // 括号表达式
        p.next()
        x := p.expr()
        p.want(_Rparen)
        return &ParenExpr{X: x}

    case _New: // new(Type)
        return p.newExpr()

    default:
        p.syntaxError("expected operand")
        return &Name{Value: "_"} // 错误恢复
    }
}

// callExpr 解析 Fun(args...)
func (p *Parser) callExpr(fun Expr) Expr {
    call := &CallExpr{Fun: fun}
    call.pos = p.pos

    p.want(_Lparen)
    if p.tok != _Rparen {
        call.Args = p.exprList()
    }
    p.want(_Rparen)

    return call
}

// indexExpr 解析 X[Index]
func (p *Parser) indexExpr(x Expr) Expr {
    idx := &IndexExpr{X: x}
    idx.pos = p.pos

    p.want(_Lbrack)
    idx.Index = p.expr()
    p.want(_Rbrack)

    return idx
}

// selectorExpr 解析 X.Sel
func (p *Parser) selectorExpr(x Expr) Expr {
    sel := &SelectorExpr{X: x}
    sel.pos = p.pos

    p.want(_Dot)
    sel.Sel = p.name()

    return sel
}

// newExpr 解析 new(Type)
func (p *Parser) newExpr() Expr {
    n := &NewExpr{}
    n.pos = p.pos

    p.want(_New)
    p.want(_Lparen)
    n.Type = p.type_()
    p.want(_Rparen)

    return n
}

// compositeLit 解析 T{elem1, key: value, ...}
func (p *Parser) compositeLit(typ Expr) Expr {
    lit := &CompositeLit{Type: typ}
    lit.pos = typ.Pos()

    p.want(_Lbrace)
    for p.tok != _Rbrace && p.tok != _EOF {
        elem := p.expr()
        if p.got(_Colon) {
            kv := &KeyValueExpr{Key: elem}
            kv.pos = elem.Pos()
            kv.Value = p.expr()
            lit.Elems = append(lit.Elems, kv)
        } else {
            lit.Elems = append(lit.Elems, elem)
        }
        if !p.got(_Comma) {
            break
        }
    }
    p.want(_Rbrace)
    return lit
}
```

### 4.7 Phase 2.3 验收标准

- [ ] 块语句 `{ ... }` 解析正确
- [ ] if/else 语句解析正确（包括 else if 链）
- [ ] for 语句解析正确（只有 `for cond {}` 形式）
- [ ] return 语句解析正确（有/无返回值）
- [ ] break/continue 语句解析正确
- [ ] 赋值语句 `x = v` 和 `x := v` 解析正确
- [ ] 表达式语句（函数调用）解析正确
- [ ] 二元表达式优先级正确（Pratt 算法）
- [ ] 一元表达式解析正确（`!`, `-`, `*`, `&`）
- [ ] 后缀表达式解析正确（调用、索引、选择器）
- [ ] 复合字面量解析正确（`T{...}`, `key: value`）
- [ ] 内建调用 `panic(...)` 语法可解析
- [ ] 优先级测试：`1 + 2 * 3` → `Op{+, 1, Op{*, 2, 3}}`
- [ ] 结合性测试：`a && b || c` → `Op{||, Op{&&, a, b}, c}`

---

## 5. Phase 2.4: 错误恢复 + 测试 + CLI 集成

### 5.1 错误报告

```go
// SyntaxError 表示语法错误
type SyntaxError struct {
    Pos Pos
    Msg string
}

func (e *SyntaxError) Error() string {
    return e.Pos.String() + ": " + e.Msg
}

// syntaxError 在当前位置报告语法错误
func (p *Parser) syntaxError(msg string) {
    p.syntaxErrorAt(p.pos, msg)
}

// syntaxErrorAt 在指定位置报告语法错误
func (p *Parser) syntaxErrorAt(pos Pos, msg string) {
    if p.abort {
        return
    }
    if p.errcnt == 0 {
        p.first = &SyntaxError{Pos: pos, Msg: msg}
    }
    p.errcnt++

    if p.errh != nil {
        p.errh(pos, msg)
    }

    p.errorLimitCheck(pos)
}

const maxErrors = 10

func (p *Parser) errorLimitCheck(pos Pos) {
    if p.errcnt >= maxErrors {
        p.abort = true
        if p.errh != nil {
            p.errh(pos, "too many errors; aborting parse")
        }
        p.tok = _EOF
    }
}
```

### 5.2 错误恢复：同步点

```go
// advance 跳过 token 直到找到同步点
func (p *Parser) advance() {
    // 同步 token 列表
    sync := map[Token]bool{
        _Semi:     true, // 语句终止符
        _Rbrace:   true, // 块结束
        _Rparen:   true, // 参数列表结束
        _Rbrack:   true, // 索引结束
        _Package:  true,
        _Import:   true,
        _Type:     true,
        _Var:      true,
        _Func:     true,
        _If:       true,
        _For:      true,
        _Return:   true,
        _Break:    true,
        _Continue: true,
        _EOF:      true,
    }

    for p.tok != _EOF && !sync[p.tok] {
        p.next()
    }

    // 消费同步点，避免在同一个 token 上重复报错
    if p.tok != _EOF {
        p.next()
    }
}
```

### 5.3 单元测试

**文件**: `internal/syntax/parser_test.go`

```go
package syntax

import (
    "strings"
    "testing"
)

func parseFile(t *testing.T, src string) *File {
    t.Helper()
    p := NewParser("test.yoru", strings.NewReader(src), nil)
    f := p.Parse()
    if f == nil {
        t.Fatal("Parse returned nil")
    }
    return f
}

func astSummary(f *File) string {
    var b strings.Builder
    Fprint(&b, f)
    return b.String()
}

func exprSummary(e Expr) string {
    switch x := e.(type) {
    case *Name:
        return x.Value
    case *BasicLit:
        return x.Value
    case *Operation:
        if x.Y == nil {
            return "Op{" + x.Op.String() + "," + exprSummary(x.X) + "}"
        }
        return "Op{" + x.Op.String() + "," + exprSummary(x.X) + "," + exprSummary(x.Y) + "}"
    case *CallExpr:
        var args []string
        for _, a := range x.Args {
            args = append(args, exprSummary(a))
        }
        return "Call{" + exprSummary(x.Fun) + ",[" + strings.Join(args, ",") + "]}"
    case *IndexExpr:
        return "Index{" + exprSummary(x.X) + "," + exprSummary(x.Index) + "}"
    case *SelectorExpr:
        return "Sel{" + exprSummary(x.X) + "," + x.Sel.Value + "}"
    case *NewExpr:
        return "New{" + exprSummary(x.Type) + "}"
    case *CompositeLit:
        var elems []string
        for _, e := range x.Elems {
            elems = append(elems, exprSummary(e))
        }
        return "Composite{" + exprSummary(x.Type) + ",[" + strings.Join(elems, ",") + "]}"
    case *KeyValueExpr:
        return exprSummary(x.Key) + ":" + exprSummary(x.Value)
    case *ParenExpr:
        return exprSummary(x.X)
    default:
        return "<unknown>"
    }
}

func TestParseDeclarations(t *testing.T) {
    tests := []struct {
        name string
        src  string
        want string // 预期 AST 摘要
    }{
        // Package 声明
        {"package", "package main", "File{PkgName:main}"},

        // Type 声明
        {"type_alias", "package main\ntype T = int", "TypeDecl{T,alias,int}"},
        {"type_struct", "package main\ntype Point struct { x int }", "TypeDecl{Point,Struct}"},

        // Var 声明
        {"var_typed", "package main\nvar x int", "VarDecl{x,int}"},
        {"var_init", "package main\nvar x int = 1", "VarDecl{x,int,1}"},

        // Func 声明
        {"func_simple", "package main\nfunc foo() {}", "FuncDecl{foo}"},
        {"func_params", "package main\nfunc add(a int, b int) int { return a }", "FuncDecl{add}"},
        {"method", "package main\nfunc (p Point) Area() int { return 0 }", "FuncDecl{recv,Area}"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            f := parseFile(t, tt.src)
            got := astSummary(f)
            if !strings.Contains(got, tt.want) {
                t.Fatalf("summary mismatch\nwant contain: %q\ngot:\n%s", tt.want, got)
            }
        })
    }
}

func TestParseExpressions(t *testing.T) {
    tests := []struct {
        src  string
        want string
    }{
        // 字面量
        {"123", "123"},
        {"3.14", "3.14"},
        {`"hello"`, "hello"},

        // 运算
        {"1 + 2", "Op{+,1,2}"},
        {"1 + 2 * 3", "Op{+,1,Op{*,2,3}}"}, // 优先级
        {"a && b || c", "Op{||,Op{&&,a,b},c}"}, // 优先级

        // 一元
        {"-x", "Op{-,x}"},
        {"!b", "Op{!,b}"},
        {"*p", "Op{*,p}"},
        {"&x", "Op{&,x}"},

        // 后缀
        {"foo()", "Call{foo,[]}"},
        {"foo(1, 2)", "Call{foo,[1,2]}"},
        {"arr[0]", "Index{arr,0}"},
        {"p.x", "Sel{p,x}"},
        {"p.x.y", "Sel{Sel{p,x},y}"},

        // 复合
        {"new(Point)", "New{Point}"},
        {"Result{0, false}", "Composite{Result,[0,false]}"},
        {"Point{x: 1, y: 2}", "Composite{Point,[x:1,y:2]}"},
        {"panic(\"boom\")", "Call{panic,[boom]}"},
    }

    for _, tt := range tests {
        t.Run(tt.src, func(t *testing.T) {
            // 包装成最小程序
            src := "package main\nfunc f() { _ = " + tt.src + " }"
            f := parseFile(t, src)
            expr := firstAssignedExpr(t, f)
            got := exprSummary(expr)
            if got != tt.want {
                t.Fatalf("expr mismatch\nwant: %s\ngot:  %s", tt.want, got)
            }
        })
    }
}

func firstAssignedExpr(t *testing.T, f *File) Expr {
    t.Helper()
    fn, ok := f.Decls[0].(*FuncDecl)
    if !ok || fn.Body == nil || len(fn.Body.Stmts) == 0 {
        t.Fatal("missing function body")
    }
    as, ok := fn.Body.Stmts[0].(*AssignStmt)
    if !ok || len(as.RHS) != 1 {
        t.Fatal("missing assignment RHS")
    }
    return as.RHS[0]
}
```

### 5.4 错误测试（20+ 用例）

```go
func TestParseErrors(t *testing.T) {
    tests := []struct {
        name     string
        src      string
        wantErr  string
        wantLine uint32 // 0 表示不校验
        wantCol  uint32 // 0 表示不校验
    }{
        // 缺少 package
        {"no_package", "func main() {}", "expected package", 1, 1},

        // 缺少标识符
        {"missing_name", "package main\ntype = int", "expected identifier", 2, 6},
        {"missing_func_name", "package main\nfunc () {}", "expected identifier", 2, 6},

        // 缺少分隔符
        {"missing_lbrace", "package main\nfunc foo() return", "expected {", 2, 12},
        {"missing_rbrace", "package main\nfunc foo() { return", "expected }", 2, 20},
        {"missing_lparen", "package main\nfunc foo) {}", "expected (", 2, 9},
        {"missing_rparen", "package main\nfunc foo( {}", "expected )", 2, 10},

        // 表达式错误
        {"unexpected_op", "package main\nfunc f() { x = + }", "expected operand", 2, 18},
        {"missing_rhs", "package main\nfunc f() { x = }", "expected", 2, 18},

        // 类型错误
        {"bad_array_type", "package main\ntype T []int", "expected type", 2, 8},
        {"missing_type", "package main\nvar x =", "expected", 2, 8},

        // 语句错误
        {"bad_if", "package main\nfunc f() { if { } }", "expected", 2, 16},
        {"bad_for", "package main\nfunc f() { for x; { } }", "expected", 2, 19},
        {"bad_return", "package main\nfunc f() { return + }", "expected operand", 2, 23},

        // 恢复测试
        {"multi_error_1", "package main\nfunc f() { x = ; y = }", "expected", 2, 20},
        {"multi_error_2", "package main\ntype T struct { x ; y int }", "expected", 2, 19},

        // 额外用例
        {"empty_params", "package main\nfunc f(,) {}", "expected", 2, 8},
        {"bad_receiver", "package main\nfunc (int) f() {}", "expected identifier", 2, 7},
        {"unclosed_paren", "package main\nfunc f() { foo( }", "expected", 2, 20},
        {"bad_index", "package main\nfunc f() { arr[] }", "expected", 2, 21},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            type parseErr struct {
                pos Pos
                msg string
            }
            var errs []parseErr
            errh := func(pos Pos, msg string) {
                errs = append(errs, parseErr{pos: pos, msg: msg})
            }

            p := NewParser("test", strings.NewReader(tt.src), errh)
            _ = p.Parse()

            if len(errs) == 0 {
                t.Errorf("expected error containing %q", tt.wantErr)
            } else {
                found := false
                for _, e := range errs {
                    if strings.Contains(e.msg, tt.wantErr) {
                        found = true
                        if tt.wantLine != 0 && e.pos.Line() != tt.wantLine {
                            t.Errorf("expected line %d, got %d", tt.wantLine, e.pos.Line())
                        }
                        if tt.wantCol != 0 && e.pos.Col() != tt.wantCol {
                            t.Errorf("expected col %d, got %d", tt.wantCol, e.pos.Col())
                        }
                        break
                    }
                }
                if !found {
                    t.Errorf("expected error containing %q, got %v", tt.wantErr, errs)
                }
            }
        })
    }
}
```

### 5.5 Fuzz 测试

```go
func FuzzParse(f *testing.F) {
    seeds := []string{
        "package main",
        "package main\nfunc main() {}",
        "package main\ntype Point struct { x int\n y float }",
        "package main\nfunc f() { if x > 0 { return 1 } else { return 0 } }",
        "package main\nfunc f() { for i < 10 { i = i + 1 } }",
        "package main\nvar x ref Point = new(Point)",
        "package main\nfunc (p *Point) Move() { p.x = p.x + 1 }",
        "package main\nfunc f() { _ = Result{0, false} }",
        "package main\nfunc f() { panic(\"boom\") }",
    }

    for _, seed := range seeds {
        f.Add(seed)
    }

    f.Fuzz(func(t *testing.T, src string) {
        errh := func(pos Pos, msg string) {
            // 语法错误可接受，关键是不 panic
        }

        p := NewParser("fuzz", strings.NewReader(src), errh)
        _ = p.Parse()
    })
}
```

### 5.6 Golden 测试

**目录结构**:
```
internal/syntax/testdata/
    parse_decl.yoru          # 声明解析测试
    parse_decl.ast.golden    # 预期 AST 输出
    parse_expr.yoru          # 表达式解析测试
    parse_expr.ast.golden    # 预期 AST 输出
    parse_stmt.yoru          # 语句解析测试
    parse_stmt.ast.golden    # 预期 AST 输出
    parse_complete.yoru      # 完整程序测试
    parse_complete.ast.golden
```

**Golden 测试实现**:

```go
var updateGolden = flag.Bool("update-golden", false, "update golden files")

func TestParseGolden(t *testing.T) {
    files, _ := filepath.Glob("testdata/parse_*.yoru")

    for _, f := range files {
        t.Run(f, func(t *testing.T) {
            src, _ := os.ReadFile(f)
            p := NewParser(f, bytes.NewReader(src), nil)
            ast := p.Parse()

            var buf bytes.Buffer
            Fprint(&buf, ast) // 格式化 AST
            got := buf.String()

            golden := strings.TrimSuffix(f, ".yoru") + ".ast.golden"

            if *updateGolden {
                os.WriteFile(golden, []byte(got), 0644)
                return
            }

            want, err := os.ReadFile(golden)
            if err != nil {
                t.Fatalf("failed to read golden file: %v", err)
            }

            if got != string(want) {
                t.Errorf("AST mismatch:\ngot:\n%s\nwant:\n%s", got, want)
            }
        })
    }
}
```

---

## 6. 可观测性设计

### 6.1 `-emit-ast` 输出格式

#### 文本格式

**文件**: `internal/syntax/print.go`

```go
package syntax

import (
    "fmt"
    "io"
    "strings"
)

// Fprint 将 AST 的文本表示写入 w
func Fprint(w io.Writer, node Node) {
    p := &printer{w: w}
    p.print(node)
}

type printer struct {
    w      io.Writer
    indent int
}

func (p *printer) printf(format string, args ...interface{}) {
    fmt.Fprintf(p.w, "%s%s", strings.Repeat("  ", p.indent), fmt.Sprintf(format, args...))
}

func (p *printer) print(node Node) {
    switch n := node.(type) {
    case *File:
        p.printf("File %s\n", n.pos)
        p.indent++
        p.printf("Package: %s\n", n.PkgName.Value)
        for _, imp := range n.Imports {
            p.print(imp)
        }
        for _, d := range n.Decls {
            p.print(d)
        }
        p.indent--

    case *FuncDecl:
        p.printf("FuncDecl %s\n", n.pos)
        p.indent++
        if n.Recv != nil {
            p.printf("Recv: %s %s\n", n.Recv.Name.Value, exprString(n.Recv.Type))
        }
        p.printf("Name: %s\n", n.Name.Value)
        if len(n.Params) > 0 {
            p.printf("Params:\n")
            p.indent++
            for _, f := range n.Params {
                p.printf("%s %s\n", f.Name.Value, exprString(f.Type))
            }
            p.indent--
        }
        if n.Result != nil {
            p.printf("Result: %s\n", exprString(n.Result))
        }
        if n.Body != nil {
            p.printf("Body:\n")
            p.indent++
            p.print(n.Body)
            p.indent--
        }
        p.indent--

    // ... 更多 case
    }
}
```

**示例输出**:
```
File test.yoru:1:1
  Package: main
  FuncDecl test.yoru:3:1
    Name: add
    Params:
      a int
      b int
    Result: int
    Body:
      BlockStmt test.yoru:3:24
        ReturnStmt test.yoru:4:5
          Operation +
            X: Name{a}
            Y: Name{b}
```

#### JSON 格式

**文件**: `internal/syntax/json.go`

```go
package syntax

import (
    "encoding/json"
    "io"
)

// FprintJSON 将 AST 的 JSON 表示写入 w
func FprintJSON(w io.Writer, node Node) error {
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(toJSON(node))
}

func toJSON(node Node) interface{} {
    switch n := node.(type) {
    case *File:
        return map[string]interface{}{
            "type":    "File",
            "pos":     n.pos.String(),
            "package": n.PkgName.Value,
            "imports": mapSlice(n.Imports, toJSON),
            "decls":   mapSlice(n.Decls, toJSON),
        }

    case *FuncDecl:
        m := map[string]interface{}{
            "type": "FuncDecl",
            "pos":  n.pos.String(),
            "name": n.Name.Value,
        }
        if n.Recv != nil {
            m["recv"] = toJSON(n.Recv)
        }
        m["params"] = mapSlice(n.Params, toJSON)
        if n.Result != nil {
            m["result"] = toJSON(n.Result)
        }
        if n.Body != nil {
            m["body"] = toJSON(n.Body)
        }
        return m

    // ... 更多 case
    }
    return nil
}
```

**示例 JSON 输出**:
```json
{
  "type": "File",
  "pos": "test.yoru:1:1",
  "package": "main",
  "imports": [],
  "decls": [
    {
      "type": "FuncDecl",
      "pos": "test.yoru:3:1",
      "name": "add",
      "params": [
        {"name": "a", "type": "int"},
        {"name": "b", "type": "int"}
      ],
      "result": {"type": "Name", "value": "int"},
      "body": {
        "type": "BlockStmt",
        "stmts": [
          {
            "type": "ReturnStmt",
            "result": {
              "type": "Operation",
              "op": "+",
              "x": {"type": "Name", "value": "a"},
              "y": {"type": "Name", "value": "b"}
            }
          }
        ]
      }
    }
  ]
}
```

### 6.2 CLI 集成

**更新**: `cmd/yoruc/main.go`

```go
var (
    emitAST   = flag.Bool("emit-ast", false, "Output AST")
    astFormat = flag.String("ast-format", "text", "AST output format (text or json)")
    noASI     = flag.Bool("no-asi", false, "Disable automatic semicolon insertion")
)

func runEmitAST(filename string) int {
    f, err := os.Open(filename)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        return 1
    }
    defer f.Close()

    var errs []string
    errh := func(pos syntax.Pos, msg string) {
        errs = append(errs, fmt.Sprintf("%s: %s", pos, msg))
    }

    p := syntax.NewParser(filename, f, errh)
    p.SetASIEnabled(!*noASI)
    ast := p.Parse()

    // 先打印错误
    for _, e := range errs {
        fmt.Fprintln(os.Stderr, e)
    }

    // 输出 AST
    switch *astFormat {
    case "json":
        if err := syntax.FprintJSON(os.Stdout, ast); err != nil {
            fmt.Fprintf(os.Stderr, "error: %v\n", err)
            return 1
        }
    default:
        syntax.Fprint(os.Stdout, ast)
    }

    if len(errs) > 0 {
        return 1
    }
    return 0
}
```

### 6.3 使用示例

```bash
# 文本格式输出（默认）
yoruc -emit-ast test.yoru

# JSON 格式输出
yoruc -emit-ast -ast-format=json test.yoru

# 禁用 ASI 后解析
yoruc -emit-ast -no-asi test.yoru
```

---

## 7. AST 遍历器

**文件**: `internal/syntax/walk.go`

```go
package syntax

// Visitor 在遍历过程中为每个节点调用
type Visitor func(node Node) bool

// Walk 以深度优先顺序遍历 AST
// 如果 visitor 返回 false，则不访问子节点
func Walk(node Node, v Visitor) {
    if node == nil || !v(node) {
        return
    }

    switch n := node.(type) {
    case *File:
        Walk(n.PkgName, v)
        for _, imp := range n.Imports {
            Walk(imp, v)
        }
        for _, d := range n.Decls {
            Walk(d, v)
        }

    case *FuncDecl:
        if n.Recv != nil {
            Walk(n.Recv, v)
        }
        Walk(n.Name, v)
        for _, p := range n.Params {
            Walk(p, v)
        }
        if n.Result != nil {
            Walk(n.Result, v)
        }
        if n.Body != nil {
            Walk(n.Body, v)
        }

    case *TypeDecl:
        Walk(n.Name, v)
        Walk(n.Type, v)

    case *ImportDecl:
        if n.Path != nil {
            Walk(n.Path, v)
        }

    case *VarDecl:
        Walk(n.Name, v)
        if n.Type != nil {
            Walk(n.Type, v)
        }
        if n.Value != nil {
            Walk(n.Value, v)
        }

    case *BlockStmt:
        for _, s := range n.Stmts {
            Walk(s, v)
        }

    case *IfStmt:
        Walk(n.Cond, v)
        Walk(n.Then, v)
        if n.Else != nil {
            Walk(n.Else, v)
        }

    case *ForStmt:
        if n.Cond != nil {
            Walk(n.Cond, v)
        }
        Walk(n.Body, v)

    case *ReturnStmt:
        if n.Result != nil {
            Walk(n.Result, v)
        }

    case *AssignStmt:
        for _, e := range n.LHS {
            Walk(e, v)
        }
        for _, e := range n.RHS {
            Walk(e, v)
        }

    case *ExprStmt:
        Walk(n.X, v)

    case *DeclStmt:
        Walk(n.Decl, v)

    case *Operation:
        Walk(n.X, v)
        if n.Y != nil {
            Walk(n.Y, v)
        }

    case *CallExpr:
        Walk(n.Fun, v)
        for _, a := range n.Args {
            Walk(a, v)
        }

    case *IndexExpr:
        Walk(n.X, v)
        Walk(n.Index, v)

    case *SelectorExpr:
        Walk(n.X, v)
        Walk(n.Sel, v)

    case *StructType:
        for _, f := range n.Fields {
            Walk(f, v)
        }

    case *ArrayType:
        Walk(n.Len, v)
        Walk(n.Elem, v)

    case *PointerType:
        Walk(n.Base, v)

    case *RefType:
        Walk(n.Base, v)

    case *Field:
        if n.Name != nil {
            Walk(n.Name, v)
        }
        Walk(n.Type, v)

    case *NewExpr:
        Walk(n.Type, v)

    case *CompositeLit:
        if n.Type != nil {
            Walk(n.Type, v)
        }
        for _, e := range n.Elems {
            Walk(e, v)
        }

    case *KeyValueExpr:
        Walk(n.Key, v)
        Walk(n.Value, v)

    case *ParenExpr:
        Walk(n.X, v)

    // 叶子节点：Name, BasicLit, EmptyStmt, BranchStmt
    }
}

// Inspect 遍历 AST 并为每个节点调用 f
// Walk 的便捷包装
func Inspect(node Node, f func(Node) bool) {
    Walk(node, Visitor(f))
}
```

---

## 8. 文件清单

Phase 2 完成后的文件结构：

```
internal/syntax/
├── token.go           # Token 定义（已有）
├── pos.go             # 位置追踪（已有）
├── source.go          # 字符读取器（已有）
├── scanner.go         # Scanner 主实现（已有）
├── scanner_test.go    # Scanner 测试（已有）
├── nodes.go           # AST 节点定义（新建）
├── parser.go          # Parser 主实现（新建）
├── parser_test.go     # Parser 测试（新建）
├── print.go           # 文本 AST 打印（新建）
├── json.go            # JSON AST 打印（新建）
├── walk.go            # AST 遍历器（新建）
└── testdata/
    ├── tokens.golden           # Token 测试（已有）
    ├── parse_decl.yoru         # 声明测试
    ├── parse_decl.ast.golden
    ├── parse_expr.yoru         # 表达式测试
    ├── parse_expr.ast.golden
    ├── parse_stmt.yoru         # 语句测试
    ├── parse_stmt.ast.golden
    ├── parse_complete.yoru     # 完整程序测试
    ├── parse_complete.ast.golden
    ├── parse_error_*.yoru      # 错误测试
    └── fuzz/
        ├── FuzzScanner/        # Scanner Fuzz 语料（已有）
        └── FuzzParse/          # Parser Fuzz 语料（新建）
```

---

## 9. 完整验收标准

### Phase 2 总体验收

| 项目 | 标准 | 验证方法 |
|------|------|----------|
| AST 节点 | 所有 Yoru 语法结构有对应节点 | 代码审查 |
| Package 解析 | `package main` 正确解析 | 单元测试 |
| Import 解析 | `import "path"` 解析（不语义化） | 单元测试 |
| Type 声明 | `type T struct {}`, `type T = U` | 单元测试 |
| Func 声明 | 函数、方法、参数、返回类型 | 单元测试 |
| Var/短声明 | `var x T = v`, `x := v` | 单元测试 |
| If 语句 | `if cond {} else {}` | 单元测试 |
| For 语句 | 只有 `for cond {}` | 单元测试 |
| Return 语句 | `return` 和 `return expr` | 单元测试 |
| Branch 语句 | `break`, `continue` | 单元测试 |
| 表达式 | 二元、一元、后缀 | 单元测试 |
| Pratt 解析 | 优先级正确 | 优先级测试 |
| 错误恢复 | 单文件报告多个错误，且不 panic | 错误测试 |
| 位置追踪 | 行:列准确 | 位置测试 |
| `-emit-ast` | 文本和 JSON 格式 | 手动验证 |
| Golden 测试 | 全部通过 | `go test` |
| 错误测试 | 20+ 用例 | `go test` |
| Fuzz 测试 | 5-10 分钟稳定 | `go test -fuzz` |

### 示例验证程序

```yoru
// internal/syntax/testdata/parse_complete.yoru
package main

import "fmt"

// 结构体类型
type Point struct {
    x int
    y float
}

// 类型别名
type Number = int

// 方法
func (p Point) Area() int {
    return p.x * p.y
}

// 函数
func add(a int, b int) int {
    return a + b
}

// 主函数
func main() {
    // 变量声明
    var p Point
    p.x = 10
    p.y = 3.14

    // 短声明
    result := add(1, 2)

    // 数组
    var arr [5]int
    arr[0] = 1

    // ref 和 new
    var r ref Point = new(Point)
    r.x = 20

    // if/else
    if p.x > 0 {
        println("positive")
    } else {
        println("non-positive")
    }

    // for 循环
    var i int = 0
    for i < 10 {
        println(i)
        i = i + 1
    }

    // 方法调用
    println(p.Area())

    // 表达式
    var x int = 1 + 2 * 3
    var b bool = x > 5 && x < 10

    // 指针
    var ptr *int = &i
    println(*ptr)
}
```

该文件应能被 Parser 正确解析，生成完整的 AST。

---

## 10. 实现建议

### 10.1 开发顺序

1. **第 1 周（Phase 2.1）**：
   - 实现 `nodes.go` 所有节点类型
   - 实现 `parser.go` 骨架（token 导航方法）
   - 实现 `Parse()` 入口点和 package 声明解析
   - 第一个测试：解析 `package main`

2. **第 2-2.5 周（Phase 2.2）**：
   - Type 声明（struct, alias）
   - Func 声明（接收者、参数、结果）
   - Var 声明
   - Import 声明
   - 每种声明类型的测试

3. **第 3-4 周（Phase 2.3）**：
   - 语句解析（if, for, return, block, branch）
   - 使用 Pratt 算法的表达式解析
   - 用测试用例验证优先级
   - 简单语句（赋值、表达式）

4. **第 5 周（Phase 2.4）**：
   - 错误恢复实现
   - 20+ 错误测试用例
   - Golden 测试基础设施
   - Fuzz 测试
   - CLI `-emit-ast` 集成

### 10.2 调试技巧

```go
// 添加调试开关
var debugParser = os.Getenv("YORU_DEBUG_PARSER") != ""

func (p *Parser) next() {
    p.scanner.Next()
    p.tok = p.scanner.Token()
    p.lit = p.scanner.Literal()
    p.pos = p.scanner.Pos()

    if debugParser {
        fmt.Printf("[parser] %s %v %q\n", p.pos, p.tok, p.lit)
    }
}
```

### 10.3 常见陷阱

1. **ASI 交互**：Scanner 处理 ASI，所以 Parser 看到的是 `_Semi` token。不要在 Parser 中添加额外的分号处理。

2. **优先级错误**：测试像 `1 + 2 * 3` 和 `a && b || c` 这样的表达式，验证优先级爬升正确工作。

3. **前瞻限制**：Parser 只有 1 个 token 的前瞻。如果需要更多，重构以不同方式处理歧义。

4. **错误级联**：正确实现 `advance()` 以在错误后跳到同步点。

5. **位置追踪**：在用 `p.next()` 消费 token **之前**总是设置 `node.pos = p.pos`。

### 10.4 参考 Go 源码

```bash
# 查看 Go parser 实现
go doc cmd/compile/internal/syntax.parser

# 或直接阅读源码
# $GOROOT/src/cmd/compile/internal/syntax/parser.go
# $GOROOT/src/cmd/compile/internal/syntax/nodes.go
```

---

## 附录 A: 运算符优先级表

| 优先级 | 运算符 | 结合性 |
|--------|--------|--------|
| 1 | `\|\|` | 左 |
| 2 | `&&` | 左 |
| 3 | `==` `!=` `<` `<=` `>` `>=` | 左 |
| 4 | `+` `-` `\|` `^` | 左 |
| 5 | `*` `/` `%` `&` `<<` `>>` | 左 |
| 6（一元） | `!` `-` `*` `&` | 右 |

## 附录 B: Token 到 AST 节点映射

| Token/语法 | AST 节点 |
|------------|----------|
| `NAME` | `*Name` |
| `LITERAL` | `*BasicLit` |
| 二元运算 | `*Operation` (Y != nil) |
| 一元运算 | `*Operation` (Y == nil) |
| `fun(args)` | `*CallExpr` |
| `x[i]` | `*IndexExpr` |
| `x.f` | `*SelectorExpr` |
| `(expr)` | `*ParenExpr` |
| `new(T)` | `*NewExpr` |
| `T{...}` | `*CompositeLit` |
| `key: value` | `*KeyValueExpr` |
| `panic(args)` | `*CallExpr`（`Fun = Name{"panic"}`） |
| `*T` | `*PointerType` |
| `ref T` | `*RefType` |
| `[N]T` | `*ArrayType` |
| `struct {}` | `*StructType` |
| `package` | `*File.PkgName` |
| `import` | `*ImportDecl` |
| `type` | `*TypeDecl` |
| `var` | `*VarDecl` |
| `func` | `*FuncDecl` |
| `if` | `*IfStmt` |
| `for` | `*ForStmt` |
| `return` | `*ReturnStmt` |
| `break/continue` | `*BranchStmt` |
| `{}` | `*BlockStmt` |
| `x = y` | `*AssignStmt` |
| `x := y` | `*AssignStmt` |
| `expr;` | `*ExprStmt` |

## 附录 C: 运行测试命令

```bash
# 运行所有 parser 测试
go test ./internal/syntax/... -v -run TestParse

# 运行特定测试
go test ./internal/syntax/... -run TestParseDeclarations

# 运行 fuzz 测试（5 分钟）
go test ./internal/syntax/... -fuzz=FuzzParse -fuzztime=5m

# 更新 golden 文件
go test ./internal/syntax/... -update-golden

# 检查测试覆盖率
go test ./internal/syntax/... -coverprofile=coverage.out
go tool cover -html=coverage.out

# 使用调试输出运行
YORU_DEBUG_PARSER=1 go test ./internal/syntax/... -v -run TestParse
```

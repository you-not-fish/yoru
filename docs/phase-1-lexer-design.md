# Phase 1: 词法分析器设计与实现文档

## 概述

本文档详细描述 Yoru 编译器词法分析器（Lexer/Scanner）的设计与实现方案。词法分析器是编译器前端的第一个阶段，负责将源代码字符流转换为 Token 流。

### 目标

1. 实现一个高效、准确的词法分析器
2. 支持完整的 Yoru 语言词法规范
3. 提供准确的位置追踪（行:列）
4. 实现自动分号插入（ASI）
5. 提供良好的错误诊断

### 参考实现

主要参考 Go 编译器的词法分析器：
- `cmd/compile/internal/syntax/scanner.go`
- `cmd/compile/internal/syntax/tokens.go`
- `cmd/compile/internal/syntax/source.go`

---

## 1. 子阶段划分

根据任务量和依赖关系，Phase 1 划分为 4 个子阶段：

| 子阶段 | 内容 | 预计时间 | 依赖 |
|--------|------|----------|------|
| 1.1 | Token 定义 + 基础设施 | 1 周 | - |
| 1.2 | Scanner 核心实现 | 1.5 周 | 1.1 |
| 1.3 | 字符串/注释 + ASI | 1 周 | 1.2 |
| 1.4 | 测试完善 + Fuzz | 0.5 周 | 1.3 |

---

## 2. Phase 1.1: Token 定义 + 基础设施

### 2.1 Token 类型定义

**文件**: `internal/syntax/token.go`

```go
package syntax

// Token 表示词法单元类型
type Token uint

const (
    // 特殊 token
    _EOF Token = iota
    _Error

    // 字面量
    _Name    // 标识符
    _Literal // 字面量（整数、浮点、字符串）

    // 运算符和分隔符（按优先级分组）
    _Operator // 占位，具体见下方
    _AssignOp // 赋值运算符
    _Semi     // 分号（显式或隐式）

    // ... 详细定义见下方
    tokenCount
)
```

#### 2.1.1 完整 Token 列表

```go
const (
    // === 特殊 Token ===
    _EOF   Token = iota  // 文件结束
    _Error               // 词法错误

    // === 字面量 ===
    _Name    // 标识符: foo, bar, Rectangle
    _Literal // 字面量值（配合 LitKind 使用）

    // === 运算符（按优先级从低到高）===
    // 赋值
    _Assign    // =
    _Define    // :=

    // 逻辑运算符
    _OrOr      // ||
    _AndAnd    // &&

    // 比较运算符
    _Eql       // ==
    _Neq       // !=
    _Lss       // <
    _Leq       // <=
    _Gtr       // >
    _Geq       // >=

    // 算术运算符
    _Add       // +
    _Sub       // -
    _Or        // |
    _Xor       // ^

    _Mul       // *
    _Div       // /
    _Rem       // %
    _And       // &
    _Shl       // <<
    _Shr       // >>

    // 一元运算符
    _Not       // !

    // === 分隔符 ===
    _Lparen    // (
    _Rparen    // )
    _Lbrack    // [
    _Rbrack    // ]
    _Lbrace    // {
    _Rbrace    // }
    _Comma     // ,
    _Semi      // ;
    _Colon     // :
    _Dot       // .

    // === 关键字 ===
    _Break
    _Continue
    _Else
    _For
    _Func
    _If
    _Import
    _New
    _Package
    _Panic
    _Ref
    _Return
    _Struct
    _Type
    _Var

    tokenCount
)
```

#### 2.1.2 字面量种类

```go
// LitKind 表示字面量的具体类型
type LitKind uint8

const (
    IntLit LitKind = iota   // 123, 0x1F, 0o77, 0b1010
    FloatLit                // 3.14, 1e10, 2.5e-3
    StringLit               // "hello", "line\n"
)
```

#### 2.1.3 Token 方法

```go
// String 返回 token 的可读名称
func (t Token) String() string

// Precedence 返回二元运算符的优先级，非运算符返回 0
func (t Token) Precedence() int

// IsKeyword 判断是否为关键字
func (t Token) IsKeyword() bool

// IsLiteral 判断是否为字面量
func (t Token) IsLiteral() bool

// IsOperator 判断是否为运算符
func (t Token) IsOperator() bool

// IsEOF 判断是否为文件结束 token
func (t Token) IsEOF() bool
```

### 2.2 关键字映射

```go
var keywords = map[string]Token{
    "break":    _Break,
    "continue": _Continue,
    "else":     _Else,
    "for":      _For,
    "func":     _Func,
    "if":       _If,
    "import":   _Import,
    "new":      _New,
    "package":  _Package,
    "panic":    _Panic,
    "ref":      _Ref,
    "return":   _Return,
    "struct":   _Struct,
    "type":     _Type,
    "var":      _Var,
}
```

**设计决策**：
- 预声明标识符 `int`/`float`/`bool`/`string`/`true`/`false`/`nil`/`println` 在词法阶段统一扫描为 `_Name`
- 这些名字在 Phase 3 的 Universe 阶段再绑定为类型、常量或函数，保持词法层简单

### 2.3 位置追踪

**文件**: `internal/syntax/pos.go`

```go
package syntax

// Pos 表示源代码中的位置
type Pos struct {
    filename string
    line     uint32  // 1-based
    col      uint32  // 1-based, byte offset
}

// NewPos 创建新的位置
func NewPos(filename string, line, col uint32) Pos

// String 返回 "filename:line:col" 格式
func (p Pos) String() string

// IsValid 检查位置是否有效
func (p Pos) IsValid() bool

// Line 返回行号
func (p Pos) Line() uint32

// Col 返回列号
func (p Pos) Col() uint32

// Filename 返回文件名
func (p Pos) Filename() string
```

**设计决策**：
- 使用 `uint32` 而非 `int` 节省内存（每个 AST 节点都有 Pos）
- 行号和列号都是 1-based（符合用户习惯）
- 列号是字节偏移，而非字符偏移（UTF-8 编码下更简单）

### 2.4 源文件读取器

**文件**: `internal/syntax/source.go`

```go
package syntax

import "io"

// source 是带位置追踪的字符读取器
type source struct {
    // 输入
    src io.Reader
    buf []byte      // 读取缓冲区

    // 位置
    filename string
    line     uint32
    col      uint32

    // 当前状态
    ch   rune    // 当前字符，-1 表示 EOF
    offs int     // 当前字符在 buf 中的偏移

    // 错误处理
    errh func(line, col uint32, msg string)
}

// newSource 创建新的 source
func newSource(filename string, src io.Reader, errh func(line, col uint32, msg string)) *source

// nextch 读取下一个字符，更新位置
func (s *source) nextch()

// pos 返回当前位置
func (s *source) pos() Pos

// error 报告词法错误
func (s *source) error(msg string)
```

### 2.5 Phase 1.1 验收标准

- [ ] `token.go`: 完整的 Token 定义，包含所有关键字和运算符
- [ ] `token.go`: Token.String() 方法正确返回名称
- [ ] `token.go`: 关键字映射表完整
- [ ] `pos.go`: Pos 类型实现完整
- [ ] `source.go`: 基础字符读取功能
- [ ] 单元测试: Token.String() 测试
- [ ] 单元测试: 关键字查找测试

---

## 3. Phase 1.2: Scanner 核心实现

### 3.1 Scanner 结构

**文件**: `internal/syntax/scanner.go`

```go
package syntax

// Scanner 词法分析器
type Scanner struct {
    source           // 嵌入 source

    // 当前 token 信息
    tok     Token    // token 类型
    lit     string   // token 字面量（标识符名、数字、字符串内容）
    kind    LitKind  // 字面量种类（仅当 tok == _Literal 时有效）
    tokPos  Pos      // token 起始位置

    // ASI 状态
    nlsemi  bool     // 是否在换行处插入分号

    // 配置
    asiEnabled bool  // 是否启用 ASI（默认 true，可通过 -no-asi 关闭）
}

// NewScanner 创建新的 Scanner
func NewScanner(filename string, src io.Reader, errh func(line, col uint32, msg string)) *Scanner

// SetASIEnabled 动态设置 ASI 开关
func (s *Scanner) SetASIEnabled(enabled bool)

// Next 读取下一个 token
func (s *Scanner) Next()

// Token 返回当前 token 类型
func (s *Scanner) Token() Token

// Literal 返回当前 token 的字面量
func (s *Scanner) Literal() string

// LitKind 返回当前字面量的种类
func (s *Scanner) LitKind() LitKind

// Pos 返回当前 token 的位置
func (s *Scanner) Pos() Pos
```

### 3.2 核心扫描逻辑

```go
func (s *Scanner) Next() {
    // 1. 检查是否需要在换行处插入分号
    nlsemi := s.nlsemi
    s.nlsemi = false

redo:
    // 2. 跳过空白字符（不含 '\n'）
    s.skipWhitespace()

    // 3. ASI: 在换行或 EOF 前按规则补分号
    if s.asiEnabled && nlsemi && (s.ch == '\n' || s.ch < 0) {
        s.tokPos = s.pos()
        s.tok = _Semi
        if s.ch == '\n' {
            s.lit = "newline"
            s.nextch()
        } else {
            s.lit = "EOF"
        }
        return
    }

    // 4. 不需要插入分号时，直接吃掉换行
    if s.ch == '\n' {
        s.nextch()
        goto redo
    }

    // 5. 记录 token 起始位置
    s.tokPos = s.pos()

    // 6. 根据当前字符判断 token 类型
    switch {
    case s.ch < 0:
        s.tok = _EOF

    case isLetter(s.ch):
        s.scanIdent()

    case isDigit(s.ch):
        s.scanNumber()

    case s.ch == '"':
        s.scanString()

    case isOperatorStart(s.ch):
        if s.scanOperator() { // true 表示跳过了注释，需要重扫
            goto redo
        }

    default:
        s.error(fmt.Sprintf("unexpected character %q", s.ch))
        s.nextch()
        goto redo
    }

    // 7. 设置 nlsemi 标志
    s.nlsemi = s.shouldInsertSemi()
}
```

### 3.3 标识符扫描

```go
func (s *Scanner) scanIdent() {
    s.startLit()
    for isLetter(s.ch) || isDigit(s.ch) {
        s.nextch()
    }
    s.lit = s.stopLit()

    // 检查是否是关键字
    if tok, ok := keywords[s.lit]; ok {
        s.tok = tok
    } else {
        s.tok = _Name
    }
}
```

### 3.4 数字扫描

```go
func (s *Scanner) scanNumber() {
    s.startLit()
    s.kind = IntLit

    // 处理进制前缀
    if s.ch == '0' {
        s.nextch()
        switch lower(s.ch) {
        case 'x':
            s.nextch()
            s.scanHexDigits()
        case 'o':
            s.nextch()
            s.scanOctalDigits()
        case 'b':
            s.nextch()
            s.scanBinaryDigits()
        default:
            // 十进制或浮点数，可能以 0 开头
            if isDigit(s.ch) || s.ch == '.' {
                s.scanDecimalOrFloat()
            }
        }
    } else {
        s.scanDecimalOrFloat()
    }

    s.lit = s.stopLit()
    s.tok = _Literal
}

func (s *Scanner) scanDecimalOrFloat() {
    // 扫描整数部分
    for isDigit(s.ch) {
        s.nextch()
    }

    // 检查小数点
    if s.ch == '.' {
        s.kind = FloatLit
        s.nextch()
        for isDigit(s.ch) {
            s.nextch()
        }
    }

    // 检查指数部分
    if lower(s.ch) == 'e' {
        s.kind = FloatLit
        s.nextch()
        if s.ch == '+' || s.ch == '-' {
            s.nextch()
        }
        if !isDigit(s.ch) {
            s.error("exponent has no digits")
        }
        for isDigit(s.ch) {
            s.nextch()
        }
    }
}
```

### 3.5 运算符扫描

```go
// scanOperator 扫描运算符，返回 true 表示应立即重扫（如跳过了注释）
func (s *Scanner) scanOperator() bool {
    ch := s.ch
    s.nextch()

    switch ch {
    case '+':
        s.tok = _Add
    case '-':
        s.tok = _Sub
    case '*':
        s.tok = _Mul
    case '/':
        if s.ch == '/' {
            // 行注释
            s.skipLineComment()
            return true
        }
        s.tok = _Div
    case '%':
        s.tok = _Rem
    case '&':
        if s.ch == '&' {
            s.nextch()
            s.tok = _AndAnd
        } else {
            s.tok = _And
        }
    case '|':
        if s.ch == '|' {
            s.nextch()
            s.tok = _OrOr
        } else {
            s.tok = _Or
        }
    case '^':
        s.tok = _Xor
    case '<':
        switch s.ch {
        case '=':
            s.nextch()
            s.tok = _Leq
        case '<':
            s.nextch()
            s.tok = _Shl
        default:
            s.tok = _Lss
        }
    case '>':
        switch s.ch {
        case '=':
            s.nextch()
            s.tok = _Geq
        case '>':
            s.nextch()
            s.tok = _Shr
        default:
            s.tok = _Gtr
        }
    case '=':
        if s.ch == '=' {
            s.nextch()
            s.tok = _Eql
        } else {
            s.tok = _Assign
        }
    case '!':
        if s.ch == '=' {
            s.nextch()
            s.tok = _Neq
        } else {
            s.tok = _Not
        }
    case ':':
        if s.ch == '=' {
            s.nextch()
            s.tok = _Define
        } else {
            s.tok = _Colon
        }
    // 分隔符
    case '(':
        s.tok = _Lparen
    case ')':
        s.tok = _Rparen
    case '[':
        s.tok = _Lbrack
    case ']':
        s.tok = _Rbrack
    case '{':
        s.tok = _Lbrace
    case '}':
        s.tok = _Rbrace
    case ',':
        s.tok = _Comma
    case ';':
        s.tok = _Semi
    case '.':
        s.tok = _Dot
    }
    return false
}
```

### 3.6 Phase 1.2 验收标准

- [ ] 标识符扫描正确
- [ ] 整数字面量扫描（十进制、十六进制、八进制、二进制）
- [ ] 浮点数字面量扫描（包括指数形式）
- [ ] 所有运算符和分隔符识别正确
- [ ] 关键字识别正确
- [ ] 位置追踪准确
- [ ] 单元测试覆盖所有 token 类型

---

## 4. Phase 1.3: 字符串/注释处理 + ASI

### 4.1 字符串字面量扫描

```go
func (s *Scanner) scanString() {
    s.nextch() // 跳过开头的 "
    var b strings.Builder

    for {
        switch {
        case s.ch == '"':
            s.nextch()
            s.lit = b.String()
            s.tok = _Literal
            s.kind = StringLit
            return

        case s.ch == '\\':
            if r, ok := s.scanEscape(); ok {
                b.WriteRune(r)
            }

        case s.ch == '\n' || s.ch < 0:
            s.error("string not terminated")
            s.lit = b.String()
            s.tok = _Literal
            s.kind = StringLit
            return

        default:
            b.WriteRune(s.ch)
            s.nextch()
        }
    }
}

// scanEscape 扫描并解码转义序列，返回解码后的 rune
func (s *Scanner) scanEscape() (rune, bool) {
    s.nextch() // 跳过 \

    switch s.ch {
    case 'n':
        s.nextch()
        return '\n', true
    case 't':
        s.nextch()
        return '\t', true
    case 'r':
        s.nextch()
        return '\r', true
    case '\\':
        s.nextch()
        return '\\', true
    case '"':
        s.nextch()
        return '"', true
    case '0':
        s.nextch()
        return 0, true
    case 'x':
        s.nextch()
        b, ok := s.scanHexEscape(2) // 返回 [0,255]
        return rune(b), ok
    default:
        s.error(fmt.Sprintf("unknown escape sequence: \\%c", s.ch))
        s.nextch()
        return 0, false
    }
}
```

**设计决策**：
- `Literal()` 对字符串 token 返回“解码后”的内容（例如源码 `"a\nb"` 对应字面量 `a`+换行+`b`）

#### 4.1.1 支持的转义序列

| 转义 | 含义 |
|------|------|
| `\n` | 换行 |
| `\t` | 制表符 |
| `\r` | 回车 |
| `\\` | 反斜杠 |
| `\"` | 双引号 |
| `\0` | 空字符 |
| `\xNN` | 十六进制字节 |

### 4.2 注释处理

```go
func (s *Scanner) skipLineComment() {
    // 已经读取了 //，继续读取直到行尾
    for s.ch != '\n' && s.ch >= 0 {
        s.nextch()
    }
}
```

**设计决策**：
- 只支持行注释 `//`，不支持块注释 `/* */`
- 注释被完全跳过，不产生 token
- 注释不影响 ASI（换行仍然可能触发分号插入）

### 4.3 自动分号插入（ASI）

Go 的 ASI 规则：在换行符或 EOF 前如果当前 token 是以下之一，则自动插入分号：
- 标识符
- 字面量（整数、浮点、字符串）
- 关键字：`break`, `continue`, `return`
- 分隔符：`)`, `]`, `}`

```go
// shouldInsertSemi 判断当前 token 后是否应该在换行处插入分号
func (s *Scanner) shouldInsertSemi() bool {
    switch s.tok {
    case _Name, _Literal:
        return true
    case _Break, _Continue, _Return:
        return true
    case _Rparen, _Rbrack, _Rbrace:
        return true
    }
    return false
}
```

**使用方式**：

```bash
# 启用 ASI（默认）
yoruc foo.yoru

# 禁用 ASI（要求显式分号）
yoruc -no-asi foo.yoru
```

```go
// cmd/yoruc/main.go
s := syntax.NewScanner(filename, src, errh)
s.SetASIEnabled(!*noASI)
```

### 4.4 Phase 1.3 验收标准

- [ ] 字符串字面量扫描正确
- [ ] 所有转义序列处理正确
- [ ] 非法转义序列产生错误
- [ ] 未终止字符串产生错误
- [ ] 行注释正确跳过
- [ ] ASI 在正确位置插入分号
- [ ] `-no-asi` 选项生效

---

## 5. Phase 1.4: 测试完善 + Fuzz

### 5.1 单元测试

**文件**: `internal/syntax/scanner_test.go`

```go
package syntax

import (
    "strings"
    "testing"
)

func TestScanTokens(t *testing.T) {
    tests := []struct {
        name   string
        src    string
        tokens []Token
        lits   []string
    }{
        // 标识符
        {"ident", "foo", []Token{_Name}, []string{"foo"}},
        {"ident_underscore", "_bar", []Token{_Name}, []string{"_bar"}},

        // 整数
        {"int_dec", "123", []Token{_Literal}, []string{"123"}},
        {"int_hex", "0x1F", []Token{_Literal}, []string{"0x1F"}},
        {"int_oct", "0o77", []Token{_Literal}, []string{"0o77"}},
        {"int_bin", "0b1010", []Token{_Literal}, []string{"0b1010"}},

        // 浮点数
        {"float_simple", "3.14", []Token{_Literal}, []string{"3.14"}},
        {"float_exp", "1e10", []Token{_Literal}, []string{"1e10"}},
        {"float_exp_neg", "2.5e-3", []Token{_Literal}, []string{"2.5e-3"}},

        // 字符串
        {"string_simple", `"hello"`, []Token{_Literal}, []string{"hello"}},
        {"string_escape", `"a\nb"`, []Token{_Literal}, []string{"a\nb"}},

        // 运算符
        {"op_add", "+", []Token{_Add}, nil},
        {"op_eq", "==", []Token{_Eql}, nil},
        {"op_assign", "=", []Token{_Assign}, nil},
        {"op_define", ":=", []Token{_Define}, nil},
        {"op_andand", "&&", []Token{_AndAnd}, nil},
        {"op_oror", "||", []Token{_OrOr}, nil},

        // 关键字
        {"kw_func", "func", []Token{_Func}, nil},
        {"kw_if", "if", []Token{_If}, nil},
        {"kw_for", "for", []Token{_For}, nil},
        {"kw_return", "return", []Token{_Return}, nil},

        // 复合
        {"expr", "1 + 2", []Token{_Literal, _Add, _Literal}, []string{"1", "", "2"}},
        {"funcall", "foo()", []Token{_Name, _Lparen, _Rparen}, []string{"foo", "", ""}},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            s := NewScanner("test", strings.NewReader(tt.src), nil)
            for i, want := range tt.tokens {
                s.Next()
                if s.Token() != want {
                    t.Errorf("token %d: got %v, want %v", i, s.Token(), want)
                }
                if tt.lits != nil && tt.lits[i] != "" {
                    if s.Literal() != tt.lits[i] {
                        t.Errorf("lit %d: got %q, want %q", i, s.Literal(), tt.lits[i])
                    }
                }
            }
            s.Next()
            if s.Token() != _EOF {
                t.Errorf("expected EOF, got %v", s.Token())
            }
        })
    }
}
```

### 5.2 ASI 测试

```go
func TestASI(t *testing.T) {
    tests := []struct {
        name   string
        src    string
        tokens []Token
    }{
        // 标识符后换行插入分号
        {
            "ident_newline",
            "foo\nbar",
            []Token{_Name, _Semi, _Name},
        },
        // return 后换行插入分号
        {
            "return_newline",
            "return\n1",
            []Token{_Return, _Semi, _Literal},
        },
        // ) 后换行插入分号
        {
            "rparen_newline",
            "foo()\nbar",
            []Token{_Name, _Lparen, _Rparen, _Semi, _Name},
        },
        // } 后换行插入分号
        {
            "rbrace_newline",
            "{\n}\nfoo",
            []Token{_Lbrace, _Rbrace, _Semi, _Name},
        },
        // + 后换行不插入分号
        {
            "op_newline",
            "1 +\n2",
            []Token{_Literal, _Add, _Literal},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            s := NewScanner("test", strings.NewReader(tt.src), nil)
            for i, want := range tt.tokens {
                s.Next()
                if s.Token() != want {
                    t.Errorf("token %d: got %v, want %v", i, s.Token(), want)
                }
            }
        })
    }
}
```

### 5.3 错误测试

```go
func TestScanErrors(t *testing.T) {
    tests := []struct {
        name    string
        src     string
        wantErr string
    }{
        {"unterminated_string", `"hello`, "string not terminated"},
        {"bad_escape", `"\q"`, "unknown escape sequence"},
        {"bad_hex", "0xGG", "invalid hex digit"},
        {"bad_binary", "0b123", "invalid binary digit"},
        {"empty_exponent", "1e", "exponent has no digits"},
        {"bad_char", "@", "unexpected character"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var errMsg string
            errh := func(line, col uint32, msg string) {
                errMsg = msg
            }
            s := NewScanner("test", strings.NewReader(tt.src), errh)
            for {
                s.Next()
                if s.Token() == _EOF {
                    break
                }
            }
            if errMsg == "" || !strings.Contains(errMsg, tt.wantErr) {
                t.Errorf("expected error containing %q, got %q", tt.wantErr, errMsg)
            }
        })
    }
}
```

### 5.4 位置追踪测试

```go
func TestPosition(t *testing.T) {
    src := `package main

func foo() {
    x := 123
}`

    expected := []struct {
        tok  Token
        line uint32
        col  uint32
    }{
        {_Package, 1, 1},
        {_Name, 1, 9},     // main
        {_Semi, 1, 13},    // ASI
        {_Func, 3, 1},
        {_Name, 3, 6},     // foo
        {_Lparen, 3, 9},
        {_Rparen, 3, 10},
        {_Lbrace, 3, 12},
        {_Name, 4, 5},     // x
        {_Define, 4, 7},
        {_Literal, 4, 10}, // 123
        {_Semi, 4, 13},    // ASI
        {_Rbrace, 5, 1},
    }

    s := NewScanner("test.yoru", strings.NewReader(src), nil)
    for i, exp := range expected {
        s.Next()
        pos := s.Pos()
        if pos.Line() != exp.line || pos.Col() != exp.col {
            t.Errorf("token %d (%v): pos = %d:%d, want %d:%d",
                i, s.Token(), pos.Line(), pos.Col(), exp.line, exp.col)
        }
    }
}
```

### 5.5 Fuzz 测试

```go
func FuzzScanner(f *testing.F) {
    // 种子语料
    seeds := []string{
        "package main",
        "func foo() { return 123 }",
        `var s string = "hello\nworld"`,
        "x := 0x1F + 0b1010",
        "if a && b || c { }",
    }
    for _, s := range seeds {
        f.Add(s)
    }

    f.Fuzz(func(t *testing.T, src string) {
        errh := func(line, col uint32, msg string) {
            // 错误是可接受的，只要不 panic
        }
        s := NewScanner("fuzz", strings.NewReader(src), errh)
        for i := 0; i < 10000; i++ { // 防止无限循环
            s.Next()
            if s.Token() == _EOF {
                break
            }
        }
    })
}
```

### 5.6 Golden 测试

**文件**: `internal/syntax/testdata/tokens.golden`

```
# Input: "package main\n\nfunc main() {\n\tprintln(123)\n}"

1:1     PACKAGE "package"
1:9     NAME    "main"
1:13    SEMI    "newline"
3:1     FUNC    "func"
3:6     NAME    "main"
3:10    LPAREN  "("
3:11    RPAREN  ")"
3:13    LBRACE  "{"
4:2     NAME    "println"
4:9     LPAREN  "("
4:10    LITERAL "123" (int)
4:13    RPAREN  ")"
4:14    SEMI    "newline"
5:1     RBRACE  "}"
5:2     SEMI    "newline"
5:2     EOF     ""
```

### 5.7 Phase 1.4 验收标准

- [ ] 100+ 单元测试用例通过
- [ ] ASI 测试覆盖所有规则
- [ ] 错误测试覆盖所有错误情况
- [ ] 位置追踪测试验证准确性
- [ ] Fuzz 测试运行 5-10 分钟不崩溃
- [ ] Golden 测试与预期输出匹配

---

## 6. 可观测性设计

### 6.1 `-emit-tokens` 输出格式

```bash
$ yoruc -emit-tokens test.yoru
```

输出格式：

```
<line>:<col>  <TOKEN>  <literal>  [(<kind>)]
```

示例：

```
1:1     PACKAGE "package"
1:9     NAME    "main"
1:13    SEMI    "newline"
3:1     FUNC    "func"
3:6     NAME    "foo"
...
```

### 6.2 实现

```go
// cmd/yoruc/main.go

func emitTokens(filename string) error {
    f, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer f.Close()

    errh := func(line, col uint32, msg string) {
        fmt.Fprintf(os.Stderr, "%s:%d:%d: %s\n", filename, line, col, msg)
    }

    s := syntax.NewScanner(filename, f, errh)
    for {
        s.Next()
        pos := s.Pos()
        tok := s.Token()

        // 格式化输出
        fmt.Printf("%d:%d\t%s", pos.Line(), pos.Col(), tok)
        if s.Literal() != "" {
            fmt.Printf("\t%q", s.Literal())
        }
        if tok.IsLiteral() {
            fmt.Printf("\t(%s)", s.LitKind())
        }
        fmt.Println()

        if tok.IsEOF() {
            break
        }
    }
    return nil
}
```

### 6.3 JSON 输出格式（可选）

```bash
$ yoruc -emit-tokens -format=json test.yoru
```

```json
{
  "tokens": [
    {"line": 1, "col": 1, "token": "PACKAGE", "literal": "package"},
    {"line": 1, "col": 9, "token": "NAME", "literal": "main"},
    {"line": 1, "col": 13, "token": "SEMI", "literal": "newline"},
    ...
  ]
}
```

---

## 7. API 设计总结

### 7.1 公开 API

```go
package syntax

// Token 类型
type Token uint
func (t Token) String() string
func (t Token) Precedence() int
func (t Token) IsKeyword() bool
func (t Token) IsLiteral() bool
func (t Token) IsOperator() bool
func (t Token) IsEOF() bool

// 字面量种类
type LitKind uint8
func (k LitKind) String() string

// 位置
type Pos struct { ... }
func (p Pos) String() string
func (p Pos) Line() uint32
func (p Pos) Col() uint32
func (p Pos) Filename() string

// Scanner
type Scanner struct { ... }
func NewScanner(filename string, src io.Reader, errh func(uint32, uint32, string)) *Scanner
func (s *Scanner) SetASIEnabled(enabled bool)
func (s *Scanner) Next()
func (s *Scanner) Token() Token
func (s *Scanner) Literal() string
func (s *Scanner) LitKind() LitKind
func (s *Scanner) Pos() Pos
```

### 7.2 内部实现

```go
// source - 字符读取器（内部）
type source struct { ... }
func (s *source) nextch()
func (s *source) pos() Pos
func (s *source) error(string)

// 辅助函数（内部）
func isLetter(r rune) bool
func isDigit(r rune) bool
func lower(r rune) rune
```

---

## 8. 文件清单

Phase 1 完成后的文件结构：

```
internal/syntax/
├── token.go          # Token 定义、关键字映射
├── pos.go            # 位置追踪
├── source.go         # 字符读取器
├── scanner.go        # Scanner 主实现
├── scanner_test.go   # 单元测试
└── testdata/
    ├── tokens.golden           # Golden 测试数据
    ├── tokens_error.golden     # 错误测试数据
    ├── *.yoru                  # 测试输入文件
    └── fuzz/
        └── FuzzScanner/        # Fuzz 语料库
```

---

## 9. 完整验收标准

### Phase 1 总体验收

| 项目 | 标准 | 验证方法 |
|------|------|----------|
| Token 定义 | 所有 Yoru token 类型完整 | 代码审查 |
| 标识符 | 正确识别（含 `_` 开头） | 单元测试 |
| 整数字面量 | 支持 10/16/8/2 进制 | 单元测试 |
| 浮点字面量 | 支持小数、指数形式 | 单元测试 |
| 字符串字面量 | 支持转义序列 | 单元测试 |
| 运算符 | 所有运算符正确识别 | 单元测试 |
| 关键字 | 所有关键字正确识别 | 单元测试 |
| 位置追踪 | 行:列准确 | 位置测试 |
| ASI | 正确插入分号 | ASI 测试 |
| 错误处理 | 错误消息清晰 | 错误测试 |
| `-emit-tokens` | 输出格式正确 | 手动验证 |
| 测试覆盖 | 100+ 测试用例 | `go test` |
| Fuzz | 运行 5-10 分钟不崩溃 | `go test -fuzz` |

### 示例验证程序

```yoru
// internal/syntax/testdata/complete.yoru
package main

type Point struct {
    x int
    y float
}

func add(a, b int) int {
    return a + b
}

func main() {
    var p Point
    p.x = 10
    p.y = 3.14

    var arr [5]int
    arr[0] = 1

    var r ref Point = new(Point)
    r.x = 20

    if p.x > 0 {
        println("positive")
    } else {
        println("non-positive")
    }

    var i int = 0
    for i < 10 {
        println(i)
        i = i + 1
    }

    result := add(1, 2)
    println(result)

    // 字符串测试
    var s string = "hello\nworld"
    println(s)

    // 各种数字格式
    var dec int = 123
    var hex int = 0xFF
    var oct int = 0o77
    var bin int = 0b1010
    var f float = 1.5e-3
}
```

该文件应能被 Scanner 正确扫描，输出所有 token 及其位置。

---

## 10. 实现建议

### 10.1 开发顺序

1. 先实现 `token.go`，确保所有 token 定义完整
2. 实现 `pos.go`，这是独立模块
3. 实现 `source.go` 的基础字符读取
4. 实现 `scanner.go` 的骨架（Next() 方法）
5. 逐步添加各类 token 的扫描逻辑
6. 最后添加 ASI 支持

### 10.2 调试技巧

```go
// 在 scanner.go 中添加调试开关
var debugScanner = os.Getenv("YORU_DEBUG_SCANNER") != ""

func (s *Scanner) Next() {
    // ...
    if debugScanner {
        fmt.Printf("[scanner] %v %q at %v\n", s.tok, s.lit, s.tokPos)
    }
}
```

### 10.3 常见陷阱

1. **UTF-8 处理**：确保 `nextch()` 正确处理多字节字符
2. **EOF 处理**：使用 -1 或特殊值表示 EOF
3. **位置更新**：换行时正确重置列号
4. **ASI 时机**：在读取下一个 token 时检查，而非当前 token

### 10.4 参考 Go 源码

```bash
# 查看 Go scanner 实现
go doc cmd/compile/internal/syntax.scanner

# 或直接阅读源码
# $GOROOT/src/cmd/compile/internal/syntax/scanner.go
```

---

## 附录 A: Token 优先级表

| 优先级 | 运算符 | 结合性 |
|--------|--------|--------|
| 1 | `\|\|` | 左 |
| 2 | `&&` | 左 |
| 3 | `==` `!=` `<` `<=` `>` `>=` | 左 |
| 4 | `+` `-` `\|` `^` | 左 |
| 5 | `*` `/` `%` `&` `<<` `>>` | 左 |
| 6（一元） | `!` `-` `*` `&` | 右 |

## 附录 B: 字符分类函数

```go
func isLetter(r rune) bool {
    return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '_'
}

func isDigit(r rune) bool {
    return '0' <= r && r <= '9'
}

func isHexDigit(r rune) bool {
    return isDigit(r) || 'a' <= lower(r) && lower(r) <= 'f'
}

func isOctalDigit(r rune) bool {
    return '0' <= r && r <= '7'
}

func isBinaryDigit(r rune) bool {
    return r == '0' || r == '1'
}

func lower(r rune) rune {
    return ('a' - 'A') | r
}

func isWhitespace(r rune) bool {
    return r == ' ' || r == '\t' || r == '\r'
}

func isOperatorStart(r rune) bool {
    switch r {
    case '+', '-', '*', '/', '%', '&', '|', '^', '<', '>', '=', '!', ':',
        '(', ')', '[', ']', '{', '}', ',', ';', '.':
        return true
    }
    return false
}
```

## 附录 C: 运行测试命令

```bash
# 运行所有 scanner 测试
go test ./internal/syntax/... -v

# 运行特定测试
go test ./internal/syntax/... -run TestScanTokens

# 运行 fuzz 测试 (5 分钟)
go test ./internal/syntax/... -fuzz=FuzzScanner -fuzztime=5m

# 更新 golden 文件
go test ./internal/syntax/... -update-golden

# 检查测试覆盖率
go test ./internal/syntax/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

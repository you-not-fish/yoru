# Yoru Runtime ABI 规范

本文档定义了 Yoru 编译器与运行时之间的 ABI 契约。

## 0. 目标平台与 DataLayout（Phase 0 锁定）

> 学习期先锁定单一平台，确保前端布局与后端完全一致。

- **Target Triple**: `arm64-apple-macosx26.0.0`
- **DataLayout**: `e-m:o-i64:64-i128:128-n32:64-S128-Fn32`

**硬性规则：**
- 编译器必须把相同的 `target triple` 与 `target datalayout` 写入 LLVM Module。
- 类型布局（size/align/offsets）必须严格按该 DataLayout 计算。
- 如果切换平台，必须同步更新本文档与布局测试用例。

## 1. 内存布局

### 1.1 对象头（Object Header）

每个堆分配的对象都有一个 16 字节的头：

```
+------------------+------------------+
|   TypeDesc* (8)  |   next_mark (8)  |
+------------------+------------------+
|              data...                |
+-------------------------------------+
```

| 字段 | 大小 | 描述 |
|------|------|------|
| `type` | 8 bytes | 指向类型描述符的指针 |
| `next_mark` | 8 bytes | GC 链表指针 + 标记位 |

`next_mark` 的布局：
- bits [63:1]: 下一个对象的指针（8 字节对齐，所以 bit 0 总是 0）
- bit 0: GC 标记位

### 1.2 类型描述符（Type Descriptor）

> 采用**固定头 + offsets 指针**，便于 LLVM 生成全局常量。

```c
typedef struct TypeDesc {
    size_t size;              // 对象数据区大小（不含头）
    size_t num_ptrs;          // GC 托管引用（ref）字段数量
    const uint32_t* offsets;  // ref 字段偏移表（相对 data 起始）
} TypeDesc;
```

**说明：**
- `offsets` 只包含 **ref 字段** 的偏移；`*T` 与 `string` 内部指针不参与 GC 扫描。
- `num_ptrs == 0` 时 `offsets` 允许为 `NULL`。
- `size` 与布局必须匹配目标 DataLayout。

## 2. 基本类型表示

### 2.1 标量类型

| Yoru 类型 | LLVM 类型 | C 类型 | 大小 | 对齐 |
|-----------|-----------|--------|------|------|
| `int` | `i64` | `int64_t` | 8 | 8 |
| `float` | `double` | `double` | 8 | 8 |
| `bool` | `i8`（内存）/ `i1`（SSA） | `int8_t` | 1 | 1 |

### 2.2 字符串类型

```c
typedef struct YoruString {
    const char* ptr;  // 字符数据指针（不以 null 结尾）
    int64_t len;      // 字节长度
} YoruString;
```

LLVM 类型：`{ i8*, i64 }`

**注意**：字符串数据不以 null 结尾，长度是显式存储的。
字符串内部指针**不参与 GC 扫描**（当前仅支持字面量字符串）。

### 2.3 数组类型 `[N]T`

- LLVM 类型：`[N x T]`
- 连续内存布局
- 如果包含指针，需要在 TypeDesc 中记录所有指针偏移

### 2.4 引用类型 `ref T`（GC 托管）

- LLVM 表示：在根上使用 `i8*`，使用时按需 `bitcast` 到 `%T*`。
- 可为 `nil`。
- **必须**作为 GC root 或 heap 字段被追踪（通过 TypeDesc offsets）。

### 2.5 指针类型 `*T`（非托管）

- LLVM 类型：`T*`
- 可为 `nil`。
- **不参与 GC 扫描**，且**禁止存入 heap 对象**（详见语言规则）。

### 2.6 接口类型

```c
typedef struct YoruInterface {
    const void* itable;  // 方法表指针
    void* data;          // 具体值指针
} YoruInterface;
```

LLVM 类型：`{ i8*, i8* }`

## 3. 运行时函数

### 3.1 内存分配

```c
// 分配指定类型的对象，返回数据区指针
// size 必须与 type->size 一致（用于一致性检查）
void* rt_alloc(uint64_t size, const TypeDesc* type);
```

编译器生成的代码：
```llvm
%obj = call i8* @rt_alloc(i64 32, %TypeDesc* @MyStruct_type)
%ptr = bitcast i8* %obj to %MyStruct*
```

### 3.2 垃圾回收

```c
// 触发 GC
void rt_collect(void);

// 初始化运行时
void rt_init(void);

// 关闭运行时
void rt_shutdown(void);
```

### 3.3 错误处理

```c
// panic 并终止程序
void rt_panic(const char* msg) __attribute__((noreturn));
void rt_panic_string(YoruString msg) __attribute__((noreturn));
```

### 3.4 I/O 函数

```c
void rt_print_i64(int64_t x);
void rt_print_f64(double x);
void rt_print_bool(int8_t b);
void rt_print_string(YoruString s);
void rt_println(void);
```

### 3.5 边界检查

```c
// 检查数组边界，越界时 panic
void rt_bounds_check(int64_t index, int64_t len);
```

## 4. GC 集成（LLVM Shadow Stack）

### 4.1 函数标记

所有可能分配内存的函数必须标记 GC 策略：

```llvm
define void @foo() gc "shadow-stack" {
  ; ...
}
```

### 4.2 GC Root 声明

每个指针类型的局部变量都需要声明为 GC root：

```llvm
define void @foo() gc "shadow-stack" {
entry:
  ; 1. 分配 root slot
  %root = alloca i8*

  ; 2. 初始化为 null（重要！防止扫描到垃圾）
  store i8* null, i8** %root

  ; 3. 声明为 gcroot
  call void @llvm.gcroot(i8** %root, i8* null)

  ; 4. 使用 root slot 存储指针值
  %obj = call i8* @rt_alloc(i64 <size>, %TypeDesc* @T_type)
  store i8* %obj, i8** %root
  ; ...
}
```

### 4.3 Root 丢失问题

**关键警告**：函数调用之间的临时指针值必须立即存入 root slot。

错误示例：
```llvm
; 危险！%tmp 可能在 @g() 期间被 GC 回收
%tmp = call i8* @f()
call void @g()           ; g() 可能触发 GC
call void @h(i8* %tmp)   ; tmp 可能已被回收
```

正确示例：
```llvm
; 安全：%tmp 立即存入 root slot
%tmp = call i8* @f()
store i8* %tmp, i8** %root1  ; 立即保存
call void @g()
%tmp2 = load i8*, i8** %root1
call void @h(i8* %tmp2)
```

### 4.4 LLVM Shadow Stack 结构

LLVM 自动维护一个全局链表：

```c
extern struct StackEntry* llvm_gc_root_chain;

struct StackEntry {
    struct StackEntry* next;     // 调用者的帧
    const struct FrameMap* map;  // 静态帧描述
    void* roots[];               // root slot 地址（void**）
};

struct FrameMap {
    int32_t num_roots;  // 帧中的 root 数量
    int32_t num_meta;   // 元数据数量
    const void* meta[]; // 每个 root 的元数据
};
```

运行时通过遍历 `llvm_gc_root_chain` 找到所有活跃的 GC roots。

## 5. 程序结构

### 5.1 入口点

编译器生成一个 `yoru_main` 函数：

```llvm
define void @yoru_main() gc "shadow-stack" {
  ; 用户的 main 函数体
}
```

运行时的 `main` 函数：

```c
int main(int argc, char** argv) {
    rt_init();
    yoru_main();
    rt_shutdown();
    return 0;
}
```

### 5.2 类型描述符生成

编译器为每个用户定义的类型生成全局类型描述符：

```llvm
; struct Point { x, y int; p *Point }
@Offsets_Point = private constant [1 x i32] [i32 16]
@Point_type = constant { i64, i64, i32* } {
    i64 24,                                     ; size
    i64 1,                                      ; num_ptrs
    i32* getelementptr ([1 x i32], [1 x i32]* @Offsets_Point, i32 0, i32 0)
}
```

## 6. 构建流程

### 6.1 编译步骤

```bash
# 1. Yoru 源码 → LLVM IR
yoruc -emit-ll foo.yoru -o foo.ll

# 2. LLVM IR + Runtime → 可执行文件
clang foo.ll runtime/runtime.c -o foo
```

### 6.2 调试选项

```bash
# 启用 GC 详细输出
YORU_GC_VERBOSE=1 ./foo

# 打印运行时统计
# （程序结束时自动打印，或 panic 时打印）
```

## 7. 内置类型描述符

运行时提供以下预定义类型描述符：

```c
extern const TypeDesc rt_type_int;
extern const TypeDesc rt_type_float;
extern const TypeDesc rt_type_bool;
extern const TypeDesc rt_type_string;
```

编译器可以直接引用这些符号。

## 8. 示例

### 8.1 简单分配

Yoru 代码：
```yoru
var p ref Point = new(Point)
```

生成的 LLVM IR：
```llvm
%p.root = alloca i8*
store i8* null, i8** %p.root
call void @llvm.gcroot(i8** %p.root, i8* null)

%obj = call i8* @rt_alloc(i64 16, %TypeDesc* @Point_type)
store i8* %obj, i8** %p.root
%p = bitcast i8* %obj to %Point*
```

### 8.2 字段访问

Yoru 代码：
```yoru
p.x = 42
```

生成的 LLVM IR：
```llvm
%p.val = load i8*, i8** %p.root
%p.typed = bitcast i8* %p.val to %Point*
%x.ptr = getelementptr %Point, %Point* %p.typed, i32 0, i32 0
store i64 42, i64* %x.ptr
```

### 8.3 函数调用（保护临时值）

Yoru 代码：
```yoru
combine(make_point(), do_work())
```

生成的 LLVM IR：
```llvm
; 为 make_point() 的返回值分配 root
%tmp.root = alloca i8*
store i8* null, i8** %tmp.root
call void @llvm.gcroot(i8** %tmp.root, i8* null)

; 调用 make_point()，立即存入 root
%tmp1 = call %Point* @make_point()
%tmp1.raw = bitcast %Point* %tmp1 to i8*
store i8* %tmp1.raw, i8** %tmp.root

; 调用 do_work()（可能触发 GC）
call void @do_work()

; 从 root 重新加载
%tmp2.raw = load i8*, i8** %tmp.root
%tmp2 = bitcast i8* %tmp2.raw to %Point*

; 调用 combine()
call void @combine(%Point* %tmp2, ...)
```

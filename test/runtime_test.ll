; Yoru Runtime Linkage Test
; This file tests that the runtime can be linked and called correctly.
;
; Build:
;   clang test/runtime_test.ll runtime/runtime.c -o /tmp/runtime_test
;
; Run:
;   /tmp/runtime_test
;
; Expected output:
;   Testing Yoru Runtime...
;   42
;   3.14
;   true
;   Hello, Yoru!
;   Testing allocation...
;   Point allocated at: <address>
;   x = 10, y = 20
;   Testing GC...
;   All tests passed!

target datalayout = "e-m:o-i64:64-i128:128-n32:64-S128-Fn32"
target triple = "arm64-apple-macosx26.0.0"

; Type descriptors
%TypeDesc = type { i64, i64, i32* }

; Yoru string: { ptr, len }
%YoruString = type { i8*, i64 }

; Point struct: { x int, y int }
; No ref fields, so offsets = null
@Point_type = internal constant %TypeDesc {
    i64 16,     ; size = 16 bytes (2 x i64)
    i64 0,      ; num_ptrs = 0
    i32* null
}

; String constants
@str.testing = private unnamed_addr constant [24 x i8] c"Testing Yoru Runtime...\00"
@str.hello = private unnamed_addr constant [12 x i8] c"Hello, Yoru!"
@str.alloc = private unnamed_addr constant [22 x i8] c"Testing allocation...\00"
@str.point_at = private unnamed_addr constant [21 x i8] c"Point allocated at: \00"
@str.gc = private unnamed_addr constant [14 x i8] c"Testing GC...\00"
@str.passed = private unnamed_addr constant [18 x i8] c"All tests passed!\00"
@str.x_eq = private unnamed_addr constant [5 x i8] c"x = \00"
@str.y_eq = private unnamed_addr constant [7 x i8] c", y = \00"
@str.newline = private unnamed_addr constant [2 x i8] c"\0A\00"

; External runtime functions
declare void @rt_init()
declare void @rt_shutdown()
declare void @rt_collect()
declare i8* @rt_alloc(i64, %TypeDesc*)
declare void @rt_print_i64(i64)
declare void @rt_print_f64(double)
declare void @rt_print_bool(i8)
declare void @rt_print_string(%YoruString)
declare void @rt_println()
declare void @rt_print_stats()

; External C functions for testing
declare i32 @printf(i8*, ...)

; Point struct type
%Point = type { i64, i64 }

; Main test function
define void @yoru_main() {
entry:
    ; Print header
    %str0 = getelementptr [24 x i8], [24 x i8]* @str.testing, i32 0, i32 0
    call i32 (i8*, ...) @printf(i8* %str0)
    call void @rt_println()

    ; Test rt_print_i64
    call void @rt_print_i64(i64 42)
    call void @rt_println()

    ; Test rt_print_f64
    call void @rt_print_f64(double 3.14)
    call void @rt_println()

    ; Test rt_print_bool
    call void @rt_print_bool(i8 1)
    call void @rt_println()

    ; Test rt_print_string
    %hello_ptr = getelementptr [12 x i8], [12 x i8]* @str.hello, i32 0, i32 0
    %hello_s0 = insertvalue %YoruString undef, i8* %hello_ptr, 0
    %hello_s1 = insertvalue %YoruString %hello_s0, i64 12, 1
    call void @rt_print_string(%YoruString %hello_s1)
    call void @rt_println()

    ; Test allocation
    %str1 = getelementptr [22 x i8], [22 x i8]* @str.alloc, i32 0, i32 0
    call i32 (i8*, ...) @printf(i8* %str1)
    call void @rt_println()

    ; Allocate a Point
    %point_raw = call i8* @rt_alloc(i64 16, %TypeDesc* @Point_type)
    %point = bitcast i8* %point_raw to %Point*

    ; Print address
    %str2 = getelementptr [21 x i8], [21 x i8]* @str.point_at, i32 0, i32 0
    call i32 (i8*, ...) @printf(i8* %str2)
    %addr = ptrtoint i8* %point_raw to i64
    call void @rt_print_i64(i64 %addr)
    call void @rt_println()

    ; Set point.x = 10, point.y = 20
    %x_ptr = getelementptr %Point, %Point* %point, i32 0, i32 0
    store i64 10, i64* %x_ptr
    %y_ptr = getelementptr %Point, %Point* %point, i32 0, i32 1
    store i64 20, i64* %y_ptr

    ; Print x = 10, y = 20
    %str3 = getelementptr [5 x i8], [5 x i8]* @str.x_eq, i32 0, i32 0
    call i32 (i8*, ...) @printf(i8* %str3)
    %x_val = load i64, i64* %x_ptr
    call void @rt_print_i64(i64 %x_val)
    %str4 = getelementptr [7 x i8], [7 x i8]* @str.y_eq, i32 0, i32 0
    call i32 (i8*, ...) @printf(i8* %str4)
    %y_val = load i64, i64* %y_ptr
    call void @rt_print_i64(i64 %y_val)
    call void @rt_println()

    ; Test GC
    %str5 = getelementptr [14 x i8], [14 x i8]* @str.gc, i32 0, i32 0
    call i32 (i8*, ...) @printf(i8* %str5)
    call void @rt_println()

    ; Force a GC cycle
    call void @rt_collect()

    ; Print success
    %str6 = getelementptr [18 x i8], [18 x i8]* @str.passed, i32 0, i32 0
    call i32 (i8*, ...) @printf(i8* %str6)
    call void @rt_println()

    ; Print stats
    call void @rt_print_stats()

    ret void
}

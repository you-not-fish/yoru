# Yoru 工具链要求

本文档说明 Yoru 编译器开发和运行所需的外部工具及版本要求。

## 必需工具

| 工具 | 最低版本 | 用途 |
|------|----------|------|
| Go | 1.21+ | 编译器实现语言 |
| clang | 15.0+ | LLVM IR 编译和链接 |

## 可选工具

| 工具 | 版本 | 用途 |
|------|------|------|
| llvm-as | 15.0+ | LLVM IR 汇编（调试用） |
| opt | 15.0+ | LLVM IR 验证和优化（调试用） |

## 目标平台

### 主要平台（Phase 0 锁定）

- **Target Triple**: `arm64-apple-macosx26.0.0`
- **DataLayout**: `e-m:o-i64:64-i128:128-n32:64-S128-Fn32`

### 后续扩展平台

- `x86_64-linux-gnu` (Linux AMD64)
- `x86_64-apple-darwin` (Intel Mac)

## 安装指南

### macOS (Homebrew)

```bash
# 安装 Go
brew install go

# 安装 LLVM 工具链
brew install llvm

# 添加到 PATH（Apple Silicon）
export PATH="/opt/homebrew/opt/llvm/bin:$PATH"

# 添加到 PATH（Intel Mac）
export PATH="/usr/local/opt/llvm/bin:$PATH"
```

### Ubuntu/Debian

```bash
# 安装 Go
sudo apt update
sudo apt install golang-go

# 安装 LLVM 工具链
sudo apt install clang llvm
```

### Arch Linux

```bash
sudo pacman -S go clang llvm
```

## 验证安装

使用 `yoruc doctor` 命令验证工具链是否正确安装：

```bash
yoruc -doctor
```

输出示例：

```
Yoru Toolchain Doctor
=====================

Go:      go1.23.3 ✓
clang:   Apple clang version 16.0.0 ✓
opt:     LLVM version 16.0.0 ✓
llvm-as: LLVM version 16.0.0 ✓

All required tools available!
```

## 版本兼容性说明

### Go 版本

- 最低支持 Go 1.21
- 推荐使用最新稳定版

### LLVM/Clang 版本

- 最低支持 LLVM 15.0
- 需要支持 `gc "shadow-stack"` 属性
- macOS 系统自带的 Apple Clang 通常满足要求

## 常见问题

### Q: clang 找不到？

确保 LLVM 已正确安装并添加到 PATH。在 macOS 上，Homebrew 安装的 LLVM 默认不在 PATH 中。

### Q: opt/llvm-as 找不到？

这些是可选工具，仅用于调试。编译器核心功能不依赖它们。

### Q: Linux 上如何指定 target triple？

目前 Phase 0 锁定 macOS arm64 平台。Linux 支持将在后续阶段添加。

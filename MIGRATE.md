# 从 yaklang 内置 Java 工具迁移到 javajive

本文面向 **yaklang 维护者**，说明如何把仓库内置的 Java 序列化 / 反序列化与 class 解析 / 反编译能力，替换为独立模块 [`github.com/yaklang/javajive`](https://github.com/yaklang/javajive)。

javajive 是从 yaklang 抽取并裁剪而来的便携、纯 Go 实现，目标是「一个 import 即可用」，并去掉 yaklang 巨型依赖图（ANTLR、memedit 等）。

> ⚠️ 迁移前请读完「行为差异」一节。javajive 为便携性做了取舍，**并非 1:1 等价替换**。

---

## 1. 包映射

| yaklang 内置（旧） | javajive（新） | 包名 |
| --- | --- | --- |
| `github.com/yaklang/yaklang/common/yserx` | `github.com/yaklang/javajive/serialization` | `yserx`（不变） |
| `github.com/yaklang/yaklang/common/javaclassparser` | `github.com/yaklang/javajive/classparser` | `javaclassparser`（不变） |
| `github.com/yaklang/yaklang/common/javaclassparser/jarwar` | `github.com/yaklang/javajive/classparser/jarwar` | `jarwar`（不变） |
| —（新增统一门面） | `github.com/yaklang/javajive` | `javajive` |

**关键点**：两个核心包的 **包名保持不变**（仍是 `yserx` / `javaclassparser`），所以迁移在多数情况下只是 **改 import 路径**，符号、签名、调用方式都不变。

此外 javajive 提供了一个统一门面包 `javajive`，聚合三类能力，推荐新代码直接用它：

```go
import "github.com/yaklang/javajive"

src, _ := javajive.Decompile(classBytes)            // 单类反编译
_ = javajive.DecompileArchive("app.jar", "out-src") // 归档反编译
obj, _ := javajive.ParseClass(classBytes)           // class 解析
objs, _ := javajive.ParseSerialized(raw)            // 序列化解析
js, _ := javajive.SerializedToJSON(objs...)         // -> JSON
back, _ := javajive.SerializedFromJSON(js)          // <- JSON
out := javajive.MarshalSerialized(back...)          // 重新编码
```

---

## 2. 行为差异（务必阅读）

javajive 相对上游做了如下取舍：

1. **移除反编译器的 ANTLR 语法安全网**
   - 上游 `javaclassparser` 在 dump 后会用 ANTLR Java 语法解析器校验生成源码，失败则把成员降级为桩（stub）。
   - javajive 删除了 `syntax_validate.go` 对 `common/yak/java/javasyntax` 的依赖：`EnableDecompileSyntaxValidation` 默认 `false`，校验函数为 no-op（恒返回 `nil`）。
   - **影响**：反编译结果直接输出、不再降级。绝大多数类输出相同；极端构造下上游会降级而 javajive 不会。若 yaklang 强依赖该安全网，请保留上游版本，或在 javajive 上层自行接入校验。

2. **`codec.MatchMIMEType` 降级为 stub**
   - 反编译字符串字面量时的可选「中文字符集（GBK/GB18030）恢复」被关闭（恒返回 nil）。
   - **影响**：含被错误解码的中文字面量时，javajive 直接走 `strconv.Quote` 常规路径，不做字符集回收。

3. **不包含 `yso`（gadget 生成器）**
   - javajive 只含「序列化 + class 解析 + 反编译」三件套，不含 `common/yso`。
   - **影响**：`yso` 仍需留在 yaklang。但 `yso` 依赖 `yserx`/`javaclassparser`，若 yaklang 把这两个包切到 javajive，则 `yso` 的 import 也要一并改到 javajive（见下）。

4. **删除了 `yserx/exports.go`（脚本引擎导出表）**
   - 上游 `yserx.Exports`（含 `"Decompile": jarwar.AutoDecompile` 等）是 yaklang 脚本引擎的绑定表，javajive 不含它（避免 `yserx` 反向耦合 `jarwar`）。
   - **影响**：脚本引擎的 `yserx` 库导出需由 yaklang 侧维护（保留这份 `exports.go`，把其中的函数指向 javajive 的同名导出即可）。

5. **不复制重型回归测试**
   - 依赖 `java2ssa`/`javasyntax` 的 `javaclassparser/tests/` 未迁移；javajive 自带更轻量的单元测试与 Java 交叉测试。

---

## 3. 迁移策略

### 方案 A（推荐）：适配层 / 薄壳重导出

**改动最小、风险最低**：保留 yaklang 现有的包路径，把实现重导出（re-export）到 javajive。其余调用方一行不改。

在 `common/yserx/yserx.go`（或新文件）里：

```go
package yserx

import jjser "github.com/yaklang/javajive/serialization"

// 类型别名：保持下游 *yserx.JavaObject 等用法不变
type (
	JavaSerializable = jjser.JavaSerializable
	JavaObject       = jjser.JavaObject
	// ……按需补齐你下游真正用到的类型
)

// 函数转发
var (
	ParseJavaSerialized    = jjser.ParseJavaSerialized
	ParseHexJavaSerialized = jjser.ParseHexJavaSerialized
	MarshalJavaObjects     = jjser.MarshalJavaObjects
	ToJson                 = jjser.ToJson
	FromJson               = jjser.FromJson
	NewJavaString          = jjser.NewJavaString
	// ……
)
```

> 注意：类型别名只能对**导出类型**逐个声明；若下游用到大量内部类型，方案 A 的样板代码会变多，这时方案 B 更干净。`yserx.Exports` 这份脚本导出表请继续留在 yaklang 维护。

### 方案 B：整体替换 import 路径

直接删除 `common/yserx`、`common/javaclassparser` 两个目录，加 javajive 依赖，然后机械重写所有 import：

```bash
cd /path/to/yaklang

# 1) 引入依赖
go get github.com/yaklang/javajive@latest

# 2) 删除内置实现（先确认没有方案 A 的薄壳）
rm -rf common/yserx common/javaclassparser

# 3) 全仓重写 import（注意顺序：先长后短）
grep -rl 'common/javaclassparser' --include='*.go' . | xargs sed -i '' \
  -e 's#github.com/yaklang/yaklang/common/javaclassparser#github.com/yaklang/javajive/classparser#g'
grep -rl 'common/yserx' --include='*.go' . | xargs sed -i '' \
  -e 's#github.com/yaklang/yaklang/common/yserx#github.com/yaklang/javajive/serialization#g'

# 4) 处理脚本导出表：把 yserx.Exports 这份文件迁回 yaklang 侧（指向 javajive 的同名导出）

# 5) 收敛 & 验证
go mod tidy
go build ./...
go test ./common/yso/... ./common/yakgrpc/... ./common/yak/yakurl/...
```

包名不变，所以 `yserx.XXX` / `javaclassparser.XXX` 的调用点 **不需要改**，只改 import 行。

---

## 4. 各消费方迁移清单

迁移时重点验证以下 yaklang 内部消费方（它们直接 import 了这两个包）：

- `common/yso/*`：gadget 生成器，依赖 `yserx` + `javaclassparser`。改 import 即可；注意它构造序列化对象，方案 A 的类型别名需覆盖其用到的全部类型。
- `common/yakgrpc/grpc_yso.go` / `grpc_facades.go` / `grpc_codec.go`：gRPC 处理器。
- `common/yak/yakurl/java_decompiler/{fs,list}.go`：反编译 URL 资源处理。直接用 `classparser` / `jarwar`，迁移最直接。
- 脚本引擎（yaklib）中注册 `yserx.Exports` 的位置：改为引用 yaklang 侧保留的导出表。
- `common/yserx/cmd/dumper.go`：示例 cmd，未迁移，可删除或自行保留。

---

## 5. 验证

迁移完成后建议运行：

```bash
go build ./...
go vet ./...
go test ./common/yso/... ./common/yakgrpc/... ./common/yak/yakurl/java_decompiler/...
```

并重点回归：

- 反编译若干真实 jar/war（对比迁移前后输出；注意「行为差异 1」可能带来的降级差异）。
- `yso` 生成的 gadget 能正常序列化 / 反序列化。
- 脚本引擎中 `yserx.*` / `Decompile` 库函数可用。

---

## 6. 何时**不要**迁移

- 你强依赖反编译器的 ANTLR 语法安全网（降级为桩的行为）。
- 你需要字符串字面量的中文字符集恢复。

以上场景请继续使用 yaklang 内置实现，或在 javajive 之上自行补回相应能力。

# javajive

便携、纯 Go 的 Java 工具链。从 [yaklang](https://github.com/yaklang/yaklang) 抽取并裁剪而来，聚焦三件事：

- **Java 反编译**：`.class` / `.jar` / `.war` / `.zip` → 可读的 Java 源码。
- **Class 解析**：解析 class 文件结构（常量池、字段、方法、版本、访问标志）。
- **Java 序列化**：解析与重组 Java 序列化（ObjectStream）二进制 ↔ JSON。

设计目标：**便携、纯 Go、依赖尽量少、自我包含**。无需 JDK、无 cgo、无 ANTLR 运行时。

> A portable, pure-Go Java toolkit: decompile classes/JARs, inspect `.class`
> structure, and convert the Java serialization wire format to/from JSON.

## 安装 / Install

```bash
go install github.com/yaklang/javajive/cmd/javajive@latest
```

或从源码构建 / build from source:

```bash
git clone https://github.com/yaklang/javajive
cd javajive
go build -o javajive ./cmd/javajive
```

## 命令行用法 / CLI

```text
javajive <command> [arguments]

Commands:
  decompile   反编译 .class/.jar/.war/.zip 或目录为 Java 源码
  classinfo   打印 .class 文件结构（版本、字段、方法）
  serial      Java 序列化工具（子命令：tojson、fromjson）
  version     打印版本
  help        显示帮助
```

### decompile

```bash
# 单个 class：默认输出到 stdout，可用 -o 写文件
javajive decompile Foo.class
javajive decompile Foo.class -o Foo.java

# 归档：默认输出到 "<输入>.src" 目录，可用 -o 指定
javajive decompile app.jar
javajive decompile app.war -o ./app-src

# 目录：递归反编译其中的 .class（必须用 -o 指定输出目录）
javajive decompile ./classes -o ./src
```

### classinfo

```bash
javajive classinfo Foo.class
```

输出示例：

```text
class:      InvisibleAnnoSeed
super:      java/lang/Object
version:    61.0
access:     public
constants:  18

fields (0):

methods (2):
   <init>()V
   run()I
```

### serial

```bash
# 序列化二进制 → JSON（-hex 表示输入是十六进制字符串，- 表示从 stdin 读取）
javajive serial tojson dump.bin
printf 'aced000574000568656c6c6f' | javajive serial tojson -hex -

# JSON → 序列化二进制（默认输出 hex；-o 写文件时输出原始字节，可用 -hex 强制 hex）
javajive serial fromjson dump.json -o out.bin
javajive serial fromjson dump.json          # 打印 hex
```

## 作为库使用 / Library

```go
import (
    classparser "github.com/yaklang/javajive/classparser"
    "github.com/yaklang/javajive/classparser/jarwar"
    yserx "github.com/yaklang/javajive/serialization"
)

// 反编译单个 class
src, err := classparser.Decompile(classBytes)

// 反编译归档（jar/war/zip）到目录
err := jarwar.AutoDecompile("app.jar", "app-src")

// 解析 class 结构
obj, err := classparser.Parse(classBytes)
_ = obj.GetClassName()

// 序列化：二进制 → JSON → 二进制
objs, _ := yserx.ParseJavaSerialized(raw)        // 或 ParseHexJavaSerialized(hex)
jsonBytes, _ := yserx.ToJson(objs)
restored, _ := yserx.FromJson(jsonBytes)
out := yserx.MarshalJavaObjects(restored...)
```

## 包结构 / Layout

```text
serialization/   Java 序列化/反序列化（源自 yaklang common/yserx）
classparser/     Class 解析与反编译器（源自 yaklang common/javaclassparser）
cmd/javajive/    命令行入口
internal/        裁剪后的自包含支撑层（log / codec / funk / utils / filesys / ...）
```

## 与上游的差异 / Notes

为保持便携与精简，相对 yaklang 上游做了以下取舍：

- **移除 ANTLR 语法安全网**：反编译器原本会用 ANTLR Java 语法解析器对生成的源码做一次语法校验，校验失败则把成员降级为桩。javajive 去掉了这条重量级依赖（数万行生成代码 + ANTLR 运行时），校验改为始终通过——直接输出反编译结果，不再降级。
- **裁剪支撑层**：`utils` / `codec` / `log` / `go-funk` 被裁剪为 `internal/` 下的最小自包含实现，避免引入大量无关依赖。
- **不含 yso**：不纳入 Java 反序列化 gadget 生成器。
- **MIME/字符集恢复降级**：反编译字符串字面量时的可选「中文字符集恢复」被降级为 no-op（绝大多数场景行为不变）。

第三方依赖被收敛到一小组纯 Go 库（`gobwas/glob`、`go-viper/mapstructure`、`samber/lo`、`tidwall/gjson`、`segmentio/ksuid`、`yeka/zip` 及若干 `golang.org/x/*`）。

## 许可 / License

开源。沿用 yaklang 上游许可。

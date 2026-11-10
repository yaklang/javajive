<p align="center">
  <img src="assets/hero-zh.png" alt="JavaJive — 纯 Go Java 工具包：反编译、Class 解析、序列化" width="880">
</p>

# JavaJive

[![CI](https://github.com/yaklang/javajive/actions/workflows/ci.yml/badge.svg)](https://github.com/yaklang/javajive/actions/workflows/ci.yml)
[![Pages](https://github.com/yaklang/javajive/actions/workflows/deploy-pages.yml/badge.svg)](https://github.com/yaklang/javajive/actions/workflows/deploy-pages.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/yaklang/javajive.svg)](https://pkg.go.dev/github.com/yaklang/javajive)
[![License: MIT](https://img.shields.io/badge/license-MIT-E23B2E.svg)](LICENSE)

[English](README.md) | **简体中文** | [官网](https://yaklang.io/javajive/)

便携、**纯 Go** 的 Java 工具链——从 [yaklang](https://github.com/yaklang/yaklang)
抽取并裁剪而来，专注四件事：

- **反编译**：`.class`、Java 归档（`.jar` / `.war` / `.ear` / `.aar` / `.par` /
  `.jmod` / `.zip`）与 JDK 运行时镜像（`.jimage` / `$JAVA_HOME/lib/modules`）
  → 可读 Java 源码，包括 WAR/EAR/AAR 与 fat JAR 内的嵌套 JAR。
- **DEX 转换**：原生将 `.dex` / `.apk` / `.apks` / `.xapk` / `.apkm` / `.aab`
  转为确定性 `.jar`，或直接输出可读 Java 源码，无需调用外部 dex2jar 工具。
- **Class 解析**：解析 class 文件结构（常量池、字段、方法、版本、访问标志）。
- **序列化**：以字节级保真解析与重组 Java 序列化（ObjectStream）二进制，并可与 JSON 互转。

为便携与嵌入而设计：

- **纯 Go、单一二进制**：**无需 JDK、无 cgo、无 ANTLR 运行时**、无原生库。可交叉编译到
  linux / macOS / windows 的 amd64 与 arm64。
- **一个 import**：统一门面包 `javajive` 聚合四类能力；子包仍可用于高级用法。
- **一流的命令行**：`decompile`、`dex2jar`、`classinfo`、`serial` 子命令，
  基于标准库实现。
- **与真实 JDK 交叉测试**：CI 用 `javac`/`java` 编译真实 `.class` / `.jar` 与 JDK 序列化字节，
  再用 JavaJive 验证（见 [HARNESS-WORKFLOW.md](HARNESS-WORKFLOW.md)）。
- **收敛的依赖图**：`utils` / `codec` / `log` / `go-funk` 被重写为 `internal/` 下最小自包含实现。

## v0.4.0 验证

全部 **38 个历史 JAR 的固定基线差分未检出回归**：原本可完整编译的 20 个包全部保留，
当前 26 个包完整编译成功；基线既有缺陷和 4 个方法 stub 保持可见。
详见[逐包结果及复现方法](docs/historical-jar-audit.md)。

**250 项隔离语义往返**分别记录编译、JVM 校验、stub 与运行结果一致性，重建类路径不含
原始应用类。CI 在 JDK 17、21 重跑语义测试，并执行全量历史对比、OS/Go 矩阵及 race 语料。
这些检查不证明全部库方法行为等价；完整说明见[实现与剩余边界](docs/decompiler-semantic-audit.md)。

## 历史评测（v0.3.0）

以下为 v0.3.0 在 2026-09-05 发布的历史数据，不作为当前版本的验收结果或行为等价证明。

在 34 个真实流行 jar（18,759 个摊平单元）上，走「反编译 → `javac --release 8` 重编译 → 重打包 → JVM 校验」：

- **单元级干净率 100%** —— **18,759 / 18,759** 个摊平单元（`Outer$Inner.java`）**零 `javac` 错误**可重编译，且全 34 jar **语法错为 0**
  （CI 硬断言把关，任何类型错都无法被词法错遮蔽）。
- **34 个库全部完整往返** —— 反编译 → 重编译 → 重打包 → 外部 JVM `-Xverify:all` 逐类校验全通过
  （codec 经调用差分与原始 jar 逐字节一致），锁进 `provenClean`。
- **14 / 14 自托管算法**（MD5 · SHA-256 · CRC32 · 快排 · Base64 · HeapSort · KMP · SwitchFSM ·
  TryFinally · UnionFind · DiamondTryCatch · DiamondTryFinally · ForContinue · ComputeIfAbsent）往返**逐字节一致**。
- **三方横评第一**（原 8 jar 对照集）—— 单元级干净率 **100%**，高于 Vineflower 1.10.1（90.8%）与 CFR 0.152（79.8%），8 个 jar 对两方全胜。

完整方法学、逐 jar 表格与复现命令见 [BENCHMARK.md](BENCHMARK.md)。

### 复杂 JAR 语料

另有一套 CI 强制的复杂 JAR 语料，覆盖历史 Java 8 库之外的语言目标：Java 21 的
record/sealed/模式 switch/switch 表达式、控制流压力样例、Java 8 enum 常量体、
100 个类的规模 JAR、多版本 JAR、模块化 JAR，以及打包后的非规约样例。每个用例都会
反编译、整树重编译、`java -Xverify:all` 校验，并与原始程序输出做行为对比。清单、
已修复缺陷与剩余边界见 [test/complexjar/REPORT.md](test/complexjar/REPORT.md)。

## 安装

```bash
# 命令行
go install github.com/yaklang/javajive/cmd/javajive@latest

# 作为库
go get github.com/yaklang/javajive@latest
```

或从源码构建：

```bash
git clone https://github.com/yaklang/javajive
cd javajive
go build -o javajive ./cmd/javajive
```

需要 Go 1.22+。

## 命令行

```text
javajive <command> [arguments]

Commands:
  decompile   反编译 .class、Java 归档、JDK 运行时镜像、DEX/APK/APKS/XAPK/APKM/AAB 或目录为 Java 源码
  dex2jar     将 DEX/APK/APKS/XAPK/APKM/AAB 转为确定性 JAR
  jimage2jar  将 JDK 运行时镜像（lib/modules、.jimage）转为 JAR 视图
  inspect     只读盘点 Java 归档与 Android 容器
  classinfo   打印 .class 文件结构（版本、字段、方法）
  serial      Java 序列化工具（子命令：tojson、fromjson）
  version     打印版本
  help        显示帮助
```

输入先按内容魔数识别，其次按归档结构，最后才回退到扩展名；因此改名后的文件仍可处理。

| 输入 | 处理方式 |
| --- | --- |
| `.class`、目录 | 单 class，或递归反编译目录中的 `.class` |
| `.jar` / `.war` / `.ear` / `.zip` | Java 归档；嵌套的 `.jar`/`.war`/`.ear`/`.par`/`.jmod`/`.zip` 会就地反编译 |
| `.aar` | Android 库归档；`classes.jar` 与 `libs/*.jar` 会反编译 |
| `.par` | Java 归档（ZIP 无更强标记时按扩展名回退） |
| `.jmod` | JDK 模块镜像；自动剥离 4 字节 `JM\x01\x00` 头 |
| `.jimage` / `$JAVA_HOME/lib/modules` | JDK 9+ 运行时镜像；转换为 package-layout 的 JAR 视图。支持未压缩、zlib `zip` 与 `compact-cp` 资源 |
| `.dex` | 裸 DEX |
| `.apk` | 根目录的 `classes.dex`、`classes2.dex`…… multidex 条目 |
| `.apks` / `.xapk` / `.apkm` / `.apkx` | 嵌套 split APK；存在 `universal.apk` 时优先使用 |
| `.aab` | Android App Bundle；翻译每个 `<module>/dex/classes*.dex` |

`--structured` 会把 Java 归档拆成 `sources/`、`resources/` 与逐字节保留的 `archives/`。
Android 容器始终输出翻译后的源码树。

JImage 会先转换为 package-layout 的 JAR 视图：class 资源按 package 路径导出，
`module-info.class` 与非 class 资源保留模块前缀。支持未压缩、zlib（`zip`）与
`compact-cp`（常量池字符串共享）资源，也支持 big-endian 镜像。完整 JDK 镜像包含
数万个 class，建议先用 `inspect` 或 `JImageToJar`。

### decompile

```bash
# 单个 class：默认输出到 stdout，可用 -o 写文件
javajive decompile Foo.class
javajive decompile -o Foo.java Foo.class

# 归档：默认输出到 "<输入>.src" 目录，可用 -o 指定
javajive decompile app.jar
javajive decompile -o ./app-src app.war

# 结构化归档导出：sources/、resources/ 与保留原字节的 archives/
# （WEB-INF/lib、EAR/lib、AAR classes.jar 与 fat-JAR 依赖都会就地反编译）
javajive decompile --structured -o ./app-src app.war

# 归档内 class 相互隔离：畸形 class 或未来版本 class 会生成带诊断的 fallback
# .java 条目，其余 class 继续导出。structured 报告会标记每个 class 为
# complete / partial / fallback，并携带 stub 方法与诊断信息。

# AAR 与 JMOD 识别后按普通归档处理
javajive decompile -o ./aar-src library.aar
javajive decompile -o ./jmod-src java.base.jmod

# JDK 运行时镜像：class 按 package 路径导出；
# module-info.class 与非 class 资源保留模块前缀
javajive decompile -o ./jdk-src /path/to/jdk/lib/modules

# DEX 或 Android 容器：使用原生 DEX-to-JAR 流程，再反编译结果
# 默认输出目录为「<输入>.src」
javajive decompile classes.dex
javajive decompile -o ./app-src app.apk
javajive decompile -o ./app-src app.apks
javajive decompile -o ./app-src app.xapk
javajive decompile -o ./app-src app.aab

# 可选的 Android 部分产物：已知但不支持的方法变成显式抛异常 stub。
# 畸形输入、未知格式和资源超限仍然失败，不会伪装成成功。
javajive decompile --best-effort -o ./app-src app.apk

# 目录：递归反编译其中的 .class（必须用 -o 指定输出目录）
javajive decompile -o ./src ./classes

# 超大生成方法：显式提高单方法分析预算。
# 大于 1,000,000 的取值同时按比例放大图/扫描上限。
javajive decompile -analysis-budget 2000000 Big.class
javajive decompile -analysis-budget 2000000 -o ./app-src app.jar
```

### dex2jar

```bash
javajive dex2jar -o classes.jar classes.dex
javajive dex2jar -o app.jar app.apk
javajive dex2jar -o app.jar app.apks
javajive dex2jar -o app.jar app.aab

# 显式 best-effort：已知不支持的方法使用抛异常 stub 代替
javajive dex2jar --best-effort -o app.jar app.apk
```

### jimage2jar

```bash
# 将 JDK 9+ 运行时镜像转换为 package-layout 的 JAR 视图，不反编译全部 class
javajive jimage2jar -o jdk.jar /path/to/jdk/lib/modules
```

输入类型按文件魔数而非扩展名识别，输出为可复现的 JAR。默认转换严格且事务性：
任一 class 或方法不受支持、或输入损坏时，整次转换失败且不产出 JAR。
`--best-effort` 只把“已识别的方法级能力缺口”替换为 verifier-safe 的
`UnsupportedOperationException` 抛异常 stub；畸形输入、未知格式、完整性错误、
资源超限和内部错误仍然失败。best-effort 产物状态为 `partial`，每个 stub 方法都有
对应诊断。两种模式都会先写同目录临时文件，再原子替换目标文件。

### 原生 DEX/APK 支持边界

原生 DEX 转换仍是**严格的垂直切片**，并非通用 Android 应用转换器。已测试范围覆盖
DEX 035、037、038、039 中 `map_list` 证明不含 call-site、method-handle 或
hidden-API 段的输入，包括 annotation、debug 元数据、`SourceFile`、普通
int/引用控制流、字段与 invoke、`new-instance`/构造器配对、数组与 filled-array
字面量、`packed-switch`/`sparse-switch`、monitor、try/catch、long/float/double
以及带 static/default 方法的接口。生成 class 会经过解析测试与 JVM 校验。

暂不支持作为方法体翻译：wide/float 元素的 `filled-new-array`；DEX 038+ 动态调用与
方法常量；CompactDex；APK 资源或原生库。使用 `--best-effort` 时，仅因上述已识别
opcode/表示能力缺口而无法翻译的方法会变成显式抛异常 stub；容器损坏、DEX
header/table/verifier 错误、未支持版本或未解析 section、资源超限与内部错误永远
不会用 stub 掩盖。

Android 容器的处理规则：

- `.apk` 只翻译根目录规范命名的 `classes.dex`、`classes2.dex`……条目。
- `.apks` / `.xapk` / `.apkm` / `.apkx` 会遍历嵌套 `.apk`；存在 `universal.apk`
  时优先使用，无 DEX 的资源型 split 会跳过。跨 split 完全相同的 class 会合并，
  定义冲突则整个转换失败。
- `.aab` 会翻译 `base` 与各 feature 模块中的每个 `<module>/dex/classes*.dex`。

所有选中的 DEX 必须事务性地全部成功，否则不生成 JAR；跨越严格边界的输入可先用
`inspect` 分析。

容器默认上限：单个 DEX 256 MB、单个归档/APK 512 MB、DEX 总量 512 MB、128 个 DEX、
512 个嵌套 APK、嵌套 APK 总量 2 GB。更大的包可通过 `DexOptions` 字段提高上限。

[`testdata/external`](testdata/external/README.md) 下的真实样本包括 Tomcat WAR、
bundletool APK/AAB/APKS（含 181 个嵌套 APK 的 split 包）、AndroidX AAR、
Androguard/Apktool/dex2jar 样本，以及合成的 XAPK/APKM/EAR。这些真实世界大样本
测试属于显式本地评测（`JAVAJIVE_EXTERNAL_EVAL=1`），不进入默认 PR CI；普通 CI
只保留小型合成容器回归与 no-panic 检查。Fuzz 种子覆盖分类器、
JImage/compact-cp 读取、`inspect` 与 Android 容器解析。

### classinfo

```bash
javajive classinfo Foo.class
```

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

# JSON → 序列化二进制（默认输出 hex；-o 写文件时输出原始字节）
javajive serial fromjson dump.json -o out.bin
javajive serial fromjson dump.json          # 打印 hex
```

## 作为库使用

推荐使用统一门面包，一个 import 即可覆盖四类能力：

```go
import (
    "os"

    "github.com/yaklang/javajive"
)

// 在内存中转换 DEX/APK/APKS/XAPK/APKM/AAB 字节
jarBytes, report, err := javajive.DexToJar(dexOrContainerBytes, javajive.DexOptions{})
if err == nil {
    err = os.WriteFile("app.jar", jarBytes, 0o644)
    _ = report.Classes
}

// 转换文件并原子写入 JAR
report, err = javajive.DexFileToJar("app.apk", "app.jar", javajive.DexOptions{})

// 经私有临时 JAR 转换，并直接反编译到源码目录
err = javajive.DecompileDEXFile("classes.dex", "app-src", javajive.DexOptions{})

// 反编译单个 class、Java 归档或 JDK 运行时镜像
src, err := javajive.Decompile(classBytes)
err = javajive.DecompileArchive("app.jar", "app-src")
err = javajive.DecompileArchive("/path/to/jdk/lib/modules", "jdk-src")

// 将 JDK 运行时镜像转换为 JAR 视图，或直接读取单个资源
err = javajive.JImageToJar("/path/to/jdk/lib/modules", "jdk.jar")
image, err := javajive.OpenJImageFile("/path/to/jdk/lib/modules")
if err == nil {
    objectClass, readErr := image.Read("java.base/java/lang/Object.class")
    _ = objectClass
    _ = readErr
    _ = image.Close()
}

// 解析 class 结构
obj, err := javajive.ParseClass(classBytes)
_ = obj.GetClassName()

// 序列化：二进制 → JSON → 二进制
objs, _ := javajive.ParseSerialized(raw)          // 或 ParseSerializedHex(hexStr)
jsonBytes, _ := javajive.SerializedToJSON(objs...)
restored, _ := javajive.SerializedFromJSON(jsonBytes)
out := javajive.MarshalSerialized(restored...)
```

### 统一接口

| 函数 | 作用 |
|---|---|
| `Decompile(classBytes) (string, error)` | 反编译单个 `.class` 字节 |
| `DecompileFile(path) (string, error)` | 从磁盘反编译单个 `.class` |
| `DecompileWithResolver(classBytes, resolve)` | 带「类字节解析器」的反编译 |
| `DecompileArchive(src, dst) error` | 将 Java 归档或 JDK 运行时镜像反编译到目录 |
| `DecompileArchiveWithAnalysisBudget(src, dst, budget) error` | 带显式单方法分析预算的归档/JImage 反编译 |
| `JImageToJar(src, dst) error` | 将 JDK 运行时镜像转换为确定性 JAR 视图 |
| `OpenJImageFile(path) (*JImageReader, error)` | 打开 JDK 运行时镜像并直接读取资源 |
| `ClassifyInput(data, name) ContainerKind` | 按 magic、归档结构、扩展名分类输入 |
| `ClassifyInputFile(path) (ContainerKind, error)` | 不加载整个归档即可分类文件 |
| `DexToJar(input, options) ([]byte, DexReport, error)` | 严格转换 DEX/APK/APKS/XAPK/APKM/AAB 为确定性 JAR |
| `DexToJarWithOptions(input, convertOptions) ([]byte, DexDetailedReport, error)` | 显式 strict/best-effort 转换并返回 stub 诊断 |
| `DexFileToJar(src, dst, options) (DexReport, error)` | 严格转换文件并使用原子临时 JAR |
| `DexFileToJarWithOptions(src, dst, convertOptions) (DexDetailedReport, error)` | 带策略的文件转换与原子替换 |
| `DecompileDEXFile(src, dst, options) error` | 经私有临时 JAR 转换并反编译 DEX/Android 容器 |
| `DecompileDEXFileWithAnalysisBudget(src, dst, options, budget) error` | DEX/Android 容器转换并带预算反编译归档 |
| `DecompileDEXFileWithPolicy(src, dst, convertOptions, budget) (DexDetailedReport, error)` | 带策略的 DEX/Android 转换与反编译 |
| `ParseClass(classBytes) (*ClassObject, error)` | 解析单个 `.class` 字节 |
| `ParseClassFile(path) (*ClassObject, error)` | 从磁盘解析单个 `.class` |
| `ParseSerialized(raw) ([]JavaSerializable, error)` | 解析序列化流 |
| `ParseSerializedHex(hexStr) ([]JavaSerializable, error)` | 解析十六进制编码的流 |
| `MarshalSerialized(objs...) []byte` | 将对象重新编码为线格式 |
| `MarshalSerializedHex(objs...) string` | 重新编码为十六进制 |
| `SerializedToJSON(objs...) ([]byte, error)` | 对象 → JSON |
| `SerializedFromJSON(raw) ([]JavaSerializable, error)` | JSON → 对象 |

也可直接使用子包（高级用法）：`classparser`、`classparser/jarwar`、`dex2jar`、`serialization`。

## 与上游 yaklang 的差异

为保持便携与精简，相对 yaklang 上游做了取舍。完整映射与迁移指南见
[MIGRATE.md](MIGRATE.md)。

| 方面 | yaklang（上游） | JavaJive |
|---|---|---|
| 反编译器 ANTLR 安全网 | 用 ANTLR Java 语法对生成源码二次校验，失败时把成员降级为桩 | 移除（重量级依赖）；校验改为 no-op，直接输出结果 |
| 支撑层（`utils`/`codec`/`log`/`go-funk`） | 共享 monorepo 包 | `internal/` 下最小自包含实现 |
| `yso` gadget 生成器 | 包含 | 不包含 |
| 字符串字面量字符集恢复（`MatchMIMEType`） | 可选 GBK/GB18030 恢复 | 降级为 no-op（绝大多数场景行为不变） |

第三方依赖被收敛到一小组纯 Go 库（`gobwas/glob`、`go-viper/mapstructure`、
`samber/lo`、`tidwall/gjson`、`segmentio/ksuid`、`yeka/zip` 及若干 `golang.org/x/*`）。

## 测试

```bash
go test ./...                 # 单元测试 + JDK 交叉测试（无 JDK 时自动跳过）
go test ./... -race           # 无数据竞争（linux）
go test ./test/cross/ -v      # 仅 Java 交叉测试（需 PATH 上有 javac/java）

# 显式真实大语料/JImage 集成评测（默认 PR CI 不跑）
JAVAJIVE_EXTERNAL_EVAL=1 go test ./test/external
JAVAJIVE_JIMAGE_INTEGRATION=1 go test ./jimage ./test/external
```

JDK 交叉测试会在测试时编译真实 Java 产物并用 JavaJive 验证；无 JDK 时自动 `t.Skip`。
[非规约 CFG 专题语料](test/cross/testdata/irreducible/README.md) 还会固定生成的 class、
精确 CFG 拓扑、人工审阅的等价 Java、重编译后的 JVM 验证及运行时等价性。对于受支持的
普通边、非构造器区域，JavaJive 会输出确定性的状态机；不承诺恢复原始源码或相同字节码。
harness 与 CI 的工作方式见 [HARNESS-WORKFLOW.md](HARNESS-WORKFLOW.md)。

## 包结构

```text
javajive.go      统一门面包（import "github.com/yaklang/javajive"）
serialization/   Java 序列化/反序列化（源自 yaklang common/yserx）
classparser/     class 解析与反编译器（源自 yaklang common/javaclassparser）
cmd/javajive/    命令行入口
internal/        裁剪后的自包含支撑层（log / codec / funk / utils / filesys / ...）
test/cross/      基于 JDK 的交叉测试（javac/java）
site/            部署到 GitHub Pages 的静态落地页
```

## 许可

[MIT](LICENSE) © 2026 VillanCh。JavaJive 衍生自
[yaklang](https://github.com/yaklang/yaklang)。

# JavaJive 反编译器评测报告（Benchmark）

本报告衡量 JavaJive 的**核心目标 —— 反编译正确性**：一个 `.class` / `.jar` 被反编译成
Java 源码后，能否被 `javac` 重新编译回去、重新打包、并且**运行出与原程序逐字节一致的结果**。

报告包含两个独立实验，并与业界两款主流反编译器 **CFR** 与 **Vineflower（Fernflower 的活跃分支）**
做同口径对照：

1. **实验一 · 自托管算法往返正确性**（语义级铁证）：对自实现的 MD5 / SHA-256 / CRC32 / 快速排序 /
   Base64 等算法，走「源码 → 编译 → 运行（基准） → JavaJive 反编译 → 重新编译 → 运行（往返）」，断言
   两次运行输出**逐字节一致**。这证明反编译产物不仅"能编过"，而且**语义保真、可执行、结果正确**。
2. **实验二 · 大规模三方可重编译对照**：在 8 个真实流行 jar（commons-codec / gson / commons-lang3 /
   jsoup / snakeyaml / spring-core / fastjson2 / guava）上，三方各自反编译整包后整体重编译，
   **以「有多少个 class 编不回去」为主口径**报告缺陷规模。

> ## 一句话结论（按缺陷 class 数，最客观口径）
>
> 三方在 8 个 jar、合计约 **2252** 个顶层类上的**缺陷类数**（越低越好）：
> **CFR 36 < Vineflower 123 < JavaJive 208**。
>
> JavaJive 在**规整代码库**上有竞争力——commons-codec **0 缺陷（三方唯一 100%）**、spring-core
> **1 缺陷（与 Vineflower 并列最佳、优于 CFR）**、jsoup / snakeyaml 接近一线；但在**泛型密集库**
> （guava / fastjson2）以及 **gson**（`$` 命名类 import 缺失）/ **commons-lang3** 上**明显落后于 CFR**。
> 自托管算法往返 **5/5 逐字节一致**，证明语义正确性。

> **为什么改用「缺陷 class 数」而非「错误行数」**：错误行数会把"少数几个类里的大量错误"和"大量类各
> 一处错误"混为一谈，对反编译器的**真实可用性**评估有偏。一个类只要有 **1 处** `javac` 错误就无法重编译、
> 无法重打包，**缺陷类数=有多少个类不可用**，是更贴近工程现实、也更客观的口径。本版报告以**缺陷 class 数**
> 为主口径，错误行数仅作上下文附录。

---

## 1. 实验环境

| 组件 | 版本 |
|---|---|
| OS / CPU | macOS 14.1.2 / arm64 (Apple Silicon) |
| JDK（javac / java） | OpenJDK 17.0.12 LTS（重编译统一 `--release 8`） |
| Go（构建 JavaJive 与 harness） | go1.22.12 |
| JavaJive | 本仓库 `HEAD`（生产 `JarFS` 路径） |
| CFR | 0.152 |
| Vineflower | 1.10.1 |

对照 jar 取自本机 Maven 仓库 `~/.m2/repository`；CFR / Vineflower 的 jar 放在 `/tmp/decompilers/`
（可用 `DECOMPILERS_DIR` 覆盖）。所有 `javac` 调用统一 `-encoding UTF-8 --release 8 -nowarn`，并锁定
英文 locale 以保证诊断稳定。

---

## 2. 实验一 · 自托管算法往返正确性

### 2.1 为什么用"自托管算法"

调用 `java.security.MessageDigest` 之类的标准库，只能测到"方法调用能不能渲染对"。而**自己实现**的
MD5 / SHA-256 充满了**移位、位运算、查表、长整型溢出、深层循环与递归**——这些正是反编译器最容易在
控制流结构化与栈模拟上出错的地方。一旦往返后输出有**一个 bit** 不同，算法结果就会雪崩式改变。因此
"自托管算法往返后结果不变"是反编译语义保真的**强证据**。

### 2.2 测试集（`test/cross/testdata/algorithms/`）

| 算法 | 覆盖的代码形态 | 校验基准 |
|---|---|---|
| `MD5` | 位运算、循环左移、静态表、`int` 溢出加法 | 与系统 `md5` CLI 一致 |
| `SHA256` | 大型常量数组、消息扩展、`rotateRight`、模加 | 与 `shasum -a 256` 一致 |
| `CRC32` | 运行期建表、嵌套循环、无符号右移 | 与 `python3 zlib.crc32` 一致 |
| `QuickSort` | 递归、数组交换、二分查找、LCG 伪随机 | 确定性输出 |
| `Base64Codec` | 编解码、3 字节/4 字符位打包、填充、反查表 | 与系统 `base64` CLI 一致 |

### 2.3 实验步骤（单个算法）

```text
1. javac 编译原始源码           ->  bench/MD5.class
2. java 运行，捕获基准输出       ->  ref_stdout
3. JavaJive 反编译 MD5.class    ->  MD5.java（反编译产物）
4. javac 重新编译反编译产物      ->  bench/MD5.class（重建）
5. java 运行重建产物，捕获输出   ->  roundtrip_stdout
6. 断言 ref_stdout == roundtrip_stdout（逐字节）
```

该实验在 `TestBenchmarkRoundTripAlgorithms` 中实现，**仅依赖 javac/java，常驻 CI**。

### 2.4 结果：5/5 逐字节一致

| 算法 | 重编译 | 往返输出 | 关键样例（输入 → 输出） |
|---|---|---|---|
| MD5 | 通过 | 逐字节一致 | `"abc"` → `900150983cd24fb0d6963f7d28e17f72` |
| SHA256 | 通过 | 逐字节一致 | `"abc"` → `ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad` |
| CRC32 | 通过 | 逐字节一致 | `"abc"` → `352441c2` |
| QuickSort | 通过 | 逐字节一致 | 200 元素排序 + 二分查找命中位 |
| Base64Codec | 通过 | 逐字节一致 | `"foobar"` → `Zm9vYmFy` → `"foobar"` |

> 全部算法反编译后**重新编译通过**，重新运行结果**与原程序逐字节相同**，且密码学算法的输出与系统
> CLI 校验值一致——证明 JavaJive 的产物可重打包、可执行、语义正确。

复现：

```bash
go test -run TestBenchmarkRoundTripAlgorithms -v ./test/cross/
```

---

## 3. 实验二 · 大规模三方可重编译对照

### 3.1 度量口径与公平性说明

对每个 jar，三方各自反编译**整包**，再把各自产出的全部 `.java` **整体** `javac` 一次性编译（依赖在
classpath 上，原 jar **不**在 classpath 上——产物必须自洽）。

**主口径 · 缺陷 class 数（packaging-independent，三方可比）。** 三方产出的**文件粒度不同**：JavaJive 把
每个嵌套类摊平成独立的顶层文件 `Outer$Inner.java`（文件数 ≈ class 数）；CFR / Vineflower 把嵌套类内联进
外层文件（一个文件 = 一个**外层类**）。因此**逐文件通过率不能横比**。本报告把所有产物**按外层（顶层）类
归一**——一个外层类只要它对应的任一单元（含被摊平的内部类）出现 `javac` 错误，就计为**一个缺陷类**——
得到与打包方式无关、三方同粒度的「缺陷 class 数」。这是**主口径**（表 A）。

为完整起见保留两个细粒度视角作上下文：表 1 逐文件通过率（**不可跨工具比**，仅看各自自洽度），表 2 错误
总行数（仅作上下文，**不作主口径**，原因见上文一句话结论下的说明）。

> 归一化细节：外层类 key = 去掉 `.java` 与 `$Inner` 后缀的包路径（如 `…/Maps$1.java`、`…/Maps.java`
> 都归到 `…/Maps`）。三方的「外层类总数」分母基本相等（=各 jar 顶层类数），个别 jar 因某工具多/少发一个
> 文件而有 ±1~2 的细微差异。本口径由 `test/cross/benchmark_test.go` 的 **Table A** 自动产出。

### 3.2 表 A · 缺陷 class 数（主口径，越低越好）

单元格 = **缺陷类 / 外层类总数（干净类率）**。缺陷类越少越好。

| jar | classes | JavaJive | CFR | Vineflower |
|---|---:|---:|---:|---:|
| commons-codec | 106 | **0/72 (100.0%)** | 2/72 (97.2%) | 2/72 (97.2%) |
| gson | 195 | 23/73 (68.5%) | 3/73 (95.9%) | **1/74 (98.6%)** |
| commons-lang3 | 345 | 28/198 (85.9%) | **4/198 (98.0%)** | 5/198 (97.5%) |
| jsoup | 238 | 3/51 (94.1%) | **1/51 (98.0%)** | 2/51 (96.1%) |
| snakeyaml | 231 | 6/122 (95.1%) | **2/123 (98.4%)** | **2/121 (98.3%)** |
| spring-core | 978 | **1/649 (99.8%)** | 2/649 (99.7%) | **1/649 (99.8%)** |
| fastjson2 | 681 | 53/529 (90.0%) | **14/530 (97.4%)** | 44/529 (91.7%) |
| guava | 1892 | 94/558 (83.2%) | **8/558 (98.6%)** | 66/558 (88.2%) |
| **合计** | | **208/2252 (90.8%)** | **36/2254 (98.4%)** | **123/2252 (94.5%)** |

> 读法：JavaJive 在 **codec 0 缺陷（三方唯一干净到底）**、**spring-core 1 缺陷（与 Vineflower 并列最佳、
> 优于 CFR）**；jsoup（3）/ snakeyaml（6）接近一线。其余 5 个 jar 落后于 CFR，其中 gson / commons-lang3 /
> fastjson2 / guava 差距明显。**CFR 在缺陷类数上整体最稳（36），JavaJive（208）当前列第三。**

### 3.3 表 1 · 逐文件通过率（干净编译文件 / 产出文件；**不可跨工具比**，仅看各自自洽度）

| jar | classes | JavaJive | CFR | Vineflower |
|---|---:|---:|---:|---:|
| commons-codec | 106 | 106/106 (100.0%) | 70/72 (97.2%) | 70/72 (97.2%) |
| gson | 195 | 156/183 (85.2%) | 71/74 (95.9%) | 74/75 (98.7%) |
| commons-lang3 | 345 | 310/339 (91.4%) | 194/198 (98.0%) | 193/198 (97.5%) |
| jsoup | 238 | 144/148 (97.3%) | 50/51 (98.0%) | 49/51 (96.1%) |
| snakeyaml | 231 | 219/231 (94.8%) | 121/123 (98.4%) | 119/121 (98.3%) |
| spring-core | 978 | 973/974 (99.9%) | 647/649 (99.7%) | 648/649 (99.8%) |
| fastjson2 | 681 | 626/681 (91.9%) | 516/530 (97.4%) | 485/529 (91.7%) |
| guava | 1892 | 1635/1825 (89.6%) | 550/558 (98.6%) | 492/558 (88.2%) |

> 注意：JavaJive 的分母是「摊平后单元数」(≈ class 数)，CFR/Vineflower 的分母是「外层类数」(少得多)，
> **故本表的百分比口径不同，不能直接横比**，只表示各工具产物「自身有多少比例能干净编过」。要横比请看表 A。

### 3.4 表 2 · `javac` 错误总行数（仅作上下文，**不作主口径**）

| jar | classes | JavaJive | CFR | Vineflower |
|---|---:|---:|---:|---:|
| commons-codec | 106 | **0** | 6 | 2 |
| gson | 195 | 117 | 20 | **1** |
| commons-lang3 | 345 | 87 | 198 | **9** |
| jsoup | 238 | 4 | 30 | **3** |
| snakeyaml | 231 | 39 | **2** | **2** |
| spring-core | 978 | **2** | 16 | 32 |
| fastjson2 | 681 | 242 | **92** | 307 |
| guava | 1892 | 478 | **29** | 169 |
| **合计** | | **969** | **393** | **525** |

> **错误行数会误导**：典型反例——commons-lang3 上 JavaJive 错误行（87）少于 CFR（198），但**缺陷类数**
> JavaJive（28）远多于 CFR（4）——CFR 的 198 行集中在 4 个类里，JavaJive 的 87 行散在 28 个类里；
> fastjson2 上 JavaJive 行数（242）少于 Vineflower（307），但**缺陷类数** JavaJive（53）多于 Vineflower（44）。
> 这正是本版改用「缺陷 class 数」为主口径的原因。

### 3.5 分析（诚实结论）

- **JavaJive 的强项 —— 规整 / 大型代码库**：commons-codec **0 缺陷类（三方唯一 100%）**，spring-core
  **1 缺陷类（与 Vineflower 并列最佳、优于 CFR）**。说明在普通业务代码、大型但结构规整的工程上，JavaJive
  已达业界一线；jsoup / snakeyaml 也接近。
- **JavaJive 的短板 —— 泛型密集 + 特殊命名**：guava（94 缺陷类）、fastjson2（53）、gson（23）、
  commons-lang3（28）均明显落后于 CFR（8 / 14 / 3 / 4）。残余几乎全部集中在**泛型擦除 → 缺造型**、
  **`$` 命名类 import 缺失**、**扁平嵌套类丢外层类型参数**三类（详见 §4「明确缺陷」）。
- **CFR 最稳**：缺陷类数合计 36，是三方最佳，泛型重建最成熟；Vineflower 居中（123），但在 gson 上近乎完美。
- **整体**：按缺陷类数 CFR(36) < Vineflower(123) < JavaJive(208)。结合实验一的 5/5 语义保真，JavaJive 是
  一款**在规整代码上达到一线、在泛型密集与特殊命名场景仍有明确差距**的反编译器，正按 `HARNESS.md`
  的方法论逐点治本（已治本项见 `classparser/CODEC_TODO.md` §2）。

> 为什么对照 CFR / Vineflower：当三方**都失败**，说明该字节码内在难结构化（编译器合成的反人类模式），
> 可诚实 stub、不必死磕；当**只有 JavaJive 失败**，说明存在结构化偏差，照着 CFR/Vineflower 的产物定位
> CFG / 栈模拟差异。该 oracle 同时是 benchmark 与 debug 工具（见 `TestThirdPartyOracle`）。

复现：

```bash
# 全量 8 包三方对照（需 ~/.m2 与 /tmp/decompilers/{cfr,vineflower}-*.jar）；主口径见日志 Table A
BENCHMARK=1 go test -run TestBenchmarkThreeWayRecompile -v -timeout 90m ./test/cross/

# 仅指定子集
BENCHMARK=1 BENCHMARK_JARS=codec,gson,guava go test -run TestBenchmarkThreeWayRecompile -v ./test/cross/
```

---

## 4. 明确缺陷（按杠杆从大到小）

下列缺陷均由 harness 真实跑出的 `javac` 诊断归类、并抽取代表样例（`PROFILE_JAR=<jar> go test -run
TestJarTreeInventory`），不是估算。每项给「影响面 · 代表 jar/类 · 真实样例 · 根因 · 现状」。
更细的工单与已治本清单见 `TODO.md` 与 `classparser/CODEC_TODO.md`。

### D1 · 泛型擦除缺造型（`incompatible types: Object cannot be converted to T/K/V/...`）—— **最大杠杆，跨 jar**

- **影响面**：缺陷类的头号来源。错误桶 `incompatible types (assignment/return)`：gson **64**、commons-lang3
  **62**、fastjson2 与 guava 也以此桶为主。
- **真实样例**（gson）：`LinkedHashTreeMap.java:163: error: incompatible types: Object cannot be converted
  to LinkedHashTreeMap$Node<K,V>`。
- **根因**：字节码经泛型擦除后，取值点静态类型是 `Object`，反编译时未补回源码原有的 `(T)` / `(Node<K,V>)`
  向下造型。Java 泛型方法/字段的形参与返回值在字节码里都被擦成 bound（多为 `Object`），需要沿「接收者参数化
  类型 + 方法/字段 Signature + 跨类型层级替换」复原精确类型并补造型。
- **现状**：JavaJive 已治本**八块**（返回点 Object 向下造型、JDK/同类/继承超类型/私有方法的实参造型、
  统一跨类泛型解析器、擦除型类型变量多余 upcast 抑制等，见 `CODEC_TODO.md` §2，累计已削减数百行）。
  **残余**：接收者本身的泛型未被传播复原成参数化类型、通配符捕获 `CAP#1`（见 D4）等长尾，业界 CFR/Vineflower
  亦非全解。

### D2 · `$` 命名类的 import 缺失（gson `$Gson$Types` / `$Gson$Preconditions`）—— **gson 最大驱动**

- **影响面**：gson `cannot find symbol` 桶 42 行里约 **30 行**（`variable $Gson$Types` 18 + `$Gson$Preconditions` 12），
  是 gson 成为 JavaJive 最差 jar（68.5%）的主因。
- **真实样例**：`GsonBuilder.java:193: $Gson$Preconditions.checkArgument(...)` → `cannot find symbol:
  variable $Gson$Preconditions`。被引用的类**确实**被正确反编译成文件
  `com/google/gson/internal/$Gson$Types.java`（`final class $Gson$Types`），引用处的标识符文本也对，
  **但产物缺少对应的 `import com.google.gson.internal.$Gson$Preconditions;`**，于是 javac 把这个跨包的
  `$Gson$Preconditions` 当作未定义的**变量**来解析而报错。
- **根因**：gson 有真实**简单名里就含 `$`** 的顶层类（`$Gson$Types`、`$Gson$Preconditions`）。JavaJive 用 `$`
  作为「外层类$内部类」摊平分隔符，import 生成逻辑把这种名字里的字面 `$` 误当成嵌套分隔，导致**跨包引用没有
  生成正确的 import**。
- **现状**：**未治本**（本轮新定位）。修法方向：识别「类的简单名本身含 `$`」与「摊平产生的 `$`」之别，对前者
  按完整二进制名生成 import / 全限定引用。

### D3 · 扁平嵌套泛型类丢外层类型参数（`cannot find symbol: class K/V/E/S`）

- **影响面**：gson `cannot find symbol` 桶里约 **12 行**（`class K` 6 + `class V` 6）；guava 同形态另有一族
  （`HashIterator` / `Segment` / `Itr`）。
- **真实样例**（gson）：`LinkedTreeMap$LinkedTreeMapIterator.java:9: LinkedTreeMap$Node<K, V> next; →
  cannot find symbol: class K`。
- **根因**：非静态内部类 `LinkedTreeMapIterator` 引用外层类 `LinkedTreeMap<K,V>` 的类型参数 `K,V`；被摊平成
  独立顶层单元后，`K,V` 在该单元里**无处声明**。
- **现状**：JavaJive 已对「**自身无形参**」的纯继承内部类注入自由类型变量（`JDEC_INNER_TYPEVAR_OFF`，历史
  治本约 2000 行）；但「**自身又有形参**」的 `Iterator<T>` 一类仍是残余——若把外层 `K,V` 并入声明，别处对它的
  引用元数会不一致（"wrong number of type arguments"），需跨类协同重写所有引用点，深且高风险，留专项。

### D4 · 泛型边界越界 / 通配符捕获（guava）

- **影响面**：guava `type argument … is not within bounds of type-variable C`（约 89 行）；通配符捕获
  `CAP#1`（约 40 行）。
- **真实样例**：`ImmutableRangeMap$1.java:21`（边界越界）；`Equivalence$Wrapper` 的
  `this.equivalence.equivalent(a,b)`（`Equivalence<? super T>` 捕获成 `CAP#1`）。
- **根因**：扁平嵌套类型丢了外层类型参数及其 bound；通配符接收者被 javac 捕获成不可命名的 `CAP#1`，对其实参
  无法造型。
- **现状**：**通配符捕获经 oracle 实证为内在难 case**——`TestThirdPartyOracle/guava/Equivalence$Wrapper`
  下 **三方（JavaJive/CFR/Vineflower）全部重编译失败**（真源码靠 `@SuppressWarnings` + 强制造型），故优先级
  下调；边界越界需在扁平单元上重建被擦掉的类型参数声明与 bound。

### D5 · 三元 LUB（`bad type in conditional expression`）

- **影响面**：fastjson2 约 11 行 + guava 约 12 行 + gson 6 行。
- **根因**：`cond ? a : b` 两臂的最小公共上界（LUB）算窄，javac 拒绝。已有 `CommonSuperType` 设施，需扩表 +
  在更多合流点接入。**部分治本**（`JDEC_TYPELUB_OFF` / `JDEC_TERNARY_DECL_LUB_OFF`）。

### D6 · 活跃区间分裂 / 槽位复用类型混淆（fastjson2 `bad operand type` / `unexpected type`）

- **影响面**：fastjson2 `bad operand type for operator`（约 14 行）、`unexpected type`（约 9 行），同源。
- **真实样例**：`JSONPathFilter$GroupFilter` 的 `var9` 既声明为 `Iterator` 又被 `(var9) != (0)` 当 int 比较
  （`bad operand: Iterator,int`）。
- **根因**：一个字节码 local 槽在**互不相交的活跃区间**里先后承载**不兼容类型**的值，反编译却合成了**单一
  变量名 + 单一声明类型**。
- **现状**：须在变量定型/分裂核（`JDEC_LIVEINTERVAL_*`，已治本 fastjson2 数百行）上按「区间+类型」更激进地拆
  分同槽，风险高、改动核心，留专项。

### D7 · 循环-switch 结构化（`break outside switch or loop`，fastjson2 约 31 行）

- **真实样例**：`JSONReader.java:1148`。根因：标号 break / 复杂循环-switch 嵌套结构化把 `break` 落到了
  循环/switch 之外。属循环重建长尾。

### 非缺陷 · 环境假阳性：`sun.misc.Unsafe`（guava 约 45 行）

- guava 的 `Striped64` / `UnsafeByteArray` / `UnsafeAtomicHelper` / `UnsafeComparator` 等被**忠实反编译**出
  `import sun.misc.Unsafe; … Unsafe.getUnsafe()`，但 harness 用 `javac --release 8` 编译——其 `ct.sym`
  **不含 `sun.*` 内部包**，故报 `程序包 sun.misc 不存在 / cannot find symbol: class Unsafe`。
- **这是 `--release` 编译模式的环境产物，不是反编译缺陷**：任何忠实反编译器（CFR/Vineflower 同样）在
  `--release 8` 下都过不了；裸 `javac` 仅警告；真实重打包用含 `sun.misc` 的 JDK 即可。它**计入**上表 guava 的
  缺陷类里（口径统一），但**不应算作可治本缺陷**。

### 非缺陷 · 内在难 case（三方同样失败）

- spring `EmitUtils$6`（合成匿名内部类的 `this.val$e` field-read pop）、guava `Equivalence$Wrapper`
  （通配符捕获）等，`TestThirdPartyOracle` 实证 **CFR 与 Vineflower 亦失败**，属字节码内在难结构化，留长尾，
  不为它冒结构化回归风险。

---

## 5. 数据可信度与复现

- 本报告所有数字均由本仓库测试 `test/cross/benchmark_test.go`（主口径 Table A）与
  `test/cross/jar_inventory_test.go`（错误分桶 / 代表样例）自动产出，**无手工填写**；任何人按上述命令在同
  环境可复现（绝对值会随 JDK 版本、jar 版本与机器小幅浮动，相对关系稳定）。
- 自托管算法源码在 `test/cross/testdata/algorithms/`，可直接 `javac` / `java` 独立核对。
- JavaJive 单点治本的承重测试与 A/B kill-switch 见 `test/cross/jar_recompile_test.go`
  （`TestJarRecompileDelta`）与 `classparser/*_test.go`；正确性方法论见 `HARNESS.md`，缺陷账本见
  `classparser/CODEC_TODO.md`。

复现缺陷分桶（任一 jar）：

```bash
PROFILE_JAR=gson  go test -run TestJarTreeInventory -v ./test/cross/   # gson 缺陷直方图 + 代表样例
PROFILE_JAR=guava go test -run TestJarTreeInventory -v ./test/cross/
```

## 6. 结论

按**最客观的「缺陷 class 数」口径**：JavaJive 已能把**规整代码库**（commons-codec / spring-core / jsoup /
snakeyaml）反编译成**可重新编译、可重新打包**的源码，commons-codec 上做到三方唯一 0 缺陷，spring-core 与
Vineflower 并列最佳；并在自托管算法上做到**反编译→重编译→执行 逐字节一致**的语义保真。

与此同时，JavaJive 在**泛型密集库**（guava / fastjson2）与 **gson / commons-lang3** 上仍明显落后于 CFR，
缺陷集中在 §4 列出的几类明确根因（泛型擦除缺造型、`$` 命名类 import 缺失、扁平嵌套类丢外层类型参数等）。
后续将以 `HARNESS.md` 的「一次一个单点治本 + A/B + 承重测试」方法论持续收敛，逐步逼近 CFR / Vineflower。

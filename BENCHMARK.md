# JavaJive 反编译器评测报告（Benchmark）

本报告衡量 JavaJive 的**核心目标 —— 反编译正确性**：一个 `.class` / `.jar` 被反编译成
Java 源码后，能否被 `javac` 重新编译回去、重新打包、并且**运行出与原程序逐字节一致的结果**。

报告包含两个独立实验，并与业界两款主流反编译器 **CFR** 与 **Vineflower（Fernflower 的活跃分支）**
做同口径对照：

1. **实验一 · 自托管算法往返正确性**（语义级铁证）：对自实现的 MD5 / SHA-256 / CRC32 / 快速排序 /
   Base64 等算法，走「源码 → 编译 → 运行（基准） → JavaJive 反编译 → 重新编译 → 运行（往返）」，断言
   两次运行输出**逐字节一致**。这证明反编译产物不仅"能编过"，而且**语义保真、可执行、结果正确**。
2. **实验二 · 大规模三方可重编译对照**：在 8 个真实流行 jar（commons-codec / gson / commons-lang3 /
   jsoup / snakeyaml / spring-core / fastjson2 / guava）上，三方各自反编译整包后重编译，**以「有多少个 class
   编不回去」为主口径**报告缺陷规模（计分用**抗 javac 阶段遮蔽**的公平口径，见 §3.1）。

> ## 一句话结论（按缺陷 class 数 · 抗阶段遮蔽的公平口径）
>
> 三方在 8 个 jar、合计约 **2252** 个顶层类上的**缺陷类数**（越低越好）：
> **JavaJive 172 < Vineflower 209 < CFR 457**。**JavaJive 在全部 8 个 jar 上均优于 CFR**，总缺陷类数三方最少。
>
> commons-codec **0 缺陷（三方唯一 100%）**、spring-core **1 缺陷（三方最佳）**、**gson 3（三方最佳，反超
> Vineflower 16 / CFR 24）**；guava 83 / fastjson2 48 **均优于 CFR**（154 / 90）；落后于 Vineflower 的是 guava /
> commons-lang3 / fastjson2 / jsoup / snakeyaml 这几个库。自托管算法往返 **5/5 逐字节一致**，证明语义正确性。

> **⚠ 本版较上一版有重大方法论修正（见 §3.1）**：上一版用「整包一次性 `javac`」对三方计分，报出
> 「CFR 36 < Vineflower 123 < JavaJive 208、JavaJive 落后 CFR」。该对照被 **javac 编译阶段遮蔽**严重扭曲——
> CFR/Vineflower 各自产出若干**语法（parse 阶段）错**的类，javac 一旦遇到 parse 错就**在解析阶段后中止、不进入
> attribution（类型检查）阶段**，于是它们自己**全部的 sun.misc / 泛型 / 类型 attribution 错被整批遮蔽不报**；
> 而 JavaJive 产物**语法干净**、attribution 完整运行，把每一处类型错都如实暴露。换言之上一版**惩罚了"诚实编出可解析
> 产物"的工具**。本版改用**抗阶段遮蔽的公平口径**重测（CFR/Vineflower 因内联可按外层类逐个 iso 编译，免遮蔽），
> 真实结论反转：**JavaJive 全面优于 CFR**。

> **为什么用「缺陷 class 数」而非「错误行数」**：错误行数会把"少数几个类里的大量错误"和"大量类各一处错误"混为
> 一谈。一个类只要有 **1 处** `javac` 错误就无法重编译、无法重打包，**缺陷类数=有多少个类不可用**，更贴近工程
> 现实。本报告以**缺陷 class 数**为主口径，错误行数仅作上下文附录。

---

## 1. 实验环境

| 组件 | 版本 |
|---|---|
| OS / CPU | macOS 15.7.4 / arm64 (Apple Silicon) |
| JDK（javac / java） | OpenJDK 21.0.2（重编译统一 `--release 8`） |
| Go（构建 JavaJive 与 harness） | go1.22+ |
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

### 3.1 度量口径与公平性说明（含重大方法论修正：javac 阶段遮蔽）

**主口径 · 缺陷 class 数（packaging-independent，三方可比）。** 三方产出的**文件粒度不同**：JavaJive 把
每个嵌套类摊平成独立的顶层文件 `Outer$Inner.java`（文件数 ≈ class 数）；CFR / Vineflower 把嵌套类内联进
外层文件（一个文件 = 一个**外层类**）。因此**逐文件通过率不能横比**。本报告把所有产物**按外层（顶层）类
归一**——一个外层类只要它对应的任一单元（含被摊平的内部类）出现 `javac` 错误，就计为**一个缺陷类**——
得到与打包方式无关、三方同粒度的「缺陷 class 数」。这是**主口径**（表 A）。

#### 3.1.1 为什么不能用「整包一次性 javac」对三方计分（javac 阶段遮蔽）

`javac` 是**分阶段**编译器：先 **parse（解析）**所有源文件，再 **enter/attribution（类型检查）**。**关键事实：
只要编译集合里任一文件存在 parse（语法）错误，javac 在解析阶段后即中止，根本不进入 attribution 阶段**，于是
**整批文件**的 import 解析 / 泛型 / 类型错（attribution 阶段才检查）**全部被遮蔽、不报告**。

这对三方**不对称**：

- **CFR / Vineflower 各自产出若干语法错的类**（如 CFR 在 guava 上有 8 个类带 `'finally' without 'try'`、
  `'(' expected` 等结构化 bug）。在「整包一次性编译」里，这几个 parse 错**遮蔽了它们自己其余全部 attribution
  失败**（`import sun.misc.Unsafe` 缺包、泛型擦除缺造型……），整包只报出那几个语法错。
- **JavaJive 产物语法干净**（8 个 jar 上 parse 错为 0），attribution 阶段完整运行，于是把**每一处**类型错都如实
  暴露。

**硬证据**（guava）：CFR 整包计 **8** 个缺陷类；但把 CFR 产出的每个外层类**单独**编译（原 jar 在 classpath），
真实缺陷类是 **157**。单独编译 CFR 的 `Striped64.java` → 报 `程序包 sun.misc 不存在`；把它和一个语法错文件
**一起**编译 → 该错**消失**（parse 错遮蔽了 attribution 错）。即上一版的「CFR 8」是测量假象，CFR 真实缺陷
远高于此。

#### 3.1.2 本版的抗遮蔽公平口径

- **JavaJive：整树（tree）。** 所有摊平单元一起 `javac`（依赖在 classpath，原 jar 不在）。兄弟扁平 `$` 引用
  互相解析，产物可直接重打包。**因 JavaJive 产物零 parse 错，整树即已完整跑到 attribution、无任何遮蔽**，是其
  公平口径（也是「能否重打包」的真口径）。
- **CFR / Vineflower：逐外层类 iso。** 二者**内联**嵌套类，故每个产出文件 = 一个自洽的外层类（无摊平 `$` 跨引用）。
  把每个文件**单独**对「原 jar + 依赖」编译——既**免阶段遮蔽**（一个文件的 parse 错不会遮蔽另一文件的 attribution
  错），又**至少与整树一样宽松**（原 jar 提供全部跨类符号的精确签名，解析比 JavaJive 的兄弟产物更宽松）。故此口径
  对 CFR/Vineflower **保守有利**，结论「JavaJive 更优」是下界。

> JavaJive 用 tree、外部工具用 iso 看似不同，实则各取**对该工具公平且免遮蔽**的口径：JavaJive 因摊平只能整树
> 解析（iso 会有 flat-`$` 系统性假阳性，见 §4），外部工具因内联可逐类 iso（整树会被自己的 parse 错遮蔽）。该口径
> 由 `test/cross/benchmark_test.go` 自动产出（`scoreExternal` 逐文件 iso、`scoreJavaJive` 整树，注释详述阶段遮蔽）。

为完整起见保留两个细粒度视角作上下文：表 1 逐文件通过率（**不可跨工具比**，仅看各自自洽度），表 2 错误总行数
（仅作上下文，**不作主口径**）。

> 归一化细节：外层类 key = 去掉 `.java` 与 `$Inner` 后缀的包路径（如 `…/Maps$1.java`、`…/Maps.java` 都归到
> `…/Maps`）。三方的「外层类总数」分母基本相等（=各 jar 顶层类数），个别 jar 因某工具多/少发一个文件而有 ±1~2
> 的细微差异。

### 3.2 表 A · 缺陷 class 数（主口径 · 抗遮蔽公平口径，越低越好）

单元格 = **缺陷类 / 外层类总数（干净类率）**。缺陷类越少越好。**粗体 = 该 jar 三方最优。**

| jar | classes | JavaJive | CFR | Vineflower |
|---|---:|---:|---:|---:|
| commons-codec | 106 | **0/72 (100.0%)** | 10/72 (86.1%) | 2/72 (97.2%) |
| gson | 195 | **3/73 (95.9%)** | 24/73 (67.1%) | 16/74 (78.4%) |
| commons-lang3 | 345 | 28/198 (85.9%) | 46/198 (76.8%) | **6/198 (97.0%)** |
| jsoup | 238 | 3/51 (94.1%) | 5/51 (90.2%) | **2/51 (96.1%)** |
| snakeyaml | 231 | 6/122 (95.1%) | 11/123 (91.1%) | **2/121 (98.3%)** |
| spring-core | 978 | **1/649 (99.8%)** | 117/649 (82.0%) | 74/649 (88.6%) |
| fastjson2 | 681 | 48/529 (90.9%) | 90/530 (83.0%) | **41/529 (92.2%)** |
| guava | 1892 | 83/558 (85.1%) | 154/558 (72.4%) | **66/558 (88.2%)** |
| **合计** | | **172/2252 (92.4%)** | 457/2254 (79.7%) | 209/2252 (90.7%) |

> 读法：**JavaJive 在全部 8 个 jar 上均优于（或并列优于）CFR**——含曾"明显落后"的 guava（83<154）、
> fastjson2（48<90）、gson（3<24）、commons-lang3（28<46）。总缺陷类 **JavaJive 172 < Vineflower 209 < CFR
> 457**，JavaJive 三方最少。**gson 经本轮治本反超三方**（3 < Vineflower 16 < CFR 24，由曾经的最差变为最佳）。
> JavaJive 仍落后 Vineflower 的是 commons-lang3 / fastjson2 / jsoup / snakeyaml / guava 这几个库（Vineflower
> 在普通业务代码的结构化更成熟）；而在 commons-codec / gson / spring-core 上 JavaJive 反超 Vineflower。
>
> **本轮（gson/fastjson2/guava 收敛）进展**：以「dup2 多临时拼接 + 通配符返回造型 + 通配符字段存储造型 +
> 扁平内部类外层形参元数对齐」四处单点治本，**gson 缺陷类 8→3、fastjson2 50→48**，并把 guava 错误行 290→273
> （扁平内部类元数对齐顺带修了一批 `wrong number of type arguments`，逼近清零）。全部 kill-switch + 承重测试 +
> 全量 A/B 八 jar 零回归，见 `classparser/CODEC_TODO.md` §2。CFR/Vineflower 数字为本版同口径全量复跑所得。

### 3.3 表 1 · 逐文件通过率（干净编译文件 / 产出文件；**不可跨工具比**，仅看各自自洽度）

| jar | classes | JavaJive | CFR | Vineflower |
|---|---:|---:|---:|---:|
| commons-codec | 106 | 106/106 (100.0%) | 62/72 (86.1%) | 70/72 (97.2%) |
| gson | 195 | 177/183 (96.7%) | 50/74 (67.6%) | 59/75 (78.7%) |
| commons-lang3 | 345 | 310/339 (91.4%) | 152/198 (76.8%) | 192/198 (97.0%) |
| jsoup | 238 | 144/148 (97.3%) | 46/51 (90.2%) | 49/51 (96.1%) |
| snakeyaml | 231 | 219/231 (94.8%) | 112/123 (91.1%) | 119/121 (98.3%) |
| spring-core | 978 | 973/974 (99.9%) | 532/649 (82.0%) | 575/649 (88.6%) |
| fastjson2 | 681 | 631/681 (92.7%) | 440/530 (83.0%) | 488/529 (92.2%) |
| guava | 1892 | 1678/1825 (91.9%) | 404/558 (72.4%) | 492/558 (88.2%) |

> 注意：JavaJive 的分母是「摊平后单元数」(≈ class 数)，CFR/Vineflower 的分母是「外层类数」(少得多)，
> **故本表的百分比口径不同，不能直接横比**，只表示各工具产物「自身有多少比例能干净编过」。要横比请看表 A。
> CFR/Vineflower 此列为逐文件 iso 通过率（同主口径，已去阶段遮蔽）。

### 3.4 表 2 · `javac` 错误总行数（仅作上下文，**不作主口径**）

| jar | classes | JavaJive | CFR | Vineflower |
|---|---:|---:|---:|---:|
| commons-codec | 106 | **0** | 17 | 2 |
| gson | 195 | 24 | 103 | **22** |
| commons-lang3 | 345 | 87 | 287 | **10** |
| jsoup | 238 | 4 | 35 | **3** |
| snakeyaml | 231 | 39 | 33 | **2** |
| spring-core | 978 | **2** | 764 | 614 |
| fastjson2 | 681 | **230** | 623 | 297 |
| guava | 1892 | 273 | 865 | **132** |
| **合计** | | **659** | 2727 | 1082 |

> CFR/Vineflower 的行数为逐文件 iso（去遮蔽）合计；上一版被遮蔽时 CFR 仅 393 行、guava 仅 29 行（假象）。
> **错误行数仍会误导**：commons-lang3 上 JavaJive 行（87）少于 CFR（287），但**缺陷类数** JavaJive（28）仍多于
> Vineflower（6）——行数散在多少个类里才决定可用性。这正是以「缺陷 class 数」为主口径的原因。

### 3.5 分析（诚实结论）

- **JavaJive 全面优于 CFR**：8 个 jar 上缺陷类数**无一落后** CFR——codec 0<10、gson 3<24、commons-lang3
  28<46、jsoup 3<5、snakeyaml 6<11、spring 1<117、fastjson2 48<90、guava 83<154。CFR 的泛型重建并不比
  JavaJive 成熟，上一版「CFR 最稳」是阶段遮蔽假象（CFR 自身的语法错把它的泛型/`sun.misc` 失败整批遮蔽了）。
- **总缺陷类三方最少**：JavaJive 172 < Vineflower 209 < CFR 457。
- **JavaJive 的强项 —— 规整 / 大型代码库**：commons-codec **0 缺陷类（三方唯一 100%）**，spring-core
  **1 缺陷类（三方最佳）**，**gson 反超三方（3 vs 16 vs 24）**，codec / gson / spring 上反超 Vineflower。
- **JavaJive 仍落后 Vineflower 的场景 —— 结构化长尾**：commons-lang3（28 vs 6）、guava（83 vs 66）、
  fastjson2（48 vs 41）、jsoup（3 vs 2）、snakeyaml（6 vs 2）。残余集中在**泛型擦除 → 缺造型**、
  **扁平嵌套类丢外层类型参数（D3）**、**槽位复用/变量合流定型（D6）**、**循环/三元结构化长尾**几类（详见 §4）。
- **CFR 真实最弱**（去遮蔽后 457），尤其 spring（117）、guava（154）、fastjson2（90）大量 attribution 失败；
  Vineflower 居中（209），在普通业务代码上结构化最成熟。
- **整体**：按抗遮蔽公平口径，缺陷类数 **JavaJive(172) < Vineflower(209) < CFR(457)**。结合实验一的 5/5
  语义保真，JavaJive 是一款**总体可重编译率三方最优、且全面优于 CFR**的反编译器，正按 `HARNESS.md` 方法论继续
  收敛与 Vineflower 在几个非泛型密集库上的差距（已治本项见 `classparser/CODEC_TODO.md` §2）。

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
- **现状**：JavaJive 已治本**多块**（返回点 Object 向下造型、JDK/同类/继承超类型/私有方法的实参造型、
  统一跨类泛型解析器、擦除型类型变量多余 upcast 抑制、`cast()` 重参数化返回造型、类字面量返回造型等，
  见 `CODEC_TODO.md` §2，累计已削减数百行）。**本轮新增三块（gson/fastjson2 收敛）**：(i) 通配符返回造型——
  同类 `this.helper()` 返回 `R<?>` 而方法声明返回 `R<T>` 时补 `(R<T>)`（gson `JsonAdapterAnnotationTypeAdapterFactory`）；
  (ii) 通配符参数化字段存储造型——同类字段 `Class<? super T>` 存入 `Class<?>` 时补 `(Class<? super T>)`
  （gson `TypeToken`，连带 fastjson2 -1）；(iii) dup2 多临时拼接（复合数组自增的栈/图 bug，见下）。
  **残余**：接收者本身的泛型未被传播复原成参数化类型、通配符捕获 `CAP#1`（见 D4）等长尾，业界 CFR/Vineflower
  亦非全解。

### D2 · `$` 命名类的 import / public 缺失（gson `$Gson$Types` / `$Gson$Preconditions`）—— **本轮已治本**

- **影响面（治本前）**：gson `cannot find symbol` 桶约 **42 行**（`$Gson$Types` / `$Gson$Preconditions` 跨包引用 +
  `is not public`），是 gson 一度成为 JavaJive 最差 jar（68.5%）的主因。
- **真实样例（原）**：`GsonBuilder.java: $Gson$Preconditions.checkNotNull(...)` → `cannot find symbol:
  variable $Gson$Preconditions`；以及 `$Gson$Preconditions is not public`。
- **根因**：gson 有真实**简单名里就含 `$`** 的顶层类（`$Gson$Types`、`$Gson$Preconditions`）。JavaJive 用 `$`
  作为「外层类$内部类」摊平分隔符：(1) import 生成把名字里的字面 `$` 误当嵌套分隔（`binaryNestedNameToSource`
  返回 `ok=false`）跳过 import；(2) `DumpClass` 把含 `$` 的顶层类误当嵌套类剥掉 `public` 修饰。
- **现状**：**已治本**。`GetAllImported` 引入 `isAnonymousOrLocalBinaryName` 区分「数字段=匿名/局部」与「合法
  `$` 前缀顶层类」，对后者按完整二进制名生成 import；`DumpClass` 的 `$` 分支显式判 `isNested`，非嵌套则保留
  原 ClassFile 访问标志（保住 `public`）。承重 `dollar_class_and_classlit_test.go`，A/B 全 jar 零回归。这是 gson
  从最差跃升为三方最佳的关键一步（23→8）。

### D3 · 扁平嵌套泛型类丢外层类型参数（`cannot find symbol: class K/V/E/S`）—— **gson 剩余主因（部分本轮治本）**

- **影响面**：gson 剩余 3 缺陷类里 **2 类**仍与此相关（`LinkedTreeMap` / `LinkedHashTreeMap` 的
  `$LinkedTreeMapIterator` 引用外层 `K,V`）；guava 同形态另有一族（`HashIterator` / `Segment` / `Itr`）。
- **真实样例**（gson 残余）：`LinkedTreeMap$LinkedTreeMapIterator.java:9: LinkedTreeMap$Node<K, V> next; →
  cannot find symbol: class K`（迭代器自身已有形参 `<T>`，再注入外层 `K,V` 会与引用点 `<元素类型>` 元数冲突）。
- **根因**：非静态内部类引用外层类的类型参数；被摊平成独立顶层单元后，外层类型变量在该单元里**无处声明**（或
  引用点元数与摊平声明不一致）。
- **本轮治本（扁平内部类外层形参元数对齐）**：非静态内部类的**引用点**总是带**完整**外层实参集
  （`LOuter<TK;TV;>.Inner;`），但注入用的用量扫描只复原内部类体**实际提及**的子集——于是 `KeySet`（只用 K）声明
  成 `<K>` 却被引用成 `<K,V>`（"wrong number of type arguments; required 1"），`GsonContextImpl`（一个都不用）
  声明成裸名却被引用成 `<T>`（"does not take parameters"）。修法：当用量子集 ⊆ 最近泛型外层类的形参集时，
  整体采用该外层类的**完整有序**形参（跨类 `foldSiblingResolver` 取得）。**效果：gson `TreeTypeAdapter` 清零
  （4→3），并把 gson `KeySet` 与 guava 一批同形态 `wrong number of type arguments` 一并修掉（guava 错误行
  290→273，虽未整类清零但显著逼近）**。kill-switch `JDEC_INNER_ENCLOSING_ARITY_OFF`，承重
  `TestInnerEnclosingArityIsLoadBearing`（空用量 + 部分用量两态），A/B 八 jar 零回归。
- **残余**：「**自身又有形参**」的 `Iterator<T>` 一类——把外层 `K,V` 并入声明会与引用点 `<元素类型>` 元数冲突，
  需跨类协同重写所有引用点，深且高风险，留专项。这是 gson 清零的最后主障之一（另一为 `LinkedTreeMap` 的
  `NATURAL_ORDER` 造型 D1、`SqlTypesSupport` 的 D6 try-catch 变量合流定型）。
- **已对「自身无形参」的纯继承内部类注入自由类型变量**（`JDEC_INNER_TYPEVAR_OFF`，历史治本约 2000 行）作为本族基础。

### D3.1 · dup2 复合数组自增的栈/图 bug（`int[] cannot be converted to int`）—— **本轮已治本**

- **影响面（治本前）**：gson `JsonReader.pathIndices[stackSize-1]++` 一族产出 `var1[var1] = var1[var1] + 1`
  （`int[]` 当下标）；连带 fastjson2 同惯用法。gson 8→6、fastjson2 50→49。
- **根因**：一条 `dup2` 为复合数组存储**同时物化数组引用与下标两个临时**时，连发的两条 `AssignStatement` 共享
  `node.Id == opcode.Id`，`idToNode` 只留最后一个（数组节点），图连边只到达数组节点，下标赋值节点成孤儿被丢弃，
  其 `JavaRef` 永不改名，与数组临时撞名。
- **现状**：**已治本**。图连边后扫描同 `Id` 的 dup-family 节点组，把孤儿按发射顺序重新拼回主节点之前
  `preds → 下标 → 数组 → store`。kill-switch `JDEC_DUP_MULTI_TEMP_SPLICE_OFF`，承重
  `TestDupMultiTempSpliceIsLoadBearing`，A/B 八 jar 零回归。

### D4 · 泛型边界越界 / 通配符捕获（guava）

- **影响面**：guava `type argument … is not within bounds of type-variable C`（约 89 行）；通配符捕获
  `CAP#1`（约 40 行）。
- **真实样例**：`ImmutableRangeMap$1.java:21`（边界越界）；`Equivalence$Wrapper` 的
  `this.equivalence.equivalent(a,b)`（`Equivalence<? super T>` 捕获成 `CAP#1`）。
- **根因**：扁平嵌套类型丢了外层类型参数及其 bound；通配符接收者被 javac 捕获成不可命名的 `CAP#1`，对其实参
  无法造型。
- **现状**：**边界越界（`is not within bounds`）本轮已治本**——扁平内部类注入外层类型变量时一律默认成 `Object`
  bound，导致 `Range<C>`（`Range<C extends Comparable>`）越界；`enclosingTypeParamBounds` 沿 `$` 链取外层类
  真实 bound 重建 `<C extends Comparable<?>>`，清空 guava `not within bounds` 三桶（54+31+5=90 行，覆盖
  TreeRangeSet / TreeRangeMap / ImmutableRangeSet / ImmutableRangeMap），**guava tree 478→365（-113）、
  blockerUnits 190→171**，它 jar 零回归（kill-switch `JDEC_INNER_TYPEVAR_BOUND_OFF`，承重
  `TestInnerTypeVarBoundIsLoadBearing`）。残余的**通配符捕获**经 oracle 实证为内在难 case——
  `TestThirdPartyOracle/guava/Equivalence$Wrapper` 下 **三方（JavaJive/CFR/Vineflower）全部重编译失败**
  （真源码靠 `@SuppressWarnings` + 强制造型），优先级下调。

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
- **这是 `--release` 编译模式的环境产物，不是反编译缺陷**：任何忠实反编译器在 `--release 8` 下都过不了；裸
  `javac` 仅警告；真实重打包用含 `sun.misc` 的 JDK 即可。它**计入**上表 guava 的缺陷类里（口径统一）。
- **它对 JavaJive vs CFR 是平局**：CFR 同样产出 `import sun.misc.Unsafe`（实测其 `Striped64`/`LittleEndianByteArray`/
  `UnsignedBytes`/`AbstractFuture` 等文件单独 iso 编译同样报 `程序包 sun.misc 不存在`）。上一版 CFR 之所以不计这些，
  纯粹是其自身语法错把它们遮蔽了（§3.1）。本版公平口径下三方在这 ~6 个 sun.misc 类上**同败**，不影响 JavaJive 相对
  CFR 的领先，故**不应算作可治本缺陷**、亦不投入治理。

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

按**抗阶段遮蔽的「缺陷 class 数」公平口径**（§3.1）：三方在 8 个真实 jar 上的总缺陷类数
**JavaJive 172 < Vineflower 209 < CFR 457**，**JavaJive 三方最少、且在每个 jar 上均不弱于 CFR**。
commons-codec 三方唯一 0 缺陷、spring-core 三方最佳、guava/fastjson2/gson/commons-lang3 全面优于 CFR；
并在自托管算法上做到**反编译→重编译→执行 逐字节一致**的语义保真。

> 上一版报告（CFR 36 < Vineflower 123 < JavaJive 208、"JavaJive 落后 CFR"）是 **javac 阶段遮蔽**导致的测量
> 失真：外部工具自身的语法错把它们的泛型/`sun.misc`/类型 attribution 错整批遮蔽，使其虚高。本版去遮蔽后结论
> 反转。这条修正同时被沉淀进 harness（`benchmark_test.go` 外部工具改逐文件 iso）——以后再不会被遮蔽误导。

JavaJive 仍落后 Vineflower 的是 commons-lang3 / gson / jsoup / snakeyaml / guava 这几个**非泛型密集**库的
结构化与 import 长尾。后续将以 `HARNESS.md` 的「一次一个单点治本 + A/B + 承重测试」方法论持续收敛这部分差距
（本轮已治本 `cast()` 重参数化返回造型，guava 94→90；其余见 `classparser/CODEC_TODO.md` §2）。

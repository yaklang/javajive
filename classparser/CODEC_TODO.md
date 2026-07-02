# JavaJive 反编译器正确性账本 (CODEC_TODO)

> **北极星**: 复杂 JAR 做到「反编译 → `javac` 重编译 → 重新打包成 jar → 被外部 JVM 加载/校验/调用」全链路正确。
>
> - 度量口径与长尾清零方法: 见根目录 [`HARNESS.md`](../HARNESS.md)。
> - 当前可执行的缺陷工单(选靶 + 复现命令): 见根目录 [`TODO.md`](../TODO.md)。
> - 面向用户的评测报告(类级干净率 + 往返 + 三方横评): 见根目录 [`BENCHMARK.md`](../BENCHMARK.md)。
>
> 本文件只登记**当前真实状态 + 剩余缺陷分类 + 生效中的安全开关**。所有数字由 harness 真实跑出(`javac`, 本机 `~/.m2`), 禁止估算或编造; 绝对值随 JDK / jar 版本浮动, 以趋势为准。

---

## 0. 为什么用整树(tree)口径, 而不是逐文件(iso)

反编译器把嵌套类发成**独立的扁平单元** `Outer$Inner.java`(见 `dumper.go` 架构注释)。这带来两种度量:

- **tree(整树)**: 所有扁平单元一起 `javac`(依赖在 classpath)。兄弟扁平 `$` 引用互相解析得到, 产物可直接重打包。**这是「能否重打包」的真口径。**
- **iso(逐文件)**: 每个单元单独 `javac`。`Outer$Inner` 这种扁平 `$` 类型引用在原始 jar 里按源名 `Outer.Inner` 索引、解析不到, 于是报海量 `cannot find symbol`/`private access`。**这是 iso 口径的系统性假阳性, 不是反编译缺陷, 也不阻碍重打包**(详见 §3)。

> 治本与验收以 **tree errLines / 缺陷类数** 为准; iso 仅用于侧写。**syntax(语法/词法)错必须为 0**: 任一语法错会令 `javac` 在 attribution 前全局中止、遮蔽同批文件的全部类型错, 使缺陷类数变成乐观低估(基准 `TestBenchmarkSelfRecompile` 对 syntax≠0 硬断言失败)。

---

## 1. 当前真实状态(8 个基准 JAR, tree 口径)

单元格口径见 [`BENCHMARK.md`](../BENCHMARK.md)。缺陷类 = 摊平后任一单元有 `javac` 错误的外层类; errLines = tree 重编译总错误行(仅上下文)。

| jar | classes | 缺陷类 | tree errLines | syntax | 重打包(repackage) |
|---|---:|---:|---:|---:|---|
| **commons-codec** 1.15 | 106 | **0** | **0** | 0 | ✅ **完整往返**(107/107 verify + 调用差分逐字节一致) |
| **gson** 2.8.9 | 195 | **0** | **0** | 0 | ✅ **完整往返**(199/199 verify) |
| **commons-lang3** 3.12.0 | 345 | 22 | 75 | 0 | 泛型擦除长尾 |
| **jsoup** 1.10.2 | 238 | 1 | 1 | 0 | 单类长尾 |
| **snakeyaml** 2.2 | 231 | 3 | 12 | 0 | 泛型/槽位长尾 |
| **spring-core** 5.3.27 | 978 | 82 | 739 | 0 | cglib 内部类 import/access 为大头 |
| **fastjson2** 2.0.43 | 681 | 15 | 33 | 0 | 泛型擦除 + 槽位复用长尾 |
| **guava** 28.2-android | 1892 | 22 | 34 | 0 | 泛型擦除/边界 + 扁平内部类长尾 |
| **合计** | | **145** | **894** | **0** | 类级干净率 **93.6%**(2107/2252) |

**codec 与 gson 已证北极星全链路**(承重于 `test/cross/jar_roundtrip_test.go`):
`decompile → javac 重编译(0 error) → archive/zip 重打包 → java -Xverify:all 逐类加载校验全通过`; codec 更经调用差分(Base64 / Hex / MD5 / SHA-256)与原始 jar 逐字节一致。

> CI 常驻承重: `TestSyntheticJarRoundTrip`(无需 `~/.m2`)对一个含枚举+switch / 泛型 / lambda / varargs / try-catch 的多类程序跑完整往返, 断言运行输出逐字节一致 + 全类 verify, 守住往返能力永不回归。

---

## 2. 剩余缺陷(tree 口径, 按杠杆从大到小)

> 这些是真正阻碍重打包的缺陷。每项给「表象·代表类·初判根因·现状」。可执行工单(复现命令 + 优先级)在根 [`TODO.md`](../TODO.md)。已生效的治本对应的安全开关见 §4。

1. **泛型擦除缺造型 `Object cannot be converted to T/K/CAP#1`** — **最大杠杆, 跨 jar**。
   - 表象: `incompatible types (assignment/return)` 桶(commons-lang3 / fastjson2 / guava 的主桶)。代表: `Object cannot be converted to LinkedHashTreeMap$Node<K,V>` 一族。
   - 根因: 字节码泛型擦除后取值点静态类型是 `Object`, 未补回源码原有的 `(T)` / `(Node<K,V>)` 向下造型。需沿「接收者参数化类型 + 方法/字段 Signature + 跨类型层级替换」复原精确类型。
   - 现状: 已治本多块(返回点向下造型、JDK/同类/继承/私有方法实参造型、统一跨类泛型解析器 `ResolveInstantiatedParamType`、擦除型类型变量多余 upcast 抑制、参数化实参/数组实参造型等, 见 §4)。**残余**: 接收者自身泛型未被传播复原成参数化类型、通配符捕获 `CAP#1`(不可命名, 三方均败, 属内在难 case)、装箱数值(非擦除, 不可盲目造型)。

2. **活跃区间分裂 / 槽位复用类型混淆 `bad operand type` / `unexpected type` / `int cannot be converted to boolean`** — fastjson2 主要长尾。
   - 表象: 一个字节码 local 槽在**互不相交的活跃区间**里先后承载**不兼容类型**的值, 反编译却合成单一变量名 + 单一声明类型。例: `JSONPathFilter$GroupFilter` 的 `var9` 既作 `Iterator` 又被当 int 比较。
   - 现状: 已治本多族 disjoint 槽(兄弟臂 LUB 合并、Object 超类臂合并、数组协变父臂合并、布尔字段/返回槽拆分、跨作用域孤儿读全方法重放等, 见 §4)。**残余**: 非布尔子形态的「区间+类型」拆分须动变量定型/分裂核(`JDEC_LIVEINTERVAL_*`), 高风险, 留专项。

3. **三元 LUB `bad type in conditional expression`** — fastjson2 + guava 若干行。
   - 根因: `cond ? a : b` 两臂最小公共上界算窄, 或三元臂里的泛型擦除(`Optional.of(next())` 缺 `(E)` 等)。
   - 现状: 已治本反射家族(Method/Field/Constructor→Member)与跨类直接子类型两支(见 §4)。**残余**: 渲染期造型未反馈到三元类型的形态、以及三元臂泛型擦除, 归入第 1 类长尾。

4. **扁平嵌套类丢外层类型参数 `cannot find symbol: class K/V/E/S`** — guava 一族(`HashIterator` / `Segment` / `Itr`)。
   - 根因: 非静态内部类引用外层类型参数, 被摊平成独立顶层单元后外层类型变量无处声明; 「自身又有形参」时注入外层 `K,V` 会与引用点元数冲突。
   - 现状: 已治本「自身无形参」的纯继承内部类(注入自由类型变量 + 外层形参元数对齐 + bound 重建, 见 §4)。**残余**: 「自身又有形参」的 `Iterator<T>` 一类须跨类协同重写所有引用点(integral rebuild), 深且高风险, 留专项。

5. **`for` 循环 `continue`-到-自增被丢弃** — gson `JsonWriter.string` 等循环重建长尾。
   - 根因: `for` 被渲染成 `do-while(true)` 且自增作为显式体语句, 内层 `continue` 无法表达为裸 `continue`(会跳过自增), 结构化遂丢弃分支致变量可能未初始化。
   - 现状: 须做 `for` 循环恢复(自增放进 for-update 槽)或等价的「continue-到-latch」结构化, 改动循环结构化核心, 影响所有 jar, 风险高, 留专项。

6. **合成内部类 `this.val$e;` field-read pop** — spring `EmitUtils$6` 单类。
   - 现状: `pop` 丢弃 `this.val$e` 字段读未被 elide; 已验证粗暴扩展 elide 集会引发 spring 大回归。oracle 旁证: CFR 与 Vineflower 对该合成匿名内部类**亦失败**, 属内在难 case, 留长尾。

7. **环境假阳性 `sun.misc.Unsafe`(非缺陷)** — guava `Striped64` / `UnsafeByteArray` 等约 45 行。
   - harness 用 `javac --release 8`, 其 `ct.sym` 不含 `sun.*` 内部包, 忠实反编译出的 `sun.misc.Unsafe` 用户编不过。**任何忠实反编译器同样过不了, 不是 JavaJive 缺陷**; harness 已补 `sun.misc` 垫片(`jdk_sunmisc_test.go`), 故不计入上表缺陷。

8. **其余小桶**: `method invocation cannot be applied`(重载消歧 + 通配符)、`invalid method reference`(构造器实参位 SAM 目标)、`abstract method not overridden`(桥接可见性)、`incompatible parameter types in lambda`(形参被用作具体类型 + raw 接收者)等, 逐类按 [`HARNESS.md`](../HARNESS.md) 流程清零。

---

## 3. iso 口径的已知假阳性(**不是缺陷**, 不要去"修")

iso 把每个扁平单元单独编译, 以下失败是方法学产物, 在 tree(重打包)口径下不存在:

- `cannot find symbol: class Outer$Inner` — 扁平 `$` 类型引用对着原始 jar 解析不到(jar 里是源名 `Outer.Inner` 的嵌套类)。
- `X has private/protected access in Y` — 内部类合法访问外层私有/保护成员, 单文件编译看不到同编译单元豁免。
- `cannot access X` — 需要兄弟类字节码而 iso classpath 未含其反编译产物。
- `程序包 sun.misc 不存在` / `cannot find symbol: class Unsafe` — 见 §2 第 7 项(`--release 8` 环境产物, 连 tree 口径也中招, 非缺陷)。

> iso 数**不是**北极星指标; 它只用来侧写"哪些类涉及跨类引用"。真正口径永远是 tree。

---

## 4. 生效中的安全开关(kill-switch)索引

> 每个非平凡治本都配 `JDEC_*_OFF` 开关(置位即关闭该治本, 用于 A/B delta 回归定位)。下表按主题归类, 只列开关与作用域。底层开关(`JDEC_SLOT_*` / `JDEC_*REACHING*` / `JDEC_IF*` / `JDEC_TRY_*` / `JDEC_LOOP_*` 等)用 `rg 'os.Getenv\("JDEC_' classparser/decompiler` 列全。

### 泛型擦除缺造型(第 1 类)
| 开关 | 作用域 |
|---|---|
| `JDEC_GENERIC_PARAM_INFER_OFF` | JDK 泛型方法实参的接收者参数化造型(总闸, 含 `Comparator.compare` 形参) |
| `JDEC_GENERIC_PARAM_FIELD_OFF` | 仅字段接收者旁路(从字段 Signature 还原类型参数) |
| `JDEC_GENERIC_SELFMETHOD_PARAM_OFF` | 同类自有泛型方法实参造型(公有+私有) |
| `JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF` | 仅私有同类方法(invokespecial 目标==当前类)扩展 |
| `JDEC_GENERIC_SUPER_METHOD_OFF` | 继承超类型泛型方法签名并入(this 接收者, 恒等映射一层) |
| `JDEC_GENERIC_RESOLVE_OFF` | 统一跨类泛型解析器(沿接收者泛型超类型 DFS + 类型实参替换 σ, 覆盖非-this/非恒等/深链) |
| `JDEC_OBJECT_RET_DOWNCAST_OFF` | 返回点 Object→具体引用类型向下造型 |
| `JDEC_THIS_REPARAM_CAST_OFF` | `cast()` 重参数化返回点造型(`(C<N1>) this`) |
| `JDEC_NO_ERASED_TYPEVAR_NOCAST` | 擦除型类型变量的多余 upcast 抑制(丢弃 `(Bound)` 无操作造型以保推断) |
| `JDEC_ENUM_COMPARETO_NOCAST_OFF` | 上一项 JDK 伴生: `Enum<E>.compareTo(E)` 实参不补 `(Enum)` 上行造型 |
| `JDEC_CLASSLIT_ARG_NOCAST_OFF` | 类字面量实参对 `Class` 形参不补 `(Class)` 造型 |
| `JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF` | 同类字段数组元素存储 `this.buf[i]=obj` 补 `(T)`(字段声明 `T[]`) |
| `JDEC_TYPEVAR_ARRAY_ARG_CAST_OFF` | 引用数组实参传给裸类型变量形参补 `(T)` |
| `JDEC_WILDCARD_RET_CAST_OFF` | 通配符返回造型(`R<?>`→`R<T>`) |
| `JDEC_WILDCARD_FIELD_CAST_OFF` | 通配符参数化字段存储造型(`Class<?>`→`Class<? super T>`) |
| `JDEC_GENERIC_SUPERWILDCARD_OFF` | `? super E` 消费者实参造型(取下界类型变量作 `(E)`) |
| `JDEC_SIBLING_DESC_SIG_OFF` | 兄弟类方法签名额外按 descriptor 收键做重载消歧(识出擦除型变形参、抑制有害 `(Comparable)` 造型) |

### 扁平内部类 / 泛型声明(第 4 类)
| 开关 | 作用域 |
|---|---|
| `JDEC_INNER_TYPEVAR_BOUND_OFF` | 扁平内部类注入的外层类型变量的 bound 重建(`<C extends Comparable<?>>`) |
| `JDEC_INNER_ENCLOSING_ARITY_OFF` | 扁平内部类外层形参元数对齐(`<K,V>`) |
| `JDEC_NESTED_PUBLIC_OFF` | 嵌套类型 public 复原 |

### disjoint 槽 / 活跃区间(第 2 类)
| 开关 | 作用域 |
|---|---|
| `JDEC_REF_SLOT_SIBLING_ARM_MERGE_OFF` | 兄弟臂引用 phi 合并到 LUB(`Method`/`Field`→`Member`) |
| `JDEC_REF_SLOT_OBJECT_SUPERTYPE_ARM_MERGE_OFF` | Object 超类臂合并(current 为 Object 时续用 Object) |
| `JDEC_OBJECT_ARM_PROVISIONAL_NARROW_OFF` | 上一项的 provisional-Object 收窄(current 是未 adopt 的 null-init ref 时收窄到具体臂类型) |
| `JDEC_REF_SLOT_ARRAY_COVARIANT_ARM_MERGE_OFF` | 数组协变父臂合并(`String[]`/`Object[]`→元素 LUB 数组) |
| `JDEC_BOOL_RETURN_SLOT_SPLIT_OFF` | 布尔方法 `return false/true` 复用不相交 int 槽时拆分 |
| `JDEC_BOOL_FIELD_SLOT_SPLIT_OFF` | boolean 标志(存入 `Z` 字段)复用不相交 int 循环计数器槽时拆分 |
| `JDEC_BOOL_SIBLING_ARM_MERGE_OFF` | try/catch 兄弟臂 boolean phi 合并 |
| `JDEC_BOOL_PARAM_REASSIGN_MERGE_OFF` | 布尔形参分支重赋 phi 合并 |
| `JDEC_ORPHAN_GLOBAL_REBIND_OFF` | 跨作用域孤儿读全方法唯一重放(补绑落在兄弟作用域的孤儿读) |
| `JDEC_NULLINIT_NARROW_OFF` | 孤儿/Object 生成局部按 AST reassignment 类型恢复声明类型 |
| `JDEC_COVER_UNDECLARED_OFF` | 同槽拆出的同名 `varN` 无声明时的名字作用域覆盖安全网 |
| `JDEC_LIVEINTERVAL_OFF` / `JDEC_LIVEINTERVAL_WEB` | 活跃区间声明摆放 / web 复用 |

### 三元 / 类型 LUB(第 3 类)
| 开关 | 作用域 |
|---|---|
| `JDEC_TYPELUB_OFF` / `JDEC_TERNARY_DECL_LUB_OFF` | 类型 LUB(含反射家族 Method/Field/Constructor→Member) / 三元声明点 LUB |
| `JDEC_TERNARY_DECL_LUB_CACHE_OFF` | 三元声明加宽时同步刷新三元 cachedType |
| `JDEC_TERNARY_DECL_LUB_CROSS_OFF` | 跨类(jar 内)三元 LUB 加宽(直接子类型关系下加宽到上界臂) |

### lambda / 方法引用 / 布尔
| 开关 | 作用域 |
|---|---|
| `JDEC_LAMBDA_CAPTURE_REBIND_OFF` | lambda/方法引用 `CustomValue` 转发 `ReplaceVar` 给捕获值(令捕获引用参与同槽拆分的 id 改写) |
| `JDEC_INSTANCEOF_REPLACEVAR_OFF` | `OP_INSTANCEOF` 的 CustomValue 转发 ReplaceVar 给操作数 |
| `JDEC_LAMBDA_CTX_RESTORE_OFF` | 内联 lambda 体懒解析后还原外围方法 FuncCtx |
| `JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF` | 未使用的 lambda 形参隐式渲染(`(Integer l0)`→`(l0)`) |
| `JDEC_LAMBDA_RAWRECV_CAST_OFF` | raw 接收者擦除 SAM 的方法调用侧实参造型(`(Consumer<FieldReader>)`) |
| `JDEC_CTOR_METHODREF_FIX_OFF` | 构造器方法引用 `::new` 渲染(修 `::new_`) |
| `JDEC_CTOR_DIAMOND_OFF` | 泛型类 `new` 带方法引用/lambda 实参时补菱形 `<>` |
| `JDEC_METHODREF_INSTANTIATED_TYPE_OFF` | 方法引用值类型从 invokedynamic instantiatedMethodType 上行为参数化 functional interface |
| `JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF` | 传给 `AccessController.doPrivileged` 的 lambda 补 `(PrivilegedAction)` 造型消歧 |
| `JDEC_BOOL_TO_INT_COERCE_OFF` | 内在布尔值赋 int 缺 `? 1 : 0` 造型 |
| `JDEC_BOOL_TO_INT_COERCE_EXPR_OFF` | 上一项结构性扩展支(比较/布尔调用/短路三元) |

### 结构化 / pop / switch / 枚举 / 注解
| 开关 | 作用域 |
|---|---|
| `JDEC_SWITCH_NONDOM_MERGE_BREAK_OFF` | switch 合并点的非支配前驱不插 break(修 `break outside switch or loop`) |
| `JDEC_SWITCH_SPURIOUS_DEFAULT_OFF` | 无 default 的 switch 不注入伪 `case math.MaxInt:`(修 `integer number too large`) |
| `JDEC_POP_ELIDE_OFF` | pop/pop2 裸值语句 elide(裸 local/常量/类引用 + `this.f` 单层实例字段读; 修 `not a statement`) |
| `JDEC_DUP_MULTI_TEMP_SPLICE_OFF` | dup/dup2 多临时拼接(复合数组自增同时物化数组+下标) |
| `JDEC_NULL_DUP_FOLD_OFF` | null 链式字段赋值临时抑制 |
| `JDEC_SYN_BRIDGE_THIS_OFF` | 合成 marker-only access-bridge 构造器空体补回 `this();` 委派 |
| `JDEC_RETURN_DECL_FIX_OFF` | 返回-嵌入赋值局部的声明合成 |
| `JDEC_CTOR_WILDCARD_CAST_OFF` | `NATURAL_ORDER` 构造器自调用通配符实参造型 |
| `JDEC_ANNO_ENUM_NESTED_DOT_OFF` | 注解值里外部嵌套枚举常量改点号源名(`Outer.Inner.CONST`) |
| `JDEC_ANNO_PRIMITIVE_CLASSLIT_OFF` | 注解值里 PRIMITIVE 类字面量原始渲染(`void.class`) |
| `JDEC_NO_ENUM_SWITCH_FOLD` | enum-switch `$SwitchMap` 折回 |
| `JDEC_NO_ENUM_FOLD` | enum 常量体子类内联 |
| `JDEC_GENERIC_INFER_OFF` | JDK 泛型方法返回实例化 |

### 性能(字节级等价, 无 kill-switch)
- `coverUndeclaredGeneratedLocals` 的单趟渲染记忆化(`stmtRenderMemo`, 树变更即失效)+ `strings.Index` 手写 ASCII 词边界取代 regexp: 超大方法体(fastjson2 `ObjectReaderBaseModule`)从 ~73s 降到 ~2.8s, 逐类 SHA-256 前后一致。承重 `TestCoverUndeclaredPerfGuard`(40s 时限, 病态版会超时失败)。

---

## 5. 验收红线(每轮治本必须全满足)

1. 选靶用 tree inventory, 单点根因, 不打过用例补丁。
2. 复杂改动配 `JDEC_*` kill-switch; A/B delta(`OFF-ON`)对**所有** jar 必 ≥ 0(本 jar 降、它 jar 不升)。
3. 配承重测试(ON/OFF 断言)+ 回归种子(`testdata/regression/*.class` + `.golden`)。
4. 全量 `go test ./...` 全绿; 确定性测试(逐类 SHA-256)不得变红; 基准 `syntax=0` 硬断言不得触发。
5. 安全契约: 永不输出不可解析的 Java; 宁可带标记 stub 也不输出"看似对实则错"; 不 panic。

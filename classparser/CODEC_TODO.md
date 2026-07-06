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
| **commons-lang3** 3.12.0 | 345 | 11 | 18 | 0 | 泛型擦除长尾 |
| **jsoup** 1.10.2 | 238 | 1 | 1 | 0 | 单类长尾 |
| **snakeyaml** 2.2 | 231 | 1 | 1 | 0 | definite-assignment 单点 |
| **spring-core** 5.3.27 | 978 | 29 | 55 | 0 | 泛型擦除造型 + 三元 LUB + bool/int 槽位长尾 |
| **fastjson2** 2.0.43 | 681 | 15 | 32 | 0 | 泛型擦除 + 槽位复用长尾 |
| **guava** 28.2-android | 1892 | 20 | 28 | 0 | 泛型擦除/边界 + 扁平内部类长尾 |
| **合计** | | **77** | **135** | **0** | 类级干净率 **96.6%**(2175/2252) |

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
| `JDEC_LAMBDA_PARAM_SCOPE_OFF` | 嵌套 lambda 形参按嵌套深度命名(`l<depth>_<i>`), 避免内层 `l0` 遮蔽外层 `l0`(javac「variable l0 is already defined」); 顶层 lambda 仍 `l<i>` 保持字节一致。修 spring MergedAnnotationPredicates/DataBufferUtils 等 |
| `JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF` | try/finally(或 synchronized)处理器栈值无法绑定到真实局部时渲染出的裸 `varN = Exception;`(ANTLR 语法网放行、javac 报「cannot find symbol」)提升为完整降级触发器: 该方法先激进重试, 失败则降级为诚实可编译 stub, 不再泄漏坏代码。修 guava Monitor.enterWhen/enterWhenUninterruptibly、InetAddresses(guava tree -3 行, 缺陷类 22→20) |
| `JDEC_NO_EMBED_ASSIGN_INT` | 缺声明安全网识别「条件内嵌赋值」目标为 int 的正则放宽到容忍一层 `()`, 使 `while ((c = this.read()) != -1)` / `(c = in.read()) < n` 这类 InputStream 抽水循环的 RHS(方法调用)被认出: 之前跨不过 `read()` 的括号, 回退成 `Object c = null` 导致 `bad operand types for '!='/'<'`。int 判定安全性不变(关系运算恒为数值; 相等式仍要求数值字面量右操作数)。修 spring-core UpdateMessageDigestInputStream(spring tree -1 行, 缺陷类 39→38) |
| `JDEC_LAMBDA_RAWRECV_CAST_OFF` | raw 接收者擦除 SAM 的方法调用侧实参造型(`(Consumer<FieldReader>)`) |
| `JDEC_CTOR_METHODREF_FIX_OFF` | 构造器方法引用 `::new` 渲染(修 `::new_`) |
| `JDEC_CTOR_DIAMOND_OFF` | 泛型类 `new` 带方法引用/lambda 实参时补菱形 `<>` |
| `JDEC_METHODREF_INSTANTIATED_TYPE_OFF` | 方法引用值类型从 invokedynamic instantiatedMethodType 上行为参数化 functional interface |
| `JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF` | 传给 `AccessController.doPrivileged` 的 lambda 补 `(PrivilegedAction)` 造型消歧 |
| `JDEC_BOOL_TO_INT_COERCE_OFF` | 内在布尔值赋 int 缺 `? 1 : 0` 造型 |
| `JDEC_BOOL_TO_INT_COERCE_EXPR_OFF` | 上一项结构性扩展支(比较/布尔调用/短路三元) |
| `JDEC_BOOL_INT_TERNARY_CMP_OFF` | `==`/`!=` 一侧为 boolean、另一侧为布尔物化三元 `cond ? 1 : 0`(含嵌套短路 `c1?(c2?1:0):0`)时折叠为 `boolVar op cond`: 之前渲染 `(boolVar) != ((cond)?(1):(0))` 被 javac 拒「incomparable types: boolean and int」。三元先经 `coerceBooleanArgument` 把 0/1 叶重定型为布尔、再 `boolReduce` 折回 `&&/\|\|/!` 连接式; 仅在物化(叶恒为 0/1)时触发, 只能修复不改语义。修 spring-core ASM MethodVisitor.visitMethodInsn / MethodWriter.canCopyMethodAttributes + commons-lang3 若干类(spring tree 缺陷类 38→36、错误行 80→77) |
| `JDEC_TYPEPARAM_BOUND_IMPORT_OFF` | 类头类型参数 BOUND(`<A extends Annotation>`)改用真实 funcCtx 渲染以注册 import: 之前用一次性空 ClassContext 渲染, 短名正确但 import 丢失, `java.lang.annotation.Annotation` 等非 java.lang 包 bound 重编译报「cannot find symbol」。修 spring MergedAnnotationSelector / MergedAnnotationPredicates$FirstRunOfPredicate(spring tree 错误行 77→75) |
| `JDEC_IFACE_DEFAULT_SUPER_OFF` | invokespecial 目标为「当前类直接实现的接口」的 default 方法时渲染 `Iface.super.m()` 而非裸 `super.m()`(裸 super 解析到超类, 报「cannot find symbol」)。经 SiblingSuperTypes 严格确认目标在直接接口列表内才触发。修 spring StandardAnnotationMetadata / StandardMethodMetadata / SimpleAnnotationMetadata 的 `super.getAnnotationTypes()` 族(spring tree 错误行 75→68、缺陷类 36→30) |
| `JDEC_NEW_RECV_DIAMOND_OFF` | RAW `new HashMap(typedMap)` 等 JDK 泛型集合类直接作 lambda 调用接收者时补菱形 `new HashMap<>(...)`: raw 接收者按 JLS 4.8 擦除方法的 functional-interface 形参, lambda 形参退化为 Object, 体内解引用报「Object cannot be converted to String」。菱形让 javac 从构造实参重新推断类型参数、重新绑定 lambda; 白名单限 HashMap/LinkedHashMap/TreeMap/ArrayList/LinkedList/HashSet/LinkedHashSet/TreeSet 且仅当本次调用带 lambda/方法引用实参。修 spring SimpleAliasRegistry `new HashMap(this.aliasMap).forEach(...)`(spring tree 错误行 68→65、缺陷类 30→29) |
| `JDEC_INNER_STANDALONE_ERASE_OFF` | 自带形参的扁平内部类, 外层类型变量作**独立类型**(字段 `K key`/`E nextEntry`、具体方法 `advanceTo(E)`)时渲染其 JVM 擦除(有界取首 bound 原始类经 resolver 沿 `$` 链回溯如 `InternalEntry`, 无界取 Object): 裸独立变量无 `<...>` 可去, 原样即未声明 K/E 报「cannot find symbol: class K」。例外: 抽象方法参数保留裸变量(擦成 Object 会与自带 K,V 的无形参兄弟子类重写 name-clash)。修 guava MapMakerInternalMap$HashIterator 整类 + AbstractMapBasedMultimap$Itr 字段(guava tree 错误行 31→28) |
| `JDEC_EXTERNAL_NESTED_DOT_OFF` | 第三方(非 JDK)嵌套类引用点号化 `Outer.Inner`: 以 SiblingSuperTypes(读恒存在的 super_class 项)判外层类是否在本 jar, 不在则该类只在 classpath 上以真正嵌套形态存在, 扁平 `Outer$Inner` 不可解析(「cannot find symbol」); 本 jar 内类仍保持扁平(Yak 摊平单元)。引用与 import 同步点号化。修 spring `reactor.blockhound.BlockHound$Builder`(spring tree 错误行 65→63) |
| `JDEC_NO_CLASSLIT_SLOT_TYPE` | class 字面量(`Foo.class`)作 rvalue 时定型为 `java.lang.Class`(而非被引类 `Foo`): 直接存储与**三元臂**两条路径共用。三元 `cond ? Foo.class : classField` 里 class 字面量臂的 `Type()` 报被引类(为驱动裸 `Foo.class` 渲染), 致朴素臂合并塌成两臂 LUB(`Object.class` vs `Class` 取 `Object`), 局部误声明 `Object c`, 后续 `c.getModifiers()/getName()` 报「cannot find symbol」。修法: 臂合并把 class 字面量臂计为 `java.lang.Class`(`TernaryArmRValueType`), 且声明处对含 class 字面量臂的三元优先取槽位 ref 已定型(`Class`, 从新鲜臂合并铸出的权威类型)。修 spring cglib `Enhancer.generateClass`(spring tree 错误行 63→57) |
| `JDEC_LAZY_INIT_SELF_TERNARY_OFF` | 懒初始化自守卫三元收窄: `x = (x != null) ? x : new Concrete()` 经 javac 编成条件存储, 控制流合流把槽位定型为 null-init 臂(`Object`)与 `new` 臂的 LUB(即 `Object`), 重建三元把 x 读回 `Object`, 后续 `x.add(..)` 报「cannot find symbol」。因槽位**唯一**具体值即 `new` 臂(另一臂是 null 守卫的自身), 把声明收窄到该臂类型安全(null 可赋给它、合流上不调任何臂特有成员)。形状门控(一臂为槽位自身 Id, 另一臂具体非 Object 引用), 只收窄不放宽。修 spring `StringDecoder`(spring tree 错误行 57→55) |
| `JDEC_REF_SLOT_JDK_SUBTYPE_ARM_MERGE_OFF` | JDK 子类型臂引用 phi 合并: 一互斥臂把 JDK 子类型分配(`new HashMap()`)存入槽, 另一臂持 JDK 超类型(`Map`, 来自 checkcast get), 两臂汇入合流后使用。jar 内子类型合并看不到 JDK 关系、兄弟臂合并在 LUB 等于某臂时退出, 故子类型臂分裂、合流读在该路径未赋值(「variable might not have been initialized」)。以 `CommonSuperType(current,val)==current` 证 val 严格子类型、仅认 `new` 分配臂、phi 门控(合流被两个 def 触达), 续用 current 不改类型。修 jsoup Whitelist.addProtocols/addAttributes |
| `JDEC_NULL_ADOPTED_SUBTYPE_REASSIGN_OFF` | null-init 槽收养具体引用类型 T 后, 又被 T 的**子类型**在某臂重赋(`InputStream in=null; in=pick(); if(gzip)in=new GZIPInputStream(in)`): null-adopt-once 守卫(为保 try-with-resources Throwable/Map.Entry 不相交复用而拆分)把子类型存当成新变量, 合流 `in.read()` 在非包装路径未赋值。子类型可赋给 T, 复用同 ref 保 T 声明; 限严格 JDK/已知子类型关系(`CommonSuperType(T,val)==T`)。配套在 hierarchy.go 补 java.io 流族(InputStream/OutputStream/Reader/Writer 装饰链, 令 `GZIPInputStream<:InputStream` 可判)。修 jsoup HttpConnection$Response.execute |
| `JDEC_REF_SLOT_CROSSCLASS_SIBLING_ARM_MERGE_OFF` | 跨类(jar 内)兄弟臂引用 phi 合并: 两互斥臂存 jar 内兄弟类型(`TextNode`/`DataNode` 同继承 `LeafNode`, 互不为子类型)汇入合流。JDK 兄弟合并(仅 JDK 表)与 jar 内子类型合并(需互为子类型)都不覆盖 jar 内兄弟对, 故晚臂分裂、合流读未初始化。以 `CrossClassCommonSuperType`(BFS 两臂 jar 内超类闭包取最近公共祖先, 排除 Object)加宽共享 ref, phi 门控; 仅认 `new` 分配臂(排除枚举常量/静态字段臂, 后者重建三元按枚举本身定型, 加宽致「bad type in conditional」如 guava LittleEndianByteArray)。修 jsoup HtmlTreeBuilder.insert |
| `JDEC_BOOL_TO_INT_COERCE_OFF`(共用) → boolean[] 元素存 | `values.CoerceBooleanAssignRHS` 经 `arrayStoreRHS` 处理 `boolean[]` 元素被 int 值赋值的逆向造型: 布尔值存入 `boolean[]` 元素经 javac 编成物化 int 菱形 `cond ? 1 : 0`(JVM 无布尔存储; bastore 同时服务 byte[]/boolean[]), 原样渲染进 `boolean[]` 元素报「int cannot be converted to boolean」。`CoerceBooleanAssignRHS` 把 0/1 叶重定型为布尔(`coerceBooleanArgument`)再折成连接式(`boolReduce`: `cond?true:false`→`cond`); 仅 leftType 为 boolean 且 rhs 为 int 时触发, 已是布尔的值(比较/谓词调用/布尔 ref)原样返回。之前仅覆盖裸 int 字面量(0/1→false/true), 现扩展到三元/表达式 RHS。修 spring ASM `ClassReader.readTypeAnnotationTarget`/`AttributeMethods.<init>` + commons-lang3 `Conversion`(spring 54→52、commons-lang3 18→16) |

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

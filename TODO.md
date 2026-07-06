# TODO — 当前缺陷工单(可执行 / 可复现)

> 这是「下一步修什么」的可执行清单。怎么验证、怎么一个一个清零, 见 [`HARNESS.md`](./HARNESS.md);
> 完整状态账本与生效中的安全开关见 [`classparser/CODEC_TODO.md`](./classparser/CODEC_TODO.md);
> 面向用户的评测报告见 [`BENCHMARK.md`](./BENCHMARK.md)。
>
> **口径**: 全部以 **tree(整树重编译)** 为准 —— 这是「反编译→重编译→重打包→可调用」的真口径。
> iso 口径的 `cannot find symbol`/`private access` 大多是扁平 `$` 假阳性, 不在此列(见 CODEC_TODO §3)。
>
> 数字快照(javac 21, 本机 `~/.m2` 含可选依赖; tree errLines / 缺陷类, 复跑见下方命令):
> codec 0/0 ✅ · gson 0/0 ✅ · jsoup 1/1 · snakeyaml 1/1 · fastjson2 31/15 · guava 28/24 · commons-lang3 12/9 · spring 46/29。（合计 119）
> (本轮一修: 条件 null 重赋值 phi 合并(`reachingRefSlotNullReassignMerge`, `JDEC_REF_SLOT_NULL_REASSIGN_MERGE_OFF`)—— 已定型局部先存具体引用值(`Character c = Character.valueOf(chars[i])`), 又在一条分支被重赋 null(`if(c.charValue()==0) c=null;`), 随后分支合流后被读(`get(c)`); 定型 def 与 `=null` def 都到达合流读, 是同一变量(null 可赋任意引用)。但各臂合并 helper 均对 null 值 bail, AssignVarGuarded 见 null 不携类型无法匹配 current 的 Character ref, 遂为 null 存储另铸 Object 变量; 定型 def 只剩 `charValue()` 一处使用被单用折叠吃进条件、其存储被丢, 合流读绑到只在 null 分支赋值的新变量 → javac「variable might not have been initialized」。修法: 值为 null 字面量且 current 为已定型非 param 非 null-init 非 Object 引用, phi 证 null 存储与 current def 共同到达下游 load 时, 保持 current(null 存储成普通 `c=null` 重赋值)—— 不加宽不收窄, 类型合法接纳 null, phi 门控使真正不相交的槽复用仍分裂。治 snakeyaml Resolver.addImplicitResolver + commons-lang3 LocaleUtils(countriesByLanguage/languagesByCountry)。同时该合并在栈模拟阶段就并回 JSONPathParser.parseFilter 的 slot-7 null, 故 orphan-rebind 承重测试改为整 jar 树口径(见 `orphan_global_rebind_test.go`)。合计 123→119。)
> (上一轮四修: try/catch 异常槽 Throwable 族上型臂合并(`reachingRefSlotThrowableArmMerge`, `JDEC_REF_SLOT_THROWABLE_ARM_MERGE_OFF`)—— 多 handler 把捕获异常写入同一槽, 捕获后读是一个 `Throwable cause` 变量(类型为各臂 LUB); DFS 序里 InterruptedException 臂先铸变量、getCause()→Throwable 上型臂被拆开, 致 `cause instanceof X`/`(X)cause` 绑到窄型 InterruptedException 变量, javac 报「InterruptedException cannot be converted to X」; 补 java.lang.Throwable 族层级边, 两臂皆 Throwable 子类且 val 为严格上型(LUB==val)时把共享 ref 加宽到 LUB(合并后 catch 变量的用法都是 Throwable 级, 加宽安全), phi 门控。治 spring core codec `Decoder`/`Encoder`(spring 48→46)。合计 125→123。)
> (本轮三修: 数组参数收 null-Object 实参造型(`arrayParamRefArgCast`, `JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF`)—— null 初始化的 Object 局部只被传给 `byte[]`/`Object[]` 数组形参时, 渲染成裸 Object 实参被 javac 拒「Object cannot be converted to byte[]」; 参数为数组类型且实参擦除为 Object 时于调用点补 `(byte[])` 造型(字节码里该值已占数组槽, 造型保义)。治 spring ASM `Attribute.computeAttributesSize`/`putAttributes` + cglib `Enhancer`(Object→Object[]), spring 51→48。合计 128→125。)
> (本轮二修: (1) 布尔累加器复用 int 循环计数器槽位(`reachingBoolAccumulatorSlotSplit`, `JDEC_BOOL_ACCUM_SLOT_SPLIT_OFF`)—— `boolean flag=false; flag|=Zcall()` 的初值 `iconst_0` 复用了不相交的(已死)int 循环计数器槽, 合并后整槽被 `|=` 累加定型 boolean, 早循环渲染成 `boolean<int`/`array[boolean]`/`boolean++`。治本: 初值 0 转布尔 false 铸新 flag, int 计数器保留自身 ref; 治 spring ASM `ClassWriter.toByteArray`(spring 52→51, 解封 ClassReader 控制流 2 处 unreachable/missing-return, 净 -1)。(2) boolean[] 元素存 int 造型(`values.CoerceBooleanAssignRHS`, 经 `arrayStoreRHS`; 复用 `JDEC_BOOL_TO_INT_COERCE_OFF`)—— 布尔值存入 `boolean[]` 元素时经 javac 编成物化 int 菱形 `cond ? 1 : 0`(JVM 无布尔存储, bastore 同时服务 byte[]/boolean[]), 原样渲染进 `boolean[]` 元素被 javac 拒「int cannot be converted to boolean」; 把 0/1 叶重定型为布尔并折成连接式(`out[i]=a||b||c`)。治 spring ASM `ClassReader`/`AttributeMethods`(spring 54→52)+ commons-lang3 `Conversion`(18→16, 但解封 LocaleUtils 折叠首赋值致 var3_1 未初始化 2 处, 净 -2)。两修合计 133→128。)
> (上一轮三修 jsoup 潜伏链 + spring: ① JDK 子类型臂合并(`JDEC_REF_SLOT_JDK_SUBTYPE_ARM_MERGE_OFF`) —— `if(m.containsKey)v=(Map)m.get(); else{v=new HashMap();m.put(k,v);}` 中 `new HashMap()` 臂被折进 `put`、丢掉对 v 的赋值, 合流读 v 未初始化; 以 `CommonSuperType==当前臂` 证子类型、仅认 `new` 分配臂、phi 门控, 保持当前 ref 不裂; 治 jsoup Whitelist。② null 收养后子类型再赋值(`JDEC_NULL_ADOPTED_SUBTYPE_REASSIGN_OFF` + hierarchy.go 补 java.io 流族)—— `InputStream in=null; in=pick(); if(gzip)in=new GZIPInputStream(in)` 中收养 InputStream 后 GZIP 子类型被 null-adopt-once 当成新变量分裂, 合流 `in.read()` 未初始化; 子类型再赋值复用同 ref; 治 jsoup HttpConnection$Response。③ 跨类兄弟臂合并(`JDEC_REF_SLOT_CROSSCLASS_SIBLING_ARM_MERGE_OFF` + `CrossClassCommonSuperType`)—— 两互斥臂存 jar 内兄弟类型(TextNode/DataNode 同继承 LeafNode), 既有 JDK 兄弟合并(仅 JDK 表)与 jar 内子类型合并(需互为子类型)都不覆盖, 晚臂分裂致合流读未初始化; 以 jar 字节证最近公共祖先并加宽共享 ref; 治 jsoup HtmlTreeBuilder。三处均随修解封 jsoup 潜伏链下一处(Attributes→Whitelist→HttpConnection→HtmlTreeBuilder→XmlTreeBuilder…, 因 javac 属性错遮蔽流分析, 逐个显形); spring `DataBufferUtils` 经既有返回桥接 55→54, fastjson2 经上述合并 32→31。合计 135→133。)
> (上一轮两修: ① 三元 class 字面量臂定型(`JDEC_NO_CLASSLIT_SLOT_TYPE`, 与直接存储共用) —— `cond ? Foo.class : classField` 是 `java.lang.Class` 值, 但 class 字面量臂 `Type()` 报被引类, 致臂合并塌成 `Object`, 局部误声明 `Object c`, `c.getModifiers()/getName()` 失败; 修法把 class 字面量臂计为 `java.lang.Class`(`TernaryArmRValueType`)且声明处优先取槽位 ref 已定型; 治 spring cglib `Enhancer.generateClass`; spring 63→57。② 懒初始化自守卫三元收窄(`JDEC_LAZY_INIT_SELF_TERNARY_OFF`) —— `x = (x!=null)?x:new Concrete()` 合流定型为 `Object`, `x.add(..)` 失败; 槽位唯一具体值即 `new` 臂, 收窄声明到该臂; 治 spring `StringDecoder`; spring 57→55。合计 143→135。)
> (上一轮: 第三方(非 JDK)嵌套类引用点号化(`JDEC_EXTERNAL_NESTED_DOT_OFF`), 以 SiblingSuperTypes 判外层类是否在本 jar; 不在则该嵌套类只在 classpath 上以真正嵌套的 `Outer.Inner` 存在, 扁平 `Outer$Inner` 不可解析, 治 spring `reactor.blockhound.BlockHound$Builder`; spring 错误行 65→63; 合计 145→143。)
> (上一轮: 扁平内部类外层类型变量独立位置擦除(`JDEC_INNER_STANDALONE_ERASE_OFF`), 治 guava HashIterator/Itr; guava 31→28。RAW `new HashMap(typedMap)` 作 lambda 调用接收者补菱形(`JDEC_NEW_RECV_DIAMOND_OFF`); Multi-Release jar 的 versions/N 单元按 `--release N` 独立编译; 合计缺陷类 79→77、干净率 96.5%→96.6%。)
> (本轮两修: ① 类头类型参数 bound 改用真实 funcCtx 渲染以注册 import(`JDEC_TYPEPARAM_BOUND_IMPORT_OFF`), 治 spring MergedAnnotationSelector / FirstRunOfPredicate 的 `<A extends Annotation>` 缺 import; ② invokespecial 目标为直接实现接口的 default 方法时渲染 `Iface.super.m()`(`JDEC_IFACE_DEFAULT_SUPER_OFF`), 治 StandardAnnotationMetadata / StandardMethodMetadata / SimpleAnnotationMetadata 的 `super.getAnnotationTypes()` 族; spring 错误行 77→68、缺陷类 36→30, 合计缺陷类 85→79、干净率 96.2%→96.5%。)

## 重新生成本清单(诚实数据)

```bash
# tree 口径(真口径, 阻碍重打包的真实缺陷, 按文件+reason 落盘)
PROFILE_JAR=all ISO_REPORT_DIR=/tmp/jdec-inv go test -run TestJarTreeInventory -v -timeout 20m ./test/cross/
# 看某 jar 的 reason 直方图 / 失败明细
cat /tmp/jdec-inv/guava.tree.reasons.txt
cat /tmp/jdec-inv/guava.tree.fails.txt
```

---

## P0 — 最大杠杆

### T1. 泛型擦除缺造型 `Object cannot be converted to T/K/CAP#1`(`incompatible types` 桶, 跨 jar 头号来源)
- 已治本多块(返回点向下造型 / JDK·同类·继承·私有方法实参造型 / 统一跨类泛型解析器 / 擦除型类型变量多余 upcast 抑制 / 参数化实参·数组实参造型等, 见 CODEC_TODO §4)。**剩余按根因分**:
  - **(a) 接收者自身泛型未被传播复原成参数化类型**: 接收者是本类型局部变量/字段而非 `this`(`this.box.put(o)`, box=`Box<E>`)时, 其泛型未沿声明传播复原, 取值点仍是 Object。业界 CFR/Vineflower 亦有此长尾。
  - **(b) 通配符捕获 `CAP#1`(guava)** —— **oracle 实证内在难, 优先级下调**: `this.equivalence.equivalent(a,b)`, 字段 `Equivalence<? super T>` 捕获 `CAP#1`, 实参 Object 不可造 `(CAP#1)`。`ORACLE_JAR=guava ORACLE_CLASS='base/Equivalence$Wrapper'` 三方全败(真源码用 `(Equivalence<Object>)` + `@SuppressWarnings`)。方向(若做): 通配符接收者**整体** `<Object>` 造型。
  - **(d) 返回点「inference variable X has incompatible bounds」(guava 头号桶, ~13 行: ImmutableEnumMap/Range/Maps/Ordering/Striped/Sets$DescendingSet/MinMaxPriorityQueue$Builder/Tables$TransposeTable 等)** —— **有界类型变量同擦除强转 javac 实测拒绝, 不可用 `parameterizedReturnCast` 治**: 形如 `return ImmutableMap.of(k.getKey(), k.getValue());`(目标 `ImmutableMap<K,V>`、K/V 或 `C extends Comparable` 有界), javac 对 `of()` 独立推断出 `<Object,Object>` 后再与目标合流报「incompatible bounds」。实测 `(ImmutableMap<K,V>)(ImmutableMap<Object,Object>)` **当目标实参有界(K extends Enum / C extends Comparable)时 javac 直接报「不兼容的类型…无法转换」**(见 `parameterizedReturnCast` 注释里「BOUNDED var 被排除」的原因)。真源码用 `@SuppressWarnings` + 不同结构强转, 从擦除字节码无法忠实复原。**根因仍是 (a): 局部变量/字段(如 raw `Map.Entry var1`)的泛型未沿声明传播复原**——只要把 `var1` 复原为 `Map.Entry<K,V>`, `getKey()/getValue()` 即回到 K/V, 无需强转。故此桶归 (a) 的局部/字段泛型传播, 不要再尝试返回点强转。
  - **(c) 装箱/数值**: `int cannot be converted to Integer` 等(**非擦除, 不可盲目造型**), 按 `Integer.valueOf` 修。
- 复现:
  ```bash
  go build -o /tmp/jj ./cmd/javajive
  /tmp/jj decompile -o /tmp/guava ~/.m2/repository/com/google/guava/guava/28.2-android/guava-28.2-android.jar
  ORACLE_JAR=guava ORACLE_CLASS=Equivalence go test -run TestThirdPartyOracle -v ./test/cross/
  ```

### T2. 活跃区间分裂 / 槽位复用类型混淆(fastjson2 `bad operand type` / `unexpected type` 一族)
- 表象: 一个字节码 local 槽在**互不相交的活跃区间**里先后承载**不兼容类型**的值(如 `JSONPathFilter$GroupFilter` 的 `var9` 既作 `Iterator` 又当 int 比较), 反编译却合成单一变量名 + 单一声明类型。
- 已治本多族 disjoint 槽(兄弟臂 LUB 合并 / Object 超类臂 / 数组协变父臂 / 布尔字段·返回槽拆分 / 跨作用域孤儿读重放, 见 CODEC_TODO §4)。**残余**: 非布尔子形态须在变量定型/分裂核(`JDEC_LIVEINTERVAL_*`)上按「区间+类型」更激进拆分同槽, 风险高、改动核心, 留专项。
- 复现: `ORACLE_JAR=fastjson2 ORACLE_CLASS=JSONPathFilter go test -run TestThirdPartyOracle -v ./test/cross/`

## P1

### T3. 扁平嵌套类丢外层类型参数 `cannot find symbol: class K/V/E/S`(guava `HashIterator`/`Segment`/`Itr`)
- 根因: 内部类**自身又有形参**时, 注入外层 `K,V,E,S` 会与引用点元数不一致。「自身无形参」的纯继承内部类已治本(`JDEC_INNER_TYPEVAR_*`)。
- 方向: 跨类协同重写所有引用点(integral rebuild), 深且高风险, 留专项。
- 复现: `rg 'cannot find symbol' /tmp/jdec-inv/guava.tree.fails.txt`, 取 file:line 后 `/tmp/jj decompile` 复现。

### T4. 三元 LUB `bad type in conditional expression`(fastjson2 + guava)
- 已有 `CommonSuperType`(`decompiler/core/values/types/hierarchy.go`), 已治本反射家族与跨类直接子类型两支(`JDEC_TYPELUB_OFF` / `JDEC_TERNARY_DECL_LUB_*`)。
- 残余: 渲染期造型未反馈到三元类型 + 三元臂泛型擦除(归 T1)。方向: 扩 JDK 层级表 + 在更多 phi/合流点接入。

### T5. `for` 循环 `continue`-到-自增被丢弃(gson `JsonWriter.string`)
- 根因: `for` 渲染成 `do-while(true)` + 自增作显式体语句, 内层 `continue` 会跳过自增故被丢弃, 致 `variable might not have been initialized`。
- 方向: `for` 循环恢复(自增放进 for-update 槽)或 continue-到-latch 结构化, 改动循环结构化核心、影响所有 jar, 风险高, 留专项(历史上循环重建易回归, 必须 opt-in 开关 + 全量 A/B)。

## P2 — 小桶 / 长尾

| 工单 | 代表 | 备注 |
|---|---|---|
| T6 `method invocation cannot be applied` | guava `SortedLists.binarySearch` | 重载消歧(`(name,arity)` 冲突丢签名, 已治本 descriptor 键消歧)+ 残余通配符 |
| T7 `invalid method reference` | fastjson2 `Throwable::setStackTrace` | 构造器实参位方法引用对非泛型构造器形参(`BiConsumer`)绑定失败 |
| T8 `abstract method not overridden` | guava | 桥接/抽象方法可见性 |
| T9 `incompatible parameter types in lambda` | fastjson2 `ObjectReaderCreator` | 形参被用作具体类型 + raw 接收者, 须复原接收者参数化类型(归 T1) |
| T10 合成内部类 `this.val$e;` field-read pop | spring `EmitUtils$6` | **CFR/Vineflower 亦失败, 内在难 case**; 粗暴 elide 会致 spring 大回归, 留长尾 |

---

## 工作纪律(摘自 HARNESS.md 红线)

- 一次只清一个单点; 动核心前先 tree inventory 定位到具体类+方法, 再用 `/tmp/jj decompile` 复现。
- 拿不准的难 case 先跑 `TestThirdPartyOracle` 看 CFR/Vineflower: 三方都败→可诚实 stub; 只有我们败→对照它们找偏差。
- 复杂改动必带 kill-switch + 承重测试 + 回归种子; A/B delta 对所有 jar 必 ≥0; 全量 `go test ./...` 全绿, 基准 `syntax=0` 硬断言不得触发。

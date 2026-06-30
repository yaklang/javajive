# JavaJive 反编译器正确性账本 (CODEC_TODO)

> **北极星**: 复杂 JAR 做到「反编译 → `javac` 重编译 → 重新打包成 jar → 被外部 JVM 加载/校验/调用」全链路正确。
>
> - 度量口径与长尾清零方法: 见根目录 [`HARNESS.md`](../HARNESS.md)。
> - 当前可执行的缺陷工单(选靶 + 复现命令): 见根目录 [`TODO.md`](../TODO.md)。
> - 本文件只登记**当前真实状态 + 已治本项 + 剩余缺陷**。所有数字均由 harness 真实跑出(`javac 17.0.12`, 本机 `~/.m2`), 禁止估算或编造; 绝对值随 JDK / jar 版本浮动, 以 delta 与趋势为准。

---

## 0. 为什么用整树(tree)口径, 而不是逐文件(iso)

反编译器把嵌套类发成**独立的扁平单元** `Outer$Inner.java`(见 `dumper.go` 架构注释)。这带来两种度量:

- **tree(整树)**: 所有扁平单元一起 `javac`(依赖在 classpath)。兄弟扁平 `$` 引用互相解析得到, 产物可直接重打包。**这是「能否重打包」的真口径。**
- **iso(逐文件)**: 每个单元单独 `javac`(原始 jar + 依赖在 classpath)。`Outer$Inner` 这种扁平 `$` 类型引用在原始 jar 里按源名 `Outer.Inner` 索引、解析不到, 于是报海量 `cannot find symbol`/`private access`。**这是 iso 口径的系统性假阳性, 不是反编译缺陷, 也不阻碍重打包**(详见 §4)。

> 结论: 治本与验收以 **tree errLines / blockerUnits** 为准; iso 仅用于侧写、对照, 不作为重打包准绳。

---

## 1. 当前真实状态(4 个真实 JAR)

| jar | units | tree errLines | tree blockerUnits | iso fails | 重打包(repackage)状态 |
|---|---|---|---|---|---|
| **codec** (commons-codec 1.15) | 106 | **0** | **0** | 38 | ✅ **全链路达标** |
| **spring** (spring-core 5.3.27) | 974 | **2** | **1** | 384 | ⚠ 仅 1 个合成内部类阻断 |
| **fastjson2** (2.0.43) | 681 | 242 | 55 | 342 | ⚠ 泛型擦除为主 |
| **guava** (28.2-android) | 1825 | **478** | 190 | 1149 | ⚠ 泛型擦除/边界为主 |

**codec 已完整证明北极星**(承重于 `test/cross/jar_roundtrip_test.go` 的 `TestJarRoundTripRepackage/codec`):
`decompile → javac 重编译(0 error) → archive/zip 重打包 → java -Xverify:all 逐类加载校验 107/107 通过 → 调用差分(Base64 / Hex / MD5 / SHA-256)与原始 jar 逐字节一致`。

> CI 常驻承重: `TestSyntheticJarRoundTrip`(无需 `~/.m2`)对一个含枚举+switch / 泛型 / lambda / varargs / try-catch 的多类程序跑完整往返, 断言运行输出逐字节一致 + 全类 verify。它守住「往返能力」永不回归。

---

## 2. 本轮已治本(每项: kill-switch + 承重测试 + 回归种子 + 实测 delta)

| 缺陷 | 根因 / 治法 | kill-switch | 实测 |
|---|---|---|---|
| **擦除型类型变量的多余 upcast 抑制(治本)** | 调用泛型方法时, 若被调形参是它自己签名里的**类型变量** `C`(描述符把它擦成 bound, 如 `<C extends Comparable> Range<C> closed(C,C)` → `closed(Comparable,Comparable)`), 实参渲染器见「实参静态类型 `Integer` ≠ 形参描述符类型 `Comparable`」便补 `(Comparable)` 造型。但实参既已沿字节码流入该形参, 必然可赋给 bound, 故这是**无操作的向上造型**——它却让 javac 在调用点把 `C` 推断成 `Comparable` 而非精确的 `Integer`, 进而 `ContiguousSet.create(Range<C>, DiscreteDomain<Integer>)` 不再适用(`method create cannot be applied`)。治法: 新增 `FunctionCallExpression.calleeParamIsErasedTypeVar`——经 `SiblingClassSig` 取被调(JAR 内)类的方法 Signature, 若第 i 形参是被调**方法自身**或**其声明类**的形式类型参数(`MethodFormalTypeParamNames`/`ClassFormalTypeParamNames`), 即判定为擦除型类型变量, 让实参造型逻辑**丢弃**该造型, 由精确实参类型驱动推断。安全: 仅当三个泛型解析器(`instantiatedParamType`/`sameClassMethodParamType`/`resolvedParamType`)**都未**复原出具体/可命名类型时才介入(解析器复原的造型是要保留的); JDK 被调(`SiblingClassSig` miss)留在描述符上不动; 抑制的恒为向上造型(字节码已保证可赋), 不改变可赋性, 唯一理论风险是重载消歧, 经 A/B 全 jar 实证零回归 | `JDEC_NO_ERASED_TYPEVAR_NOCAST` | **guava tree 492→478(delta -14, 归一化 diff 14 修复/0 新错)**, codec/fastjson2/spring 零回归(A/B `TestJarRecompileDelta` 四 jar delta 全 +0)。修复横跨 `ContiguousSet`/`Range`/集合家族的 `(Comparable)` 假造型; `method invocation cannot be applied` 桶 50→46。承重 `TestErasedTypeVarCastSuppressionIsLoadBearing`(ON 丢造型 / OFF 回归, 必须 `DecompileWithResolver` 双类), 种子 `ErasedTV.class`+`ErasedTVUser.class`(`--release 8`)。**残余**: 被调方法在 `SortedLists.binarySearch` 这类**同名同元重载**上时, `buildSiblingClassSig` 按 (name,arity) 冲突丢弃签名, 故抑制未命中(约 6-8 行); 需描述符级消歧, 留下一轮 |
| **构造器方法引用渲染 `::new_` → `::new`(治本)** | `Type::new` 的 invokedynamic impl 句柄是 `REF_newInvokeSpecial` → `<init>`, 渲染器把成员名设为 `new` 后**误过了 `SafeIdentifier`**——该函数把关键字 `new` 改写成 `new_`, 于是输出 `Type::new_`。javac 视之为「invalid method reference」(去找一个名为 `new_` 的方法, 不存在)。治法: 成员为 `<init>` 时直接返回 `funcCtx.ShortTypeName(impl)+"::new"`, 让关键字 `new` 绕过 `SafeIdentifier`(它只该清洗普通标识符) | `JDEC_CTOR_METHODREF_FIX_OFF` | **fastjson2 tree 248→242(delta -6, 归一化 diff 9 修复/3 推进到更深的 functional-interface 造型错——非回归, 原行就已失败)**, codec/guava/spring 零回归(A/B 四 jar delta 全 ≥0)。承重 `TestCtorMethodRefIsLoadBearing`(ON=`::new` / OFF=`::new_`), 种子 `CtorMethodRefSeed.class/.golden`(`--release 8`) |
| **泛型擦除缺造型 · 统一跨类解析器(治本)** | 前几行的 JDK 写死表 / 同类方法 / 直接超类型恒等一层都是**特例拼盘**, 漏掉三类残余: (i) 接收者是本类型的**局部/字段**而非 `this`(`var0.setCount(objVal)`, var0=`Multiset<E>`); (ii) 超类型**非恒等**实参映射(`Sub<X,Y> implements Super<Y,X>` 换序 / 具体化); (iii) 方法声明在**深链**祖类/祖接口。治法(参照 CFR/Vineflower 的「类型模型+替换+传播」): 新增纯函数替换引擎 `types.SubstituteTypeVars(t, σ)`(递归改写类型变量/参数化实参/数组元素) + 统一原语 `types.ResolveInstantiatedParamType` —— 给定接收者参数化类型 `C<A..>` 沿其泛型超类型层级 DFS, 在每条 `extends/implements Super<...>` 边上**复合类型实参替换 σ**(支持换序/具体化, 非仅恒等), 命中被调方法的泛型 Signature 后施加 σ 取第 i 形参, 喂给既有实参造型。跨类签名经 dumper 顶层闭包 `buildSiblingClassSig`(复用 `foldSiblingResolver`+`Parse`+缓存, 仅传字符串签名)下沉到 `ClassContext.SiblingClassSig`, 避免 types↔parser 循环依赖。安全: **附加式** —— 仅当 JDK 表(`instantiatedParamType`)与同类路径(`sameClassMethodParamType`)**都返回 nil** 才调用, 产物是旧行为超集; 可造型闸门只放行**当前类类作用域类型变量**(`IsTypeParam`, → `(K)`)或**具体(带点)类名**(→ `(Foo)`), 方法作用域 `<T>` / 残留外来裸类型变量 / 通配符捕获 / 参数化·数组结果一律返回 nil(不可命名则不造); JDK/外部超类型(resolver miss)留给 JDK 小表 | `JDEC_GENERIC_RESOLVE_OFF` | **guava tree 522→492(delta +30, 0 新错)**, codec/fastjson2/spring 零回归(A/B `TestJarRecompileDelta` 自身计数下 guava +15、它 jar +0)。归一化 diff 实证: 30 条修复全在 `Object cannot be converted to E/N/K/V/C/B` 桶, 横跨 collect/graph/hash/util.concurrent 14 个真实类; blockerUnits 202→194。承重 `TestGenericResolveArgCastIsLoadBearing`(三场景 ON/OFF 双断言, 必须 `DecompileWithResolver` 多类), 种子 `DeepThisSeed/GenA/GenB/SwapSeed/Pair/FieldRecvSeed/Box.class`(`--release 8`); 替换引擎单测 `TestSubstituteTypeVars` |
| **继承超类型泛型方法实参造型 · this 接收者** | `this.get(objVal)` —— 被调方法**声明在直接超类型**(接口/父类)而非本类(guava `AbstractLoadingCache` 抽象类 `this.get(k)`, get 来自接口 `LoadingCache<K,V>`)。本类 `MethodSignatures` 只含自有方法, 故同类方法造型未命中, javac 报 `Object cannot be converted to K`。治法: 反编译本类时用 `foldSiblingResolver`(jar 路径已具备的跨类字节加载)加载**直接超类型**字节, 在**恒等类型实参映射**(`Sub<K,V> implements Super<K,V>`, 每个实参都是与超类型形参同名、且本类自有的类型变量)下, 把超类型方法的泛型 Signature **原样并入**本类 `MethodSignatures`(同 (name,arity) 本类优先, 多超类型冲突丢弃), 复用 `sameClassMethodParamType`。安全: 仅恒等映射(换序/具体实参的非恒等保守跳过, 避免造错类型变量); JDK/外部超类型不在 jar(resolver miss), 由 `InstantiateJDKMethodParam` 路径覆盖, 二者互补; 仅上溯**一层**直接超类型(深链留残余) | `JDEC_GENERIC_SUPER_METHOD_OFF` | **guava 529→522(delta +7)**, codec/fastjson2/spring 零回归。承重 `TestInheritedThisMethodArgCastIsLoadBearing`(必须 `DecompileWithResolver` 双类), 种子 `InheritedThisSeed.class`+`SuperSeed.class`(`--release 8`) |
| **私有同类自有泛型方法实参造型** | `this.updateInverseMap(k, b, objVal, v)` —— 被调方法是当前类**私有**泛型方法(`private void updateInverseMap(K,boolean,V,V)`)。私有方法在 Java 8 字节码里走 `invokespecial`(与 `super.m()` **同一指令**), 上一行的同类方法造型为避免把 `super.m()` 误判成同类调用而**一刀切跳过所有 invokespecial**, 连私有同类方法一并漏掉 → `Object cannot be converted to V`(guava `AbstractBiMap.updateInverseMap`/`removeFromBothMaps` 等家族)。治法: invokespecial 仍按**目标类**区分 —— 目标类==当前类(`f.isCurrentClass`)即私有同类调用, 其签名就在 `funcCtx.MethodSignatures`, 照常补造型; 目标类!=当前类才是 `super.m()`, 跳过 | `JDEC_GENERIC_SELFMETHOD_PARAM_OFF`(总) / `JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF`(仅私有扩展) | **guava 550→529(delta +21)**, codec/fastjson2/spring 零回归。承重 `TestPrivateSelfMethodArgCastIsLoadBearing`, 种子 `PrivateSelfMethodArgCast.class/.golden`(`--release 8` 编译以走 invokespecial) |
| **返回-嵌入赋值局部的声明合成** | 唯一定义是嵌入在条件里的赋值 (`if ((var2 = parse(...)) == null){}else{return var2;}`) 的局部, 被发成**无声明** → `cannot find symbol`(fastjson2 `JSONReaderJSONB` 的 `readLocalDateTime12/14/16` 等日期解析家族)。两处耦合根因: (1) `generatedLocalDeclRe` 把 `return varN` 误当声明(关键字 `return` 命中其类型标识符分支), 使补声明网误以为已声明而跳过; (2) 即便补声明, 因 RHS 是跨类调用(`DateUtils.parseLocalDateTime12`)无符号表可推断返回类型而退化成 `Object`(再触发 return 类型不符)。治法: 收集已声明集时跳过关键字开头的伪声明; 对被 `return` 的未声明局部, 以**所在方法的返回类型**声明(JLS 14.17 返回值必可赋给返回类型, 初值 null + 条件内赋值在 return 前必达) | `JDEC_RETURN_DECL_FIX_OFF` | **fastjson2 285→248(delta +37)**, codec/guava/spring 零回归。承重 `TestReturnLocalDeclSynthesisIsLoadBearing`, 种子 `ReturnLocalDeclSynthesis.class/.golden` |
| **同类自有泛型方法实参造型** | `this.tailSet(var1)` —— 被调方法是**当前类自己声明**的泛型方法(`SortedSet<E> tailSet(E)`), 描述符把形参擦除为 bound(Object), 且其泛型签名位于**同类的另一个方法**上, 故 §字段/JDK 实参造型都未命中, javac 报 `Object cannot be converted to E`。这是泛型擦除阻断**当前最大剩余块**(guava `Forwarding*`/集合家族 `tailSet/headSet/subSet(E)` 等极其普遍)。治法: `ClassContext.MethodSignatures` 按 `(name, arity)` 一次性登记同类方法的原始泛型 Signature(同名同元重载丢弃以防误判); `FunctionCallExpression.sameClassMethodParamType` 在调用点 `ParseMethodSignatureFull` 还原形参类型, **仅对类作用域类型变量**(`funcCtx.IsTypeParam`)造型 —— 绝不碰方法作用域 `<T>`(调用点不在其作用域, `(T)` 不可编译)或具体类型, 喂给既有实参造型基建 | `JDEC_GENERIC_SELFMETHOD_PARAM_OFF` | **fastjson2 307→285(delta +22) + guava 634→550(delta +84)**, codec/spring 零回归。承重 `TestGenericSelfMethodArgCastIsLoadBearing`, 种子 `GenericSelfMethodArgCast.class/.golden` |
| **JDK 泛型方法实参造型 · 字段接收者** | `this.function.accept(var1, var3)` —— 接收者 `this.function` 是同类字段 `BiConsumer<T,V>`, 但 getfield 的字段访问值只带**擦除描述符**(raw `BiConsumer`), 故上一项的接收者参数化造型未命中, javac 报 `BigDecimal cannot be converted to V`。这是泛型擦除阻断的**最大剩余块**(fastjson2 `FieldReader{BigDecimal,BigInteger}Func` 家族 + guava)。治法: `ClassContext.FieldSignatures` 一次性登记同类**参数化字段**的原始 Signature(`Ljava/util/function/BiConsumer<TT;TV;>;`); `FunctionCallExpression.receiverParamTypeArgs` 在调用点按需 `types.ParseSignature` 还原接收者类型参数, 喂给 `InstantiateJDKMethodParam` 即复用既有实参造型基建。字段值类型本身不动(零字段类型涟漪) | `JDEC_GENERIC_PARAM_FIELD_OFF`(字段旁路) / `JDEC_GENERIC_PARAM_INFER_OFF`(全部实参推断) | **fastjson2 332→307(delta +25) + guava 647→634(delta +13)**, codec/spring 零回归。承重 `TestGenericFieldArgCastIsLoadBearing`, 种子 `GenericFieldArgCast.class/.golden` |
| **JDK 泛型方法实参造型 · 值接收者** | 传给 `Map.put(K,V)` / `BiConsumer.accept(T,V)` / `Collection.add(E)` 等的实参, 形参被描述符擦除为 `Object`, 旧实现漏掉源码原有的 `(K)`/`(V)` 造型 → javac 报 `Object cannot be converted to K`。`InstantiateJDKMethodParam`(InstantiateJDKMethodReturn 的形参版)按接收者参数化类型还原形参泛型类型, 复用既有实参造型逻辑重发造型 | `JDEC_GENERIC_PARAM_INFER_OFF` | **fastjson2 334→332 + guava 651→647**(delta +2/+4), codec/spring 零回归。承重 `TestGenericArgCastIsLoadBearing`, 种子 `GenericArgCast.class/.golden`。命中参数化的 local/参数/this 接收者(字段接收者见上一行) |
| **返回点 Object 向下造型** | 方法声明返回具体引用类型, 但返回值静态类型是被擦除的 `Object`(泛型擦除 / try-with-resources 的 null-only 槽, 如 `JSON.parseObject`)。旧实现发 `return objVar;` → javac 报 `Object cannot be converted to JSONObject`。`objectReturnDowncast` 补合法且行为保持的 `(T)` 造型(Object 是一切引用类型的父类, 向下造型恒合法, 与 CFR/Fernflower 一致) | `JDEC_OBJECT_RET_DOWNCAST_OFF` | **fastjson2 tree 355→334**(delta +21), guava 652→651, codec/spring 零回归。承重 `TestReturnObjectDowncastIsLoadBearing`, 种子 `ReturnObjectDowncast.class/.golden` |
| **pop/pop2 裸值语句** | `aload x; pop` 等死载入被渲染成 `var0;`(JLS 14.8 非语句, 不可编译)。`keepDiscardedStackValue` 对无副作用且非语句的丢弃值(裸 local/常量/类引用)直接 elide | `JDEC_POP_ELIDE_OFF` | **spring tree 14→2**(delta +12), codec/fastjson2/guava 零回归。承重 `TestPopElideIsLoadBearing`, 种子 `SpringCglibEmitUtils.class/.golden` |
| **enum-switch `$SwitchMap` idiom** | 跨类 `switch(Outer$N.$SwitchMap$E[e.ordinal()])` 折回 `switch(e){case CONST}` | `JDEC_NO_ENUM_SWITCH_FOLD` | iso 净 +4, 零回归。承重 `TestEnumSwitchFoldIsLoadBearing` |
| **核心非确定性** | CFG 结构化按 Go map 随机序决策 + import 顺序 → 同输入多产物 | (确定性, 无开关) | 4 jar 整树数跨进程恒定。承重 `TestDecompileIsDeterministic` / `TestRegressionSeedsAreDeterministic` |
| **局部变量数据流** | 到达定义 web + 活跃区间分割, 修声明摆放 | `JDEC_LIVEINTERVAL_OFF` / `JDEC_LIVEINTERVAL_WEB` | fastjson2 tree -324。承重 `TestLiveIntervalSplitIsLoadBearing` |
| **类型层级 / 三元 LUB** | `CommonSuperType` + astore 声明点向上拓宽到双臂 LUB | `JDEC_TYPELUB_OFF` / `JDEC_TERNARY_DECL_LUB_OFF` | codec/fastjson2 tree 各 -1, 零回归。承重 `TestTypeLUBIsLoadBearing` |
| **泛型实例化** | `Iterable.iterator()` / `Iterator.next()` 回填实参类型 | `JDEC_GENERIC_INFER_OFF` | guava iso +2 / tree +12, 零回归 |
| **嵌套类型 public 复原** | 从 `InnerClasses` 取回 `ACC_PUBLIC`, 修跨包不可见 | `JDEC_NESTED_PUBLIC_OFF` | fastjson2 多类可见性修复 |

> 完整 kill-switch 索引见 §5。

---

## 3. 剩余缺陷(tree 口径, 按杠杆从大到小)

> 这些是真正阻碍重打包的缺陷。每项给「计数(error lines)·代表类·样例·初判根因」。可执行工单(复现命令 + 优先级)在根 [`TODO.md`](../TODO.md)。

1. **泛型擦除缺造型 `Object cannot be converted to T/K/CAP#1`** — **最大杠杆。当前 `incompatible types (assignment/return)` 桶: fastjson2 113 + guava 202**(含装箱等非擦除项)
   - **已治本八块**: 返回点 Object 向下造型(fastjson2 -21); JDK 泛型方法实参 · 值接收者(fastjson2 -2 / guava -4); JDK 泛型方法实参 · 字段接收者(fastjson2 -25 / guava -13); 同类自有泛型方法实参 · 公有(fastjson2 -22 / guava -84); 同类自有泛型方法实参 · 私有 invokespecial(guava -21); 继承超类型泛型方法实参 · this 接收者(guava -7, 恒等映射一层); 统一跨类泛型解析器(guava 522→492 = -30)——以「类型实参替换 σ + 超类型层级 DFS」一举覆盖原 (a) 三子项; **擦除型类型变量多余 upcast 抑制(本轮治本, guava 492→478 = -14, 0 新错)——丢弃 `(Comparable)` 一类无操作向上造型, 让精确实参驱动调用点推断(`method ... cannot be applied` 桶 50→46)**。**剩余类别**(按当前 `cannot be converted to` 直方图):
     - **(a) 非-this 接收者 / 非恒等映射 / 深链 —— 本轮已治本(统一跨类解析器)**: (i) 接收者是本类型的**局部变量/字段**而非 `this`(`this.box.put(o)`, box=`Box<E>`); (ii) 超类型**非恒等**实参映射(`Sub<X,Y> implements Super<Y,X>` 换序/具体化); (iii) 超类型**深链**(方法在父类的父类/祖接口)。三者统一由 `types.ResolveInstantiatedParamType`(沿接收者泛型超类型 DFS + 逐边复合 σ)+ `types.SubstituteTypeVars` 解决, 附加式落地(JDK 表/同类路径均 miss 后兜底), kill-switch `JDEC_GENERIC_RESOLVE_OFF`。实测 guava -30(归一化 diff 0 新错, 横跨 collect/graph/hash/util.concurrent 14 类), 它 jar 零回归。残余同桶里仍有「接收者本身泛型未被传播复原成参数化类型」的 long-tail(业界 CFR/Vineflower 亦有, 见调用链丢泛型), 留下一轮。
     - **(b) 通配符捕获 `CAP#1`(guava 40)** —— **oracle 实证为内在难 case, 优先级下调**: `this.equivalence.equivalent(a,b)`, 字段类型 `Equivalence<? super T>` 捕获成 `CAP#1`, 实参 Object 不可造 `(CAP#1)`(不可命名)。`TestThirdPartyOracle/guava/Equivalence$Wrapper` **三方(JavaJive/CFR/Vineflower)全部重编译失败**: CFR 发 `Equivalence<? super T> e = this.equivalence;` 仍不可编译; 真源码用 `(Equivalence<Object>) this.equivalence` + `@SuppressWarnings`。方向(若做): 通配符接收者**整体** `<Object>` 参数化造型, 非对实参造型。
     - **(c) 装箱/数值**: `int cannot be converted to Integer` 等(**非擦除, 不可盲目造型**), 按 `Integer.valueOf` 修。

2. **`break outside switch or loop`(fastjson2 31)**
   - 例: `JSONReader.java:1148`。根因: 标号 break / 复杂循环-switch 嵌套结构化把 break 落到了循环/switch 外。属循环重建长尾(参见 §3.6 与历史 Phase 4 档案)。

3. **泛型边界 `type argument K is not within bounds of type-variable C`(guava 53+31+5 ≈ 89)**
   - 例: `ImmutableRangeMap$1.java:21`。根因: 扁平嵌套类型丢了外层类型参数与其 bound, 引用处实参越界。需在扁平单元上重建被擦掉的类型参数声明与 bound。

4. **三元 LUB `bad type in conditional expression`(fastjson2 11 + guava 12)**
   - 根因: `cond ? a : b` 两臂最小公共上界算窄, javac 拒绝。已有 `CommonSuperType` 设施, 需扩表 + 在更多合流点接入。

5. **`bad operand type for operator`(fastjson2 14) / `unexpected type`(fastjson2 9)** — 同一深根因: **活跃区间分裂 / 槽位复用类型混淆**
   - 本轮复现确认这两桶与 `FieldWriter` 的 `Field/Class→Method` 一族(§3.8)同源: 一个字节码 local 槽在**互不相交的活跃区间**里先后承载**不兼容类型**的值, 反编译却合成了**单一变量名 + 单一声明类型**。例: `JSONPathFilter$GroupFilter` 的 `var9` 既声明为 `Iterator` 又被 `(var9) != (0)` 当 int 比较(`bad operand: Iterator,int`); `JSONSchema` 的 `var4` 既 `(var4)==(0)`(int)又 `var4 instanceof String`(Object)。治法须在变量定型/分裂核(`JDEC_LIVEINTERVAL_*`)上按「区间+类型」更激进地拆分同槽, 风险高、改动核心, 留作专项(本轮不动)。

6. **合成内部类 `this.val$e;` field-read pop(spring 2, 单类 `EmitUtils$6`)**
   - 现状: `pop` 丢弃 `this.val$e`(field read)未被 elide。**已验证不可粗暴扩展**: 把 `RefMember` 纳入 elide 集会引发 spring tree 812 错误的大回归(结构化依赖该节点), 故 §2 的治法只 elide 裸 local/常量/类引用。
   - oracle 旁证(`TestThirdPartyOracle/spring/EmitUtils$6`): **CFR 与 Vineflower 对该合成匿名内部类亦失败**, 属内在难 case, 留长尾, 不为它冒结构化回归风险。

7. **`cannot find symbol`(tree 口径: fastjson2 42 + guava 96)** — 注意这是 **tree(整树)** 残留, 与 §4 的 iso 扁平 `$` 假阳性**不同**。已治本一类(返回-嵌入赋值局部声明合成, fastjson2 -37, 见 §2)。本轮分桶后明确**两块各自不可治本**:
   - **(a) `sun.misc.Unsafe` 一族(guava 96 中约 45 行, 环境假阳性, 非缺陷)**: `Striped64`/`LittleEndianByteArray$UnsafeByteArray`/`AbstractFuture$UnsafeAtomicHelper`/`UnsignedBytes$...$UnsafeComparator` 等忠实反编译出 `import sun.misc.Unsafe; ... Unsafe.getUnsafe()`, 但 harness 用 **`javac --release 8`** 编译——其 `ct.sym` 不含 `sun.*` 内部包, 故报 `程序包sun.misc不存在 / cannot find symbol: class Unsafe`(已用最小例 `import sun.misc.Unsafe` 实证: `--release 8` 失败、裸 `javac` 仅警告)。这与 §4 的 iso 假阳性同性质——**任何忠实反编译器(CFR/Vineflower 同样)在 `--release 8` 下都过不了**, 不是 JavaJive 缺陷, 也不阻碍真实重打包(用含 `sun.misc` 的 JDK 编译即可)。已登记到 §4。
   - **(b) 扁平嵌套类丢外层类型参数(`HashIterator`/`Segment`/`Itr` 一族, 约 40+ 行)**: 非静态内部类被扁平成 `Outer$Inner<T>` 后, 其成员引用的外层类型变量 `K,V,E,S` 无处声明 → `cannot find symbol: class K`。`dumper.go` 已对「**自身无形参**」的纯继承内部类注入自由类型变量(`JDEC_INNER_TYPEVAR_OFF`, 历史治本约 2000 行), 但「**自身又有形参**」的 `HashIterator<T>` 是显式残余: 若把外层 `K,V,E,S` 并入声明, 别处对 `HashIterator` 的引用仍只带自有元数, 元数不一致("wrong number of type arguments")。须跨类协同重写所有引用点(integral rebuild), 深且高风险, 留专项。
   - 其余少量为局部命名/分裂不一致、合成 lambda/捕获变量名丢失等, 逐类复现处理。

8. **其余小桶**: `method invocation cannot be applied`(guava 50→46, 本轮 -4 经擦除型类型变量造型抑制; 残余以 `SortedLists.binarySearch` 重载消歧与通配符为主)、`invalid method reference`(本轮 `::new_` 一类已治本, 残余为 functional-interface / 构造器引用目标类型擦除——fastjson2 `AtomicBoolean::new`/`File::new` 因目标 SAM 被擦成 raw `Function` 而 `invalid constructor reference`)、`abstract method not overridden`(guava 6)、`an enum annotation value must be an enum constant`(guava 3) 等。逐个按 [`HARNESS.md`](../HARNESS.md) 流程清零。

---

## 4. iso 口径的已知假阳性(**不是缺陷**, 不要去"修")

iso 把每个扁平单元单独编译, 以下失败是方法学产物, 在 tree(重打包)口径下不存在:

- `cannot find symbol: class Outer$Inner` — 扁平 `$` 类型引用对着原始 jar 解析不到(jar 里是源名 `Outer.Inner` 的嵌套类)。codec 30/38、fastjson2 281/342、guava 568/1149 的 iso 失败几乎全是这一类。
- `X has private/protected access in Y` — 内部类合法访问外层私有/保护成员, 单文件编译看不到同编译单元豁免。
- `cannot access X` — 需要兄弟类字节码而 iso classpath 未含其反编译产物。
- **`程序包sun.misc不存在` / `cannot find symbol: class Unsafe`(`sun.misc.Unsafe`, 连 tree 口径也中招)** — harness 用 `javac --release 8`, 其 `ct.sym` 只暴露 JDK 8 受支持 API, **不含 `sun.*` 内部包**。`guava` 里忠实反编译出 `sun.misc.Unsafe` 的类(`Striped64`/`UnsafeByteArray`/`UnsafeAtomicHelper`/`UnsafeComparator` 等, 约 **45** 行 `cannot find symbol`)因此编不过。这是 **`--release` 编译模式的环境产物, 不是反编译缺陷**(任何忠实反编译器同样过不了; 裸 `javac` 仅警告), 真实重打包用含 `sun.misc` 的 JDK 即可。计入 guava 478 的口径里但**不应算作可治本缺陷**。

> 因此 iso 数**不是**北极星指标; 它只用来侧写"哪些类涉及跨类引用"。真正口径永远是 tree。

---

## 5. kill-switch 索引

| 开关(置位即关闭对应治本) | 作用域 |
|---|---|
| `JDEC_GENERIC_PARAM_INFER_OFF` | JDK 泛型方法实参的接收者参数化造型(全部) |
| `JDEC_GENERIC_PARAM_FIELD_OFF` | 仅关字段接收者旁路(从字段 Signature 还原类型参数) |
| `JDEC_GENERIC_SELFMETHOD_PARAM_OFF` | 同类自有泛型方法实参造型(从方法 Signature 还原类作用域类型变量, 公有+私有) |
| `JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF` | 仅关私有同类方法(invokespecial 且目标类==当前类)的扩展, 恢复旧的 invokespecial 一刀切 |
| `JDEC_GENERIC_SUPER_METHOD_OFF` | 继承超类型泛型方法签名并入(this 接收者, 恒等映射一层, 跨类 resolver 加载) |
| `JDEC_GENERIC_RESOLVE_OFF` | 统一跨类泛型解析器(沿接收者泛型超类型层级 DFS + 类型实参替换 σ, 覆盖非-this/非恒等/深链; 附加式兜底, 关闭后回退到上面的特例拼盘) |
| `JDEC_NO_ERASED_TYPEVAR_NOCAST` | 擦除型类型变量的多余 upcast 抑制(被调形参是其自身/声明类的形式类型变量时, 丢弃 `(Bound)` 无操作造型以保推断; 关闭后回归旧的擦除-bound 造型) |
| `JDEC_CTOR_METHODREF_FIX_OFF` | 构造器方法引用 `::new` 渲染(关闭后回归 `SafeIdentifier` 误清洗出的 `::new_`) |
| `JDEC_RETURN_DECL_FIX_OFF` | 返回-嵌入赋值局部的声明合成(跳过 `return varN` 伪声明 + 以方法返回类型声明) |
| `JDEC_OBJECT_RET_DOWNCAST_OFF` | 返回点 Object→具体引用类型 向下造型 |
| `JDEC_POP_ELIDE_OFF` | pop/pop2 裸值语句 elide |
| `JDEC_NO_ENUM_SWITCH_FOLD` | enum-switch `$SwitchMap` 折回 |
| `JDEC_NO_ENUM_FOLD` | enum 常量体子类内联 |
| `JDEC_LIVEINTERVAL_OFF` / `JDEC_LIVEINTERVAL_WEB` | 活跃区间声明摆放 / web 复用 |
| `JDEC_TYPELUB_OFF` / `JDEC_TERNARY_DECL_LUB_OFF` | 类型 LUB / 三元声明点 LUB |
| `JDEC_GENERIC_INFER_OFF` | JDK 泛型方法返回实例化 |
| `JDEC_NESTED_PUBLIC_OFF` | 嵌套类型 public 复原 |

> 更多底层开关(`JDEC_SLOT_*` / `JDEC_*REACHING*` / `JDEC_IF*` / `JDEC_TRY_*` / `JDEC_LOOP_*` 等)散落在 `decompiler/` 源码中, 用 `rg 'os.Getenv\("JDEC_' classparser/decompiler` 列全。

---

## 6. 验收红线(每轮治本必须全满足)

1. 选靶用 tree inventory, 单点根因, 不打过用例补丁。
2. 复杂改动配 `JDEC_*` kill-switch; A/B delta(`OFF-ON`)对**所有** 4 jar 必 ≥ 0(本 jar 降、它 jar 不升)。
3. 配承重测试(ON/OFF 断言)+ 回归种子(`testdata/regression/*.class` + `.golden`)。
4. 全量 `go test ./...` 30s 内全绿; 确定性测试不得变红。
5. 安全契约: 永不输出不可解析的 Java; 宁可带标记 stub 也不输出"看似对实则错"; 不 panic。

# TODO — 当前缺陷工单(可执行 / 可复现)

> 这是「下一步修什么」的可执行清单。怎么验证、怎么一个一个清零, 见 [`HARNESS.md`](./HARNESS.md);
> 完整状态账本与生效中的安全开关见 [`classparser/CODEC_TODO.md`](./classparser/CODEC_TODO.md);
> 面向用户的评测报告见 [`BENCHMARK.md`](./BENCHMARK.md)。
>
> **口径**: 全部以 **tree(整树重编译)** 为准 —— 这是「反编译→重编译→重打包→可调用」的真口径。
> iso 口径的 `cannot find symbol`/`private access` 大多是扁平 `$` 假阳性, 不在此列(见 CODEC_TODO §3)。
>
> 数字快照(javac 17 Corretto, 本机 `~/.m2`; tree errLines / 缺陷类, 复跑见下方命令):
> codec 0/0 ✅ · gson 0/0 ✅ · jsoup 1/1 · snakeyaml 1/1 · fastjson2 17/11 · guava 27/23 · commons-lang3 11/8 · spring 25/16。（合计 82）
> (本轮一修: if/else 兄弟臂 boolean phi 合并(`reachingBoolVarCopyMerge`, `JDEC_BOOL_VAR_COPY_MERGE_OFF`)—— 一臂复制/三元生成 boolean-default(`previous = (features & mask) != 0` 编成 `iconst_1/0`, 或直接三元 `(c && cond) ? 1 : 0`), 另一兄弟臂存真 boolean 值(Z-返回调用 `isRefDetect()`)。复制臂的 int-typed ref 与 boolean 臂的新 boolean ref 分裂同一变量, 合流读 `if (itemRefDetect)` / `previous = itemRefDetect` 渲染成 `int = boolean` / `boolean != int`, javac 报「boolean cannot be converted to int」。`reachingBoolDefaultMerge` 用 `reachingSlotStoreOps` 走 Source 回溯看不到兄弟臂定义(无路径), 故不触发; 本修复锚点在 boolean 臂 store, 直接从全局 slot 表的 `current` ref 找其 creator store(新增 `refToCreatingStore` 索引, 绕开 opcodeIdToRef 的 map 无序迭代), 见 RHS 是 int-0/1 字面量(shape a) 或复制另一槽而该槽源是 int-0/1 字面量(shape b), phi 证同变量即重定型为 boolean(连同源 default 的 0/1 字面量)。修 fastjson2 `FieldWriterList.writeList`(复制臂) + `ObjectWriterImplList.write`(三元臂, fastjson2 tree 19→17、缺陷类 12→11)。承重 `bool_var_copy_merge_test.go`(BoolVarCopyMergeSeed)。零回归(A/B delta 全 8-jar ≥0)。证明了 CODEC_TODO §8a 所述「19 条铁板一块须整体重构」可逐条甄别单点突破。)
> (本轮修三块, 均零回归、A/B delta≥0:
> ① 方法引用不补函数式接口造型——`Type::m`/`receiver::m`/`Type::new` 原生可绑到 raw SAM(无显式形参可冲突),
> 造型反而在 SAM 嵌套通配符处(`Stream.flatMap` 的 `Function<? super T,? extends Stream<? extends R>>`)钉死具体参数化、
> 挫败 javac 多态推断。靠 `CustomValue.IsMethodRef`(bootstrap 方法引用分支置位)在
> `lambdaArgFunctionalCast`/`lambdaArgRawJDKReceiverCast` 跳过方法引用。修 fastjson2 `ObjectReaderCreator.toFieldReaderArray`
> `flatMap(Collection::stream)`(fastjson2 tree -1)、spring `AnnotatedTypeMetadata` `collect(Collector<...>)`(spring tree -3)。
> ② 活跃区间 web 读/写重定向修复翻成默认开(`JDEC_LIVEINTERVAL_WEB_OFF`)——重测 8-jar tree 口径是严格改进,
> fastjson2 24→22(`ObjectReaderCreator` 3→2、`JSONPathParser` 2→1), 其余 jar 全持平。
> ③ 泛型方法返回 `Supplier<T>`/`Function<T,R>` 的 lambda 体返回擦除 Object, 丢源码 unchecked `return (T)/(R) expr;` 造型,
> javac 拒「bad return type in lambda expression」。从 enclosing 方法 Signature 返回类型取该 FI 返回位类型变量,
> 注入 lambda 体值返回处(`JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF`)。修 fastjson2 `ObjectReaderProvider.createObjectCreator`
> `() -> (T) objectReader.createInstance(0)` + `ObjectReaderCreator.createBuildFunctionLambda` `(l0) -> (R) m.invoke(...)`(fastjson2 tree -2)。
> ④ 构造器 RAW 函数式接口形参位收 UNBOUND 实例方法引用(如 `Throwable::setStackTrace`, 实现元数 (Throwable,StackTraceElement[]))
> 绑不到 raw (Object,Object) SAM, javac 报「invalid method reference」。从 invokedynamic instantiatedMethodType 取实参类型,
> 重发 `(<FIRawClass><<具体类型>>) Type::method` 造型(`JDEC_CTOR_RAWFI_METHODREF_CAST_OFF`)。修 fastjson2
> `ObjectReaderCreator` `new FieldReaderStackTrace(..., Throwable::setStackTrace)`(fastjson2 tree -1、缺陷类 13→12)。CFR/Vineflower 亦丢此造型。
> 合计本轮 25→19 / 缺陷类 14→12。)
> (本轮调查 jsoup/snakeyaml 清零未果, 记录潜伏链供后续: ① jsoup `XmlTreeBuilder.insert(Token$Comment)` 的「`Comment var3 = var2; ...;
> var3 = new XmlDeclaration();` 首声明窄化致兄弟再赋失败(`XmlDeclaration cannot be converted to Comment`)」**单点可修**——
> 跨类兄弟臂合并已把 slot ref 扩宽到 LUB(Node), 但首声明 RHS 仍是窄臂类型 Comment; 修法: 在 AssignStatement 首声明处见 ref 被合并 helper
> 扩宽(经 `PhiWidened` 标记证)时采纳 slot 类型。**但**: 该修解锁 jsoup `QueryParser.combinator` **潜伏 7 条** definite-assignment
> (var5_1/var5_2 单分支赋值后无条件用, 属 T4b computeIfAbsent 惯用式, 之前被 XmlTreeBuilder 类型错遮蔽)——原 1 错变 7 错, 净 -6。
> ② snakeyaml `SafeConstructor.createNumber` 即 **T4b 文档化的 special-project**(try/catch 合流 `var6` 只在 catch 赋值,
> `return var6` 未初始化)—— 核心数据流 + 全量 A/B, 留专项。结论: 两单点清零均被 T4b 遮蔽依赖阻塞, 须先解 T4b(单用折叠前判 slot 下游 load
> 或合流 load 建 phi)才能净清零; 本轮先回退 jsoup 修保 96 合计、零回归, AtomicReference 修复保留。)
> (上一轮修: 方法形参自由类型变量注入(dumper.go 扁平内部类 enclosing-arity 注入块增方法形参扫描,
> `JDEC_INNER_METHODPARAM_TYPEVAR_INJECT_OFF`)—— 静态泛型方法内的匿名类捕获方法的类型变量, javac 发出的
> 该匿名类 Signature **不带** `<...>` 形参段(只列自由变量引用: guava `Futures$2` 的类签名是
> `Ljava/lang/Object;LFuture<TO;>;`, 无 `<O:...>` 前缀), 于是 O 从父类 `Future<O>` 被识别为自由变量并声明,
> 但只在**私有方法形参**里出现的自由 `I`(`private O applyTransformation(I var1)`)不在父类/字段里, 保持未声明
> ("cannot find symbol: class I")。修法: 在注入块补扫方法形参签名(`TypeVarRefsInMethodParams`), 把这类只在
> 方法形参出现的自由变量也声明成形参(`Futures$2<O, I>`), 与 O 的复原对称; raw `new Futures$2(...)` 站点是
> 原始实例化不受影响。承重测试 `method_param_typevar_inject_test.go`(MethodParamTypeVarSeed$1)。
> 治 guava Futures$2 1 条, guava 28→27, 合计 98→97, 零回归。)
> (上一轮四修: `EnumSet.of(...)` 实参禁 `(Enum)` 上转(`jdkCalleeParamIsErasedTypeVar` 增 EnumSet.of 分支,
> `JDEC_ENUMSET_OF_NOCAST_OFF`)—— `EnumSet.of(E first, E... rest)` 的方法作用域 `E extends Enum<E>` 在描述符里
> 擦成 `java.lang.Enum`, 实参造型逻辑把具体枚举常量上转成 raw `(Enum)`, 反而塌掉 javac 对 E 的推断、破坏重载决议
> ("no suitable method found for of(Enum,TaskOption[])")。修法: 与 Enum.compareTo 同根(JDK 被调方形参是擦成
> 非 Object 边界的自身类型变量), 按精确被调方(java.util.EnumSet + of + Enum 擦除形参)键控禁造型, 让 javac 从实参
> 推断 E; 字节码中实参本就流入 E, 去造型保行为。承重测试 `enumset_of_nocast_test.go`(EnumSetOfSeed)。
> 治 spring ConcurrentReferenceHashMap$Task 构造器 1 条, spring 32→31, 合计 99→98, 零回归。)
> (本轮三修: `System.getenv()` 返回补 raw 造型(`concreteParamReturnSubtypeRawCast` Shape 3, 同开关)——
> 声明返回 `Map<String,Object>`, 返回值是 0 参 JDK 静态 `System.getenv()`, 其 JDK 签名是**固定的**
> `Map<String,String>`(同擦除、不同实参, 裸返回永不隐转、直接参数化造型 inconvertible), 源码原带 raw `(Map)` 造型。
> 上上轮该 helper 因「同擦除跳过」条件治不了此条; 修法: 在同擦除早退**之前**加 Shape 3 分支, 按**精确被调方**识别
> (静态 + 0 参 + getenv + java.lang.System), 绝不按值类型匹配, 故 poly/推断工厂不可能命中; 声明实参恰为
> `<String,String>` 时不补(对照)。承重测试 `getenv_return_raw_cast_test.go`(GetenvReturnSeed, 含精确匹配对照方法)。
> 治 spring AbstractEnvironment.getSystemEnvironment 1 条, AbstractEnvironment 全清, spring 33→32, 合计 100→99, 零回归。)
> (本轮二修: `this(...)` 裸类型变量实参补造型(`thisCtorTypeVarArgCast`, `JDEC_THIS_CTOR_TYPEVAR_ARG_OFF`)—— 便捷构造器经 `this(...)` 委托给同类兄弟构造器, 目标形参是**裸类类型变量** `T`(自已记录的构造器 Signature 恢复), 实参被擦除成边界(`new Object()`)。源码原写 `this(name, (T) new Object())`, unchecked `(T)` 造型在字节码无 checkcast 被丢, 裸渲染被 javac 拒("Object cannot be converted to T")。修法: 仿 `ctorWildcardArgCast`(通配符参数化版), 同门控(当前类 `<init>`+接收者必须是 `this`, 排除静态工厂里类型变量出栈的 `new CurrentClass(...)`), 形参必须是裸类作用域类型变量, 实参非 null/非原语/非已是该变量, 补 `(T)` unchecked 造型。承重测试 `this_ctor_typevar_arg_cast_test.go`(ThisCtorTypeVarSeed)。治 spring PropertySource(String) 1 条, spring 34→33, 合计 101→100, 零回归。)
> (本轮一修: `Class.forName(...)` 返回补造型(`classForNameReturnCast`, `JDEC_CLASS_FORNAME_RET_CAST_OFF`)—— 方法声明返回是**提及类型变量的** `Class<...>` 参数化(`Class<ObjectInstantiator<T>>`), 返回值是 `Class.forName(name)` 静态调用。JDK 签名是 `Class<?> forName(String)`, javac 把通配符 capture 成 CAP#1 判 `Class<CAP#1>`→`Class<ObjectInstantiator<T>>` 不可转, 源码原带 unchecked `(Class<...>)` 造型(擦除 checkcast 被字节码丢)。修法: 精确匹配 `Class.forName` 这一「通配符返回但反编译渲染成 raw」的 JDK 方法(经 ClassName/接收者类型确认 java.lang.Class), 见 `Class<...>` 提及类型变量的返回即补单造型 —— `Class<?>`→`Class<X>` 恒是合法 unchecked capture, 绝不 inconvertible。窄门控只认 `Class.forName`, 绝不碰 poly 工厂(其返回 javac 独立推断出具体参数化, 即 guava `ImmutableMap.of()`「incompatible bounds」族——正是上上轮「raw 值参数化返回造型」过宽 +47 回归的根源, 此次窄化避开)。承重测试 `class_forname_return_cast_test.go`(ClassForNameRetSeed)。治 spring objenesis DelegatingToExoticInstantiator.instantiatorClass 1 条, spring 35→34, 合计 102→101, 零回归。)
> (上一轮二修: 跨接收者通配符捕获返回造型(`crossRecvWildcardReturnCast`, `JDEC_CROSS_RECV_WILDCARD_RET_CAST_OFF`)—— 返回值是**非 this 接收者**(字段/局部, jar 内类)上的实例调用, 其复原泛型返回是与「提及类型变量的返回类型」同擦除的**通配符**参数化(`Class<A> getType(){ return this.mapping.getAnnotationType(); }`, getAnnotationType 真返回 `Class<? extends Annotation>`)。javac 把通配符 capture 成 CAP#1 判 `Class<CAP#1>`→`Class<A>` 不可转, 源码原带 unchecked `(Class<A>)` 造型(擦除 checkcast 被字节码丢)。typeVarReturnCast 自身的通配符分支只认 this 接收者同类方法(`funcCtx.MethodSignature`); 本修经跨类兄弟解析器(`ResolveInstantiatedReturnType`)复原任意 jar 内接收者(字段/局部)上被调方法的返回签名, 见通配符+同擦除即补单造型(通配符→类型变量同擦除是 unchecked 合法)。承重测试 `cross_recv_wildcard_return_cast_test.go`(CrossRecvWildcardSeed, 需 resolver 见兄弟 Holder 签名)。治 spring TypeMappedAnnotation.getType 1 条, spring 36→35, 合计 103→102, 零回归。)
> (上一轮一修: 具体参数化返回收「非泛型子类型」raw 造型(`concreteParamReturnSubtypeRawCast`, `JDEC_CONCRETE_PARAM_RET_SUBTYPE_RAW_CAST_OFF`)—— 方法声明返回是**具体**参数化类型(`Map<String,Object>`, 不提及在场类型变量), 而返回值静态类型是该擦除的**非泛型子类型**、其父类型实例化被固定为**不同**参数化(`Properties` 即 `Map<Object,Object>`、`AbstractEnvironment$1` 即 `Map<String,String>`)。裸 `return System.getProperties();` 被 javac 拒「Properties cannot be converted to Map<String,Object>」, 且直接 `(Map<String,Object>)` 造型因不变量互斥判为 inconvertible; 源码原带 raw `(Map)` 造型(擦除后 checkcast 是 no-op 被字节码丢弃, 须由泛型分析重铸)。修法: 返回具体参数化(有≥1 具体实参、不提及类型变量)且返回值为子类型(擦除不同)时——jar 内非泛型子类型经 `genericReturnSubtypeCastNeeded`(0 形参+跨类解析器确认), 或白名单 JDK 非泛型子类型(`Properties`, 经 `IsReferenceSubtypeBridged` 桥接确认 `Properties<:Map`)——补 raw `(Map)` 造型。areturn 保证值可赋返回擦除, 故 raw 造型恒合法、raw→`Map<String,Object>` 是 unchecked 合法。承重测试 `concrete_param_return_raw_cast_test.go`(ConcreteParamReturnSeed)。治 spring AbstractEnvironment getSystemProperties(Properties+`$1`)/getSystemEnvironment(`$2`) 3 条, spring 39→36, 合计 106→103, 零回归。残 getenv 1 条(`Map<String,String>`→`Map<String,Object>` 同擦除, 反编译渲染 raw 未见实参, 属另一根 T?)。)
> (上一轮一修: 泛型局部不变量重赋值 raw 造型(`parameterizedLocalReassignRawCast`, `JDEC_PARAM_LOCAL_REASSIGN_RAW_CAST_OFF`)—— 局部变量从首赋值(参数 `Class<T> type`)定型为不变量参数化 `Class<T>`, 随后被同擦除但泛型不兼容的方法调用重赋(`result = result.getSuperclass()`, getSuperclass 真返回 `Class<? super T>` → capture, 不能转不变量 `Class<T>`)。反编译按首赋值定型太窄, 裸重赋值 javac 报「Class<CAP#1> cannot be converted to Class<T>」。修法: 局部(JavaRef 非 this)重赋值, LHS 声明为不变量(`<...>` 无 `?`)且提及在场类型变量, RHS 为方法调用、同擦除但渲染不同时, 补 raw `(Class)` 造型 —— raw 再 unchecked 转回 `Class<T>` 合法、运行时一致。承重测试 `local_reassign_raw_cast_test.go`(LocalReassignRawSeed)。治 objenesis SerializationInstantiatorHelper + PercSerializationInstantiator(各 1 条 getSuperclass 重赋), spring 41→39, 合计 108→106, 零回归。同轮试过「raw 值参数化返回造型」补 DelegatingToExoticInstantiator 的 `return Class.forName()`→`Class<ObjectInstantiator<T>>`, 但过宽(裸方法调用返回参数化到处都是)致 +47 回归(guava 28→64/spring 39→50), 已回退——该条(concrete 嵌套泛型返回目标)留后续窄化。)
> (上一轮一修: 扁平化匿名子类「覆写方法参数」外围类型变量擦除 + 生擦收方泛型局部定型回退(`superIsOwnFormalFlattenedSibling` + `typeVarLocalDeclName` 生擦收方 bail)—— spring-core `ConcurrentReferenceHashMap$1..$5 extends $Task<V>` 系一族匿名子类。基类 `$Task<T>`(own-formal `<T>`, 非静态内部类)的 `execute(Reference<K,V>, Entry<K,V>, ...)` 因无法声明外围 K/V 走 case(a) 生擦为 raw `execute(Reference, Entry)`; 而匿名子类无自身形参、经 enclosing-arity 注入声明了 `<K,V>`, 覆写渲染成泛型 `execute(Reference<K,V>, Entry<K,V>)` → 与 raw 基类同擦除但互不覆写, javac「name clash ... same erasure, yet neither overrides」×5。修法: 新增 case(b)——no-own-formal 类的直接父类若是「同一顶层嵌套、own-formal、`$` 命名的兄弟」(`superIsOwnFormalFlattenedSibling` 经 foldSiblingResolver 解析父类确认), 则把其覆写方法参数位的外围类型变量一并擦除, 使覆写与基类同为 raw、恢复覆写关系。配套两处: (1) 声明变量(注入的 K/V)不做 standalone-erase, 保住 `V execute(...)` 返回不被降成 Object(否则「cannot override」); (2) `typeVarLocalDeclName` 见接收方类型实参含被生擦的类型变量(如 `Entry<K,V> var2` 渲染为 raw `Entry`)时 bail, 令 `V v = var2.getValue()` 退回 `Object v`(getValue 在 raw 接收方上是 unchecked 调用返回擦除界), `(V)` 由返回造型补。承重测试 `override_param_erase_test.go`(OverrideParamEraseSeed: case(b) raw 参数 + V 返回 + 生擦收方 `(V)` 造型), 开关 `JDEC_INNER_RAW_ERASE_METHOD_PARAM_OFF`。治 spring ConcurrentReferenceHashMap$1..$5 name-clash 5 条(残 `$Task` `EnumSet.of` 造型 1 条属另一根 T?), spring 46→41, 零回归, 合计 113→108。)
> (上一轮一修: 兄弟臂分配桥接 LUB 合并(`reachingRefSlotObjectSiblingArmMerge`, `JDEC_REF_SLOT_OBJECT_SIBLING_ARM_MERGE_OFF`)—— 两条互斥分支各自 `new T()` 分配一个引用类型存入同一槽, 分支合流后被共同读取(`Object obj; if(c=='{')obj=new JSONObject(); else obj=new JSONArray(); reader.handleResolveTasks(obj); return obj;`)。窄合并都无法关联两臂: 跨类 jar 内 LUB 需两臂在 jar 内共享祖先(JSONObject/JSONArray 各继承 JDK LinkedHashMap/ArrayList, jar provider 只看到直接 JDK 父名, 看不到 Map/List/Object), JDK 表 LUB 又不认 jar 叶。于是 DFS 晚臂分裂、合流读在另一分支未初始化 → definite-assignment。修法: 补「桥接 LUB」`BridgedCommonSuperType`(jar 父链走 provider, 触到 JDK 类即接入 jdkSuperEdges 表继续走), 把共享 ref 加宽到桥接最近公共祖先。两臂皆 `new`(经 `resolveIsNonArrayAllocation` 穿透 aload 复制链)+phi 门控, 桥接 LUB 精准: JSONObject|JSONArray→java.lang.Object(JSON.parse), HashMap|JSONObject(extends LinkedHashMap extends HashMap)→java.util.HashMap(JSONReaderJSONB.readObject, 若盲目升 Object 则 `map.put` cannot find symbol)。承重测试 `object_sibling_arm_merge_test.go`(ObjectSiblingSeed: Object 与桥接 HashMap 两形态)。治 fastjson2 JSON.parse(3 DA→1 类型)+JSONReaderJSONB.readObject(4→2), fastjson2 31→25, 零回归。合计 119→113。残余 JSON.java:82 系 slot6 复用(JSONObject 临时槽 + Object 返回持有)本被 DA 遮蔽, 现显形为类型不符, 属活跃区间分裂长尾(T2)。)
> (上一轮一修: 条件 null 重赋值 phi 合并(`reachingRefSlotNullReassignMerge`, `JDEC_REF_SLOT_NULL_REASSIGN_MERGE_OFF`)—— 已定型局部先存具体引用值(`Character c = Character.valueOf(chars[i])`), 又在一条分支被重赋 null(`if(c.charValue()==0) c=null;`), 随后分支合流后被读(`get(c)`); 定型 def 与 `=null` def 都到达合流读, 是同一变量(null 可赋任意引用)。但各臂合并 helper 均对 null 值 bail, AssignVarGuarded 见 null 不携类型无法匹配 current 的 Character ref, 遂为 null 存储另铸 Object 变量; 定型 def 只剩 `charValue()` 一处使用被单用折叠吃进条件、其存储被丢, 合流读绑到只在 null 分支赋值的新变量 → javac「variable might not have been initialized」。修法: 值为 null 字面量且 current 为已定型非 param 非 null-init 非 Object 引用, phi 证 null 存储与 current def 共同到达下游 load 时, 保持 current(null 存储成普通 `c=null` 重赋值)—— 不加宽不收窄, 类型合法接纳 null, phi 门控使真正不相交的槽复用仍分裂。治 snakeyaml Resolver.addImplicitResolver + commons-lang3 LocaleUtils(countriesByLanguage/languagesByCountry)。同时该合并在栈模拟阶段就并回 JSONPathParser.parseFilter 的 slot-7 null, 故 orphan-rebind 承重测试改为整 jar 树口径(见 `orphan_global_rebind_test.go`)。合计 123→119。)
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
    - **本轮实验**: 尝试在 `astore` 点对首声明局部(`Iterator it = map.entrySet().iterator()`)用 `ResolveInstantiatedReturnType` 复原参数化返回定型(`upgradeLocalParamType`), 并扩 `InstantiateJDKMethodReturn` 覆盖 `Map.entrySet/keySet/values`。**回归**: guava A/B delta=-1~-2(ON=28~41 vs OFF=27), 新增 `Iterator<Entry<CAP#1,CAP#2>> cannot be converted to Iterator<Entry<? extends K,? extends V>>` 一族——参数化后不变型严格性反比 raw 更严。**结论**: T1(a) 与 T1(b) 通配符捕获**深度耦合**, 单点局部泛型传播不可行, 须 T1(a)+T1(b) **协同专项**(通配符接收者整体 raw 造型 + 局部泛型传播联动)。已回退, 留协同专项。
  - **(b) 通配符捕获 `CAP#1`(guava)** —— **oracle 实证内在难, 优先级下调**: `this.equivalence.equivalent(a,b)`, 字段 `Equivalence<? super T>` 捕获 `CAP#1`, 实参 Object 不可造 `(CAP#1)`。`ORACLE_JAR=guava ORACLE_CLASS='base/Equivalence$Wrapper'` 三方全败(真源码用 `(Equivalence<Object>)` + `@SuppressWarnings`)。方向(若做): 通配符接收者**整体** `<Object>` 造型。
    - **本轮增量**: 新增**通配符上界擦除窄化**(`wildcardExtendsBoundErasure`, kill-switch `JDEC_TYPEVAR_FIELD_WILDCARD_NOCAST_OFF`)——当字段/返回值的通配符是 `? extends ConcreteClass` 且上界擦除与目标对应参数擦除**不同**时(guava `ImmutableMultimap.asMap`: 字段 `ImmutableMap<K, ? extends ImmutableCollection<V>>` → 返回 `ImmutableMap<K, Collection<V>>`), 不补 inconvertible 造型, 改诚实裸 `return this.map`(走 unchecked conversion)。全量零回归(guava/spring/fastjson2/commons-lang3 tree errLines 均持平 27/31/25/12), 渲染更接近 CFR。`? super X` 场景(如 `Comparator<? super E>`→`Comparator<Object>`)不 block, 保留原 unchecked 造型。CAP#1 本身仍内在难(三方 oracle 均败)。
  - **(d) 返回点「inference variable X has incompatible bounds」(guava 头号桶, ~13 行: ImmutableEnumMap/Range/Maps/Ordering/Striped/Sets$DescendingSet/MinMaxPriorityQueue$Builder/Tables$TransposeTable 等)** —— **有界类型变量同擦除强转 javac 实测拒绝, 不可用 `parameterizedReturnCast` 治**: 形如 `return ImmutableMap.of(k.getKey(), k.getValue());`(目标 `ImmutableMap<K,V>`、K/V 或 `C extends Comparable` 有界), javac 对 `of()` 独立推断出 `<Object,Object>` 后再与目标合流报「incompatible bounds」。实测 `(ImmutableMap<K,V>)(ImmutableMap<Object,Object>)` **当目标实参有界(K extends Enum / C extends Comparable)时 javac 直接报「不兼容的类型…无法转换」**(见 `parameterizedReturnCast` 注释里「BOUNDED var 被排除」的原因)。真源码用 `@SuppressWarnings` + 不同结构强转, 从擦除字节码无法忠实复原。**根因仍是 (a): 局部变量/字段(如 raw `Map.Entry var1`)的泛型未沿声明传播复原**——只要把 `var1` 复原为 `Map.Entry<K,V>`, `getKey()/getValue()` 即回到 K/V, 无需强转。故此桶归 (a) 的局部/字段泛型传播, 不要再尝试返回点强转。
  - **(c) 装箱/数值**: `int cannot be converted to Integer` 等(**非擦除, 不可盲目造型**), 按 `Integer.valueOf` 修。
    - **本轮评估**: baseline 全量扫描(guava/spring/fastjson2/commons-lang3)**无真正原语→包装类错误**(`int cannot be converted to Integer` 等均不存在)。唯一 `Long cannot be converted to Integer`(fastjson2 ObjectWriterCreatorASM:2381) 实为 T2 槽位复用混淆(var11 槽位先存 Integer 后存 Long), 非 T1(c)。**T1(c) 当前无选靶, 跳过**。
- 复现:
  ```bash
  go build -o /tmp/jj ./cmd/javajive
  /tmp/jj decompile -o /tmp/guava ~/.m2/repository/com/google/guava/guava/28.2-android/guava-28.2-android.jar
  ORACLE_JAR=guava ORACLE_CLASS=Equivalence go test -run TestThirdPartyOracle -v ./test/cross/
  ```

### T2. 活跃区间分裂 / 槽位复用类型混淆(fastjson2 `bad operand type` / `unexpected type` 一族)
- 表象: 一个字节码 local 槽在**互不相交的活跃区间**里先后承载**不兼容类型**的值(如 `JSONPathFilter$GroupFilter` 的 `var9` 既作 `Iterator` 又当 int 比较), 反编译却合成单一变量名 + 单一声明类型。
- 已治本多族 disjoint 槽(兄弟臂 LUB 合并 / Object 超类臂 / 数组协变父臂 / 布尔字段·返回槽拆分 / 跨作用域孤儿读重放, 见 CODEC_TODO §4)。**本轮新增**: 活跃区间 web 读/写重定向(`reachingSlotVersionByWeb`/`reachingSlotStoreContinuationByWeb`)翻成默认开(`JDEC_LIVEINTERVAL_WEB_OFF`), 把 DFS 序漏进槽位表的「同源变量(同 VarUid)」load/store 重定向到该 web 规范 ref。A/B 全 8-jar delta≥0, fastjson2 tree 24→22(`ObjectReaderCreator` 3→2、`JSONPathParser` 2→1), 其余 jar 持平。**残余**: 非布尔子形态须在变量定型/分裂核上按「区间+类型」更激进拆分同槽, 风险高、改动核心, 留专项。
  - **本轮评估**: baseline 非布尔槽位混淆仅 ~3 错误(guava `LocalCache$Segment:72` Object→V、`MapMakerInternalMap$Segment:315` InternalEntry→E、fastjson2 `ObjectWriterCreatorASM:2381` Long→Integer), 占总 ~87 错误 ~3%。非 bool 分裂逻辑复杂度与 bool 版本相当(数百行)且回归风险高, **性价比不足**, 暂不投入, 保留现有 bool 分裂。
- 复现: `ORACLE_JAR=fastjson2 ORACLE_CLASS=JSONPathFilter go test -run TestThirdPartyOracle -v ./test/cross/`

## P1

### T3. 扁平嵌套类丢外层类型参数 `cannot find symbol: class K/V/E/S`(guava `HashIterator`/`Segment`/`Itr`)
- 根因: 内部类**自身又有形参**时, 注入外层 `K,V,E,S` 会与引用点元数不一致。「自身无形参」的纯继承内部类已治本(`JDEC_INNER_TYPEVAR_*`)。
- 方向: 跨类协同重写所有引用点(integral rebuild), 深且高风险, 留专项。
- 复现: `rg 'cannot find symbol' /tmp/jdec-inv/guava.tree.fails.txt`, 取 file:line 后 `/tmp/jj decompile` 复现。

### T4. 三元 LUB `bad type in conditional expression`(fastjson2 + guava)
- 已有 `CommonSuperType`(`decompiler/core/values/types/hierarchy.go`), 已治本反射家族与跨类直接子类型两支(`JDEC_TYPELUB_OFF` / `JDEC_TERNARY_DECL_LUB_*`)。
- 残余: 渲染期造型未反馈到三元类型 + 三元臂泛型擦除(归 T1)。方向: 扩 JDK 层级表 + 在更多 phi/合流点接入。

### T4b. 折叠首赋值在分支后仍活跃 → definite-assignment(snakeyaml createNumber / commons-lang3 LocaleUtils)
- 根因(已实证): 手写 computeIfAbsent 惯用式 `V x = expr; if(x==null){...; x=...;} return x;`(字节码 `astore V; aload V; ifXX; ...; astore V; L: aload V; areturn`)。def@首 store 到 slot 的值被单用折叠**吃进条件**(`if(expr==null)`)并丢掉首 store, 但该 slot 在分支后 `return x` 仍被读(分支未取的那条路径 def@首仍到达)。可达定义分析把合流 load 只归给分支体内的 def, 首 def 遂被判"单用"折走 → 合流读绑到只在分支体赋值的变量 → `variable might not have been initialized`。
- 关键: 两 def 同 slot 同类型(List/Number), 是**同一变量**, 应在合流 load 处做 phi 统一(load 侧), 而非 store 侧臂合并——本轮试过 store 侧 supertype-arm 合并, 因 try/catch 两 store 互不为对方 current 而不触发, 且放开会 +10 回归(已回退)。
- 方向(高风险, 留专项): 单用折叠前, 若被折 ref 的 slot 在紧邻使用点之外还有下游 load(经 `slotDefPhiReachesLoad` 可判), 则不折、保留首 store; 或在合流 load 建 phi 统一同槽同型 def。核心 dataflow, 需 kill-switch + 全量 A/B。
- 复现: `go run` JarFS ReadFile(注意 tree 用 snakeyaml-2.2 / commons-lang3-3.12.0, 与 `find` 默认版本不同), 看 createNumber(slot6 Integer/Number) 与 languagesByCountry/countriesByLanguage(slot1 List)。

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

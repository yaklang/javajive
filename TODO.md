# TODO — 当前缺陷工单(可执行 / 可复现)

> 这是「下一步修什么」的可执行清单。怎么验证、怎么一个一个清零, 见 [`HARNESS.md`](./HARNESS.md);
> 完整状态账本与生效中的安全开关见 [`classparser/CODEC_TODO.md`](./classparser/CODEC_TODO.md);
> 面向用户的评测报告见 [`BENCHMARK.md`](./BENCHMARK.md)。
>
> **口径**: 全部以 **tree(整树重编译)** 为准 —— 这是「反编译→重编译→重打包→可调用」的真口径。
> iso 口径的 `cannot find symbol`/`private access` 大多是扁平 `$` 假阳性, 不在此列(见 CODEC_TODO §3)。
>
> 数字快照(javac 21, 本机 `~/.m2` 含可选依赖; tree errLines / 缺陷类, 复跑见下方命令):
> codec 0/0 ✅ · gson 0/0 ✅ · jsoup 1/1 · snakeyaml 8/2 · fastjson2 32/15 · guava 31/20 · commons-lang3 19/12 · spring 81/39。

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

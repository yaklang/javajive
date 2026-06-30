# TODO — 当前缺陷工单(可执行 / 可复现)

> 这是「下一步修什么」的可执行清单。怎么验证、怎么一个一个清零, 见 [`HARNESS.md`](./HARNESS.md);
> 完整状态账本与已治本项见 [`classparser/CODEC_TODO.md`](./classparser/CODEC_TODO.md)。
>
> **口径**: 全部以 **tree(整树重编译)** 为准 —— 这是「反编译→重编译→重打包→可调用」的真口径。
> iso 口径的 `cannot find symbol`/`private access` 大多是扁平 `$` 假阳性, 不在此列(见 CODEC_TODO §4)。
>
> 数字快照(javac 17.0.12, 本机 `~/.m2`; 复跑见下方命令):
> codec tree=0 ✅ · spring tree=2 · fastjson2 tree=248 · guava tree=529。

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

### T1. 泛型擦除缺造型 `Object cannot be converted to T/K/CAP#1`(当前 `incompatible types` 桶: fastjson2 110 + guava 249, 含装箱非擦除项)
- **已治本五块**(见下「已治本」): 返回点 `objectReturnDowncast`(fastjson2 -21); JDK 泛型方法实参 · 值接收者 `InstantiateJDKMethodParam`(fastjson2 -2 / guava -4); JDK 泛型方法实参 · 字段接收者 `FieldSignatures` 旁路(fastjson2 -25 / guava -13); 同类自有泛型方法实参 · 公有 `MethodSignatures` 旁路(fastjson2 -22 / guava -84); 同类自有泛型方法实参 · 私有 invokespecial(guava -21, 本轮)。**剩余三类不同根因**(按 `cannot be converted to` 直方图):
  - **(a) 继承/非 this 接收者(下一步首选)**: 同类 public+private receiver 已全覆盖; 残余是 `this.get(objVal)` 的 `get` 来自接口/父类(`super`/继承未重写, 目标类!=本类), 或接收者是本类型局部变量/字段而非 `this`。需把 `MethodSignatures` 沿 jar 内父类型链上溯(跨类签名查表), 或对同类型非-this 接收者放宽 receiver 判定。
  - **(b) 通配符捕获 `CAP#1`(guava 40)** —— **oracle 实证内在难, 优先级下调**: `this.equivalence.equivalent(a,b)`, 字段类型 `Equivalence<? super T>` 捕获 `CAP#1`, 实参 Object 不可造 `(CAP#1)`。`ORACLE_JAR=guava ORACLE_CLASS='base/Equivalence$Wrapper'` 实测三方全败(CFR 发 `Equivalence<? super T> e = this.equivalence;` 亦不可编译; 真源码用 `(Equivalence<Object>)` + `@SuppressWarnings`)。方向(若做): 通配符接收者**整体** `<Object>` 造型(非对实参造型)。
  - **(c) 装箱/数值**: `int cannot be converted to Integer` 等(**非擦除, 不可盲目造型**), 按 `Integer.valueOf` 修。
- 复现:
  ```bash
  go build -o /tmp/jj ./cmd/javajive
  /tmp/jj decompile -o /tmp/guava ~/.m2/repository/com/google/guava/guava/28.2-android/guava-28.2-android.jar
  ORACLE_JAR=guava ORACLE_CLASS=Equivalence go test -run TestThirdPartyOracle -v ./test/cross/
  ```

### T1b. `cannot find symbol`(tree 口径: fastjson2 42 + guava 96)
- **注意**: 这是 **tree(整树)** 残留, 与 iso 扁平 `$` 假阳性不同(见 CODEC_TODO §4), 是真缺陷。
- **已治本一类**: 返回-嵌入赋值局部声明合成(`JDEC_RETURN_DECL_FIX_OFF`, fastjson2 -37, 本轮)。
- 剩余: 局部命名/分裂导致声明与使用 varN 名不一致、合成 lambda/捕获变量名丢失等。
- 复现: `rg 'cannot find symbol' /tmp/jdec-inv/guava.tree.fails.txt`, 取 file:line, `/tmp/jj decompile` 后看是「未声明」还是「名不一致」, 分别对应 rewrite_var.go 的声明合成 / 命名碰撞两条线。

## P1

### T2. `break outside switch or loop`(fastjson2 31)
- 代表类: `com/alibaba/fastjson2/JSONReader`(`:1148`)。根因: 标号 break / 循环-switch 嵌套结构化把 break 落到外面。
- 复现: `ORACLE_JAR=fastjson2 ORACLE_CLASS=JSONReader.class go test -run TestThirdPartyOracle -v ./test/cross/`
- 注意: 循环重建历史上易回归(见 CODEC_TODO 历史 Phase 4 档案), 必须 opt-in 开关 + 全量 A/B。

### T3. 泛型边界 `type argument K is not within bounds of type-variable C`(guava ≈89)
- 代表类: `com/google/common/collect/ImmutableRangeMap$1`(`:21`)。根因: 扁平嵌套类型丢了外层类型参数与 bound。
- 方向: 在扁平单元重建被擦的类型参数声明与 bound。

### T4. 三元 LUB `bad type in conditional expression`(fastjson2 11 + guava 12)
- 已有 `CommonSuperType`(`decompiler/core/values/types/hierarchy.go`)。方向: 扩 JDK 层级表 + 在更多 phi/合流点接入。开关 `JDEC_TYPELUB_OFF`。

## P2 — 小桶 / 长尾

| 工单 | 计数 | 代表 | 备注 |
|---|---|---|---|
| T5 `bad operand type` / `unexpected type` | fj2 14 / 9 | — | boolean/int 混淆、lvalue/rvalue 误判 |
| T6 `cannot be applied / no suitable method` | guava 52 | — | 实参类型/重载选择偏差 |
| T7 `invalid method reference` | fj2 9 | — | 方法引用 `::` 重建 |
| T8 `abstract method not overridden` | guava 6 | — | 桥接/抽象方法可见性 |
| T9 合成内部类 `this.val$e;` field-read pop | spring 2 | `EmitUtils$6` | **CFR/Vineflower 亦失败, 内在难 case**; 已知粗暴 elide field-read 会致 spring 812 大回归, 留长尾 |

---

## 已治本(勿重复; 详见 CODEC_TODO §2)

私有同类自有泛型方法实参造型(`JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF`, guava 550→529, 本轮: invokespecial 目标类==本类即私有同类调用, 非 super) · 返回-嵌入赋值局部声明合成(`JDEC_RETURN_DECL_FIX_OFF`, fastjson2 285→248) · 同类自有泛型方法实参造型 · 公有(`JDEC_GENERIC_SELFMETHOD_PARAM_OFF`, fastjson2 307→285 / guava 634→550) · JDK 泛型方法实参造型 · 字段接收者(`JDEC_GENERIC_PARAM_FIELD_OFF`, fastjson2 332→307 / guava 647→634) · JDK 泛型方法实参造型 · 值接收者(`JDEC_GENERIC_PARAM_INFER_OFF`, fastjson2 334→332 / guava 651→647) · 返回点 Object 向下造型(`JDEC_OBJECT_RET_DOWNCAST_OFF`, fastjson2 355→334) · pop/pop2 裸值语句(`JDEC_POP_ELIDE_OFF`, spring 14→2) · enum-switch 折回 · 核心非确定性 · 局部变量数据流 · 类型/三元 LUB 基建 · 泛型实例化 · 嵌套 public 复原。

## 工作纪律(摘自 HARNESS.md 红线)

- 一次只清一个单点; 动核心前先 tree inventory 定位到具体类+方法, 再用 `/tmp/jj decompile` 复现。
- 拿不准的难 case 先跑 `TestThirdPartyOracle` 看 CFR/Vineflower: 三方都败→可诚实 stub; 只有我们败→对照它们找偏差。
- 复杂改动必带 kill-switch + 承重测试 + 回归种子; A/B delta 对 4 jar 必 ≥0; 全量 `go test ./...` 全绿。

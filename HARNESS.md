# HARNESS — 正确性如何检验 / 长尾如何一个一个清零

> 本文回答三个问题: **(1) JavaJive 反编译器的正确性怎么验证? (2) 复杂 JAR 怎么做到反编译→重编译→
> 重打包→可调用? (3) 长尾缺陷怎么一个一个全部解决掉?**
>
> 配套: 当前状态账本 [`classparser/CODEC_TODO.md`](./classparser/CODEC_TODO.md) · 可执行工单 [`TODO.md`](./TODO.md)
> · CI 与跨平台 cross-test 说明 [`HARNESS-WORKFLOW.md`](./HARNESS-WORKFLOW.md)。
>
> **铁律: 所有结论以 harness 真跑出的数字为准, 禁止估算或编造**(用户规则: 以诚实数据为荣)。
> 调试日志走 `internal/log`, 一律英文。

---

## 0. 正确性的定义(北极星)

一个反编译器对复杂 JAR「正确」, 当且仅当:

1. **可反编译**: 不 panic、不漏类、不返回不可解析的 Java(无法重建的成员退化为带标记的 stub)。
2. **可重编译**: 反编译产物能用 `javac` 编回去。
3. **可重打包**: 重编译出的 `.class` 能打回 jar。
4. **可被调用**: 外部 JVM 能加载并 `-Xverify:all` 校验每个类; 关键 API 调用结果与原始 jar 一致。
5. **语义等价**(最重要): 产物语义不得与原始字节码有别。

> **codec(commons-codec 1.15)已完整达成 1–5**(`TestJarRoundTripRepackage/codec`): 106 类反编译 → `javac` 0 错误
> → 重打包 → `java -Xverify:all` 加载校验 **107/107** → Base64/Hex/MD5/SHA-256 调用差分与原 jar **逐字节一致**。

---

## 1. 度量口径: 为什么用 tree 而不是 iso(关键!)

反编译器把嵌套类发成**独立扁平单元** `Outer$Inner.java`。于是:

| 口径 | 怎么编 | 用途 |
|---|---|---|
| **tree(整树)** | 所有扁平单元一起 `javac`(依赖在 classpath) | **重打包真口径**。兄弟 `$` 引用互相解析, 产物可打回 jar。**治本与验收只认它。** |
| **iso(逐文件)** | 每个单元单独 `javac`(原始 jar + 依赖在 classpath) | 侧写。但 `Outer$Inner` 扁平引用对原始 jar 解析不到(jar 内是源名 `Outer.Inner`), 产生海量 `cannot find symbol`/`private access` **假阳性**。 |

实测对照(同一批 jar): codec **iso 38 / tree 0**, spring **iso 384 / tree 2**(fastjson2 tree 248, guava tree 522)。iso 的绝大多数失败是扁平 `$` 假阳性, **不是缺陷、不阻碍重打包**。所以:

> **选靶、验收、A/B delta 一律用 tree。iso 只用于侧写"哪些类涉及跨类引用"。**

---

## 2. 正确性检验阶梯(从快到深, 全部 opt-in 缺工具自动 skip)

### 2.1 CI 常驻承重(无需 `~/.m2`, 只要 JDK)
```bash
go test ./...                       # 全量; 含下列常驻测试
go test -run TestSyntheticJarRoundTrip ./test/cross/   # 合成多类 jar 全链路往返, 运行输出逐字节一致
go test ./classparser/             # 回归种子 + 确定性 + 各承重测试 (≤30s)
```
- `TestSyntheticJarRoundTrip`: `source→javac→jar→反编译→javac→重打包→run`, 断言输出一致 + 全类 verify。**往返能力的回归闸门。**
- `TestDecompileSyntaxRegression`: 遍历 `classparser/testdata/regression/*.class` 种子 + `.golden` 规则。
- `TestDecompileIsDeterministic` / `TestRegressionSeedsAreDeterministic`: 同输入产物恒定。

### 2.2 真实 JAR 重打包 + 校验 + 调用(需 `~/.m2`)
```bash
ROUNDTRIP_JAR=codec go test -run TestJarRoundTripRepackage -v ./test/cross/   # codec 硬断言 0 错 + 全类 verify
ROUNDTRIP_JAR=all   go test -run TestJarRoundTripRepackage -v ./test/cross/   # 其余 jar 报告"差多远"
```

### 2.3 缺陷盘点(选靶数据源)
```bash
# tree 口径(真口径): 把每条 javac error 归属到 文件+reason, 落盘
PROFILE_JAR=all ISO_REPORT_DIR=/tmp/jdec-inv go test -run TestJarTreeInventory -v -timeout 20m ./test/cross/
cat /tmp/jdec-inv/<jar>.tree.reasons.txt   # reason 直方图(最大杠杆一眼可见)
cat /tmp/jdec-inv/<jar>.tree.fails.txt     # 每个失败文件 + 首条 javac error

# iso 口径(侧写, 慢): 同结构, 但记得它有扁平 $ 假阳性
PROFILE_JAR=all ISO_REPORT_DIR=/tmp/jdec-inv go test -run TestJarIsoInventory -v -timeout 30m ./test/cross/
```

### 2.4 A/B kill-switch delta(治本验收 / 防回归)
```bash
# 修复 ON vs OFF 的 decErr 差。delta(OFF-ON) ≥ 0 表示治本有效且不回归; <0 测试直接报错
RECOMPILE_MODE=tree PROFILE_JAR=all KILL_SWITCH=JDEC_POP_ELIDE_OFF \
  go test -run TestJarRecompileDelta -v ./test/cross/
# 单 jar 基线快照
PROFILE_JAR=fastjson2 RECOMPILE_MODE=tree go test -run TestJarRecompileProfile -v ./test/cross/
```

### 2.5 算法 battery(语义层)
```bash
# 控制流 / TEA 加解密 / try-finally 等 round-trip 差分(语义不变量), 见 codec_roundtrip_test.go
go test -run TestRoundTrip ./test/cross/
```

---

## 3. 第三方反编译器作 oracle(CFR / Vineflower)

难 case 不能只看自己的产物。本机已就位 `/tmp/decompilers/{cfr-0.152.jar, vineflower-1.10.1.jar}`(可用 `DECOMPILERS_DIR` 覆盖)。

```bash
# 对同一个 .class, 三方(javajive / CFR / Vineflower)各自反编译 + 各自 javac 重编译, 打印 pass/fail 表
ORACLE_JAR=spring ORACLE_CLASS='EmitUtils$6' go test -run TestThirdPartyOracle -v ./test/cross/
ORACLE_JAR=guava  ORACLE_CLASS=Joiner        go test -run TestThirdPartyOracle -v ./test/cross/
```

**怎么读 oracle 结果**:
- **三方都失败** → 这段字节码内在难结构化(编译器合成的反人类模式, 如某些匿名内部类/状态机)。安全契约满足下可**诚实 stub**, 记录后跳下一个, 不为它冒结构化回归风险。
  - 实例: `EmitUtils$6` 三方皆败, 故其 2 个 spring tree 错误留作长尾(见 TODO T9)。
- **只有我们失败** → 我们有 CFG/栈模拟/类型偏差。拿 CFR/Vineflower 的产物当"正确答案"对照, 找差在哪。
- **我们也能编过** → 该 case 已解。

手动直接跑(脱离 harness):
```bash
java -jar /tmp/decompilers/cfr-0.152.jar X.class --outputdir /tmp/oracle/cfr
java -jar /tmp/decompilers/vineflower-1.10.1.jar X.class /tmp/oracle/vine
javap -p -c -v -classpath <dir> <fqcn>   # 字节码真相(控制流 / 局部变量槽 / 异常表)
```

---

## 4. 长尾清零: 一次一个 class 的标准循环

**严禁批量乱改**。每个长尾根因往往不同, 批量改互相掩盖、引入回归。一轮 = 一个单点缺陷:

1. **定位**(用 §2.3 tree inventory): 选最大 reason 桶里**最小/最干净**的代表类。优先级 `panic > 语法错(malformed) > 类型/泛型 > 其它`。
2. **复现**: `go build -o /tmp/jj ./cmd/javajive && /tmp/jj decompile -o /tmp/x <jar>`, 打开报错文件那几行, 看反编译器到底输出了什么。
3. **拿正确答案**:
   - 上游源码(失败类几乎都来自知名开源库, 按版本 tag 拉 `.java`): `curl https://raw.githubusercontent.com/<owner>/<repo>/<tag>/<path>`。
   - 字节码真相: `javap -p -c -v`。
   - 成熟反编译器 oracle: §3 的 `TestThirdPartyOracle`。
4. **定根因**: 对照"正确答案"判断是哪个结构没重建对(类型擦除造型 / 三元 LUB / 循环-switch 结构化 / 栈 dup-pop / slot 合流 …)。**对着乱码猜根因不可靠, 必须有 oracle。**
5. **治本(带护栏)**:
   - 改核心针对**根因**, 不为过单用例打特例补丁。
   - 复杂/有风险改动**必带 `JDEC_*` kill-switch**(默认开启治本, 置位回退旧行为)。
   - 高风险结构化改动**先合成最小 MVP 样本**(`javac` 一个等价最小源)验证, 再上核心 —— 把爆炸半径锁死。
6. **量化 + 防回归**: 跑 §2.4 A/B delta, **对全部 4 jar 的 delta(OFF-ON) 必 ≥ 0**(本 jar 降、它 jar 不升)。<0 即回退(见红线)。
7. **锁定**: 把修过的真实 `.class` 放进 `classparser/testdata/regression/`, 配 `.golden`(`+必须含 / -必须不含 / +/正则/`), 再写一个 `Test*IsLoadBearing`(断言 kill-switch ON/OFF 的可见差异)。
8. **收尾闸门**(全满足才算完成): 新增承重 + 种子通过; 确定性测试不红; 全量 `go test ./...` ≤30s 全绿; 在 `CODEC_TODO.md` 登记本轮根因/修复点/before-after 真实数字。
9. **复扫**: 回 §2.3 重跑 tree inventory, 确认目标桶下降、其它桶没反弹, 进入下一轮。

> 实战范例(本轮): `EmitUtils` 的 `var0;` → tree inventory 定位(spring 全部 14 错都在 cglib EmitUtils)→ `javap` 看到死的 `aload_0; aload_0; pop` → 根因是 pop 把无副作用裸值渲染成非语句 → `keepDiscardedStackValue` 按 JLS 14.8 elide(kill-switch `JDEC_POP_ELIDE_OFF`)→ A/B delta spring +12 / 它 jar +0 → 承重 `TestPopElideIsLoadBearing` + 种子 `SpringCglibEmitUtils.class` → spring tree **14→2**。

---

## 5. 红线与安全契约

- **以认真查阅为荣**: 动核心代码前必须先 inventory 定位到具体类+方法并复现。
- **以复用现有为荣**: 复用上述 harness 与回归机制, 不另造平行测试入口。
- **以主动测试为荣**: 每轮必须新增承重 + 种子并跑过 30s 快回归。
- **以遵循规范为荣 / 回归即回退**: A/B delta 对任一 jar <0 即回退该改动(本轮 `RefMember` elide 致 spring 812 大回归, 已回退)。
- **安全契约**: 永不输出不可解析的 Java; 宁可带标记 stub 也不输出"看似对实则错"; `Decompile` 不 panic、不让 error 逃逸。
- **以诚实数据为荣**: 文档里的 before/after 必须由命令真跑出。

---

## 6. 命令速查

| 目的 | 命令 |
|---|---|
| CI 全量(含往返/回归/确定性) | `go test ./...` |
| 合成往返(承重, 无需 m2) | `go test -run TestSyntheticJarRoundTrip ./test/cross/` |
| 真实 jar 重打包+verify+调用 | `ROUNDTRIP_JAR=codec go test -run TestJarRoundTripRepackage -v ./test/cross/` |
| **tree 缺陷盘点(选靶)** | `PROFILE_JAR=all ISO_REPORT_DIR=/tmp/jdec-inv go test -run TestJarTreeInventory -v -timeout 20m ./test/cross/` |
| iso 缺陷盘点(侧写) | `PROFILE_JAR=all ISO_REPORT_DIR=/tmp/jdec-inv go test -run TestJarIsoInventory -v -timeout 30m ./test/cross/` |
| A/B kill-switch delta | `RECOMPILE_MODE=tree PROFILE_JAR=all KILL_SWITCH=<JDEC_X> go test -run TestJarRecompileDelta -v ./test/cross/` |
| 单 jar 基线 | `PROFILE_JAR=<jar> RECOMPILE_MODE=tree go test -run TestJarRecompileProfile -v ./test/cross/` |
| CFR/Vineflower oracle | `ORACLE_JAR=<jar> ORACLE_CLASS=<substr> go test -run TestThirdPartyOracle -v ./test/cross/` |
| 单类反编译复现 | `go build -o /tmp/jj ./cmd/javajive && /tmp/jj decompile -o /tmp/x <jar-or-class>` |
| 列全 kill-switch | `rg 'os.Getenv\("JDEC_' classparser/decompiler` |
| 回归种子目录 | `classparser/testdata/regression/*.class (+ .golden)` |

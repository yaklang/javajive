# JavaJive 核心字节码反编译对抗性评估

日期：2026-11-05（rebase 到最新 main 后复跑，结果不变）  
范围：仅 JVM `.class` 反编译（不含 DEX/APK/WAR）  
基线：`origin/main` @ `469d873`（PR #18 squash；内容与评估时的 `4e2a395` 一致）  
环境：macOS arm64，OpenJDK 21.0.2，Go 1.22.12

## 复现

```bash
JAVAJIVE_ADVERSARIAL_EVAL=1 go test ./test/adversarial \
  -run 'TestAdversarial(Eval|SourceShapes|Scale|Malformed)' -v -count=1 -timeout=15m
```

所有 class 由仓库内的 `classfilebuilder` 生成，或由仓库自写 Java 源经 `javac --release 8` 编译。
原始 class 先通过 `java -Xverify:all`，再作为行为 oracle；重建源码单独编译并再次
`-Xverify:all`，最后逐输入比较 `run(int,int)` 的反射调用矩阵。

## 结论摘要

| 维度 | 结果 |
|---|---|
| 真实 javac 形状 | **10 / 10** 恢复、重编译、校验、行为等价 |
| 规模压力 | **5 / 5** 通过（2000 基本块、200 路 switch、300 连续 if、50 catch、250 locals） |
| 文档化 fail-closed 边界 | **5 / 5** 精确降级为 stub，无静默错误 |
| 对抗性非规约形状 | **2 / 6** 完全恢复；**1** 不可编译；**2** 静默行为错误；**1** fail-closed stub |
| 畸形输入 | 564 个变异：0 进程崩溃、0 超时；382 拒绝、182 接受；**41 次内部 panic 被 recover 兜住（26 种越界签名）** |

核心判断：**常规编译器产物与规模维度表现很强；非规约恢复在“单出口 + 循环外多路前驱”
和“跨状态局部变量”两个形状上存在真实缺陷，其中前者是静默错误，优先级最高。**

## 一、非规约边界矩阵

| case | 预期 | 结果 | dispatch 转移 | 行数 | 耗时 |
|---|---|---|---|---:|---:|
| SwitchEightEntry | 恢复 | **behavior-mismatch** | 24 | 143 | 203ms |
| IfChainEightEntry | 恢复 | **behavior-mismatch** | 24 | 148 | 16ms |
| TwoSequentialKernels | 恢复 | recovered | 12 | 90 | 12ms |
| DeepRegional | 恢复 | **compile-failed** | 15 | 77 | 8ms |
| FourExitEffects | 恢复 | incomplete（stub） | 0 | 3 | 14ms |
| FallthroughEntry | 恢复 | recovered | 6 | 48 | 6ms |
| ConstructorIrreducible | fail-closed | fail-closed-stub | 0 | 5 | 2ms |
| TryCatchIrreducible | fail-closed | fail-closed-stub | 0 | 3 | 3ms |
| MonitorIrreducible | fail-closed | fail-closed-stub | 0 | 3 | 1ms |
| NewIrreducible | fail-closed | fail-closed-stub | 0 | 3 | 1ms |
| StackCarryEntry | fail-closed | fail-closed-stub | 0 | 3 | 4ms |

### F1（严重，静默错误）：单出口区域的循环外前驱无法跳过 dispatcher

`SwitchEightEntry` 与 `IfChainEightEntry` 都用 8 个外部入口进入同一 8 节点环，
默认路径直接到共享出口。原始语义：entry ∉ [0,7] 时返回 0。重建源码：

```java
switch (var0) {
  case 0: var2 = 1; break;
  ...
  case 7: var2 = 0; break;
  default: var3 = 0;          // 出口链的第一条语句
}
LOOP_1:
do { switch (var2) { case 0: var5 += 8; ... } } while (true);
```

`default` 臂设置 `var3 = 0` 后**落入调度循环**，而 `var2` 初始为 0，于是执行了 A7 的
`+8` 语义。实测 `entry=-2,budget=1`：原始为 0，重建为 8；`entry=-1,budget=10`：
原始 0，重建 45。重建源码可编译、可 verify，因此是静默 miscompile。

根因假设：单出口时 `buildExitDispatch` 直接返回出口节点（不建独立 dispatch 节点）。
出口节点同时被循环退出边和循环外前驱引用；后者需要在结构化 Java 中“跳出整个循环”，
但 lowering 只做了 `exitState` 赋值 + 目标重定向。单出口下 `exitState` 是死值，
重定向被优化掉后，出口链只能以 fallthrough 形式落在 default 臂，随后继续流入循环。
两种入口形态（tableswitch / if-chain）都复现，说明不是 switch 渲染特例。

建议：对“出口存在非成员前驱、且该前驱不是区域外层的 guard”的单出口区域，
要么构造独立的循环后 handoff 节点并处理 terminal chain 的 ownership，要么 fail-closed。
当前行为必须优先修复，因为违反了“宁可不恢复，不可静默错误”的合同。

### F2（严重，不可编译）：跨状态局部变量未提升并初始化

`DeepRegional` 在两个嵌套自然循环内放置 2 节点非规约核。重建源码把局部变量声明
放在 `do { ... }` 内且无初始化：

```java
LOOP_1:
do {
    int var6;              // case 1 赋值，case 2/6/8 读取
    switch (var2) {
    case 1: var6 = 2; ...
    case 2: if (var6 > 0) ...
```

`javac` 报 4 处 “可能尚未初始化变量 var6”。JVM 校验器保证原始 bytecode 在读取前
必有赋值，因此把声明初始化为类型默认值（`int var6 = 0;`）语义安全；当前输出直接失败。
根因是 dispatcher 只把合成的 `jdecState`/`jdecExit` 提升到方法根，未处理
“首次赋值位于某个 case、读取位于另一个 case”的原始局部。

建议：lowering 前收集区域内赋值、区域外/跨 case 读取的局部，统一在方法根声明并以
类型默认值初始化；或以声明支配（declaration dominance）规则处理。

### F3（中等，能力边界）：多出口共享后置 dispatch 未支持

`FourExitEffects` 的四个出口各自设置 exit-state 后汇入同一后置 dispatch，
被拒绝为：

```text
unsupported_irreducible_control_flow: nonlinear terminal exit id=17
statement=*statements.AssignStatement next=1
```

最终 `status=partial`，方法整体 stub。该行为是 fail-closed（无静默错误），但与
“multiple exits 已支持”的表述存在形状差异：当前支持的是出口链各自线性终止，
不支持多出口先汇合再分派。建议在文档中收窄描述，或扩展 `linearTerminalExit`
以接受共享后置 merge。

## 二、真实 javac 形状（全部通过）

| case | 结果 | 类数 | 行数 | dispatch | 耗时 |
|---|---|---:|---:|---:|---:|
| StringSwitchShape | recovered | 1 | 82 | 0 | 41ms |
| EnumSwitchShape | recovered | 2 | 43 | 0 | 17ms |
| TryWithResourcesShape | recovered | 2 | 50 | 0 | 18ms |
| NestedLabeledShape | recovered | 1 | 40 | 3 | 11ms |
| SynchronizedShape | recovered | 1 | 22 | 0 | 6ms |
| LambdaCaptureShape | recovered | 2 | 25 | 0 | 9ms |
| FinallyDupShape | recovered | 1 | 36 | 0 | 9ms |
| FlattenedStateShape | recovered | 1 | 37 | 1 | 18ms |
| OpaquePredicateShape | recovered | 1 | 32 | 0 | 6ms |
| InterfaceDefaultShape | recovered | 2 | 28 | 0 | 8ms |

包括字符串/枚举 switch、try-with-resources 的抑制路径、labeled break/continue、
monitor、lambda 捕获、finally 复制、扁平化状态机、opaque predicate、接口 default/static
方法。全部通过整树重编译、`-Xverify:all` 与 99 组输入的行为比较。

## 三、规模与性能（全部通过）

| case | 结果 | 行数 | 字节 | 反编译耗时 |
|---|---|---:|---:|---:|
| DeepGotoChain2000 | recovered | 2005 | 20108 | 354ms |
| DenseSwitch200 | recovered | 408 | 5307 | 30ms |
| ManySequentialIfs300 | recovered | 919 | 16737 | 957ms |
| DeepCatch50 | recovered | 272 | 7125 | 172ms |
| WideLocals250 | recovered | 506 | 15548 | 831ms |

2000 基本块线性链耗时约 0.35s，300 个连续条件约 0.96s，未见明显超线性爆炸；
无栈溢出。输出行数与输入块数基本线性。

## 四、畸形输入鲁棒性

对 `IrreducibleTwoEntry.class` 生成 564 个确定性变异：

- 截断（0..len，步长 64）：全部拒绝
- 单字节翻转 300 个：145 个仍被接受
- 4 字节覆写 100 个：31 个仍被接受
- 删除 1–4 字节 50 个：全部拒绝
- 随机垃圾 + CAFEBABE 100 个：全部拒绝
- 魔数合法但 major version 改为 0/1/44/45/99/255 的 6 个：**全部被接受**

0 次进程崩溃、0 次超过 2s。日志中出现 **41 次** 被内部 recover 兜住的
`runtime error: index out of range`（26 种不同越界签名，例如
`[32792] with length 27`），最终以 “panic: ...” 转成方法 stub。说明部分畸形路径缺少边界检查，但不会击穿进程。

建议：把这类路径改为结构化解析错误；对 classfile major version 增加明确支持范围
（至少拒绝 0/1/255），避免把未知版本当作当前语义反编译。

## 五、边界与后续

- 本轮不覆盖：DEX/APK、WAR、多版本 JAR、真实第三方 jar 的规模回归；这些由其他工作流负责。
- `test/adversarial` 为 opt-in 包，默认 `t.Skip`，不进入现有 CI 快照。
- 建议修复优先级：**F1 静默错误 > F2 不可编译 > F3 能力边界 > 畸形输入的 panic 噪音**。
- F1/F2 都应先补 opt-in 回归，再进入 required gate；F1 在修复前可作为已知缺陷记录，
  但不应发布“多入口完全支持”的结论。

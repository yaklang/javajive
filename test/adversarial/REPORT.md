# JavaJive 核心字节码反编译对抗性评估

日期：2026-11-05
范围：仅 JVM `.class` 反编译（不含 DEX/APK/WAR）  
基线：`origin/main` @ `469d873`（PR #18 squash）
修复分支：`fix/irreducible-shared-exit-locals`
环境：macOS arm64，OpenJDK 21.0.2，Go 1.22.12

## 复现

```bash
# 完整对抗评估（opt-in，默认 skip）
JAVAJIVE_ADVERSARIAL_EVAL=1 go test ./test/adversarial \
  -run 'TestAdversarial(Eval|SourceShapes|Scale|Malformed)' -v -count=1 -timeout=15m
```

所有 class 由仓库内的 `classfilebuilder` 生成，或由仓库自写 Java 源经 `javac --release 8` 编译。
原始 class 先通过 `java -Xverify:all`，再作为行为 oracle；重建源码单独编译并再次
`-Xverify:all`，最后逐输入比较 `run(int,int)` 的反射调用矩阵。

## 结论摘要（修复后）

| 维度 | 结果 |
|---|---|
| 真实 javac 形状 | **10 / 10** 恢复、重编译、校验、行为等价 |
| 规模压力 | **5 / 5** 通过（2000 基本块、200 路 switch、300 连续 if、50 catch、250 locals） |
| 对抗性非规约形状 | **6 / 6** 完全恢复 |
| 文档化 fail-closed 边界 | **5 / 5** 精确降级为 stub，无静默错误 |
| 畸形输入 | 564 个变异：0 崩溃、0 超时；41 次内部 panic 被 recover 兜住 |
| 既有 irreducible corpus | 8 / 8 源码 hash、行为、编译、校验全部不变 |

本轮修复了在合并后的 #18 代码上确认的三个缺陷：

- **F1（已修复）**：单出口非规约区域中，循环外多路前驱无法跳过 dispatcher 循环，
  导致越界 entry 执行错误状态（静默 miscompile）。
- **F2（已修复）**：跨 case 局部变量被提升为 `do { int var6; ... }` 裸声明，
  javac 因 definite-assignment 拒绝编译。
- **F3（已修复）**：多个出口汇入同一后置 continuation 时被 `linearTerminalExit`
  拒绝，方法退化为 stub。

F4（畸形输入内部 panic 与 classfile major version 校验）由独立 PR 处理。

## 一、非规约边界矩阵（修复后）

| case | 预期 | 结果 | dispatch 转移 | 行数 | 耗时 |
|---|---|---|---|---:|---:|
| SwitchEightEntry | 恢复 | recovered | 25 | 147 | 206ms |
| IfChainEightEntry | 恢复 | recovered | 25 | 151 | 16ms |
| TwoSequentialKernels | 恢复 | recovered | 13 | 94 | 12ms |
| DeepRegional | 恢复 | recovered | 15 | 77 | 9ms |
| FourExitEffects | 恢复 | recovered | 13 | 102 | 21ms |
| FallthroughEntry | 恢复 | recovered | 6 | 48 | 6ms |
| ConstructorIrreducible | fail-closed | fail-closed-stub | 0 | 5 | 3ms |
| TryCatchIrreducible | fail-closed | fail-closed-stub | 0 | 3 | 3ms |
| MonitorIrreducible | fail-closed | fail-closed-stub | 0 | 3 | 1ms |
| NewIrreducible | fail-closed | fail-closed-stub | 0 | 3 | 1ms |
| StackCarryEntry | fail-closed | fail-closed-stub | 0 | 3 | 5ms |

### F1 修复：选择性 skip 守卫

原缺陷：单出口时 `buildExitDispatch` 直接返回出口节点，循环外前驱被改写为
`jdecExit=0 -> exit`。`jdecExit` 是死值，且该前驱在结构化 Java 中无法表达
“跳过循环”；switch default / if-chain 末端的出口链第一条语句落入 default 臂后
继续进入 LOOP。修复前实测 `entry=-2,budget=1` 原始 0 / 重建 8，
`entry=-1,budget=10` 原始 0 / 重建 45。

修复策略：

1. 在接入 loop 之前快照 exit 的非成员前驱，避免把新 loop 当成外部前驱。
2. 仅当存在无法由普通 loop guard 消费的前驱时启用守卫：
   - 同一 exit 有 ≥2 个非成员前驱；或
   - 唯一前驱不是支配全部 region entry 的 `ConditionStatement`；或
   - 存在原始 switch 出口。
3. 合成 `boolean jdecSkipN = false;`，在 dispatcher 循环体顶部插入
   `if (jdecSkipN) { break LOOP_N; }`。
4. 循环外出口前驱改写为 `jdecSkipN = true`（多出口时再写 `jdecExit`），然后进入循环；
   循环体首条 guard 立即 break 到循环后的 exit dispatch。
5. 对原始入口 switch，在 switch 与 loop 之间插入一个无操作 gate 节点，使 switch
   重新获得非环 merge，从而恢复逐 case 的 `break`；gate 只由 switch 自身路径进入。

修复后两个 8 入口用例均恢复，行为矩阵与原 class 完全一致；既有 corpus 的 8 个
单出口/多出口案例不触发守卫，源码 hash 不变。

### F2 修复：跨 case 局部变量提升到方法根

原缺陷：`placeCrossScopeDeclarations` 把跨 case 局部变量提升为 do-body 顶部的裸声明
`int var6;`；javac 拒绝“可能尚未初始化”，因为 sibling case 可能先于声明 case 执行。

修复策略（新增 `dispatch_local_hoist.go`，在 `RewriteVar` 末尾运行）：

1. 通过 `DoWhileStatement.IrreducibleDispatch` 标记识别 dispatcher 循环
   （该标记在 lowering 时设置，变量重命名后仍有效）。
2. 递归收集方法内被 dispatcher case 引用、但未在方法根初始化声明的局部变量。
3. 对每个候选调用 `relocateDeclarations`，删除所有裸声明并降级 case 内的
   `IsFirst/IsDeclare/Final` store。
4. 以同一个 `*values.JavaRef` 在方法根插入
   `IsFirst=true`、类型默认值初始化的声明（int/boolean/long/float/double/char 精确匹配；
   String/引用/数组/泛型使用 Object 类型 `null` 字面量，避免渲染成 `"null"`）。
5. kill-switch：`JDEC_DISPATCH_LOCAL_HOIST_OFF=1`。

修复后 `DeepRegional` 与 `TwoSequentialKernels` 均可编译、校验并通过行为矩阵；
新增单测覆盖提升、幂等和普通循环不受影响。

### F3 修复：共享出口 continuation 坍缩

`FourExitEffects` 的四个出口并不是四个独立终止链：它们各自写入同一个原始
exit-state 局部变量，然后汇入同一段 if-chain。旧实现把每个出口交给
`linearTerminalExit`，在共享节点处因 `len(Source) != 1` 拒绝，方法退化为 stub。

修复策略：

1. 在 lowering 前识别“所有出口经线性前缀到达同一共享 continuation”的形状；
   只要存在独立终止出口，就不坍缩，保留原有 multi-exit 处理。
2. 坍缩为单个 dispatch exit，并把每个出口的前缀语句复制到对应的成员退出
   case body；case 以 `break LOOP_n` 结束，共享 continuation 在循环之后只执行一次。
3. 共享 continuation 的循环外前驱（预算检查、入口 switch default）由 F1 的
   skip 守卫处理，避免 fall through 进 dispatcher。
4. 前缀节点只断开前向边，保留 Source 边供成员退出分析与后续外部前驱改写使用。

修复后 `FourExitEffects` 可编译、JVM 校验并通过 99 组输入的行为矩阵，输出
102 行；既有 corpus 8/8 源码 hash 与行为不变。

## 二、真实 javac 形状（全部通过）

| case | 结果 | 类数 | 行数 | dispatch | 耗时 |
|---|---|---:|---:|---:|---:|
| StringSwitchShape | recovered | 1 | 82 | 0 | 37ms |
| EnumSwitchShape | recovered | 2 | 43 | 0 | 17ms |
| TryWithResourcesShape | recovered | 2 | 50 | 0 | 18ms |
| NestedLabeledShape | recovered | 1 | 40 | 3 | 10ms |
| SynchronizedShape | recovered | 1 | 22 | 0 | 6ms |
| LambdaCaptureShape | recovered | 2 | 25 | 0 | 9ms |
| FinallyDupShape | recovered | 1 | 36 | 0 | 9ms |
| FlattenedStateShape | recovered | 1 | 37 | 1 | 16ms |
| OpaquePredicateShape | recovered | 1 | 32 | 0 | 6ms |
| InterfaceDefaultShape | recovered | 2 | 28 | 0 | 7ms |

包括字符串/枚举 switch、try-with-resources 的抑制路径、labeled break/continue、
monitor、lambda 捕获、finally 复制、扁平化状态机、opaque predicate、接口 default/static
方法。全部通过整树重编译、`-Xverify:all` 与 99 组输入的行为比较。

## 三、规模与性能（全部通过）

| case | 结果 | 行数 | 字节 | 反编译耗时 |
|---|---|---:|---:|---:|
| DeepGotoChain2000 | recovered | 2005 | 20108 | 347ms |
| DenseSwitch200 | recovered | 408 | 5307 | 33ms |
| ManySequentialIfs300 | recovered | 919 | 16737 | 951ms |
| DeepCatch50 | recovered | 272 | 7125 | 171ms |
| WideLocals250 | recovered | 506 | 15548 | 835ms |

2000 基本块线性链耗时约 0.35s，300 个连续条件约 0.95s，未见明显超线性爆炸；
无栈溢出。输出行数与输入块数基本线性。

## 四、畸形输入鲁棒性（未在本 PR 修复）

对 `IrreducibleTwoEntry.class` 生成 564 个确定性变异：

- 截断（0..len，步长 64）：全部拒绝
- 单字节翻转 300 个：145 个仍被接受
- 4 字节覆写 100 个：31 个仍被接受
- 删除 1–4 字节 50 个：全部拒绝
- 随机垃圾 + CAFEBABE 100 个：全部拒绝
- 魔数合法但 major version 改为 0/1/44/45/99/255 的 6 个：全部被接受

0 次进程崩溃、0 次超过 2s。日志中出现 **41 次** 被内部 recover 兜住的
`runtime error: index out of range`（26 种不同越界签名，例如
`[32792] with length 27`），最终以 “panic: ...” 转成方法 stub。
说明部分畸形路径缺少边界检查，但不会击穿进程；classfile major version 也缺少
支持范围校验。建议作为后续独立修复。

## 五、修复验证

```bash
go build ./...
go test ./classparser/decompiler/rewriter/... ./classparser/decompiler/core/... -count=1
go test ./test/cross -run '^TestRoundTripIrreducibleCFGCorpus$' -count=1
JAVAJIVE_ADVERSARIAL_EVAL=1 go test ./test/adversarial -count=1
go test -race ./classparser/decompiler/rewriter/... ./classparser/decompiler/core/... -count=1
go vet ./...
```

既有 corpus 8/8 的源码 hash、行为、JVM 校验均保持不变，确认本轮为纯增量修复。

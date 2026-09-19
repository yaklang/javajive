# 历史 JAR 全量差分复测（2026-09-19）

**最终差分通过：38 个 JAR 均未检出新增编译诊断、stub、缺失源码或验证退化。**
基线可完整编译的 20 个包全部保留，当前 26 个包完整编译成功。
这不是“38 个包全部零错误”或“所有方法行为等价”的声明。

## 范围与验收

- 基线：`e1710d5e63736a747dd9a02c164507348c59ad5e`。
- 全部 **38 个历史 JAR**，无抽样或 MAXFILES 限制；目标及依赖共 **174 个固定版本文件**，由 [SHA-256 锁文件](../test/cross/testdata/historical-jars.lock.json) 校验。
- **24,883 个输入 class 条目、24,359 个输出源码单元**。不含 module-info，包含 MR 版本及 Mockito 的 class 格式 `.raw` 文件；折入 enum 的匿名类不单独输出。
- 同机 Corretto **17.0.12**、相同依赖、相同观测 harness，分别进行整包反编译、整树 javac、重打包、独立 `-Xverify:all` 及反射成员签名解析。
- 重建 classpath 排除目标原始 JAR。编译失败时的部分验证结果不作为完整验证成功。
- CI 现在独立执行全量历史对比并上传观测产物；缺少 JAR、哈希不匹配、测量不完整或差分退化都会失败。

| 指标 | 基线 | PR 初版 | 最终修复 |
| --- | ---: | ---: | ---: |
| 完成整包观测 | 38 | 38 | 38 |
| 整树编译成功 | 20 | 8 | 26 |
| 丢失基线可编译包 | — | 12 | 0 |
| 检出新增差分失败的包 | — | 22 | 0 |
| 方法 stub | 4 | 见初版记录 | 4 |

机器可读结果：[最终差分](validation/historical-final.json)。早期失败证据保留在
[PR 初版](validation/historical-pr-head.json)和[中间修复](validation/historical-follow-up.json)，不作为发布验收结果。
普通测试曾跳过 opt-in 整包语料，现已通过专用 CI 门禁消除该遗漏。
最终提交的精确 SHA 和完整源码、编译日志、验证记录由 CI artifact 绑定；此处汇总来自发布前本地完整复测。

## 逐包编译诊断

每个编译单元、诊断类别及次数分别比较；总错误减少不能抵消新增错误。

| JAR | 基线错误 | 最终错误 | stub 基线 → 最终 | 差分 |
| --- | ---: | ---: | --- | --- |
| asm | 0 | 0 | 0 → 0 | 通过 |
| assertj | 7 | 7 | 0 → 0 | 通过 |
| bytebuddy | 1 | 0 | 3 → 3 | 通过 |
| caffeine | 2 | 2 | 0 → 0 | 通过 |
| codec | 0 | 0 | 0 → 0 | 通过 |
| collections4 | 0 | 0 | 0 → 0 | 通过 |
| commons-io | 1 | 1 | 0 → 0 | 通过 |
| commons-lang3 | 6 | 0 | 0 → 0 | 通过 |
| compress | 0 | 0 | 1 → 1 | 通过 |
| fastjson2 | 0 | 0 | 0 → 0 | 通过 |
| freemarker | 17 | 5 | 0 → 0 | 通过 |
| gson | 0 | 0 | 0 → 0 | 通过 |
| guava | 7 | 7 | 0 → 0 | 通过 |
| hikaricp | 0 | 0 | 0 → 0 | 通过 |
| httpclient | 10 | 0 | 0 → 0 | 通过 |
| jackson | 0 | 0 | 0 → 0 | 通过 |
| javassist | 8 | 3 | 0 → 0 | 通过 |
| jedis | 0 | 0 | 0 → 0 | 通过 |
| joda-time | 0 | 0 | 0 → 0 | 通过 |
| jsoup | 6 | 0 | 0 → 0 | 通过 |
| junit | 0 | 0 | 0 → 0 | 通过 |
| log4j | 15 | 11 | 0 → 0 | 通过 |
| logback | 0 | 0 | 0 → 0 | 通过 |
| lucene | 0 | 0 | 0 → 0 | 通过 |
| math3 | 0 | 0 | 0 → 0 | 通过 |
| mockito | 0 | 0 | 0 → 0 | 通过 |
| netty | 10 | 0 | 0 → 0 | 通过 |
| okhttp | 4 | 3 | 0 → 0 | 通过 |
| picocli | 2 | 1 | 0 → 0 | 通过 |
| pool2 | 1 | 1 | 0 → 0 | 通过 |
| protobuf | 0 | 0 | 0 → 0 | 通过 |
| rxjava | 4 | 4 | 0 → 0 | 通过 |
| slf4j | 0 | 0 | 0 → 0 | 通过 |
| snakeyaml | 0 | 0 | 0 → 0 | 通过 |
| spring | 3 | 3 | 0 → 0 | 通过 |
| spring-beans | 0 | 0 | 0 → 0 | 通过 |
| xstream | 0 | 0 | 0 → 0 | 通过 |
| zxing | 2 | 0 | 0 → 0 | 通过 |

## 修复与运行时验证

本轮修复包括嵌套 JSR 的事务式展开与 PC 校验、基于不可变 def-use 的引用汇合、
泛型重载与可访问类型选择、调用描述符类型隔离、条件赋值的身份和优先级、
构造器声明顺序、switch/loop 出口、重试异常区间、资源 catch 身份及 Kotlin monitor 边界。
删除了与正确结构冲突的旧 break/声明源码修复；仍保留的兼容规则继续记录来源。

**250 项隔离语义往返通过本地 JDK 17 验证**，覆盖两种模式、debug/no-debug、
合法 wide/JSR 字节码、资源关闭顺序与 suppressed 异常、引用/数组重赋值、嵌套 switch、
反射异常和构造器求值。独立记录编译、JVM 验证、stub、输出相等；unsupported 用例单独验收降级诊断。
CI 在 JDK 17 和 21 重跑这组测试，并保留 OS/Go 矩阵及完整分片 race 语料。
详见[语义审计与实现边界](decompiler-semantic-audit.md)。

## 复现

使用同一 JDK 运行两份工作树。建议审计目录放在仓库之外，保留足够磁盘空间。
以下命令从当前仓库根目录执行；旧 macOS Go 工具链使用外部链接器。

```sh
python3 scripts/historical_jar_audit.py prepare \
  --cache /tmp/javajive-audit/m2 \
  --manifest /tmp/javajive-audit/manifest.json

git worktree add --detach /tmp/javajive-audit-baseline e1710d5e63736a747dd9a02c164507348c59ad5e
cp test/cross/historical_audit_test.go /tmp/javajive-audit-baseline/test/cross/

(cd /tmp/javajive-audit-baseline && CGO_ENABLED=1 go test -ldflags=-linkmode=external -c ./test/cross -o /tmp/javajive-audit/baseline.test)
CGO_ENABLED=1 go test -ldflags=-linkmode=external -c ./test/cross -o /tmp/javajive-audit/candidate.test

(cd /tmp/javajive-audit-baseline/test/cross && \
 HISTORICAL_JAR_MANIFEST=/tmp/javajive-audit/manifest.json \
 HISTORICAL_JAR_REPORT=/tmp/javajive-audit/baseline \
 HISTORICAL_JAR_REVISION=e1710d5 \
 /tmp/javajive-audit/baseline.test -test.run '^TestHistoricalJarAudit$' -test.timeout=2h -test.v)

(cd test/cross && \
 HISTORICAL_JAR_MANIFEST=/tmp/javajive-audit/manifest.json \
 HISTORICAL_JAR_REPORT=/tmp/javajive-audit/candidate \
 HISTORICAL_JAR_REVISION=working-tree \
 /tmp/javajive-audit/candidate.test -test.run '^TestHistoricalJarAudit$' -test.timeout=2h -test.v)

python3 scripts/historical_jar_audit.py compare \
  --baseline /tmp/javajive-audit/baseline \
  --candidate /tmp/javajive-audit/candidate \
  --output /tmp/javajive-audit/comparison.json

CGO_ENABLED=1 go test -ldflags=-linkmode=external ./test/cross -run '^TestAudit' -count=1 -timeout=30m
CGO_ENABLED=1 go test -ldflags=-linkmode=external ./classparser/... -count=1 -timeout=30m
```

观测测试的 PASS 只表示 38 个包的测量完成。必须接着运行 `compare`：
退出码 1 表示检出回归，2 表示缺少证据或不可比较；初版比较返回 1，最终比较返回 0。
比较同时检查诊断所属源码单元及出现次数、stub、缺失源码、原本可编译的包退化，
以及双方完整编译成功时的 JVM 验证退化。相同基线对比自身返回 0，作为工具自检。
重新测量应使用新的报告目录，测试拒绝覆盖旧观测。

## 证据边界

- 全量 JAR 对比走生产 JarFS 默认兼容路径；没有执行这 38 个包全部 API 的行为差分。
- 基线已有编译失败和 4 个 stub，本次门禁要求没有新增失败，并保留这些残余记录。
- 依赖沿用历史测试的环境 shim，只提供编译/链接支持，不代表可选平台功能可用。
- MR 版本参与对应 release 的源码编译；验证器枚举普通类条目，不证明所有 MR 分支。
- JVM 验证日志最多展示 40 个失败类，失败总数另记；不把截断日志当作成功。
- 重新编译和 JVM 验证都不证明行为等价；运行时结论仅适用于上述已执行用例。
- 完整 operand-stack SSA、通用类型约束与不可约图重构仍是后续架构工作。

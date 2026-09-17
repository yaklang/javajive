# 历史 JAR 全量差分复测（2026-09-17）

**结论：未通过，存在回归。PR #9 保持草稿，未合并、未打标签、未发布新版本。**

此前普通 CI 的绿色结果没有覆盖历史整包测试：`TestJarRecompileProfile` 和
`TestJarRoundTripRepackage` 需要显式设置环境变量，缺少 JAR 时也会跳过。
不能把此前的 167 项合成语义测试通过解释为历史 JAR 无回归。

## 范围与结果

- 基线：`e1710d5e63736a747dd9a02c164507348c59ad5e`。
- PR 初版：`d656ead8a02c96fdbf386f63c6c67bcbfce44b83`。
- 历史列表全部 **38 个 JAR**：原 34 个加 Byte Buddy、Mockito、Spring Beans、Lucene。
- **24,883 个输入 class 条目、24,359 个输出源码单元**；无抽样、无 MAXFILES 限制。
  条目统计不含 module-info，包含 MR 版本和 Mockito 的 class 格式 `.raw` 文件；
  折入 enum 的匿名类不再输出为独立源码单元。
- 目标及依赖共 **174 个固定版本文件**，见 [SHA-256 锁文件](../test/cross/testdata/historical-jars.lock.json)。
- 同一台机器、Corretto **17.0.12**、相同依赖和编译参数，分别执行整包反编译、
  整树 javac 编译、重打包、独立 JVM `-Xverify:all` 和反射解析成员签名。
  重建编译/验证 classpath 不包含目标原始 JAR。

| 指标 | 基线 | PR 初版 | 本轮后续修复 |
| --- | ---: | ---: | ---: |
| 完成整包观测 | 38 | 38 | 38 |
| 整树编译成功 | 20 | 8 | 11 |
| 基线可编译、当前不可编译 | — | 12 | 11 |
| 出现新增编译诊断或 stub 的包 | — | 22 | 18 |

后续修复仍未达到验收标准；Math3、Mockito 等包的编译诊断甚至比 PR 初版更多。
基线自身也有编译失败，不能把它称为“全部通过”。错误总数降低不会抵消同一包中的新增错误。

完整机器可读差分：[PR 初版](validation/historical-pr-head.json)、
[后续修复](validation/historical-follow-up.json)。后续列来自开发中的 `working-fix3`
测试二进制，不是已发布版本，也不是可发布的最终验收结果。
**后续代码修复和新增运行时用例仍保留在本地工作树，未推送到 PR。**
本次提交仅补充审计工具、固定输入清单及复测证据；PR 中的生产代码仍对应初版列。

## 逐包编译错误

“未检出新增项”仅表示本次记录的差分指标没有新增失败，不表示运行行为等价。

| JAR | 基线错误 | PR 初版错误 | 后续错误 | stub 基线 → 后续 | 差分结果 |
| --- | ---: | ---: | ---: | --- | --- |
| asm | 0 | 0 | 0 | 0 → 0 | 未检出新增项 |
| assertj | 7 | 7 | 7 | 0 → 0 | 未检出新增项 |
| bytebuddy | 1 | 29 | 5 | 3 → 5 | 未通过 |
| caffeine | 2 | 5 | 2 | 0 → 0 | 未检出新增项 |
| codec | 0 | 0 | 0 | 0 → 0 | 未检出新增项 |
| collections4 | 0 | 1 | 0 | 0 → 0 | 未检出新增项 |
| commons-io | 1 | 1 | 1 | 0 → 0 | 未检出新增项 |
| commons-lang3 | 6 | 5 | 1 | 0 → 0 | 未通过 |
| compress | 0 | 6 | 1 | 1 → 1 | 未通过 |
| fastjson2 | 0 | 16 | 4 | 0 → 0 | 未通过 |
| freemarker | 17 | 21 | 13 | 0 → 0 | 未通过 |
| gson | 0 | 1 | 1 | 0 → 0 | 未通过 |
| guava | 7 | 13 | 7 | 0 → 0 | 未检出新增项 |
| hikaricp | 0 | 0 | 0 | 0 → 0 | 未检出新增项 |
| httpclient | 10 | 10 | 0 | 0 → 0 | 未检出新增项 |
| jackson | 0 | 9 | 1 | 0 → 0 | 未通过 |
| javassist | 8 | 10 | 3 | 0 → 0 | 未检出新增项 |
| jedis | 0 | 0 | 0 | 0 → 0 | 未检出新增项 |
| joda-time | 0 | 2 | 1 | 0 → 0 | 未通过 |
| jsoup | 6 | 6 | 0 | 0 → 0 | 未检出新增项 |
| junit | 0 | 0 | 0 | 0 → 0 | 未检出新增项 |
| log4j | 15 | 17 | 13 | 0 → 0 | 未通过 |
| logback | 0 | 0 | 0 | 0 → 0 | 未检出新增项 |
| lucene | 0 | 25 | 22 | 0 → 0 | 未通过 |
| math3 | 0 | 30 | 48 | 0 → 0 | 未通过 |
| mockito | 0 | 6 | 9 | 0 → 0 | 未通过 |
| netty | 10 | 11 | 2 | 0 → 0 | 未检出新增项 |
| okhttp | 4 | 4 | 5 | 0 → 0 | 未通过 |
| picocli | 2 | 2 | 2 | 0 → 0 | 未检出新增项 |
| pool2 | 1 | 1 | 1 | 0 → 0 | 未检出新增项 |
| protobuf | 0 | 10 | 2 | 0 → 0 | 未通过 |
| rxjava | 4 | 9 | 8 | 0 → 0 | 未通过 |
| slf4j | 0 | 0 | 0 | 0 → 0 | 未检出新增项 |
| snakeyaml | 0 | 0 | 0 | 0 → 0 | 未检出新增项 |
| spring | 3 | 3 | 3 | 0 → 0 | 未检出新增项 |
| spring-beans | 0 | 1 | 2 | 0 → 0 | 未通过 |
| xstream | 0 | 6 | 3 | 0 → 0 | 未通过 |
| zxing | 2 | 11 | 9 | 0 → 0 | 未通过 |

## 已修复与仍阻断的问题

本轮修复覆盖 JSR 展开后的 PC 长度校验、局部 Class[] 反射参数的异常捕获保留、
分支目标在临时变量折叠时的身份保留，以及部分引用类型汇合和声明类型问题。
Collections 的新增编译错误已消除；Byte Buddy 的新增 stub 大幅减少，仍有两个
嵌套 JSR 方法未恢复。类型汇合的后续修复尚未收敛，不能发布。

剩余问题包括嵌套 JSR、外部依赖和泛型类型约束、不可访问的公共父类型选择、
构造器调用前的数组初始化、switch/loop 出口重建，以及旧源码修复覆盖新类型分析。
具体编译单元和诊断保存在上述 JSON 中。

另加入 12 项运行时对照测试（List 重赋值、Class/Type、流包装；两种 debug 设置、
两种模式）及一个合法 JSR classfile 用例。**180 项隔离往返用例在 JDK 17 上通过**，
unsupported irreducible 用例单独检查降级诊断，不计入成功往返。
List 用例曾出现“编译与 JVM 验证均通过、分支反转导致输出不同”，现已修复；
基线也存在该用例的错误，不能把此发现归因为 PR 新引入。

更广的 `go test ./classparser/... -count=1` **仍失败 7 项**，失败未忽略或改为跳过：

- `TestSevenZFileBoundedStreamWidenIsLoadBearing`
- `TestFramedLZ4BoundedStreamWidenIsLoadBearing`
- `TestSevenZOutputFileWrapperIsLoadBearing`
- `TestExecutableArmMergeIsLoadBearing`
- `TestExecutableArmMergeSpringObjectToObjectConverter`
- `TestJavassistSignatureAttributeTypeIsLoadBearing`
- `TestLazyInitSelfTernaryNarrowIsLoadBearing`

其中包含真实类型错误，也包含需要在语义正确性证明后重新评估的旧修复开关断言。
本报告不以此理由豁免失败。core、类型、class_context、rewriter 测试包通过。

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
退出码 1 表示检出回归，2 表示缺少证据或不可比较；本次初版与后续比较均返回 1。
比较同时检查诊断所属源码单元及出现次数、stub、缺失源码、原本可编译的包退化，
以及双方完整编译成功时的 JVM 验证退化。相同基线对比自身返回 0，作为工具自检。
重新测量应使用新的报告目录，测试拒绝覆盖旧观测。

## 证据边界

- 整包测试使用生产 JarFS 默认兼容路径，没有对 38 个包的全部 API 执行运行时差分。
- 依赖沿用历史测试的环境 shim；这些只是编译/链接支持，不代表相应可选平台功能可用。
- javac 失败可能只留下部分或没有 class。此时即使验证失败数为 0，也不能记为验证成功。
- MR 版本参与各自 release 的源码编译；验证器仅枚举普通类条目，不证明所有 MR 运行分支。
- JVM 验证失败记录最多展示 40 个类，失败总数单独记录；不会把日志截断解释为全部通过。
- 重新编译与 JVM 验证均不证明方法行为相同。发布必须继续解决剩余差分失败，
  补充对应运行时用例，再验证最终 PR 版本。

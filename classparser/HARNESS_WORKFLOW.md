# Java 反编译长尾清零工作流 (HARNESS_WORKFLOW) — 已迁移

> 本文原描述 yaklang 时代(`common/javaclassparser/...` + `TestM2StubReasons`/`TestDiagDecompileClass`
> 等 `~/.m2` 扫描入口)的长尾清零流程。这些入口在 javajive 仓库已不存在, 故本文不再维护。
>
> **权威方法学已迁移并升级到根目录 [`HARNESS.md`](../HARNESS.md)**, 其中:
>
> - 正确性如何检验: tree / iso 盘点、合成往返、真实 jar 重打包+verify+调用差分、算法 battery。
> - 长尾如何一个一个修: 定位 → 复现 → CFR/Vineflower oracle 对照 → 根因 → kill-switch 治本
>   → A/B delta 承重锁定 → 复扫(原 §「遇到难 case 的通用解题法」与「红线」已整体折入)。
> - 当前可执行工单见根 [`TODO.md`](../TODO.md); 状态账本见 [`CODEC_TODO.md`](./CODEC_TODO.md)。

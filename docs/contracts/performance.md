# T32 performance integration hooks (additive)

Owner: evidence worker. **Main integration owner** applies these on the IR-integrated candidate. Do not write the IR worktree (`5b87e39`) from this branch.

## Historical SHAs

| Role | SHA |
|---|---|
| Milestone | `24b9d377c91ba75f3a277672973782299a958d5f` |
| PR base / historical main | `81f8ef55603419bf737f10aa36e582f94619b35d` |
| Evidence checkpoint | `56b59fea8955e7a5f3479866fc042cdb50cdf613` |
| Frozen T01 replay tree (read-only) | `31de113dded2fec7c34799776f43cca213aa1d77` |
| IR analyses checkpoint (read-only) | `5b87e39bce0d6ef1835a0b84f81b9dff070be867` |

Do not claim 56b59fe or 31de113 is the latest integrated candidate.

## Request-bound instrumentation

Every timed stage must use a **fresh** `*core.Decompiler` per repeat (no shared mutated CFG). Set `DecompileOptions.MaxAnalysisUpdates` / `d.MaxAnalysisUpdates` from the request; do not read process-global env for budgets.

## Production APIs to time (pure vs combined)

| Stage ID | Kind | API | Notes |
|---|---|---|---|
| `parse` | pure | `javajive.ParseClass` | |
| `decode` | pure | `(*core.Decompiler).ParseOpcode` | |
| `cfg_opcode_only` | pure opcode | `ScanJmp` + `DropUnreachableOpcode` | Not SemanticCFG |
| `semantic_cfg_build` | **pure production CFG** | `(*core.Decompiler).BuildSemanticCFG` | Additive export of existing `buildSemanticCFG`. IR `5b87e39` already has this. Evidence tree adds `t32_timing_hooks.go`. |
| `dataflow_stacksim_opcode_only` | pure opcode | `CalcOpcodeStackInfo` | Not T25 sparse |
| `dataflow_reaching_defs` | **requested of IR merge** | `(*SemanticCFG).ReachingDefinitions` | Exported on both trees; wire as a separate timed stage when harvest is stable |
| `t25_sparse_reaching` | **IR-only** | `NewSparseReaching` / `IndependentReachingOracle` | Present at `5b87e39`, **absent** on 81f8ef5/56b59fe |
| `t26_graph_cache` | **IR-only** | `GetOrCompute`, `AnalysisComputeCount`, `AnalysisNodesTouched` | Present at `5b87e39` |
| `region_and_statements_combined` | combined | `decompiler.ParseBytesCode` | Includes earlier passes |
| `render_class_dump_reruns_decompile` | combined | `ClassObject.Dump` | Re-runs decompile |
| `e2e_kernel` | combined Precision | `DecompileWithOptions` | javac/java **never** in kernel ns |

## Input legality

Original class bytes used for T32 **must** pass `java -Xverify:all`. Unverifiable input is not accepted. No `-Xverify:none` fallback. Families are `javac --release 8` output.

## Counters for T24/T25/T26

`WorkCounts` already has `nodes`, `edges`, `def_merge`, `region_roots`, `pending_t24_t25_t26`.

When merging IR:

1. After `BuildSemanticCFG`, set `nodes=len(g.Nodes)`, `edges=len(g.Edges)`.
2. T25: fill `def_merge` from sparse reaching work units, not opcode-graph proxies.
3. T26: record `AnalysisComputeCount` / `AnalysisNodesTouched` per request; do not share CFG caches across requests.
4. Set `pending_t24_t25_t26=false` once those counters are production-backed on the integrated SHA.

## Evidence tree vs IR tree

This evidence branch may export `BuildSemanticCFG` only. It must **not** copy T25/T26 algorithms from `5b87e39`. Main integration uses the canonical IR method `(*Decompiler).BuildSemanticCFG` (do **not** add `t32_timing_hooks.go`; it would clash with the IR export) and wires T25/T26 stages without changing default decompile output. On this integrated tree `pending_t24_t25_t26` is false.

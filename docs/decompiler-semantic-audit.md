# Decompiler semantic audit implementation

Baseline: `e1710d5e63736a747dd9a02c164507348c59ad5e` (the revision examined by the optimization report). This change repairs the report's immediate semantic defects and introduces an immutable instruction-flow analysis alongside the existing structurer. The implementation is intentionally explicit about the boundary between the new analysis and the remaining legacy machinery.

## API and evidence

```go
result, err := javajive.DecompileWithOptions(classBytes, javajive.DecompileOptions{
    Mode: javajive.Precision,
    MaxAnalysisUpdates: 1_000_000, // per method; zero selects this default
    Resolve: resolveClassBytes,   // optional JVM internal-name resolver
})
```

`Precision` is the default for this new API. It excludes the registered class/method source-recovery pipeline. `Compatibility` retains that pipeline, recording rule IDs, phases and before/after hashes for every applied rule. The original `Decompile` and archive APIs retain their compatibility behavior.

`Status` describes decompilation only: `invalid_input`, `unsupported`, `partial`, or `complete`. `StubMethods` and `Diagnostics` identify method degradation and its reason. **Complete does not mean recompiled, JVM-verified, or behaviorally equivalent.** These observations belong to the independent execution harness, not to the decompiler. Precision still uses existing core and region rewrites; it is not a verified or entirely heuristic-free decompiler.

Requests own their mode, analysis budget, resolver, result and rewrite history. The API never mutates the process environment. Legacy `JDEC_*` environment switches still exist in older core passes. Shared mutable rendering probe contexts were removed, and nested lambda method context is restored with `defer`, including on panic.

## Report-to-code mapping

| Report item | Implemented change | Remaining boundary |
| --- | --- | --- |
| P0-01 switch identity | Dedicated default offset/index and node identity; every signed integer remains a real case value; shared targets survive decoding and rewrites; physical fallthrough order is retained. | String-switch desugaring still uses the existing structurer. |
| P0-02 wide branches | Signed 16/32-bit displacement decoding; range and instruction-boundary checks, including unreachable branches. | Legacy jsr/ret must be successfully inlined before semantic analysis. |
| P0-03 local accesses | One opcode metadata table for read/write, slot, width and wide legality; FLOAD_3; iinc reads and writes; category-2 overlap invalidation. | This is not a full JVM bytecode verifier. |
| P0-04 invented behavior | Removed the case-0/8 guessed break and aliased-Throwable wrapping rules; prevented transitive folding through catch parameters. Other source recovery is explicit in compatibility mode and absent from precision mode. | Other legacy source rules remain available in compatibility mode and are reported. |
| P0-05 literal corruption | Java lexical shielding for hardjar/Mockito code-pattern rewrites, covering quoted strings, chars, text blocks and comments; actual package-declaration gating. | Lexical shielding is containment for these passes, not a complete Java lexer/parser. |
| P0-06 truthful acceptance | Independent compile, verification, stub and runtime observations; original and rebuilt classpaths are isolated; required Java tools fail instead of skipping audit tests; failed cases also write their observation. | Existing historical jar tests still measure compile diagnostics and are not semantic proof. |
| P1-01 exceptional CFG | Immutable typed instruction edges; exception edges originate at potentially throwing instructions, use input locals, preserve handler order and stop at catch-all; distinct try starts sharing a switch predecessor keep separate handler ownership. | Exact exception-class dispatch and the existing synthetic try structurer are not replaced. |
| P1-02 fixed point | Monotone cached reaching definitions on immutable flow; stable opcode definition IDs, loop/iinc joins and bounded work. | Full block-frame and operand-stack SSA migration is not complete. |
| P1-03 variable identity | Reference web coalescing follows reaching definitions, separately joins RHS types, and preserves null and array facts. | Some older variable repairs remain; this is not complete SSA destruction. |
| P1-04 explicit expressions | Cast and embedded-assignment nodes expose dependencies; a cycle-safe visitor records reference uses and effects. | Other CustomValue nodes remain opaque barriers. |
| P1-05 type identity | Joins use binary class identity and hierarchy providers; nested-name spelling alone no longer proves receiver subtyping. | Full member/overload constraints, intersection types and all legacy suffix rules remain a future type-system migration. |
| P1-06 regions | Switch preparation is idempotent; shared continuations take precedence over interior candidates; merge identity follows node replacement; switch-only loops materialize back edges and label loop exits captured by switches; unsupported multi-entry normal regions produce diagnostics. | Handler regions and general irreducible restructuring/state machines are not implemented; existing natural-loop reconstruction remains. |
| P1-07 effects | Array initializer folding checks dependencies, distinct entry paths and exception-table coverage; preserves self reads and partially filled arrays observed by handlers. | A universal motion/scheduling proof for every rewrite is not implemented. |
| P1-08 rewrite contracts | Precision/compatibility policy, rule/phase provenance, class-source oscillation rejection, and regression tests that allow proven patch retirement. | All older structural passes have not been migrated to declarative preconditions/postconditions. |
| P2-01 cost control | Slot queries reuse a method-local fixed point; declaration-placement probes cache within an invalidated mutation epoch; stack Size is O(1). | General graph-versioned dominator/postdominator caches are not implemented. |
| P2-02 contexts | Per-request options and diagnostics, explicit analysis budget, panic-safe lambda context restoration, concurrent request race test. | Old environment-controlled core behavior is not fully replaced by options. |
| P2-03 adversarial validation | Generated legal classfiles, metamorphic debug/no-debug and key-renaming tests, two policies, multiple Java compiler versions in CI, decoder fuzzing and durable evidence. | No claim of a complete random-program generator, ECJ matrix, or external holdout benchmark. |

## Adversarial families

The single-class source suite has 39 shapes × 2 debug configurations × 2 policies = **156 independent round trips**. Each target is compiled with `--release 8`, run with `-Xverify:all`, decompiled, compiled into a fresh directory, and executed again with only that directory on the application classpath. Verification is a separate JVM process using reflection without target initialization; execution runs afterward in a fresh process. Inputs include signed boundaries, NaN, signed zero, branch arms, side-effect traces, exception class and identity, suppressed exceptions, and lock release.

| Families | Observed contract |
| --- | --- |
| A01–A02 | Dense/sparse signed switches, min/max keys, shared default targets, default in the middle. |
| A03–A04 | Short/40K forward/40K backward goto_w; wide slot 300; float slot 3 including NaN and negative zero. |
| A05–A09 | Parameter reuse, sibling locals, null/String/Integer joins, loop-carried locals, ordinary and wide signed iinc, category-2 values. |
| A10–A14 | Throw-site locals, ordered handlers, finally capture/override, resource close order/suppressed exceptions, exception identity. |
| A15–A18 | Renamed switch keys, real break vs fallthrough, common tails, short-circuit traces, postincrement, self-referencing arrays, effect order and handler-visible partial fills. |
| A19–A21 | Floating comparisons, integer overflow/shifts/narrowing, multi-continue loops with effectful non-unit steps. |
| A22 | Normal arm next to an infinite loop; the infinite arm is intentionally not executed, so termination equivalence is not claimed. |
| A23 | JVM-verified legal multi-entry loop is reported as unsupported with a method stub. This is a diagnostic test, **not** a successful round trip. |
| A24–A26 | Generic overload selection, identical simple names from different packages, unrelated nested classes; every application class is rebuilt. |
| A27–A30 | Final constructor assignments, literal/comment shielding, lambda context, nested monitor release. |

Five generated version-49 classfiles cover legal wide bytecode that javac would otherwise normalize away. Six further round trips rebuild complete multi-class source sets. No original target class is retained on their rebuilt classpaths. The decoder unit/fuzz suite separately exercises truncation, illegal wide targets, extreme switch counts, invalid branches and malformed ranges.

## Reproduction

Java 17+ and Go 1.22+ are required for the semantic audit. On macOS with older Go toolchains, use the external linker (`CGO_ENABLED=1` and `-ldflags=-linkmode=external`).

```sh
# Independent evidence, including failures (optional output location).
JDEC_SEMANTIC_REPORT_DIR=/tmp/javajive-semantic go test ./test/cross -run '^TestAudit' -count=1 -timeout=10m -v
# Decoder and analysis invariants.
go test ./classparser/decompiler/... -count=1
# Request isolation and lexical containment.
go test -race ./classparser -run 'TestDecompilePolicy|TestSourceRewriteCycle|TestJavaCodeRewrite|TestMockitoPackageGate' -count=1
# Bounded adversarial decoding.
go test ./classparser/decompiler/core -run '^$' -fuzz '^FuzzAuditDecoder$' -fuzztime=30s -parallel=2
# Whole repository; optional Maven artifacts expand the historical corpus.
go test ./... -count=1 -timeout=30m
```

CI runs the dedicated semantic audit on JDK 17 and 21 and uploads the observation records even on failure. The pre-existing operating-system/Go matrix and race jobs remain enabled.

## Baseline and performance interpretation

The exact base revision's CI was already failing all six test jobs (run `34084803010`). A local baseline also failed the VarFold and SuperTest source snapshots, several obsolete ON/OFF sensitivity assertions, and timed out in the Maven-backed cross corpus. This change updates those two embedded snapshots to the actual baseline output; the source files and their embedded archive agree.

A narrow repair becoming redundant is allowed when both modes retain the positive invariant. Historical aggregate-error tests use non-regression comparisons and print residual error counts; those residual counts are not converted into semantic passes. The new isolated audit remains a separate acceptance gate.

On one macOS arm64/Go 1.22.12 run, a 65,535-element stack Size query changed from about 76–78 microseconds to about 0.72–0.74 nanoseconds with zero allocations. This is an intentionally deep-stack microbenchmark demonstrating O(depth) → O(1), **not** an end-to-end decompiler speedup. The decoder fuzz run completed approximately 1.26 million executions in 30 seconds without a crash. Machine-dependent counts and timings are evidence from that run, not CI thresholds.

# Changelog

## v0.4.0 — 2026-09-19

Preserve bytecode semantics and separate decompilation results from independent
compilation, JVM verification, and runtime observations.

- Fix signed switch/default identity, wide branches and local accesses,
  exception-handler ownership, loop exits, reference joins, and array side effects.
- Add `DecompileWithOptions` with precision/compatibility modes, method-local
  analysis budgets, type resolution, rewrite provenance, and degradation diagnostics.
  Existing APIs retain compatibility behavior.
- Add immutable instruction-flow/reaching-definition analysis; remove selected
  guessed source repairs and isolate concurrent request and synthetic-catch state.
- Add 250 isolated semantic round trips to the JDK 17 and 21 CI matrix, with original
  classes absent from rebuilt classpaths and separate compile/verify/runtime records.
  Cover legal wide bytecode, malformed decoder inputs, determinism, and race checks.
- Preserve full OS/Go test coverage while sharding the large race corpus without
  reducing determinism repetitions. Cache persistent stack depth for O(1) Size.
- Repair nested JSR expansion, reference-web declaration types, constructor and
  assignment ordering, retry/cleanup regions, catch identity and Kotlin monitor boundaries.
- Enforce all 38 historical JARs in CI with 174 SHA-256-pinned artifacts. Local
  comparison records zero new regressions, retaining all 20 compile-clean baseline
  JARs and increasing full compilations to 26; 4 existing stubs remain.

The historical v0.3.0 benchmark below is not current semantic acceptance evidence.
See the [complete corpus results](docs/historical-jar-audit.md) and
[implementation boundaries](docs/decompiler-semantic-audit.md). Full operand-stack
SSA, general type constraints and irreducible restructuring remain incomplete.

## v0.3.0 — 2026-09-05

34-jar tree-zero: the original 8 benchmark jars plus 6 expansion jars and 20
typical libraries all decompile, tree-recompile with 0 `javac` errors,
repackage, and pass external JVM `-Xverify:all`.

- **Tree-clean 18,759 / 18,759** flattened units across 34 libraries. Syntax
  errors remain 0.
- **Full round-trip on 34/34** (locked in `provenClean`):
  original 8 (codec / gson / lang3 / jsoup / snakeyaml / fastjson2 / guava /
  spring-core), expansion 6 (jackson-databind / okhttp / collections4 / netty-handler /
  log4j-core / protobuf-java), and twenty typical libraries (asm, joda-time,
  commons-io, commons-compress, httpclient, slf4j-api, logback-core, caffeine,
  rxjava, javassist, xstream, commons-math3, HikariCP, jedis, junit, assertj-core,
  picocli, commons-pool2, zxing-core, freemarker).
- Self-hosted algorithm round-trip still **14/14** byte-identical.
- Dump reconstructs for the 20-jar remainder (boolean-zero, int-as-boolean,
  empty-synchronized missing return, checked-exception catch unions, `$closeResource`
  throws, and per-library remaining families), all kill-switched.

## v0.2.0 — 2026-09-04

8-jar tree-zero: every benchmark jar decompiles, tree-recompiles with 0 `javac` errors,
repackages, and passes external JVM `-Xverify:all`.

- **Tree-clean 4,487 / 4,487** flattened units across commons-codec 1.15, gson 2.8.9,
  jsoup 1.10.2, snakeyaml 2.2, fastjson2 2.0.43, commons-lang3 3.12.0, guava 28.2-android,
  spring-core 5.3.27. Syntax errors remain 0.
- **Full round-trip** on all 8 (locked in `provenClean`): codec 107/107, gson 199/199,
  lang3 346/346, jsoup 241/241, snakeyaml 233/233, fastjson2 689/689, guava 1892/1892,
  spring 952/952 verify.
- Rebuilt previously stubbed `Monitor.enterWhen(Guard,long,TimeUnit)` and
  `InetAddresses.textToNumericFormatV6` (`JDEC_LEAKED_EXCEPTION_SENTINEL_OFF`).
- Self-hosted algorithm round-trip **14/14** byte-identical (was 5/5).
- Generic/CFG reconstructs that zeroed guava and spring (factory raw-return bridges,
  enclosing type-var arg casts, diamond+try sentinel reconstruct, switch-break missing
  return, method-ref FI casts, objenesis NSME wrap, and the rest of the kill-switched
  dump/rewriter chain).

## v0.1.3

Prior tagged release on `main`.

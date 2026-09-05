# Changelog

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

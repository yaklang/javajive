# Changelog

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

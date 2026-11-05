# Complex JAR adversarial evaluation

This report records the JAR-level coverage added after the irreducible-CFG and
adversarial-classfile rounds. It answers two questions directly:

1. How many JARs are in the evaluated corpus?
2. Are they fully decompilable, recompilable, verifiable and behaviorally
   equivalent?

The numbers below are the checked-in CI claims. Historical-corpus numbers are
the durable audit in `docs/historical-jar-audit.md`; this report adds the new
complex corpus.

## Corpus inventory

| Corpus | JARs | Classes / units | Result |
|---|---:|---:|---|
| Historical libraries (`test/cross/testdata/historical-jars.lock.json`) | 38 | 24,883 classes / 24,359 units | 26/38 full tree compile; 4 stub units; 12 packages retain baseline residual errors |
| Checked-in small fixtures | 4 | — | used by archive/unit tests |
| New complex corpus (this suite) | 7 | 151 decompiled units | all tree-compile, verify and (where a driver exists) behavior-match |
| **Total JAR inputs** | **49 paths / 48 unique files** | | plexus-cipher is mirrored in two test roots |
| External Kotlin stdlib 2.0.21 stress test (not gated) | +1 | 994 classes | 29 whole-class stubs, 104 method stubs, 77 files with stubs |

The historical 38 are not vendored; CI downloads and SHA-256-pins them. The
four checked-in fixtures are
`classparser/jarwar/testdata/plexus-cipher-2.0.jar`,
`javajive_tests/jarwar/testdata/plexus-cipher-2.0.jar`,
`classparser/testdata/regression/Join-declarations.jar`, and
`classparser/classes/static/test.jar`.

This round did not re-run the complete 38-JAR audit. A local smoke check of
commons-codec 1.15 passed with 106 units, 0 tree errors and 106/106 verified.
The historical figures above remain the durable v0.4.0 audit baseline.

## New complex corpus

`JAVAJIVE_COMPLEX_JAR_EVAL=1 go test ./test/complexjar -count=1 -v`

| Case | JAR content | Result |
|---|---|---|
| `TestComplexModernJarRoundTrip` | Java 21 records, sealed interfaces, guarded pattern switch, switch expressions/yield, text blocks, lambdas/method refs, local/anonymous classes, generics/bridges, interface private/default/static methods, enum constant bodies, try-with-resources, annotations, synchronized, method handles | 16 units; tree compile; `-Xverify:all`; outputs byte-identical |
| `TestComplexControlFlowJarRoundTrip` | nested/labeled loops, switch-in-loop, dense/sparse/fall-through switches, string/enum switch, try/finally/return, try-with-resources, monitor blocks, exception loops, state machine, recursion, ternaries, short-circuit, arrays, evaluation order | 12 units; outputs byte-identical |
| `TestComplexLegacyJarRoundTrip` | Java 8 bytecode: enum constant bodies (legacy synthetic bridge shape), lambdas, anonymous classes, string switch, try-with-resources, generics, nested loops | 9 units; enum bodies folded into owner; outputs byte-identical |
| `TestComplexScaleJarRoundTrip` | 100 generated package classes, each with loop/switch/array/string methods, plus a chunked driver | 101 units; outputs byte-identical |
| `TestComplexMultiReleaseJar` | base `--release 8`, `META-INF/versions/17`, `META-INF/versions/21`, `Multi-Release: true` | 3 units; each release recompiled with its own `--release` and run; outputs identical |
| `TestComplexModuleJar` | `module-info.class` + exported package | 1 source unit; classpath compile/run identical; module descriptor is rendered as a synthetic-descriptor comment |
| `TestComplexIrreducibleJarArchive` | checked-in major-49 multi-entry/irreducible classes packed as a JAR | 8 classes; tree compile; behavior is covered by `TestRoundTripIrreducibleCFGCorpus` |

The suite asserts **zero decompiler stub markers** in the produced sources, so
a method that silently degrades to a throwing stub fails the case instead of
being counted as success.

## Defects found and fixed in this round

1. **Switch-expression values were silently discarded.** javac leaves each
   arm's value on the operand stack and jumps to one join. The decompiler used
   one predecessor's stack snapshot, so `return switch (x) { case 0 -> "zero";
   ... }` emitted empty cases and returned only the default. Acyclic stack
   joins now materialize the selected word in a synthetic local on each
   incoming edge. Null arms, try-protected arms, long/double values, and
   stored/local-consumer joins are covered.
2. **A multi-arm join whose arms are ternaries was mistaken for one ternary.**
   The combiner built a condition tree from independent case predicates and
   dropped all but the first arm. Multi-source joins now require the closed
   routing-tree certificate; otherwise the stack-phi path handles every arm.
3. **Cross-scope phi locals could split into sibling names.** The synthetic
   join local was first named inside a guarded arm and again in sibling cases.
   The join now gets one method-root declaration before `RewriteVar`.
4. **Pattern-switch restart variable lost its rebinding.** The `typeSwitch`
   selector and restart operands live only inside a render closure; the restart
   kept its unnamed pre-rewrite id and rendered with a depth-derived name that
   collided with the selector. The closure now forwards `ReplaceVar`, and it
   streams through the request output budget. The no-match sentinel is
   `labels.length`, so `case null` no longer aliases the default arm.
5. **Whole-method pattern-switch reconstruction was applied to stored
   switches.** T21 emitted `return switch (...)` for a pattern switch nested in
   a loop or assigned to a local, replacing the entire method body. It now
   requires the switch result to be the method return; other shapes keep their
   surrounding statements and use the core dispatcher.
5. **A switch that is the whole infinite-loop body used unlabeled breaks.**
   Each case break exited only the switch, so the loop spun forever and the
   post-loop return was unreachable. Direct case breaks are now labeled
   `break LOOP_n` when the switch is the sole loop body.
6. **Record `implements` clauses were dropped.** Dropping `extends Record` for
   a record declaration also cleared `implements Shape`. Only the extends part
   is removed now.
7. **Record compact constructors could contain illegal `return`.** The
   decompiler falls back to an explicit canonical constructor, renames its
   parameters to the component names, and keeps the required field
   assignments.
8. **Promoted nested records had package-private canonical constructors.**
   Flattening can promote a package-private nested record to public; its
   canonical constructor must then be public. The dumper now aligns the
   constructor access with the rendered record declaration.
9. **Private interface methods rendered `private default`.** Java 9+ private
   interface methods have bodies but are not default methods.
10. **Modern enum constant bodies were rejected.** Java 9+ classfiles omit the
    legacy synthetic trailing constructor argument, use a private body
    constructor and carry `NestHost`; Kotlin also encodes array clone with
    `java/lang/Object`. The proof now accepts both forwarding shapes and the
    modern nest metadata.
11. **Discarded static field reads produced invalid Java.** A `getstatic; pop`
    (Kotlin's `Result.Companion;` idiom) rendered a bare field expression, which
    is not a Java statement. It is now evaluated in a boolean sink
    (`(Owner.FIELD == null)` / `== 0`) so `<clinit>` and exceptional exits are
    preserved without inventing a field-statement.
12. **Int-to-boolean coercion was rejected under an output budget.** The
    `(word) != (0)` sandbox renderer now streams through the request writer
    instead of being classified as an unbounded opaque value.

## Remaining boundaries

- **Kotlin/JVM metadata is not a supported source target.** The external
  `kotlin-stdlib-2.0.21.jar` test decompiles 994 classes but leaves 29
  whole-class stub comments and 104 method stubs. Most whole-class stubs are
  Kotlin enums carrying a synthetic `$ENTRIES` field initialized through
  `EnumEntriesKt.enumEntries($VALUES)`, a shape the enum-regeneration proof
  does not yet model. This JAR is an observation, not a CI gate.
- **Pattern switches without a source-level reconstruction.** When javac emits
  `MatchException`/complex deconstruction, the decompiler may emit an
  equivalent `instanceof`/state-dispatch form instead of a Java 21 pattern
  switch. The complex JAR and adversarial suites compile, verify and compare
  behavior for these outputs; they are not pretty-printed back to the exact
  source form.
- **Java 21 record-deconstruction patterns are partially supported.** The
  checked-in T21-C06 fixture reconstructs its shape. A separate adversarial
  shape (`case Circle(double r)` over a nested sealed hierarchy) compiles its
  accessor calls under shared `Throwable` handlers that wrap failures in
  `MatchException`; the current shared-handler structurer rejects that shape
  and emits a method stub with an `exception-handler structuring failed`
  diagnostic. It does not emit silently wrong source. Simple type patterns,
  `case null`, guards, and equivalent-dispatcher forms are covered.
- **Very large single methods.** A generated 250-class `main` with ~7,500
  statements and 47 KB of bytecode exceeds the documented
  `MaxAnalysisUpdates=1,000,000` reaching-definition budget and degrades that
  method to a stub. Splitting the same work into ten methods passes. This is a
  policy boundary, not a silent miscompile.
- **Constructor prefixes.** Java cannot declare a synthetic local before the
  `super()`/`this()` delegation, so acyclic stack phis and discarded-value
  sinks are disabled inside constructors; constructor switch-expression joins
  remain on the established prefix path.
- **`module-info.class` is inspected but not regenerated.** The archive path
  emits a comment-only unit; modular descriptors are not reconstructed. The
  exported classes still compile and run on the classpath.
- **Flattened nested names.** `Outer$Inner` is emitted as a top-level
  `Outer$Inner.java`. This is the established archive design; reflection
  (`getDeclaringClass`) and record `toString` prefixes can differ even when
  method behavior is identical. The behavior driver avoids class-name text.
- **Multi-release JARs require per-release compilation.** The decompiler keeps
  base and `META-INF/versions/N` units separate, but one javac invocation
  cannot compile two classes with the same binary name. The suite compiles
  each release in its own pass.

# Changelog

## Unreleased — 2026-11-03

Add a native, pure-Go DEX/APK-to-JAR path as a strict first vertical slice.

- Add deterministic in-memory and atomic file conversion through `DexToJar` and
  `DexFileToJar`, plus direct source output through `DecompileDEXFile` and the
  CLI `dex2jar` / DEX-aware `decompile` commands.
- Cover standard DEX 035 ordinary integer/reference control flow, fields and
  invokes, safe constructor initialization, and basic arrays. Translation is
  transactional: unsupported input fails the whole product and never emits
  fallback method stubs.
- Treat APK input as multidex code input only: translate canonical
  `classes*.dex` entries, not resources or native libraries.
- Keep the initial boundary explicit: try/catch and monitors; float, long, and
  double operations; switches, payloads, and filled arrays; DEX 038+ dynamic
  invokes; annotations/debug metadata; and CompactDex remain unsupported.
- Recover normal-edge JVM irreducible regions with deterministic state dispatch,
  including multiple entries, switch entries, multiple exits, and irreducible
  kernels nested in natural loops. The checked-in major-49 corpus verifies original,
  reviewed-equivalent, and rebuilt behavior without requiring identical bytecode.
  Constructors, exception-entry, monitor, uninitialized-object,
  non-empty-stack-boundary, and uninlined `jsr`/`ret` regions remain explicit
  unsupported boundaries.
- Fix shared-exit handoff for irreducible regions whose pre-loop predecessors
  cannot be consumed as an ordinary loop guard. A synthetic skip flag keeps the
  dispatcher loop skippable while an acyclic gate preserves per-case switch
  breaks; this removes a silent miscompile for switch-default and entry-chain
  exits. Single-exit corpus sources are unchanged.
- Hoist cross-case dispatcher locals to the method root with a type-default
  initializer, fixing rebuilt sources that javac rejected as possibly
  uninitialized. The pass reuses the original variable identity and is gated by
  `JDEC_DISPATCH_LOCAL_HOIST_OFF`.
- Collapse irreducible exits that converge on one shared post-exit continuation
  into a single dispatch exit. Each member exit carries its state prefix into
  the exiting case body; the shared continuation executes once after the loop
  and reuses the skip guard for pre-loop predecessors.
- Add an opt-in adversarial classfile evaluation suite and a CI gate covering
  real javac shapes, irreducible boundary behavior, scale, and malformed input.
- Reject classfiles outside the supported major-version range (45..65) at
  decompile requests while keeping `Parse` permissive for structural
  inspection. Validate bytecode constant-pool operands before stack simulation,
  so a missing or wrong-kind constant produces a structured
  `invalid_constant_pool_reference` method diagnostic instead of a recovered
  runtime panic.

### Complex JAR hardening

- Recover stack-carried values at acyclic control-flow joins. Java switch
  expressions and ordinary shared-return joins now write the selected arm into
  a synthetic local; previously the decompiler emitted empty cases and silently
  returned only one arm. Null arms, try-protected arms, and joins whose
  producers are themselves ternaries are included.
- Do not classify a multi-source join whose value producers are independent
  switch arms as a ternary. The ternary combiner treated all arm conditions as
  one routing tree and dropped every arm except the one it picked; these joins
  now use the stack-phi path.
- Render int-to-boolean argument coercions through the streaming writer so a
  request output budget cannot reject an otherwise valid method.
- Give those synthetic join locals one method-root declaration before variable
  naming, so a value defined first inside a guarded arm cannot acquire sibling
  names.
- Forward variable rebindings through the `typeSwitch` selector/restart closure
  and render it through the request output budget, fixing colliding
  pattern-switch locals and bounded-render failures. The rendered no-match
  sentinel is `labels.length`, not `-1`, so a `case null` arm is no longer
  aliased with the default arm.
- Reconstruct a source-level pattern switch only when its value is the method
  return. A pattern switch nested in a loop or stored to a local now keeps the
  surrounding statements and uses the core dispatcher.
- Label direct switch-case breaks when a switch is the whole infinite-loop body,
  so the loop exits and the post-loop continuation stays reachable.
- Preserve record `implements` clauses when removing `extends Record`; emit an
  explicit canonical constructor when a compact constructor would contain an
  illegal `return`; and align canonical-constructor access with a flattened
  record's rendered visibility.
- Stop rendering private interface methods as `private default`, and materialize
  discarded static field reads in a boolean sink so `getstatic; pop` remains
  legal Java without losing class initialization.
- Accept modern enum constant-body classfiles (Java 9+ direct forwarding and
  `NestHost`; Kotlin's `java/lang/Object.clone` array encoding).
- Reconstruct Kotlin enums that publish their backing array through
  `EnumEntriesKt.enumEntries($VALUES)`: the `$ENTRIES` read is certified, the
  erased backing field is regenerated as `values()`, and `getEntries()` keeps
  declaration order and identity. Kotlin stdlib whole-class stubs drop from 29
  to 8 in the external stress observation.
- Resolve nested-type spelling from the archive's own emitted sibling units
  before applying stdlib dotted shortcuts, so Kotlin's flat `Outer$Inner`
  classes compile while external `java.util.Map$Entry` stays dotted.
- Add an opt-in large-method analysis budget: `DecompileOptions` values above
  1,000,000 scale the derived graph/scan/set/node caps up to 64× (64M ceiling),
  and the new `DecompileArchiveWithAnalysisBudget`,
  `DecompileDEXFileWithAnalysisBudget`, `NewJarFSFromLocalWithAnalysisBudget`
  and CLI `-analysis-budget` route the same policy through archives, DEX
  translation and single classes. The default policy is unchanged.
- Reconstruct Java 21 record deconstruction pattern switches instead of
  failing closed. T21 certifies javac's shared `Throwable`/`MatchException`
  handler, reads the target record's declared component list, emits
  `case Record(component...)` patterns, and reproduces the compiler default
  throw. Nested record components reconstruct recursively, array components
  keep a `T[]` binding, and the compiler default throw is emitted explicitly.
  The adversarial shape compiles, verifies and matches runtime behavior; a
  generated 3-round corpus exercises 1-3 records with 1-3 primitive components
  per case, plus nested and array component cases.
- Recover switch-expression joins in constructor bodies by placing the
  synthetic join local immediately after the `this()`/`super()` delegation.
  Joins in the constructor prefix (for example a switch expression passed to
  `super(...)`) have no Java statement form and now fail that method closed
  with a diagnostic instead of emitting empty cases that lose the value.
- Reconstruct `module-info.class` as a real source module declaration instead
  of a comment: `open module`, `requires` transitive/static, `exports ... to`,
  `opens ... to`, `uses`, and `provides ... with`. Fix the value-receiver
  `ConstantModuleInfo`/`ConstantPackageInfo` readers that left module and
  package names empty. The modular JAR fixture now compiles and runs with
  identical classpath and module-path behavior.
- Add `test/complexjar` with seven round-trip JARs (Java 21 features,
  control-flow torture, Java 8 legacy, 100-class scale, multi-release, module,
  and packaged irreducible classes) covering decompile → tree recompile → JVM
  verify → behavior comparison, plus Java 21 adversarial source shapes in the
  existing CI-gated suite. See `test/complexjar/REPORT.md`.

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
SSA and general type constraints remain incomplete; irreducible restructuring is
limited to the audited normal-edge, stack-empty region described above.

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

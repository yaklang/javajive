# Changelog

## Unreleased — 2026-11-10

Universal input containers across the Java archive and Android pipelines, plus
bounded failure recovery for malformed and partially unsupported inputs.

- Harden DEX IR decoding: validate switch and fill-array payload targets with
  signed/`uint64` arithmetic before indexing, reject negative, wrapped, and
  final-word payloads, and cap instruction×register analysis state before
  allocation instead of amplifying memory from an admitted DEX.
- Add explicit DEX failure policy APIs: `DexToJarWithOptions`,
  `DexFileToJarWithOptions`, and `DecompileDEXFileWithPolicy`. The existing
  strict APIs remain transactional and unchanged. `--stub-methods-only` permits
  verifier-safe stubs for recognized method-local capability gaps;
  `--best-effort` additionally recovers class failures through fresh
  declaration-preserving, reduced-metadata, and minimal shells. Dependency
  recovery propagates from minimal superclasses/interfaces, and unsupported
  static initializers fail closed through a throwing `<clinit>`.
- Successful degradation reports `partial` with a nil error. Partial JARs carry
  deterministic `META-INF/javajive/diagnostics.json`; recovered classes carry a
  bounded `JavaJiveRecovery` attribute that the decompiler renders as leading
  source comments. Malformed containers, whole-file validation failures,
  resource limits, and internal failures still abort.
- Add `--best-effort` and `--stub-methods-only` to `dex2jar` and Android
  `decompile`; the default remains strict.
- Isolate Java archive class failures: malformed and future-version class
  entries become diagnostic fallback `.java` files while other classes continue.
  Structured `ArchiveEntryReport` now carries `status`, `stub_methods`, and
  `diagnostics`, distinguishing complete, partial, and fallback sources.
  Resource/cancellation failures still terminate the request.
- Make real external-corpus and JDK/JLink integration tests explicit local
  evaluations (`JAVAJIVE_EXTERNAL_EVAL=1`,
  `JAVAJIVE_JIMAGE_INTEGRATION=1`). Required PR CI keeps small synthetic
  container, archive-containment, DEX-policy, and no-panic regressions.
- Add `internal/inputkind`: classify inputs by content magic, then archive
  structure, and only then the filename extension. Renamed APK/APKS/JMOD/AAR
  files now reach the correct pipeline.
- JMOD: strip the four-byte `JM\x01\x00` prefix when opening a local archive and
  when recursing into nested JMOD entries, so JDK module images decompile as
  ordinary archives.
- AAR and PAR: accept both as Java archives. `classes.jar` and `libs/*.jar` in
  AAR inputs are decompiled through the existing nested-archive path.
- Fix flat archive export for nested JARs. `DumpToLocalFileSystem` now resolves
  raw bytes through `JarFS.ReadRawFile` and recovers the physical `.class` name
  behind UnifiedFS's virtual `.java` entries. WAR `WEB-INF/lib`, EAR `EAR/lib`,
  AAR `classes.jar`, fat-JAR `BOOT-INF/lib`, and arbitrary JAR-in-JAR layers
  export sources instead of logging `create child node failed`.
- Android split bundles: `.apks` / `.xapk` / `.apkm` / `.apkx` walk nested
  `.apk` entries, prefer `universal.apk`, skip resource-only splits, and merge
  identical classes across splits. Conflicting definitions fail the conversion.
- Android App Bundles: `.aab` translates every `<module>/dex/classes*.dex`
  entry across base and feature modules.
- Add bounded nested-container limits (`MaxTotalAPKSize`, `MaxNestedAPKs`) and
  global DEX count/size checks across all collected sources. The default
  nested-APK bound is 512, enough for bundletool split bundles with hundreds of
  resource/config APKs.
- Add a native `jimage` package for JDK 9+ runtime images (`lib/modules`,
  `.jimage`): parse the header, redirect/offset tables, attribute streams,
  string table, and resources on demand; inflate `zip`-compressed resources with
  zlib. `JImageToJar` and `OpenJImageFile` expose the format.
- `decompile` / `DecompileArchive` convert a JImage into a package-layout JAR
  view and reuse the flat or structured exporter. The new `jimage2jar` CLI
  subcommand exposes the conversion directly. Class resources are exported
  at package paths; `module-info.class` and non-class resources keep their
  module prefix. `inspect` reports `jimage` with per-resource hashes and
  compression state.
- Support `compact-cp` (constant-pool string sharing): reconstruct externalized
  UTF8 and descriptor entries, including `L;` type tokens, long/double slots,
  and the trailing class body. Reconstructed classes are byte-identical to the
  uncompressed image. Big-endian JImages (`jlink --endian big`) are supported
  too.
- Add pure-Go unit tests for compressed-index decoding and compact-cp constant
  pool reconstruction, plus opt-in `jlink --compress=1` and `--endian big`
  integration tests.
- Add `testdata/external` with real Tomcat WARs, bundletool APK/AAB/APKS
  (universal and a 181-APK split bundle), an AndroidX AAR, Androguard/Apktool/
  dex2jar fixtures, and synthetic XAPK/APKM/EAR containers. Tests classify,
  inspect, decompile, and translate every sample; malformed truncations assert
  no panics.
- Add fuzz seed corpora for the input classifier, JImage and compact-cp readers,
  `inspect`, and Android container parsing.
- Required CI runs small synthetic container/JImage tests and focused archive
  class-containment regressions; real external samples and JLink generation are
  opt-in local evaluations.
- `inspect` reports `jmod` / `jimage` / `aar` / `ear` / `par` / `apks` /
  `xapk` / `apkm` / `aab`, and records nested APK or AAB module DEX provenance.
- CLI `decompile` and `dex2jar` route by detected container instead of file
  extension; `--structured` now rejects Android containers with a clear error.
- Add classifier, container-parser, flat-export, JMOD/AAR/PAR, JImage, inspect,
  and end-to-end JAR tests for the new inputs, including real JDK
  `lib/modules` and `jlink --compress=2` images.

## Unreleased — 2026-11-06

Complete real-application DEX translation.

- Discover constructor sites from the register categories: an allocation's
  constructor is any direct `<init>` call whose receiver still holds that
  allocation. This is path-sensitive, so conditional constructors with
  different arguments on different branches work, and an allocation that is
  never constructed (dead API outlines) no longer fails the class.
- Capture the value of a field store that DEX places before `super()` into a
  fresh local, and emit the store after delegation, so the stored value
  survives register reuse. Constructor delegation accepts `this` aliases
  created by any move opcode, aliases retired by redefinition, and branches
  before delegation when the deferred stores dominate the constructor call.
- Model exception edges precisely: only instructions that can throw dispatch
  to a handler, the handler entry merges the register state at the start of
  each protected instruction, handler live-ins are made live in the protected
  blocks, and the backward constant analysis injects handler demands at those
  instructions. Registers that are dead at a throw point no longer conflict.
- The reference analysis uses the same exception entries, ignores unreachable
  blocks, and only requires a reference value for object array stores, since
  `aastore` performs the runtime element check.
- Array element typing follows the opcode family; a tracked component that
  contradicts the opcode (a stale descriptor from another lifetime) no longer
  rejects the method.
- Interfaces with static or default methods are emitted as Java 8 class files
  when every method is branch-free and handler-free (the translator does not
  emit stack maps), and interface method access is validated per version.
- On the Termux APK all 2,696 classes and on the F-Droid APK all 19,701
  classes now translate strictly (100%).

## Unreleased — 2026-11-06

Translate switches, array literals, monitors, and discarding exception handlers.

- Decode switch and fill-array-data payloads as data instead of rejecting the
  code stream. `packed-switch` carries a `first_key` followed by its targets,
  `sparse-switch` carries a key array followed by a target array (sorted on
  output), and
  `fill-array-data` carries its element width and raw bytes.
- Lower packed switches to `tableswitch`, sparse switches to `lookupswitch`,
  and array literals to element stores selected by the array register's flow
  type, so `int[]`, `byte[]`, `float[]` and `double[]` literals keep their
  values.
- Lower `monitor-enter`/`monitor-exit` directly; synchronized bodies and the
  handler that releases the monitor and rethrows are JVM-verified.
- Accept exception handlers that never bind the caught exception. DEX only
  makes the exception available through `move-exception`, so a handler without
  one discards it; the JVM still pushes it, and a `pop`/`goto` trampoline
  discards it before the shared handler block runs.
- Allow unreachable trailing blocks: DEX emits dead code after a terminator,
  and it is emitted without being verified or executed.
- Replace the reference descriptor of allocation aliases at `new-instance` and
  the constructor instead of merging it with an earlier register lifetime.
  This fixed false "not provably assignable" rejections for reused registers.
- Prove assignability for types declared in the application and accept
  relations whose expected type is outside it: the platform hierarchy the
  input was verified against is not in the DEX. Reference arrays are covariant
  in their components; primitive arrays stay invariant.
- Resolve `invoke-super` targets through the application superclass chain
  instead of requiring the direct superclass, and accept any DEX access flags
  on `<clinit>` (the JVM ignores them).
- Leave an unresolved array register untouched during the category fixpoint
  instead of merging an empty reference into it; the empty category used to
  poison later passes and produce spurious join conflicts.
- Walk invoke arguments by register words in the reference analysis, so the
  second half of a long or double argument is not checked as a reference.
- Model exception edges precisely: a handler entry merges the register state at
  the start of every protected instruction, and the backward demand analysis
  injects handler demands at those instructions instead of at the protected
  block exit. Definitions that happen after the last throwing instruction no
  longer leak into the handler.
- Compute block reachability after exception edges are added, and ignore
  unreachable predecessors when joining register categories.
- Read sparse switch payloads as a key array followed by a target array, and
  treat nested reference arrays as covariant in their components.
- Validate each DEX file once per conversion instead of once per method. The
  per-method whole-file check was quadratic: inspecting the F-Droid APK
  (19,701 classes) went from over 40 minutes to 15 seconds, and the Termux APK
  from 135 seconds to 4 seconds.
- On the Termux APK this raises strictly translatable classes to 2628 (97.5%);
  on the F-Droid APK (19,701 classes) to 17,431 (88.5%).

## Unreleased — 2026-11-06

Translate long, float and double register values.

- Add `Long`, `Double` and `Float` register categories. A category-2 value
  reserves two JVM local slots, so the register pair can never collide with an
  unrelated value in the second half.
- Model DEX constants as raw bit patterns of a known width: a 32-bit constant
  can become an int or a float, and a 64-bit constant a long or a double. A
  backward demand analysis over the CFG resolves each constant definition from
  its use sites, so `const-wide` feeding a `double` operation materializes a
  `CONSTANT_Double` even when definition and use sit in different blocks.
- Support `move-wide`, `move-result-wide`, `return-wide`, all `const-wide`
  forms, float/double/long comparisons, every long/float/double arithmetic and
  conversion opcode, wide fields, and wide array elements. Array element
  opcodes are selected from the array register's flow type, which also makes
  `aget`/`aput` on `float[]` emit `FALOAD`/`FASTORE`.
- Add `CONSTANT_Long`, `CONSTANT_Float` and `CONSTANT_Double` to the class
  builder, including the unusable second constant-pool slot a long or double
  occupies, plus `ldc2_w` and wide `ConstantValue` attributes.
- Fix invoke result typing: `move-result*` stores the result with the category
  from the callee prototype and the move-result definition instead of the
  register's pre-invoke category. Previously a reference result stored into a
  fresh register emitted `istore` and failed JVM verification.
- On the Termux APK this raises strictly translatable classes to 1657 (61.5%).

## Unreleased — 2026-11-06

Add DEX try/catch exception-flow translation.

- Validate the DEX try table: protected ranges must end on instruction
  boundaries, handler addresses must be instruction boundaries, handler types
  must be object descriptors, and empty or out-of-range items are rejected with
  precise diagnostics.
- Make every handler entry a basic block and add exception edges from the
  protected blocks, so liveness, definite assignment and value-category
  analysis see the same control flow the JVM will.
- `move-exception` is decoded and modelled as the definition of the caught
  exception (typed `java.lang.Throwable`), and must open a handler block.
- Lowering emits the JVM exception table preserving DEX handler order (typed
  handlers first, then catch-all), stores the dispatched exception at handler
  entry, and labels try boundaries independently of debug metadata.
- Lowering now consumes the IR's single use/define table instead of keeping a
  second copy, which is what made `move-exception` look like a register read.
- On the Termux APK this raises strictly translatable classes to 1433 (53.2%).

## Unreleased — 2026-11-05

Complete the reverse-engineering toolchain for nested archives and real DEX input.

- Fix WAR/EAR export: every `WEB-INF/lib/*.jar` and `EAR/lib/*.jar` layer is
  decompiled instead of being read from the outer ZIP. Structured export
  (`decompile --structured`, `DecompileArchiveWithOptions`) separates
  `sources/`, `resources/`, and byte-exact `archives/`, with per-entry and
  per-nested-archive provenance. The WSO2 portal WAR goes from 0 to 944 source
  files and the Jenkins WAR from 1 to its full nested inventory.
- Add `javajive inspect INPUT --format json`: read-only inventory, multidex
  provenance, deterministic DEX disassembly, per-class strict translation
  verdicts, and precise first-blocker diagnostics. `--disassembly=false`
  keeps large-application reports small without changing verdicts.
- Admit real DEX metadata: decode `debug_info` into `LineNumberTable`,
  `LocalVariableTable`, and parameter names; decode annotation directories and
  emit user annotations, `Signature`, `Exceptions`, `InnerClasses`,
  `EnclosingMethod`, `AnnotationDefault`, and `SourceDebugExtension`; emit
  `SourceFile` and `ConstantValue` for integer and string static fields.
  Malformed debug metadata degrades to "no debug information" instead of
  rejecting the class.
- Gate DEX versions by validated evidence instead of the exact 035 string:
  037/038/039 are accepted only when `map_list` proves the file carries no
  call-site, method-handle, or hidden-API sections, and rejections name the
  exact section or opcode.
- Validate header Adler-32/SHA-1, prototype shorties, `outs_size`, direct/virtual
  membership, and initializer access flags; accept DEX-compliant
  `ACC_CONSTRUCTOR` on `<clinit>` and array method owners such as `[I.clone()`.
- Extend the supported DEX subset with `filled-new-array`,
  `filled-new-array/range`, and boolean/byte/char/short array element access.
- Keep `dex2jar` transactional: unsupported semantics still produce no JAR and
  no substitute methods; the real-world `arsylk-dexfile-tiny.dex` fixture now
  converts end-to-end.

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
  non-empty-stack-boundary without a stable carrier, and uninlined `jsr`/`ret`
  regions remain explicit unsupported boundaries.
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

### Algorithm deepening

- Add a two-pass source typing pass. Pass one collects each VarUid's
  declaration and assigned value types; pass two derives a common supertype
  through the archive hierarchy and retypes only when every structural use
  accepts it. Member/array receivers, ternary arms, lambda captures and opaque
  values veto a retype; solved def-use webs and parameters are untouched.
- Use MethodIR/SSA as production stack-join evidence. A primitive/reference
  contradiction between the SSA phi and the expression-level join type now
  fails the method closed with a structured diagnostic; unsupported SSA inputs
  preserve the existing structural proof.
- Solve shared-handler branch results by common supertype instead of exact
  source-type equality, allowing heterogeneous but assignable results while
  preserving exception-table gaps and terminal-handler proofs.
- Normalize top-tested loop lowering into `while`/`for` forms when the body has
  no unlabeled continue and a declaration does not escape. Labeled loops and
  ambiguous bodies are untouched; `ForStatement` renders its condition and body
  with standard indentation.
- Add a generic method type-variable constraint solver for bare formals:
  repeated occurrences meet at a hierarchy LUB, null contributes no bound, and
  nested invariant occurrences fall back to the strict structural binder.
- Extend the finally proof and its cleanup-binding walker across normalized
  `while` and `for` loops. Loop-form normalization previously changed a
  protected `do-while` after the finally flag proof had been written for that
  only loop form, so a safe finally could remain desugared and duplicate close
  or cleanup effects. The proof now validates condition/init/update coverage
  and loop ownership for all three statement forms.
- Replace the blanket empty-stack gate for irreducible state dispatch with a
  per-word carrier certificate plus a target-merge certificate. Constants and
  materialized locals always cross; an effectful word may cross only when its
  producer reaches the boundary through instructions with no observable effect.
  A target with unequal incoming words must use a materialized stack-phi local.
  Carried constants, static reads, helper calls, reference values and equal
  category-2 constants now recover, while a field read carried across a
  mutating call and unequal literal merges fail closed instead of silently
  changing behavior. The adversarial matrix adds the recovered shapes and six
  fail-closed barriers, including a stale local-slot overwrite and a multi-word
  merge with a differing lower word.
- Add generated loop, SSA-contradiction and generic-constraint regression tests;
  the 38-JAR tree profile remains unchanged on commons-codec, fastjson2, guava
  and spring.

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

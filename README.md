<p align="center">
  <img src="assets/hero-en.png" alt="JavaJive — Pure-Go Java Toolkit: Decompile, Class Parse, Serialization" width="880">
</p>

# JavaJive

[![CI](https://github.com/yaklang/javajive/actions/workflows/ci.yml/badge.svg)](https://github.com/yaklang/javajive/actions/workflows/ci.yml)
[![Pages](https://github.com/yaklang/javajive/actions/workflows/deploy-pages.yml/badge.svg)](https://github.com/yaklang/javajive/actions/workflows/deploy-pages.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/yaklang/javajive.svg)](https://pkg.go.dev/github.com/yaklang/javajive)
[![License: MIT](https://img.shields.io/badge/license-MIT-E23B2E.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md) | [Website](https://yaklang.io/javajive/)

A portable, **pure-Go** Java toolkit — extracted and trimmed from
[yaklang](https://github.com/yaklang/yaklang) — focused on four things:

- **Decompile** — `.class`, Java archives (`.jar` / `.war` / `.ear` / `.aar` /
  `.par` / `.jmod` / `.zip`), and JDK runtime images (`.jimage` /
  `$JAVA_HOME/lib/modules`) → readable Java source, including nested JARs inside
  WAR/EAR/AAR and fat JARs.
- **Translate DEX** — native `.dex` / `.apk` / `.apks` / `.xapk` / `.apkm` /
  `.aab` → deterministic `.jar`, or directly to readable Java source, without
  invoking an external dex2jar tool.
- **Parse classes** — inspect the structure of a `.class` file (constant pool,
  fields, methods, version, access flags).
- **(De)serialize** — parse and re-marshal the Java serialization (ObjectStream)
  wire format with byte fidelity, and convert it to/from JSON.

Built for portability and embedding:

- **Pure Go, single binary** — **no JDK, no cgo, no ANTLR runtime**, no native
  libraries. Cross-compiles to linux / macOS / windows on amd64 and arm64.
- **One import** — a unified `javajive` facade package wraps all four
  capabilities; the sub-packages remain available for advanced use.
- **First-class CLI** — `decompile`, `dex2jar`, `classinfo`, and `serial`
  subcommands, built on the standard library.
- **Cross-tested against a real JDK** — CI compiles real `.class` / `.jar` and
  JDK-serialized blobs with `javac`/`java`, then verifies JavaJive against them
  (see [HARNESS-WORKFLOW.md](HARNESS-WORKFLOW.md)).
- **Trimmed dependency graph** — `utils` / `codec` / `log` / `go-funk` are
  reimplemented as a minimal, self-contained `internal/` core.

## Validation of v0.4.0

The complete **38-JAR differential audit reports zero recorded regressions** against
the fixed baseline: all 20 previously compile-clean JARs are retained and 26 now
compile in full. Baseline defects and 4 existing method stubs remain visible.
See the [per-JAR results and reproduction](docs/historical-jar-audit.md).

**250 isolated semantic round trips** record compilation, JVM verification, stubs
and runtime equality separately; original application classes are absent from rebuilt
classpaths. CI runs the semantic audit on JDK 17 and 21, the full historical comparison,
and the OS/Go and race matrices. These checks do not prove equivalence of every library
method. See the [implementation and remaining boundaries](docs/decompiler-semantic-audit.md).

## Historical benchmarks (v0.3.0)

The following figures were reported for v0.3.0 on 2026-09-05. They are not
current-version acceptance results or proof of behavioral equivalence.

Measured on 34 real-world jars (18,759 flattened units) via decompile → `javac --release 8`
recompile → repackage → JVM verify:

- **100% unit-clean rate** — **18,759 / 18,759** flattened units (`Outer$Inner.java`)
  recompile with **zero `javac` errors**, and **0 syntax errors** across all 34 jars (a CI-enforced
  hard assertion, so no type error can hide behind a lexer failure).
- **All 34 libraries fully round-trip** — decompile → recompile → repackage → external
  JVM `-Xverify:all` per-class verification passes end-to-end (codec is byte-identical to the
  original jar under a call differential). Locked in `provenClean`.
- **14 / 14 self-hosted algorithms** (MD5 · SHA-256 · CRC32 · quicksort · Base64 · HeapSort ·
  KMP · SwitchFSM · TryFinally · UnionFind · DiamondTryCatch · DiamondTryFinally · ForContinue ·
  ComputeIfAbsent) round-trip **byte-for-byte**.
- **#1 in a fair 3-way comparison** on the original 8-jar set — clean-unit rate **100%** vs
  Vineflower 1.10.1 (90.8%) and CFR 0.152 (79.8%); winning all 8 jars against both.

See [BENCHMARK.md](BENCHMARK.md) for the full methodology, per-jar tables and reproduction commands.

### Complex JAR corpus

A separate, CI-gated corpus exercises modern language targets rather than the
historical Java 8 libraries: Java 21 records/sealed types/pattern switches/switch
expressions, control-flow torture, Java 8 enum bodies, a 100-class scale JAR,
multi-release and modular JARs, and the packaged irreducible fixtures. Each case
is decompiled, tree-recompiled, verified with `java -Xverify:all`, and compared
against original runtime output. See
[test/complexjar/REPORT.md](test/complexjar/REPORT.md) for the inventory, fixed
defects and remaining boundaries.

## Install

```bash
# CLI
go install github.com/yaklang/javajive/cmd/javajive@latest

# Library
go get github.com/yaklang/javajive@latest
```

Or build from source:

```bash
git clone https://github.com/yaklang/javajive
cd javajive
go build -o javajive ./cmd/javajive
```

Requires Go 1.22+.

## CLI

```text
javajive <command> [arguments]

Commands:
  decompile   decompile .class files, Java archives, JDK runtime images, or Android containers into Java source
  dex2jar     convert DEX/APK/APKS/XAPK/APKM/AAB input to a deterministic JAR
  jimage2jar  convert a JDK runtime image (lib/modules, .jimage) to a JAR view
  inspect     read-only inventory and best-effort analysis of Java/Android artifacts
  classinfo   print the structure of a .class file (version, fields, methods)
  serial      Java serialization tools (subcommands: tojson, fromjson)
  version     print the version
  help        show help
```

Input is detected by content magic first, archive structure second, and the
filename extension last, so renamed artifacts keep working.

| Input | Handling |
| --- | --- |
| `.class`, directory | one class, or recursive `.class` decompilation |
| `.jar` / `.war` / `.ear` / `.zip` | Java archive; nested `.jar`/`.war`/`.ear`/`.par`/`.jmod`/`.zip` layers are decompiled in place |
| `.aar` | Android library archive; `classes.jar` and `libs/*.jar` are decompiled |
| `.par` | Java archive (extension fallback when the ZIP has no stronger marker) |
| `.jmod` | JDK module image; the four-byte `JM\x01\x00` prefix is stripped automatically |
| `.jimage` / `$JAVA_HOME/lib/modules` | JDK 9+ runtime image; converted to a package-layout JAR view. Uncompressed, zlib `zip`, and `compact-cp` resources are supported |
| `.dex` | raw DEX |
| `.apk` | root `classes.dex`, `classes2.dex`, ... multidex entries |
| `.apks` / `.xapk` / `.apkm` / `.apkx` | nested split APKs; `universal.apk` is preferred when present |
| `.aab` | Android App Bundle; every `<module>/dex/classes*.dex` is translated |

`--structured` separates `sources/`, `resources/`, and byte-exact `archives/`
for Java archives. Android containers always produce a translated source tree.

JImage inputs are converted to a package-layout JAR view: class resources are
exported at their package paths, while `module-info.class` and non-class
resources keep their module prefix. Uncompressed, zlib-compressed (`zip`), and
`compact-cp` (constant-pool string sharing) resources are supported, including
big-endian images. A full JDK image contains tens of thousands of classes, so
`inspect` or `JImageToJar` is a good first step.

### inspect

```bash
javajive inspect app.apk --format json
javajive inspect app.apks --format json
javajive inspect app.aab --format json
javajive inspect app.war --format json
javajive inspect java.base.jmod --format json
javajive inspect --format json /path/to/jdk/lib/modules

# Large applications: omit per-instruction disassembly and raw code units.
javajive inspect --disassembly=false --format json app.apk
```

`inspect` never translates input or writes a JAR. It reports the artifact hash
and format, a sorted archive inventory, canonical multidex provenance (including
nested APK and AAB module DEX entries), per-class and per-method inventory,
deterministic DEX disassembly, a per-class strict translation verdict, and
precise diagnostics for the first unsupported feature of each method. This keeps
real applications analyzable even when the strict translator cannot yet convert
them.

### decompile

```bash
# Single class: prints to stdout by default, or -o to write a file.
javajive decompile Foo.class
javajive decompile -o Foo.java Foo.class

# Archive: defaults to a "<input>.src" directory, or -o to choose one.
javajive decompile app.jar
javajive decompile -o ./app-src app.war

# Structured archive export: sources/, resources/, and preserved nested
# archives/ (WEB-INF/lib, EAR/lib, AAR classes.jar and fat-JAR libraries
# are decompiled in place).
javajive decompile --structured -o ./app-src app.war

# AAR and JMOD are ordinary archives once the container is identified.
javajive decompile -o ./aar-src library.aar
javajive decompile -o ./jmod-src java.base.jmod

# JDK runtime image: classes are exported at package paths; module-info.class
# and non-class resources keep their module prefix.
javajive decompile -o ./jdk-src /path/to/jdk/lib/modules

# DEX or Android container: run the native DEX-to-JAR pipeline and decompile
# the result. The default output directory is "<input>.src".
javajive decompile classes.dex
javajive decompile -o ./app-src app.apk
javajive decompile -o ./app-src app.apks
javajive decompile -o ./app-src app.xapk
javajive decompile -o ./app-src app.aab

# Directory: recursively decompile .class files (requires -o output dir).
javajive decompile -o ./src ./classes

# Very large generated methods: opt into a higher per-method analysis budget.
# Values above 1,000,000 also scale graph/scan caps.
javajive decompile -analysis-budget 2000000 Big.class
javajive decompile -analysis-budget 2000000 -o ./app-src app.jar
```

### dex2jar

```bash
javajive dex2jar -o classes.jar classes.dex
javajive dex2jar -o app.jar app.apk
javajive dex2jar -o app.jar app.apks
javajive dex2jar -o app.jar app.aab
```

### jimage2jar

```bash
# Convert a JDK 9+ runtime image to a package-layout JAR view without
# decompiling every class.
javajive jimage2jar -o jdk.jar /path/to/jdk/lib/modules
```

Input is detected by magic rather than the filename extension. Output is a
reproducible JAR. `dex2jar` fails the complete conversion on any unsupported or
malformed class or method; it writes a same-directory temporary file before
replacing the destination and never substitutes method stubs.

### Native DEX/APK support boundary

Native DEX translation is a **strict vertical slice**, not a general Android
application converter. The tested subset covers DEX 035, 037, 038, and 039
inputs whose `map_list` proves they carry no call-site, method-handle, or
hidden-API sections; annotations, debug metadata, and `SourceFile`; non-default
integer/string static constants; ordinary integer/reference control flow; fields
and invokes on object and array owners; safely paired `new-instance`/constructor
initialization; int, reference, boolean, byte, char, and short arrays including
`filled-new-array` and `fill-array-data` literals; `packed-switch` and
`sparse-switch`; `monitor-enter`/`monitor-exit`; register lifetime reuse within
one method; try/catch with `move-exception`, typed handlers, catch-all handlers,
and handlers that discard the caught exception; unreachable dead code; long,
float, and double values, including category-2 register pairs, wide constants,
conversions, comparisons, wide fields, and wide array elements; and interfaces
with static or default methods, emitted as Java 8 class files when their methods
are branch-free and handler-free. Generated class files are parser-tested and
JVM-verified, including executable branch, switch, field, invoke, constructor,
array, numeric, monitor, exception, and interface paths.
Assignability between reference types is proven through the application
hierarchy declared in the same DEX; a relation whose expected type is outside
the application is accepted, because the platform hierarchy the input was
verified against is not part of the DEX.

Still unsupported: `filled-new-array` of wide or floating elements; DEX 038+
dynamic invokes and method constants; CompactDex; and APK resources or native
libraries.

Android container handling is structural:

- `.apk` input translates only canonically named `classes.dex`,
  `classes2.dex`, ... entries at the archive root.
- `.apks` / `.xapk` / `.apkm` / `.apkx` input walks nested `.apk` entries. A
  `universal.apk` is preferred when present; resource-only split APKs are
  skipped. Identical classes repeated across splits are collapsed, while
  conflicting definitions fail the conversion.
- `.aab` input translates every `<module>/dex/classes*.dex` entry across
  `base` and feature modules.

All selected DEX files must succeed transactionally or no JAR is produced; use
`inspect` to analyze inputs that cross the strict boundary.

Container bounds default to 256 MB per DEX, 512 MB per archive/APK, 512 MB of
total DEX data, 128 DEX files, 512 nested APKs, and 2 GB of total nested APK
data. `DexOptions` fields raise these limits for larger bundles.

The container test suite is backed by real samples under
[`testdata/external`](testdata/external/README.md): Tomcat WARs, bundletool
APK/AAB/APKS bundles (including a 181-APK split bundle), an AndroidX AAR,
Androguard/Apktool/dex2jar fixtures, and synthetic XAPK/APKM/EAR containers.
Fuzz seed corpora cover the classifier, JImage and compact-cp readers,
`inspect`, and Android container parsing.

### classinfo

```bash
javajive classinfo Foo.class
```

```text
class:      InvisibleAnnoSeed
super:      java/lang/Object
version:    61.0
access:     public
constants:  18

fields (0):

methods (2):
   <init>()V
   run()I
```

### serial

```bash
# Serialized binary -> JSON. (-hex: input is a hex string, -: read from stdin)
javajive serial tojson dump.bin
printf 'aced000574000568656c6c6f' | javajive serial tojson -hex -

# JSON -> serialized binary. (default prints hex; with -o writes raw bytes)
javajive serial fromjson dump.json -o out.bin
javajive serial fromjson dump.json          # prints hex
```

## Library

Use the unified facade package — one import covers all four capabilities:

```go
import (
    "os"

    "github.com/yaklang/javajive"
)

// Translate DEX/APK/APKS/XAPK/APKM/AAB bytes in memory.
jarBytes, report, err := javajive.DexToJar(dexOrContainerBytes, javajive.DexOptions{})
if err == nil {
    err = os.WriteFile("app.jar", jarBytes, 0o644)
    _ = report.Classes
}

// Translate a file and atomically write the JAR.
report, err = javajive.DexFileToJar("app.apk", "app.jar", javajive.DexOptions{})

// Translate through a private temporary JAR and decompile directly to source.
err = javajive.DecompileDEXFile("classes.dex", "app-src", javajive.DexOptions{})

// Decompile a single class, a Java archive, or a JDK runtime image.
src, err := javajive.Decompile(classBytes)
err = javajive.DecompileArchive("app.jar", "app-src")
err = javajive.DecompileArchive("/path/to/jdk/lib/modules", "jdk-src")

// Convert a JDK runtime image to a JAR view, or read one resource directly.
err = javajive.JImageToJar("/path/to/jdk/lib/modules", "jdk.jar")
image, err := javajive.OpenJImageFile("/path/to/jdk/lib/modules")
if err == nil {
    objectClass, readErr := image.Read("java.base/java/lang/Object.class")
    _ = objectClass
    _ = readErr
    _ = image.Close()
}

// Inspect class structure.
obj, err := javajive.ParseClass(classBytes)
_ = obj.GetClassName()

// Java serialization: binary -> JSON -> binary.
objs, _ := javajive.ParseSerialized(raw)          // or ParseSerializedHex(hexStr)
jsonBytes, _ := javajive.SerializedToJSON(objs...)
restored, _ := javajive.SerializedFromJSON(jsonBytes)
out := javajive.MarshalSerialized(restored...)
```

### Unified API

| Function | Purpose |
|---|---|
| `Decompile(classBytes) (string, error)` | Decompile one `.class`'s bytes |
| `DecompileFile(path) (string, error)` | Decompile one `.class` from disk |
| `DecompileWithResolver(classBytes, resolve)` | Decompile with a class-bytes resolver |
| `DecompileArchive(src, dst) error` | Decompile a Java archive or JDK runtime image into a directory |
| `DecompileArchiveWithAnalysisBudget(src, dst, budget) error` | Archive/JImage decompilation with an explicit per-method analysis budget |
| `JImageToJar(src, dst) error` | Convert a JDK runtime image to a deterministic JAR view |
| `OpenJImageFile(path) (*JImageReader, error)` | Open a JDK runtime image for direct resource access |
| `ClassifyInput(data, name) ContainerKind` | Classify bytes by magic, archive structure, then extension |
| `ClassifyInputFile(path) (ContainerKind, error)` | Classify a file without loading the whole archive |
| `DexToJar(input, options) ([]byte, DexReport, error)` | Convert DEX/APK/APKS/XAPK/APKM/AAB bytes to a deterministic JAR |
| `DexFileToJar(src, dst, options) (DexReport, error)` | Convert a DEX/Android container file using a temporary JAR |
| `DecompileDEXFile(src, dst, options) error` | Convert DEX/Android container through a private temporary JAR and decompile it |
| `DecompileDEXFileWithAnalysisBudget(src, dst, options, budget) error` | DEX/Android container conversion plus budgeted archive decompilation |
| `ParseClass(classBytes) (*ClassObject, error)` | Parse one `.class`'s bytes |
| `ParseClassFile(path) (*ClassObject, error)` | Parse one `.class` from disk |
| `ParseSerialized(raw) ([]JavaSerializable, error)` | Parse a serialization stream |
| `ParseSerializedHex(hexStr) ([]JavaSerializable, error)` | Parse a hex-encoded stream |
| `MarshalSerialized(objs...) []byte` | Re-encode objects to the wire format |
| `MarshalSerializedHex(objs...) string` | Re-encode to hex |
| `SerializedToJSON(objs...) ([]byte, error)` | Convert objects to JSON |
| `SerializedFromJSON(raw) ([]JavaSerializable, error)` | Rebuild objects from JSON |

The sub-packages are also exported for advanced use:
`classparser`, `classparser/jarwar`, `dex2jar`, `serialization`.

## Differences from upstream yaklang

To stay portable and small, JavaJive makes a few deliberate trade-offs versus
yaklang. See [MIGRATE.md](MIGRATE.md) for the full mapping and migration guide.

| Area | yaklang (upstream) | JavaJive |
|---|---|---|
| Decompiler ANTLR safety net | re-validates dumped source via an ANTLR Java grammar; degrades members to stubs on failure | removed (heavy dependency); validation is a no-op, output is emitted directly |
| Support layer (`utils` / `codec` / `log` / `go-funk`) | shared monorepo packages | minimal self-contained re-implementations under `internal/` |
| `yso` gadget generator | included | not included |
| String literal charset recovery (`MatchMIMEType`) | optional GBK/GB18030 recovery | stubbed (no-op); behaviour unchanged for the vast majority of cases |

Third-party dependencies are confined to a small set of pure-Go libraries
(`gobwas/glob`, `go-viper/mapstructure`, `samber/lo`, `tidwall/gjson`,
`segmentio/ksuid`, `yeka/zip`, and a few `golang.org/x/*`).

## Testing

```bash
go test ./...                 # unit tests + JDK cross-tests (skipped if no JDK)
go test ./... -race           # data-race free (linux)
go test ./test/cross/ -v      # Java cross-tests only (needs javac/java on PATH)
```

The JDK cross-tests compile real Java artifacts at test time and verify them
against JavaJive; they `t.Skip` automatically when no JDK is present. The
[irreducible CFG corpus](test/cross/testdata/irreducible/README.md) additionally
pins generated classfiles, exact CFG topology, reviewed equivalent Java, rebuilt
JVM verification, and runtime equivalence. JavaJive emits deterministic state
machines for supported normal-edge, non-constructor regions; it does not claim
to recover the original source or identical bytecode. See
[HARNESS-WORKFLOW.md](HARNESS-WORKFLOW.md)
for how the harness and CI work.

## Project layout

```text
javajive.go      unified facade package (import "github.com/yaklang/javajive")
serialization/   Java serialization/deserialization (from yaklang common/yserx)
classparser/     class parser + decompiler (from yaklang common/javaclassparser)
cmd/javajive/    CLI entry point
internal/        trimmed self-contained support layer (log / codec / funk / utils / filesys / ...)
test/cross/      JDK-backed cross-tests (javac/java)
site/            static landing page deployed to GitHub Pages
```

## License

[MIT](LICENSE) © 2026 VillanCh. JavaJive is derived from
[yaklang](https://github.com/yaklang/yaklang).

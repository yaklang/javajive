# Bounded JDK invocation metadata

The generated catalog contains complete declared method and constructor tables
and complete superclass/interface closure for selected JDK types. It retains
private, generic, bridge, static, and varargs flags. Schema 2 also retains exact
Exceptions attributes, including known-empty declarations, indexed by the original
name and descriptor. It is not a whitelist of
method names presumed unique. For example, Map.get/merge remain generic, whereas
String.valueOf remains a competing overload family.

Roots include Object, String, Map, Record where available, and the standard
collection, stream, functional, I/O, channel, reflection, concurrency and TLS
extension APIs listed in the generator, including XML, JavaBeans, naming and SQL
namespaces. On modular JDKs the generator indexes the original JMOD classfiles
across the supplied image, rejects duplicate class identities, and records each
declaration's module archive and SHA256. Class and
method Signature attributes are extracted verbatim from the same classfiles
and indexed by exact descriptor, preserving first-bound erasure and overload
identity for generic binding proofs. Reference types in root
method descriptors are included for source denotability, together with their full
ancestors. Other classes remain unavailable; no open-world completeness is claimed.
No method execution, reflection, or host JDK lookup occurs during decompilation.

## Exact source-target profiles

The fallback selects only an exact `TargetSourceVersion` of 8, 11, 17, or 21.
When that option is zero, the existing class-major-to-source-version mapping
selects the profile. Other target levels receive no new fallback. Explicit
resolver-supplied bytes are preferred; invalid supplied bytes do not silently
fall back. The pre-existing special handling of Object outside catalog profiles
is retained unchanged.

This is a fixed standard-JDK platform assumption for those source targets, not a
claim to model patched/custom java.base definitions or all future JDK updates.
Callers targeting a different platform must supply actual declaration bytes or
accept an unsupported result. Parent and method completeness flags cover the
actual extracted class declaration table for each specific snapshot below.

The generator reads classfile bytes, not javap prose or incomplete ct.sym API
stubs. Each profile records the JDK release metadata, archive SHA256, and individual
classfile SHA256 and major version. Constructors are included for exact checked-exception evidence; they still do not
participate in ordinary virtual or interface overload families. Class initializers
are omitted. The absence of an Exceptions attribute is a known empty declaration,
not unavailable metadata. Returned exception slices are copied for each request.

Snapshots used:

- JDK8: Amazon Corretto 8.422.05.1, locally installed rt.jar.
- JDK11: locally installed Homebrew OpenJDK; exact release/build metadata is in JSON.
- JDK17: Eclipse Temurin 17.0.18+8 JMOD archives.
- JDK21: locally installed Homebrew OpenJDK 21.0.2 JMOD archives.

The JDK17 archive was downloaded from the official
[Temurin release](https://github.com/adoptium/temurin17-binaries/releases/tag/jdk-17.0.18%2B8)
and checked against its published SHA256:
`d81de06d938384fe76c4aa3c13395933aa11e2d19b0428743f810db06b05e312`.
Only JMOD archives and the release file were extracted; downloaded programs were
not executed. The expanded extraction verifies that the original java.base.jmod
and release file remain identical. JMODs are only read as ZIPs of classfile data.

Regenerate with reviewed JDK installations:

```sh
python3 classparser/jdk_metadata_catalog_generate.py \
  --jdk8 /path/to/jdk8 --jdk11 /path/to/jdk11 \
  --jdk17 /path/to/jdk17 --jdk21 /path/to/jdk21 \
  --out classparser/jdk_metadata_catalog.json
```

## Evidence and limits

Before this change, the unchanged `TestT22_C06_noRegression` rejected LongTest
with status unsupported because Map's external overload family was unknown.
LongTest has class major 55, requiring the Java11 profile. With complete Map
metadata, its generic recovery path receives positive uniqueness evidence and the
same test passes without changing its assertions.

New tests validate profile closure and provenance, descriptor validity, Map's
actual generic families, competing String overloads, public String/CharSequence
metadata, profile boundaries (Map.of, String.indent, Record), missing-type/unknown-
release refusal, request isolation, source-profile selection, and resolver
precedence. Existing invocation-binding regressions remain applicable.

Record's ancestor metadata and Class.isRecord metadata are complete in profiles
17/21. The existing record test still exposed a separate expression-layer class-
literal receiver typing problem in the unintegrated branch; that layer is owned
by the parent/lowering task, not modified here. This catalog alone is not a full
Java overload-resolution proof or a claim that all native tests pass.

Local validation used `CGO_ENABLED=1 GOTOOLCHAIN=local
GOFLAGS=-ldflags=-linkmode=external` on macOS:

- Catalog tests, unchanged T22_C06, existing virtual/interface round-trip test,
  and missing-ancestor regression: pass.
- Catalog tests with race detector: pass (macOS linker LC_DYSYMTAB warning only).
- Independent local cross-check decoded all 125 source classfiles with the
  repository's Go classfile parser and compared every class identity, access flag,
  parent, and non-constructor method name/descriptor/flag against the Python-
  generated tables: zero differences. This check read trusted JDK bytes only;
  its machine-specific temporary test is not committed as a CI dependency.
- `git diff --check`: pass.

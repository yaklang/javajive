# Harness & Workflow

How JavaJive is verified. This document explains the **JDK-backed cross-test
harness** (`test/cross/`) and the **GitHub Actions workflows** (`ci.yml` and
`deploy-pages.yml`) so you can run, extend, and reason about them.

> TL;DR — JavaJive is a pure-Go re-implementation of Java tooling, so the
> strongest possible test is differential: let a **real JDK** produce the
> ground-truth artifacts (`.class`, `.jar`, serialized blobs) and assert that
> JavaJive agrees. That is exactly what `test/cross/` does, on every CI run,
> across linux / macOS / windows.

---

## 1. The cross-test harness (`test/cross/`)

The package compiles Java with the local JDK at test time, then feeds the output
to JavaJive's public API and asserts the result. If no JDK is on `PATH`, every
cross-test **skips itself** (`t.Skip`) so `go test ./...` stays green on machines
without Java.

```
test/cross/
  doc.go              package doc (keeps the test-only package buildable)
  harness_test.go     helpers: locate JDK, compile, run, zip, read
  decompile_test.go   decompile + class-parse cross-tests
  serial_test.go      serialization cross-tests
```

### Harness helpers (`harness_test.go`)

| Helper | What it does |
|---|---|
| `lookJavac(t)` / `lookJava(t)` | `exec.LookPath`; `t.Skip` when the JDK is absent |
| `writeSources(t, dir, map)` | write `name → source` files into a temp dir |
| `compileJava(t, dir, map)` | write sources and run `javac -encoding UTF-8 -d dir *.java` |
| `runJava(t, dir, class, …)` | run `java -cp dir <class> args…`, return combined output |
| `zipClassesToJar(t, dir, jar)` | pack every `.class` under `dir` into a `.jar` (a plain zip, package layout preserved) using Go's `archive/zip` — no external `jar` tool needed |
| `readClass(t, dir, name)` | read a compiled `.class` from the temp dir |

Everything runs in `t.TempDir()`, so artifacts are isolated per test and cleaned
up automatically.

### Decompile / parse cross-tests (`decompile_test.go`)

- **`TestCrossDecompileSingleClass`** — compiles a non-trivial `Sample.java`
  (fields, a constructor, branching, a loop) with `javac`, decompiles
  `Sample.class` with `javajive.Decompile`, and asserts the recovered source
  contains the expected class/method tokens.
- **`TestCrossParseClass`** — parses the same `Sample.class` with
  `javajive.ParseClass` and asserts the class name, super class
  (`java/lang/Object`), and method count agree with what `javac` produced.
- **`TestCrossDecompileJar`** — compiles two classes, packs them into a `.jar`
  with `zipClassesToJar`, runs `javajive.DecompileArchive`, and asserts a
  `.java` file is produced for each class. (This test also guards the Windows
  file-handle lifecycle: `DecompileArchive` must close the archive so the temp
  dir can be removed.)

### Serialization cross-tests (`serial_test.go`)

A small `SerGen.java` is compiled and **executed** to serialize objects via
`ObjectOutputStream` into a file — i.e. the JDK writes the canonical bytes.

- **`TestCrossSerialStringRoundTrip`** — for a `String`, asserts an **exact byte
  round-trip**: `ParseSerialized` → `MarshalSerialized` reproduces the JDK's
  bytes verbatim, and the value survives a JSON round-trip.
- **`TestCrossSerialComplexObjects`** — for `ArrayList` and `HashMap`, asserts the
  stream parses, the JSON contains the expected class name, and the structure is
  **stable** across a marshal → parse round-trip.

### Run it locally

```bash
# All tests; cross-tests skip automatically if no JDK is found.
go test ./...

# Cross-tests only, verbose (requires javac/java on PATH).
go test ./test/cross/ -v

# Check your JDK.
javac -version && java -version
```

### Add a new cross-test

1. Put Java source in a `const … = ` string (default package keeps paths simple).
2. `compileJava(t, dir, map[string]string{"X.java": xSrc})`.
3. Read the artifact (`readClass`, or run with `runJava`, or `zipClassesToJar`).
4. Drive it through the public `javajive` API and assert.

---

## 2. CI workflow (`.github/workflows/ci.yml`)

Triggered on push / PR to `main` and via `workflow_dispatch`. Three jobs:

### `lint`

- `gofmt -l .` must be empty (formatting gate).
- `go vet ./...`.
- `go mod tidy` must produce **no diff** in `go.mod` / `go.sum`.

### `test` (matrix)

`{ ubuntu-22.04, macos-latest, windows-latest } × Go { 1.22, 1.23 }` — six
combinations, `fail-fast: false`.

- Sets up Go and **Temurin JDK 21** (so the cross-tests actually run on every OS,
  rather than skipping).
- `go build ./...`, then `go test ./... -count=1`.
- **Linux** additionally runs `go test ./... -race`.
- **macOS** runs the test step with the **external linker**
  (`CGO_ENABLED=1 -ldflags=-linkmode=external`). Go 1.22's internal linker omits
  `LC_UUID` on `darwin/arm64`, which modern macOS dyld rejects
  (`missing LC_UUID load command` → abort trap); the system linker writes it.

### `cross-build` (matrix)

Cross-compiles the CLI with `CGO_ENABLED=0` for
`linux|darwin|windows × amd64|arm64`, proving the module is pure Go and
portable, and uploads each binary as a build artifact.

> The whole module is `GOTOOLCHAIN: local`, pinned to the `go` directive in
> `go.mod`, so CI never silently downloads a newer toolchain.

---

## 3. Pages workflow (`.github/workflows/deploy-pages.yml`)

Publishes the static landing page in `site/` to GitHub Pages.

- Triggered on push to `main` that touches `site/**` (or the workflow), and via
  `workflow_dispatch`.
- **build** job: sanity-checks that the required `site/` files exist, then
  uploads `site/` with `actions/upload-pages-artifact@v3`.
- **deploy** job: publishes with `actions/deploy-pages@v4` to the
  `github-pages` environment.
- Permissions: `pages: write`, `id-token: write`; concurrency group `pages`.

The site reuses the visual language of
[yaklang/hack-skills](https://github.com/yaklang/hack-skills) (dual light/dark
theme, ambient dot-grid canvas) recolored with a **Java-red** accent. It is a
single static page — no build step, no data fetch.

> One-time setup: the repository's **Settings → Pages → Source** must be set to
> **GitHub Actions** for `deploy-pages` to succeed.

# JVM irreducible-CFG corpus

This directory contains eight deterministic Java 5 (classfile major 49) fixtures for control-flow reconstruction tests. The checked-in `original/*.class` files are the originals: they are assembled directly with `classparser/classfilebuilder`, not compiled from Java, so they can contain irreducible graphs that Java source cannot express directly.

Every class exposes:

```java
public static int run(int entry, int budget)
public static void main(String[] args)
```

`main` prints one integer per line for the six vectors recorded in `manifest.json`. Every cycle decrements `budget`, so every input terminates; non-positive budgets return without entering a loop.

## Contents

- `generate/main.go` is the pure-Go source of truth for original classfiles, `manifest.json`, and `SHA256SUMS`.
- `original/*.class` are checked-in major-49 originals.
- `equivalent/*.java` are structured, human-reviewed semantic equivalents. They preserve `run` and `main` results but intentionally do not claim byte-for-byte or CFG equivalence.
- `bytecode/*.md` documents each original graph with a Mermaid diagram, exact block PCs, typed edges, SCC entries, and symbolic bytecode.
- `manifest.json` records graph classification, exact control-flow metadata, deterministic outputs, sizes, and original hashes.
- `SHA256SUMS` covers all checked-in classfiles, equivalent Java, bytecode docs, `README.md`, and `manifest.json` (not itself or generator source).

## Case matrix

| Case | Classification | Purpose |
| --- | --- | --- |
| `NaturalSingleEntry` | reducible | Baseline natural loop with one entry and latch. |
| `NaturalMultipleLatch` | reducible | Baseline natural loop with two latches. |
| `DeadSecondaryEntry` | reachable-reducible | Unreachable bytecode adds a syntactic second edge into a natural loop. |
| `IrreducibleTwoEntry` | irreducible | Two-node SCC with independent entries at A and B. |
| `IrreducibleMidBodyPhi` | irreducible | Secondary entry reaches a body-local value merge. |
| `IrreducibleSwitchThreeEntry` | irreducible | `tableswitch` independently enters all nodes of a three-node SCC. |
| `IrreducibleMultiExitEffects` | irreducible | Two-entry SCC with effectful paths and three exits. |
| `RegionalIrreducibility` | regionally irreducible | Reducible outer loop containing a two-entry A/B region. |

## Regeneration

From the repository root:

```sh
go run ./test/cross/testdata/irreducible/generate
(cd test/cross/testdata/irreducible && shasum -a 256 -c SHA256SUMS)
```

The generator derives its output directory from its own source path, so it can also be invoked from another working directory. It writes files only after all eight classes have assembled successfully. Run it twice and compare the resulting hashes to check reproducibility.

The switch fixture uses `Code.TableSwitch`, including JVM alignment padding and relative offsets. The corpus therefore depends on the `classfilebuilder` switch API tested in `classparser/classfilebuilder/builder_test.go`.

## Independent review recipe

The equivalent Java files must be compiled away from this directory so their compiler-produced classes cannot replace the originals:

```sh
review_dir=$(mktemp -d)
javac --release 8 -d "$review_dir" test/cross/testdata/irreducible/equivalent/*.java
for class in NaturalSingleEntry NaturalMultipleLatch DeadSecondaryEntry \
  IrreducibleTwoEntry IrreducibleMidBodyPhi IrreducibleSwitchThreeEntry \
  IrreducibleMultiExitEffects RegionalIrreducibility; do
  java -Xverify:all -cp test/cross/testdata/irreducible/original "$class"
  java -Xverify:all -cp "$review_dir" "$class"
done
```

Use `javap -v -c` on the `original/` classfiles to inspect the exact major version, offsets, instructions, and branch targets. `manifest.json` is the machine-readable output oracle.

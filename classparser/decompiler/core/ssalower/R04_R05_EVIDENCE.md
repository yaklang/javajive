# R04 / R05 production lowering evidence

Baseline: `7668795a91f3c1df94ba5595e1bac5c9477e99a0`.
Dependency commits tested: R02 `6858293` (local cherry-pick `3421a5e`),
R03/R11 `4bde926` (local cherry-pick `d324d08`).

## Before

Baseline `go test ./classparser/decompiler/core/ssalower -count=1` exited 0
(`ok .../ssalower 0.346s`), but did not test structural identity collisions or
executable exception-phi transfers. The old identity formula maps
OriginInstr(PC=10, Slot=100) and OriginInstr(PC=11, Slot=0) to the same ID;
Aux is omitted entirely. Exception edges returned an empty move record.

## Changes and invariants

- Method-local structural keys include Kind, PC, Slot, and Aux. All logical
  origins and phi definitions are collected before assigning deterministic IDs;
  phi-use keys alias the exact definition. Unregistered keys and cycles fail.
- Temps reserve the full registry namespace and preserve logical type, width,
  origin, and saved source identity. Category-2 tails are not copied separately.
- Checked deterministic parallel-copy scheduling rejects nonpositive IDs,
  duplicate destinations (including self copies), and reused/overflowed temps.
  Critical-edge moves have exactly one owner. Synthetic entry phi operands
  become EntryMoves.
- Exceptional phi locals become pre-throw local snapshots, with original
  coverage order. Catch objects are separate catch-parameter bindings. Unsafe
  origins, missing/duplicate operands, uninitialized values, conversions, mixed
  normal/handler entry, and exhausted spill budgets return nil plus an error.
  The input SSA and IR are not mutated.
- Existing T14 assertions are retained. Their IDs now resolve through a method
  registry. The exception fixture now explicitly describes its synthetic call
  as ()V and returns an int on its normal ()I path.

## Verification

All commands used `CGO_ENABLED=1 GOTOOLCHAIN=local
GOFLAGS=-ldflags=-linkmode=external`.

1. `go test ./classparser/decompiler/core/ssalower ./classparser/decompiler/core -count=1`
   exited 0: ssalower 0.661s, core 0.469s.
2. `go test -race ./classparser/decompiler/core/ssalower -count=1`
   exited 0: ssalower 2.344s. The macOS external linker printed an
   LC_DYSYMTAB warning; there was no race report.
3. `git diff --check` exited 0.

Production tests include 5,000 fixed-seed randomized copy groups checked against
an independent old-state oracle, reversed-input schedule equality, fresh-temp
checks, identity collisions and alias cycles, entry-phi def/use identity,
critical-edge ownership, and exception rejection/budget cases. An independent
small bytecode evaluator executes the real two-IDIV MethodIR fixture with and
without the generated spill plan: f(0,1)=7 with one division, f(1,0)=11 with two,
and f(1,1)=11 with two. A shared-handler test confirms same-site spill dedup and
exception-table order retention.

## Scope and rollback

This is the ssalower module's complete emission plan, not integration into the
legacy Java source printer. Consumers must execute EntryMoves, edge/split moves,
Exception.Sites before the throwing instruction, and Exception.Catch from catch
parameters. The supported exceptional subset uses already-typed local reads;
reference widening requiring an additional proof is conservatively rejected.
No unknown JAR was executed; this evidence is not a universal JVM equivalence
proof. Revert the single R04/R05 commit to roll back these changes; keep the
independent R02/R03/R11 commits.

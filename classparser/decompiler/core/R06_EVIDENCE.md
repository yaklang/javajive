# R06 expression evaluation evidence

Tested above R01 `5f85ebe` (local cherry-pick `ffe6e16`) and R02/R03/R11/R04/R05.

## Production changes

- The existing value dependency visitor now follows rendered JavaRef StackVar /
  CustomValue children, explicit lambda intersections, compound writes, and
  known concat operands. Unknown CustomValue remains an opaque barrier.
- Access summaries distinguish stable local identities from printed names, and
  record local reads/writes separately from heap/call/throw effects. CanSwap
  rejects RAW/WAR/WAW dependencies, observable effects, and handler-order changes.
- The active printer's single-use and chained-assignment folds now prove their
  complete linear motion before mutating the graph. Missing origin, ambiguous
  paths, cycles, effect barriers, and handler changes preserve the temporary.
  Traversal charges the shared request work budget; exhausted work is returned.
  No detached scheduler is claimed as an active production pass.
- Actual invokedynamic handling materializes dynamic arguments in JVM operand
  order, then materializes the reconstructed concat/lambda result at its original
  PC. These temps cannot fold back into mutable sources. Primitive declarations
  use the invocation descriptor; source wrappers stay on assignment RHSs for the
  existing reaching-definition repair. The snapshot allocation budget is checked
  before mutation. Several assignments at one PC retain stable emission order.
- Lambda marker intersections are explicit before result materialization;
  malformed markers fail, nonzero explicit bridge lists remain unsupported.
  Constructors keep safe String/primitive concat operands inside this()/super()
  because Java forbids preceding temp declarations. If an unsnapshotted operand
  may invoke object conversion, the adapter returns unsupported rather than a
  conversion/evaluation-interleaved + expression.

## Independent checks

- 1,000 fixed-seed local read/write cases compare every approved swap with an
  independent state interpreter; same spelling with distinct IDs is a positive
  case, aliases with the same VarUid are a negative case.
- Production snapshot/dispatch tests require all operand assignments before the
  concat expression, exactly one reference to each effectful dynamic operand,
  preserved OriginPC, immutable capture identity, and transactional budget failure.
- The Java 17 round-trip fixture compares javac/JVM behavior in precision and
  compatibility modes: an already-present String.valueOf conversion throws before
  a later operand call (trace EC, call count 0), lambda capture remains 7 after
  later mutation, local concat reads print 78 around ++y, and effectful String
  operands inside super(...) retain left-to-right output LRLR.
- Existing T15 covers a[i++]=f(), short circuit, OOB before call, partial array
  initialization, class initialization, volatile fields, and monitor release.
  T18 covers throwing toString and actual recipe operands. T19 covers captures,
  slot reuse, method references, markers, and unsupported serializable lambdas.

## Commands and outcomes

All used `CGO_ENABLED=1 GOTOOLCHAIN=local GOFLAGS=-ldflags=-linkmode=external`.

- `go test ./classparser/decompiler/core ./classparser/decompiler/core/values -count=1`
  exit 0; latest full package results core 0.918s, values 1.109s.
- `go test ./classparser -run 'Test(T15_|TaskT18|TaskT19|R06|BindingPlanVirtualAndInterfaceDispatch)' -count=1`
  exit 0; 29.472s.
- `go test -race ./classparser/decompiler/core ./classparser/decompiler/core/values -count=1`
  exit 0; core 1.614s, values 2.360s. macOS external linker emitted LC_DYSYMTAB
  warnings; there were no race reports.
- After final adapter boundary checks, targeted core/values/classparser tests for
  snapshot/capture/budget/direct concat/markers/T18/R06 again exited 0; classparser
  11.517s. `git diff --check` exited 0.

The initial integrated snapshot experiment exposed unstable ordering of same-PC
assignments and lost marker-interface identity. Both were corrected before the
successful runs above; no existing expected outputs were weakened or skipped.
Only generated, reviewed tiny Java fixtures were executed, never an unknown JAR.

## Scope

The legacy printer uses its stable JavaRef identities and materialized snapshots;
this does not claim that its complete expression pipeline has migrated to SSA.
Motion is intentionally conservative and may retain more locals. Arbitrary
bootstrap recipes, nonzero bridges, and serializable lambda reconstruction are
not claimed supported. Revert the single R06 commit to roll back this change.

# R09 request-budget integration

Depends on R03/R11 `4bde926`, R02 `0864d13` and R01 `5f85ebe`.
The latter two are identical local cherry-picks `6858293` and `1882ce5`.

A clean detached `1882ce5` baseline failed the pre-canceled malformed-input
regression: `DecompileWithOptions([]byte{0}, canceledContext)` returned
`invalid_input` / truncated magic instead of `canceled`. After this change,
request cancellation and input admission run before SHA-256, parser construction
and constant-pool allocation. An admitted request retains its input hash;
rejected/canceled admission deliberately leaves it empty.

Production changes reuse the existing bounded ClassReader and attribute
Subreader. Input/read/parser-item counters share the original request Budget;
attribute children do not recharge input, and resolver metadata parsing and
folded child dumps carry the same budget. An explicit options Context is checked
even with externally supplied Work. Table minimum sizes and logical allocation
work are checked before allocation; annotation-element array recursion now uses
the existing nesting guard. Budget failures remain sticky and clear returned
source, including failures raised by final diagnostic emission.

ChargeMany commits counter deltas and aggregate request work atomically, with
sorted counter failure order and checked accumulation. Zero Limits fields keep
the existing unlimited meaning. Local SSA counter overflow and final phi/value
binding charges are covered; final phi enumeration uses the indexed incoming
adjacency. No unused request adapter was introduced: current production shadow
collection constructs MethodIR; all ssabuild.Build call sites are tests. Existing
production sparse-set unions and exception-index construction already charge
the shared request budget and were retained.

Verified with `CGO_ENABLED=1 GOTOOLCHAIN=local
GOFLAGS='-ldflags=-linkmode=external'`:

- `go test ./internal/workbudget ./internal/jdecenv ./classparser/decompiler/core/ssabuild -race -count=1` (exit 0).
- `go test ./classparser -run 'TestRequest|TestTaskT0[5678]|TestT22_C0[12345]|TestT31C0[123]' -race -count=1` (exit 0).
- The same classparser selection without race after final early-exit guards
  (exit 0).
- `git diff --check` (exit 0).

New tests cover cancellation before malformed magic/hash and during parsing,
input caps, constant-pool allocation preflight, bounded child-reader isolation,
read transaction rollback, resolver shared budgets, unlimited parse byte
round-trip equivalence, ten concurrent workers sharing one cap, sticky first
failure after subsequent cancellation, deterministic multi-counter diagnostics,
aggregate overflow, checked products and final SSA budget accounting.

The complete T22 selection still fails only its strict LongTest `complete` or
`partial` status expectation: R01 now reports unresolved `java.util.Map.get`
and `Map.merge` declaration families as unsupported. This was reproduced on
the clean pre-R09 `1882ce5` baseline and is owned by R01 integration. No expected
status was relaxed. Broader classparser/JAR equivalence is not claimed here.

Limits: parser item/read counts bound logical work, not total heap retention or
all allocations in legacy source repair. Public standalone Parse retains its
unlimited request budget behavior while keeping existing structural bounds.
No arbitrary JARs were run, no tests were disabled, and no speedup is claimed.
Reverting this commit rolls back R09 independently of the listed dependencies.

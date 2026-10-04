# Draft 11 continuation notes

These notes record reproducible evidence and debugging decisions for the Draft 11
algorithm follow-up. They are intended to help the next maintainer continue from
the evidence without repeating exploratory work. They are not a claim that every
acceptance contract has passed.

## CFG diamond incorrectly emitted as a loop

`TestNonConditionalForkDiamondRoundTrip` compiles `WeakPhi.java` with `javac
--release 8`. In `select`, javac emits a forward-only diamond: `ifnonnull`, a null
arm, a `goto` to the merge, then a `WeakReference.get()` arm and `checkcast`,
followed by a shared `astore`. The reconstructed source previously replaced this
region with `do { ... } while (true)`, removed the null branch, and threw an NPE
when run. The original and reconstructed classes are compared by output under
`java -Xverify:all`; source compilation alone would not catch this semantic bug.

The route walk in `RewriteManager.ScanCoreInfo` identifies candidate loop heads
when a node is revisited along its traversal route. A shared merge can satisfy
that traversal condition without belonging to a cycle. Loop reconstruction needs
the stronger graph invariant: a loop head must be a member of a cyclic SCC in the
executable control-flow graph. `cyclicCFGNodes` computes that set with iterative
Kosaraju in O(V+E), and `ScanCoreInfo` now intersects route candidates with SCC
members. Include exception edges: a handler can transfer back into a protected
region, and dropping dispatch edges caused three existing exception/loop shape
regressions. This keeps real exception-mediated cycles while rejecting the
forward-only `WeakPhi` diamond.

The Mockito hard-jar shape test also exposed a brittle assertion: generated local
names changed from `var4` to `var5` once the corrected graph let the later method
body decompile. The invariant is that the local is declared as `MethodGraph`, not
that a particular synthetic number is chosen. The test now checks the type and
declaration form while still rejecting the malformed `varN varM =` shape.

Fast reproductions:

```bash
go test ./classparser -run '^TestNonConditionalForkDiamondRoundTrip$' -count=1 -v
go test ./classparser/decompiler/rewriter -run '^TestScanCoreInfo' -count=1 -v
```

When debugging a similar CFG issue, first print the original bytecode and the
node/edge graph, then classify edges as forward, back, or exception edges. Check
whether the suspected header is actually in an SCC before changing branch order
or loop rewriting. Keep both a real-loop neighbor and an acyclic-diamond neighbor
in the regression set.

## Request budget must follow the work, not a size estimate

The first shadow lowering integration charged `instruction count + phi count`
once, then built the value registry, handler coverage, exception plan and edge
copies without using the request counter. That is only a size proxy: the real
cost can be dominated by handler-by-instruction checks, incoming-edge copies,
phi operands, sorting, or copy-cycle scheduling. The production path now passes
the same `WorkCounter` through SSA construction, lowering and exception-plan
validation. Lowering charges traversed records and estimated sort comparisons
before expensive copies/sorts, and returns no plan after exhaustion. The old
unmetered APIs remain available for focused algorithm callers.

When debugging a budget report, identify the phase whose loop or allocation is
unbounded, charge its input relationships before entering that work, and verify
both the low-budget failure and a high-budget oracle. The low-budget test should
assert `nil` output and unchanged input; the high-budget test should compare the
entire output plan to the unmetered result. This separates “the counter fired”
from “the algorithm still computes the same plan.”

The lowerer keeps the old `Options` struct stable and exposes a separate
`DestroyWithWorkCounter` entry point. The regression first measures a
high-budget result and compares it with the legacy result; then it reruns with
one fewer work unit than the successful total. That forces a late failure after
the local plan has been mostly assembled, and proves the public return is still
`nil` with no mutation to the SSA input. `TestT11_C01_ShadowDoesNotInterfere`
also verifies that production shadow work contributes to the request's shared
`CounterAnalysisUpdates`.

## Separate new regressions from baseline failures

`TestRegressionSeedsAreDeterministic/AbstractObjectArrayAssert.class` fails on
both the clean continuation base `856109f1d8c8857a469432376f5b33636b9587eb`
and the working tree with the same 8-run nondeterminism assertion. Compare the
same named subtest at a clean fixed base before attributing a flaky result to a
change. Do not add this broad seed sweep to the fast required CI unless its
baseline is repaired and its runtime is measured.

The successful focused checks on the current working tree so far are:

```bash
CGO_ENABLED=1 GOTOOLCHAIN=local go test -ldflags=-linkmode=external \
  ./classparser/decompiler/core/... ./classparser/decompiler/rewriter/... \
  ./internal/workbudget ./internal/jdecenv -race -count=1 -timeout=3m
CGO_ENABLED=1 GOTOOLCHAIN=local go test -ldflags=-linkmode=external ./classparser \
  -run '^Test(BindingPlan|CtorNPECheckBeforeThis|EnumClinit|GenericNull|ImmediateZeroArgCheckcastInvoke|NonConditionalForkDiamond|InheritedJDK|InvocationMetadata|JDKListNull|MemberLedger|LogbackPutUninterruptibly|ProtobufRemainingReconstructs|ProtobufLazyStringArrayListThis|PublicSuffixDatabase|ThisCtorOverloadCast|ToListSingleCallableCast|JacksonRemainingMapEntryDeserializerCast|JoinStreamCtorCast|RawGenericReceiver|SyntaxObservation|RetryRollback|ShadowBuilder|Sentinels|Request|R06|T04|T05|T06|T07|T08|T18|T19|TaskT20)' \
  -count=1 -timeout=3m
CGO_ENABLED=1 GOTOOLCHAIN=local go build -ldflags=-linkmode=external ./...
python3 -m unittest tools.curated_roundtrip.test_roundtrip
```

The full package run is not green on the fixed base or candidate. On this tree,
`go test ./classparser -count=1 -json -timeout=30m` exited 1 after 441.154s with
82 failing top-level tests and no panic. The clean tree at the same commit
`856109f1d8c8857a469432376f5b33636b9587eb` had 85 failures after 425.809s.
All 82 candidate failures are shared with the clean base; three base failures
(`TestWrapUnresolvedNestedNewJarFS`, `TestXstreamCGLIBFactoryFlagIsLoadBearing`,
and `TestXstreamCGLIBInterfaceLoopIndexIsLoadBearing`) pass on the candidate.
The JSON logs are `/private/tmp/javajive-classparser-base-20260923.jsonl` and
`/private/tmp/javajive-classparser-current-final-20260923.jsonl`. Keep this
baseline distinction when triaging the broad historical suite; it does not
replace the focused required gate or the 36-row R10 result.

These are not the final gate. Still run the 36-row trusted-fixture round trip,
bind its evidence to the final commit, push the same PR #11 as a draft, and wait
for the latest-head required check to finish. Keep `ai long term` until every
task and applicable check is independently verified.

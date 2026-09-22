# R07 independent region oracle

Base region implementation: `7668795a91f3c1df94ba5595e1bac5c9477e99a0`.
The preceding R02 commit only changes frametransfer and has no effect on these
region calculations.

## Scope and evidence

The T09 dominance-backedge/remnant-cycle algorithm and all existing tests are
unchanged. New test-only helpers use node deletion plus BFS reachability for the
entire dominance matrix, and Kahn elimination for reducibility. They consume
fixture adjacency directly and do not call production dominance, reachability,
root-selection, or cycle helpers.

- All 512 directed three-node graphs, every one of 3 roots, all 9 dominance
  relations: 13,824 comparisons, no differences. Each root is also installed as a
  distinct exception-handler root (with duplicate exception edges), and the
  production reducibility result is checked against independent per-root checks.
- 240 additional graphs of 4–16 nodes, fixed seed `2026092207`, with two handler
  roots and shared normal-flow domains: no differences.
- Explicit infinite loop, nested multiple-entry cycle, ordinary nested loop,
  shared handler tail, independent handler cycle, and switch-only loop fixtures.
- Exit reachability is explicitly existential; an infinite alternative does not
  become a mandatory return path. A virtual exit never becomes a real node.
- Mutable cache tests check actual answers after adding/deleting a bypass,
  replacing a node, changing roots, and including/excluding exceptional edges.
  Previously returned snapshots stay unchanged.

One new regression failed before the fix:
`TestR07ExitReachabilityAndPostdominanceDomain` reported
`unreachable node postdominates itself outside analysis domain`.
`GraphAnalysis.PostDominates` special-cased self queries before consistently
rejecting the `IPDom == -1` out-of-domain marker. The narrow fix applies that
marker check to self queries too, retaining reachable self-postdominance.

## Commands

Environment for every command: `CGO_ENABLED=1 GOTOOLCHAIN=local GOFLAGS=-ldflags=-linkmode=external` on macOS.

Before the fix, `go test -count=1 ./classparser/decompiler/core -run TestR07`
exited 1 with the new unreachable self-postdominance regression.

After the fix:

- `go test -count=1 ./classparser/decompiler/core -run 'TestR07|TestT09|TestNextStageT09|TestT26'`: exit 0.
- Same command with `-race`: exit 0 (existing macOS LC_DYSYMTAB linker warning).
- `go test -count=1 ./classparser/decompiler/core`: exit 0.
- `git diff --check`: exit 0.

These are bounded graph checks, not proof for every graph or JVM method. No
production graph-size rejection threshold, new CFG representation, postdominator
exit policy, or performance claim was added. SemanticCFG remains an immutable
snapshot; mutation invalidation is exercised through MutableAnalysisGraph's
supported epoch API. Rollback is a revert of this bounded R07 commit.

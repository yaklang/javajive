# R03 / R11 production evidence

Base draft: `7668795a91f3c1df94ba5595e1bac5c9477e99a0`.
R02 dependency: `0864d13` (same patch locally cherry-picked as `6858293`).

Before implementation, installing the supplied production follow-up tests and
running `go test ./classparser/decompiler/core/ssabuild -run TestDraftFollowup
-count=1` failed both negation and same-type swap identity assertions (exit 1).
The supplied tests remain installed; only the call sites were adapted to the
new error-returning transfer signature.

After implementation, these real-module commands passed (exit 0):

- `go test ./classparser/decompiler/core/ssabuild -count=1`
- `go test ./classparser/decompiler/core/ssabuild -race -count=1`

Both used `CGO_ENABLED=1 GOTOOLCHAIN=local
GOFLAGS='-ldflags=-linkmode=external'` on macOS. The race binary links with an
Apple ld LC_DYSYMTAB warning, then all tests pass.

Tests exercise every legal dup/pop/swap category form, malformed pairs, unary
and binary results, stable result identities with rebound operands, prefix
preservation, wide local aliases, calls, checked casts, multi-arrays, constructor
exception poisoning, permanent entry type/value contributions, unavailable
predecessors, parallel edge identities and final nonzero operand IDs. A fixed
seed generates 1,000 actual MethodIR graphs; FIFO, LIFO and shuffled Build runs
are compared against a separately implemented synchronous reaching-definition
oracle. The oracle does not call production transfer, join or phi helpers.

The preexisting T13 throw-site test expected the store PC as value identity.
Its assertion now independently verifies each producer/store bytecode pair and
continues comparing the store PCs to the existing CFG reaching-definition
oracle. Explicit ()V metadata was added to its synthetic constant-pool-free
invocations; missing descriptors remain unsupported in production.

A broader core/... run before merging the concurrent R02/R04/R05 work passed
all packages except ssalower's T14_C04 fixture, whose missing call descriptor
is being corrected by its owning agent. This is not recorded as a full suite
pass. No JAR execution, goldens weakened, skipped tests, or performance claims.

Limitations: MethodIR currently has no authoritative max_locals, max_stack or
superclass metadata. Initial locals include every receiver/argument slot and
all accessed slots, with propagated descriptor/store errors. Constructor
ThisClass is supplied; direct-super constructor owner validation remains
unsupported when superclass metadata is absent. This is the shadow SSA path,
not proof of arbitrary JVM verification or legacy printer correctness.

Revert this commit to roll back R03/R11; the R02 dependency is independent.

# R02 frame semantics follow-up

Base: `7668795a91f3c1df94ba5595e1bac5c9477e99a0`.

## Reproduction and validation

All Go commands below used `CGO_ENABLED=1 GOTOOLCHAIN=local GOFLAGS=-ldflags=-linkmode=external` on macOS.

Before implementation, copying the two reviewed production regressions and running
`go test -count=1 ./classparser/decompiler/core/frametransfer -run TestDraftFollowup`
exited 1: `aload accepted int`; `long load accepted double tail`.

After implementation:

- `go test -count=1 ./classparser/decompiler/core/frametransfer`: exit 0.
- `go test -race -count=1 ./classparser/decompiler/core/frametransfer`: exit 0; macOS linker emitted an LC_DYSYMTAB warning.
- `git diff --check`: exit 0.

Tests include exhaustive supported local-load type combinations, invalid indexes
including integer overflow boundaries, category-2 stack limits, nonmutation,
2,000 deterministic pair-preserving store/join iterations (seed `0x52F02`),
commutative/idempotent joins, dead locals versus strict operand stacks, normal and
exceptional constructor aliases, invocation owner/descriptor/receiver rejection,
uninitialized reference transport versus member use, and positive/negative iinc.

A cross-package run of the unchanged baseline ssabuild tests found a synthetic
invokestatic fixture with no descriptor. That call now correctly returns
unsupported instead of assuming `()V`. Integration owner is updating that fixture
and method-entry sizing/context; it is not counted as a passed integration test.

## Corrected expectations

The old T12 tests asserted that constructor exception locals remained reusable
uninitialized aliases, and that iinc on a long tail behaved like an integer store.
Those expectations were incorrect and were replaced with stricter assertions;
all existing legal dup, store-overlap, float-bit, and SMT cases are retained.

- [JVMS SE25 4.9.2](https://docs.oracle.com/javase/specs/jvms/se25/html/jvms-4.html#jvms-4.9.2)
  requires handler-local copies of a constructor target to become unusable.
- [JVMS SE25 iinc](https://docs.oracle.com/javase/specs/jvms/se25/html/jvms-4.html#jvms-4.10.1.9.iinc)
  requires an integer local before incrementing it.

The task kit linked SE21, whose constructor prose and rules contain older
formulations. The explicit unusable-local constraint above is from SE25.

## API and integration limits

`NewFrame(n)` fixes the local count at n, with legacy stack capacity 65535.
`NewFrameWithLimits(maxLocals, maxStack)` also enforces declared stack limits,
including zero. `Frame.Validate` checks expanded representations and bounds.
Clone and JoinFrames preserve stack limits and constructor context.

The caller supplies `ThisClass`, `DirectSuperClass`, and constructor-entry
`ThisUninitialized`; StoreLocal of UninitThis also sets the flag. Allocation
instructions now retain their class. Missing constructor ownership context is
unsupported rather than guessed. A candidate exception frame contains Throwable
regardless of max_stack; the solver must Validate it only when attaching an actual
handler, since a no-handler throwing void call can legally have max_stack zero.

MethodIR did not carry declared Code maxima/superclass on this base. The parent
integration owns propagation of that metadata and correct argument-local sizing.
This is a checked transfer kernel, not a complete JVM verifier: reference joins
use Object conservatively; accessibility, complete reference assignability,
return-descriptor checks, field resolution, and complete StackMapTable validation
remain outside this change. No old printer pipeline or JAR execution is involved.

Rollback: revert this bounded R02 commit (and coordinate dependent solver API use).

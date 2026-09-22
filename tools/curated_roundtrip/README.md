# Curated round-trip gate

This directory vendors the nine reviewed R10 AlgorithmKit fixture families:
virtual, interface, generic, handler, effects, phis, markers, unicode, and metadata.
These small sources only perform bounded local computation and print observations.
The JDK helpers independently parse Java syntax and verify class loading without
running static initializers. The runner separately executes each reviewed main.

Build and run from the repository root:

```sh
python3 tools/curated_roundtrip/build_adapter.py --out /tmp/draft-followup-probe
python3 tools/curated_roundtrip/roundtrip.py --adapter /tmp/draft-followup-probe \
  --out /tmp/curated-roundtrip-new-run --allow-trusted-fixture-execution
```

A normal run is a strict gate over 9 families × 2 debug settings × 2 API modes.
There is no expected-failure list. `--observe` is an explicit investigation mode;
its zero exit status means observation completed, never that failed rows passed.
`--case NAME` selects one family and rejects an unknown name. Missing source files,
JDK tools, failed original compilation, timeouts, output limits, missing rebuilt
classes, non-complete API status, stubs, parser failures, type errors, verifier
failures, or runtime differences cannot count as success.

Each report records the checked-out revision, dirty-tree state, adapter hash,
fixture/class hashes, compiler/runtime versions, API results, independent stage
commands and outputs. Rebuilt compilation and execution never include original
application classes on the classpath. Outputs go to a new directory, preserving
previous observations.

This is a developer/CI tool for the reviewed sources in this directory, not a
sandbox. Do not add arbitrary downloaded classes/JARs or run untrusted input on
the host; use the isolated worker. CI uses a disposable GitHub-hosted runner with
read-only permissions, no persisted checkout credentials, and no injected secrets.

The build wrapper writes a binary-hash-bound revision manifest. This supports Go
1.22 worktrees, where the Go tool does not recognize a `.git` file for automatic
VCS stamping. The audit rejects a stale revision or mismatched binary hash.

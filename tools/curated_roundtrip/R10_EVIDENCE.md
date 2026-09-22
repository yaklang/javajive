# R10 CI portability and hermetic round trips

Reviewed prior-7668795 CI logs identify three repaired infrastructure failures:
Windows compilation referenced unavailable syscall.Getrusage symbols;
T24/T31 fixture tests referenced a developer's /private/tmp task pack; Linux
seatbelt-profile tests received invented Darwin /private/tmp path aliases.

Changes preserve all existing assertions:

- Platform-separated RSS uses getrusage with explicit Linux/Darwin units and
  Windows GetProcessMemoryInfo PeakWorkingSetSize in bytes. API failures and
  unsupported OSes return an error and serialize unavailable RSS as null, never
  a measured zero. Existing tests still require real cold-process RSS.
- Three reviewed tiny Java sources are committed under
  classparser/testdata/task_contracts and accessed relative to the test package.
- Darwin aliases are generated only on Darwin. Existing filesystem/network deny
  rules are retained, with a new Linux/Darwin alias regression.
- Nine reviewed R10 fixture families, JDK ParseOnly/VerifyOnly helpers, and the
  independent Python runner are committed here. The CI job builds the current
  adapter, runs 36 strict rows, checks parser versus typechecker separation, and
  always uploads evidence. No expected-failure list or observation-mode gate.
- The runner records hashes, toolchains, HEAD, dirty-tree state, commands/results,
  and requires the adapter's embedded VCS revision or hash-bound build manifest to
  match checked-out HEAD. The manifest handles Go 1.22's unstamped worktrees.
  Rebuilt classpaths never contain original target classes.

Local validation (macOS; Go commands use CGO_ENABLED=1 GOTOOLCHAIN=local
GOFLAGS=-ldflags=-linkmode=external unless cross-compiling):

- Targeted native RSS tests: pass.
- RSS package cross-builds for Windows amd64, Windows arm64, and Linux amd64
  with CGO_ENABLED=0: pass. Native Windows runtime remains a CI check, not a
  locally observed pass.
- T24 exception fixture test and T31 unknown-bootstrap fixture test: pass.
- Six Python runner/profile tests: pass. Includes mocked Linux alias behavior,
  actual tiny-process timeout/output-limit/missing-executable failures.
- Initial full curated round-trip run: 36/36 pass, on local branch with R02/R07
  and parent invocation-binding dependency 5f85ebe cherry-picked as f8ae45d.
  Initial output: /private/tmp/javajive-r10-curated-frames-first/report.json.
  The integration owner must rerun at the final combined HEAD; this earlier
  result is not evidence for subsequent changes.

Scope limits: no unknown JARs were executed. This host runner is for reviewed
curated fixtures only; external inputs still require the isolated worker.
Unrelated prior CI failures (Docker capability gates, native semantic failures,
historical comparisons) are not made green by these fixes. No full native
Windows/Linux test-suite claim is made from cross-compilation.

Windows API references:
https://learn.microsoft.com/windows/win32/api/psapi/nf-psapi-getprocessmemoryinfo
https://learn.microsoft.com/windows/win32/api/psapi/ns-psapi-process_memory_counters

Rollback: revert this bounded R10 commit; the existing adapter is a separate
parent-owned dependency.

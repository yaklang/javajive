# Regression CI

Pull requests run one Ubuntu/Go-module-version/JDK21 job with a ten-minute ceiling:

1. Build all Go packages.
2. Run the complete core algorithm packages, request budgets and environment
   isolation tests with the race detector.
3. Run production entry-path regressions for invocation, frames/metadata,
   bootstrap behavior, request limits, ledgers, and shadow observations.
4. Recompile and execute nine reviewed tiny Java families in both modes and
   with/without debug information (36 observations).

Each failed Java observation names its case and stage, and retains source,
compiler output and original/rebuilt stdout. The failure artifact contains these
files and the checkout revision. No downloaded application JAR is run on the host.
Use the commands in `.github/workflows/ci.yml` to reproduce a failing step.

Large historical JAR comparisons and the full Go corpus run through the manual
Extended regression workflow. Platform/sandbox and full task-pack infrastructure
checks are manual. Their failures remain failures; moving them out of the hot PR
path does not turn them into passed checks or remove the underlying tests.

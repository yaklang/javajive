# R08 production observations and retry rollback

The public result now has one owner/kind/name/descriptor row per original member.
Rows record emission or a specific regeneration path; skipped members without a
recorded proof remain unsupported. Stub and dropped rows cannot become complete.
The ledger is an audit of reconstruction, not a Java equivalence proof. Additional
bridge/enum regeneration proofs remain intentionally conservative.

Shadow capture records disabled/unavailable/ok/failed and the originating method;
missing builders, returned errors, panics and empty successful snapshots are no
longer silently discarded. Shadow observations do not change rendered source.
Syntax validation has a request-local callback. Missing callbacks and the absent
legacy parser return unavailable, never valid. Syntax success is separate from
compilation, JVM verification and execution (the R10 harness performs those).

Declined aggressive retries restore the dumper's method cache, field initializer
map, lambda maps/counters, diagnostic/rule/member/shadow records, mutable context
maps/slices/imports, and exact indentation stack. The caller's context pointer is
restored in place. Immutable class metadata and resolver caches are shared; work
already spent is deliberately not refunded. Failure markers in literals, comments
and text blocks are protected by the shared Java lexical shield.

Validation on the integrated working candidate:

- Member ledger negative states and a real synthetic-but-unconsumed lambda body.
- Missing/invalid validator and shadow missing/error production entry paths.
- Retry rollback with simultaneous mutations to cache, captures, fields, imports,
  context, diagnostics and ledger, asserting original pointer aliases are restored.
- Targeted tests and race pass.
- Full curated 36-row dual-mode/debug roundtrip pass after ledger integration.
- Final fixed-commit CI remains required; no ready or equivalence claim here.

"""Frozen protocol constants for the T01 milestone ledger."""

from __future__ import annotations

# Immutable pack baseline. Do not replace with origin/main.
MILESTONE_SHA = "24b9d377c91ba75f3a277672973782299a958d5f"
# Reviewer PR base / origin/main at dispatch.
PR_BASE_SHA = "81f8ef55603419bf737f10aa36e582f94619b35d"

SCHEMA_VERSION = 1

MODES = frozenset({"compatibility-cli", "precision", "compatibility"})
CLI_MODES = frozenset({"compatibility-cli"})
API_MODES = frozenset({"precision", "compatibility"})
DEBUGS = frozenset({"debug", "nodebug"})
INTERFACES = frozenset({"cli", "api"})

# Ledger result statuses. Decompiler "complete" is not a ledger pass.
RESULT_STATUSES = frozenset(
    {
        "pass",
        "fail",
        "unsupported",
        "infra_error",
        "not_run",
        "invalid_input",
        "partial",
        "budget",
        "incomparable",
        "observe",
        "unknown",
    }
)
SUCCESS_STATUSES = frozenset({"pass"})
NON_SUCCESS_STATUSES = RESULT_STATUSES - SUCCESS_STATUSES
# Statuses that must never be tallied as pass (M03).
NEVER_PASS_STATUSES = frozenset(
    {
        "fail",
        "unsupported",
        "infra_error",
        "not_run",
        "invalid_input",
        "partial",
        "budget",
        "incomparable",
        "observe",
        "unknown",
        "complete",
        "xfail",
        "skip",
        "empty",
    }
)

# Next-stage runner 13-check raw fields (CLI observations).
HISTORICAL_CLI_THIRTEEN_FIELDS = (
    "case",
    "debug",
    "mode",
    "release",
    "original_compile",
    "original_verify",
    "original_run",
    "decompile",
    "rebuilt_compile",
    "rebuilt_verify",
    "rebuilt_run",
    "pass",
    "input_sha256",
)

# Extra historical keys preserved when present; never rewritten.
HISTORICAL_CLI_OPTIONAL_FIELDS = (
    "tier",
    "decompile_result",
    "failure",
)

# Production historical JAR observation fields (Go struct + scripts/historical_jar_audit.py).
HISTORICAL_JAR_RAW_FIELDS = (
    "Jar",
    "Revision",
    "InputSHA256",
    "Compiler",
    "InputClasses",
    "SourceUnits",
    "DecompileFailures",
    "StubMethods",
    "StubUnits",
    "SourceHashes",
    "CompilePasses",
    "CompilerErrors",
    "CompileSucceeded",
    "OriginalVerifyOK",
    "OriginalVerifyFail",
    "RebuiltVerifyOK",
    "RebuiltVerifyFail",
    "RebuiltClasses",
    "Seconds",
    "Completed",
)

REQUIRED_EVIDENCE_FIELDS = (
    "input_hash",
    "compiler_version",
    "release",
    "debug",
    "mode",
    "output_target",
    "dependency_lock_digest",
    "harness_digest",
    "revision",
)

COMPARABILITY_AXES = (
    "compiler_version",
    "harness_digest",
    "dependency_lock_digest",
    "input_hash",
)

# Next-stage isolated verifier source (seed_runner.py). Digest is harness identity.
NEXT_STAGE_VERIFIER_SOURCE = (
    "public class NextStageVerify { public static void main(String[] a) throws Exception { "
    "Class<?> c=Class.forName(a[0],false,NextStageVerify.class.getClassLoader()); "
    "c.getDeclaredMethods(); c.getDeclaredConstructors(); c.getDeclaredFields(); "
    'System.out.println("VERIFIED"); }}'
)

SEED_SOURCE_SHA256 = {
    "Baseline": "7e70c7377a2530a8c2e6f6d2fabf80f130c0f5a91afbb8eaf5d4a78259cc85f1",
    "UnicodePair": "f233a586da9cd42f578ba40a406f5577665ce259ef5c7c75303213b5475a6c1c",
    "ConcatProbe": "a1637c9286d08b02ad92c4cb9400b3a17a3fcaaaec7e9333f380cda57ddd2e2a",
    "ParamAnnotation": "9e347262a2993bef70a329498900b65b22bab8b6f99a90954fb3828a80385c38",
}

SOURCES = frozenset(
    {
        "synthetic",
        "historical-cli",
        "historical-jar",
        "api-live",
        "api-not-executed",
    }
)

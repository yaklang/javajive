"""T28: legal generators, metamorphic morphs, holdout split, failure-preserving reduce."""

from .identity import CompilerIdentity, discover_compilers, compile_sources
from .generate import FAMILIES, SAMPLES_PER_FAMILY, GENERATOR_SEED, generate_family
from .morph import morph_source, MorphKind
from .reduce import reduce_source, ReductionError
from .holdout import split_holdout, HoldoutLeakError
from .evidence import FailureEvidence, classify_failure

__all__ = [
    "CompilerIdentity",
    "discover_compilers",
    "compile_sources",
    "FAMILIES",
    "SAMPLES_PER_FAMILY",
    "GENERATOR_SEED",
    "generate_family",
    "morph_source",
    "MorphKind",
    "reduce_source",
    "ReductionError",
    "split_holdout",
    "HoldoutLeakError",
    "FailureEvidence",
    "classify_failure",
]

"""Semantics-preserving morphs: debug, names, independent slots, switch-key bijection."""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import Any

from .generate import Sample
from .identity import CompilerIdentity, InfraError, compile_sources, java_command, verify_and_run


class MorphKind(str, Enum):
    DEBUG = "debug"
    NAME = "name"
    SLOT = "slot"
    SWITCH_KEY = "switch_key"


@dataclass(frozen=True)
class MorphResult:
    kind: MorphKind
    original: Sample
    morphed_source: str
    original_stdout: str
    morphed_stdout: str
    original_class_sha256: str
    morphed_class_sha256: str
    oracle_equal: bool
    notes: str

    def to_dict(self) -> dict[str, Any]:
        return {
            "kind": self.kind.value,
            "class_name": self.original.class_name,
            "original_source_sha256": self.original.source_sha256(),
            "morphed_source_sha256": hashlib.sha256(self.morphed_source.encode()).hexdigest(),
            "original_class_sha256": self.original_class_sha256,
            "morphed_class_sha256": self.morphed_class_sha256,
            "oracle_equal": self.oracle_equal,
            "notes": self.notes,
        }


def morph_source(sample: Sample, kind: MorphKind) -> str:
    src = sample.source
    if kind is MorphKind.NAME:
        # Rename locals without changing semantics. Keep public API.
        renamed = src
        renamed = re.sub(r"\bint a =", "int alpha =", renamed)
        renamed = re.sub(r"\bint b =", "int beta =", renamed)
        renamed = re.sub(r"\ba\b", "alpha", renamed)
        renamed = re.sub(r"\bb\b", "beta", renamed)
        # Repair class/API names accidentally touched: none of our APIs use a/b.
        return renamed
    if kind is MorphKind.SLOT:
        # Swap independent initializers when both exist.
        pattern = re.compile(
            r"int a = (?P<a>\d+);\n    int b = (?P<b>\d+);",
        )
        if not pattern.search(src):
            raise ValueError(f"slot morph requires independent a/b locals: {sample.class_name}")
        return pattern.sub(r"int b = \g<b>;\n    int a = \g<a>;", src)
    if kind is MorphKind.SWITCH_KEY:
        if sample.family != "switch":
            raise ValueError("switch_key morph only applies to switch family")
        table: dict[int, int] = {int(k): int(v) for k, v in sample.params["table"].items()}
        # Bijection: k -> k+100, discriminator inverted at the switch argument.
        cases = []
        for k, v in table.items():
            cases.append(f"      case {k + 100}: return {v};")
        probes = sample.params["probes"]
        probe_prints = "\n".join(
            f"    System.out.println(run({p}));" for p in probes
        )
        keys_src = ",".join(str(k) for k in table)
        src = f"""public class {sample.class_name} {{
  public static int remap(int x) {{
    int[] keys = new int[] {{{keys_src}}};
    for (int i = 0; i < keys.length; i++) {{
      if (x == keys[i]) return keys[i] + 100;
    }}
    return x;
  }}
  public static int run(int x) {{
    switch (remap(x)) {{
{chr(10).join(cases)}
      default: return -1;
    }}
  }}
  public static void main(String[] args) {{
{probe_prints}
  }}
}}
"""
        return src
    if kind is MorphKind.DEBUG:
        return src
    raise ValueError(kind)


def _compile_one(identity: CompilerIdentity, source: str, class_name: str, dest: Path, debug: str) -> bytes:
    dest.mkdir(parents=True, exist_ok=True)
    src = dest.parent / f"{class_name}.java"
    src.write_text(source, encoding="utf-8")
    compiled = compile_sources(identity, [src], dest, debug=debug)
    if compiled["rc"] != 0:
        raise InfraError(f"morph compile failed ({debug}): {compiled['stderr']}")
    class_file = dest / f"{class_name}.class"
    return class_file.read_bytes()


def apply_morph(
    sample: Sample,
    kind: MorphKind,
    identity: CompilerIdentity,
    work: Path,
) -> MorphResult:
    original_debug = "nodebug"
    morph_debug = "debug" if kind is MorphKind.DEBUG else "nodebug"
    morphed = morph_source(sample, kind)
    orig_dir = work / "orig"
    morph_dir = work / "morph"
    orig_bytes = _compile_one(identity, sample.source, sample.class_name, orig_dir, original_debug)
    morph_bytes = _compile_one(identity, morphed, sample.class_name, morph_dir, morph_debug)
    java_bin = java_command(identity)
    orig_run = verify_and_run(orig_dir, sample.class_name, java_bin=java_bin)
    morph_run = verify_and_run(morph_dir, sample.class_name, java_bin=java_bin)
    if not orig_run["verified_and_ran"]:
        raise InfraError(f"original failed verify/run: {orig_run['stderr']}")
    if not morph_run["verified_and_ran"]:
        raise InfraError(f"morphed program is not a legal equivalent: {morph_run['stderr']}")
    equal = orig_run["stdout"] == morph_run["stdout"] == sample.expected_stdout
    notes = f"{kind.value}: debug={morph_debug} stdout_equal={equal}"
    return MorphResult(
        kind=kind,
        original=sample,
        morphed_source=morphed,
        original_stdout=orig_run["stdout"],
        morphed_stdout=morph_run["stdout"],
        original_class_sha256=hashlib.sha256(orig_bytes).hexdigest(),
        morphed_class_sha256=hashlib.sha256(morph_bytes).hexdigest(),
        oracle_equal=equal,
        notes=notes,
    )


def compare_decompiled_pair(
    original_source: str,
    morphed_source: str,
    identity: CompilerIdentity,
    work: Path,
    *,
    modes: tuple[str, ...] = ("precision", "compatibility"),
    debug: str = "nodebug",
) -> dict:
    """Rebuild/run JavaJive outputs for every requested mode. Infra cannot be a pass."""
    from .pipeline import run_pipeline

    rows = []
    for mode in modes:
        orig = run_pipeline(original_source, identity, work / f"orig-{mode}", mode=mode, debug=debug)
        morph = run_pipeline(morphed_source, identity, work / f"morph-{mode}", mode=mode, debug=debug)
        if orig.failure_class == "infra_error" or morph.failure_class == "infra_error":
            raise InfraError(
                f"compiler/probe infra_error cannot satisfy T28-C02 behavioral acceptance: "
                f"orig={orig.failure_class} morph={morph.failure_class} mode={mode}"
            )
        # Byte-different decompiled source may still be semantically equal via rebuilt stdout.
        semantic_equal = (
            orig.failure_class == "pass"
            and morph.failure_class == "pass"
            and orig.rebuilt_stdout == morph.rebuilt_stdout
            and orig.original_stdout == morph.original_stdout
        )
        rows.append(
            {
                "mode": mode,
                "original": orig.to_dict(),
                "morphed": morph.to_dict(),
                "semantic_equal": semantic_equal,
                "same_failure_class": orig.failure_class == morph.failure_class,
                "source_bytes_equal": orig.decompiled_source == morph.decompiled_source,
            }
        )
    return {"modes": rows}

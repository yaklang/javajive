"""Fixed-seed legal source families: loop, switch, exception, stack."""

from __future__ import annotations

import hashlib
import json
import random
import re
import struct
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Iterable

from .identity import CompilerIdentity, InfraError, compile_sources, java_command, verify_and_run

FAMILIES = ("loop", "switch", "exception", "stack")
SAMPLES_PER_FAMILY = 100
GENERATOR_SEED = 20260921
GENERATOR_VERSION = "t28-sourcegen-v1"


@dataclass(frozen=True)
class Sample:
    family: str
    index: int
    class_name: str
    source: str
    params: dict[str, Any]
    expected_stdout: str
    seed: int

    def source_sha256(self) -> str:
        return hashlib.sha256(self.source.encode("utf-8")).hexdigest()

    def provenance(self) -> dict[str, Any]:
        return {
            "generator": GENERATOR_VERSION,
            "seed": self.seed,
            "family": self.family,
            "index": self.index,
            "params": self.params,
            "source_sha256": self.source_sha256(),
        }


def _ident(family: str, index: int) -> str:
    return f"T28{family.title()}{index:03d}"


def generate_family(family: str, *, seed: int = GENERATOR_SEED, count: int = SAMPLES_PER_FAMILY) -> list[Sample]:
    if family not in FAMILIES:
        raise ValueError(f"unknown family {family}")
    rng = random.Random(seed + FAMILIES.index(family) * 1009)
    out = []
    for i in range(count):
        out.append(_one(family, i, rng, seed))
    return out


def generate_all(*, seed: int = GENERATOR_SEED, count: int = SAMPLES_PER_FAMILY) -> list[Sample]:
    samples: list[Sample] = []
    for family in FAMILIES:
        samples.extend(generate_family(family, seed=seed, count=count))
    return samples


def _one(family: str, index: int, rng: random.Random, seed: int) -> Sample:
    name = _ident(family, index)
    if family == "loop":
        n = rng.randint(1, 12)
        mul = rng.randint(1, 9)
        add = rng.randint(0, 7)
        a, b = rng.randint(1, 5), rng.randint(1, 5)
        acc = 0
        for i in range(n):
            acc += i * mul + add + a - b
        src = f"""public class {name} {{
  public static int run() {{
    int a = {a};
    int b = {b};
    int s = 0;
    for (int i = 0; i < {n}; i++) {{
      s += i * {mul} + {add} + a - b;
    }}
    return s;
  }}
  public static void main(String[] args) {{
    System.out.println(run());
  }}
}}
"""
        return Sample(family, index, name, src, {"n": n, "mul": mul, "add": add, "a": a, "b": b}, f"{acc}\n", seed)
    if family == "switch":
        keys = sorted({rng.randint(1, 40) for _ in range(4)})
        while len(keys) < 4:
            keys = sorted(set(keys) | {rng.randint(1, 40)})
        vals = [rng.randint(10, 99) for _ in keys]
        table = dict(zip(keys, vals))
        probes = list(keys) + [0, 99]
        lines = "\n".join(f"      case {k}: return {table[k]};" for k in keys)
        expected = "".join(f"{table.get(p, -1)}\n" for p in probes)
        probe_prints = "\n".join(f"    System.out.println(run({p}));" for p in probes)
        src = f"""public class {name} {{
  public static int run(int x) {{
    switch (x) {{
{lines}
      default: return -1;
    }}
  }}
  public static void main(String[] args) {{
{probe_prints}
  }}
}}
"""
        return Sample(family, index, name, src, {"table": table, "probes": probes}, expected, seed)
    if family == "exception":
        kind = rng.choice(["illegal", "state", "npe"])
        caught = rng.choice([True, False])
        mapping = {
            "illegal": ("IllegalArgumentException", "throw new IllegalArgumentException(\"x\");"),
            "state": ("IllegalStateException", "throw new IllegalStateException(\"s\");"),
            "npe": ("NullPointerException", "String z = null; extra += z.length();"),
        }
        catch_type, throw_stmt = mapping[kind]
        src = f"""public class {name} {{
  public static int run(boolean fire) {{
    int extra = 0;
    try {{
      extra = fire ? 1 : 0;
      if (extra == 1) {{
        {throw_stmt}
      }}
      return 10 + extra;
    }} catch ({catch_type} e) {{
      return 20 + extra;
    }} finally {{
      extra = extra + 0;
    }}
  }}
  public static void main(String[] args) {{
    System.out.println(run(true));
    System.out.println(run(false));
  }}
}}
"""
        _ = caught
        return Sample(
            family,
            index,
            name,
            src,
            {"kind": kind, "catch_type": catch_type},
            "21\n10\n",
            seed,
        )
    # stack / nested expression
    depth = rng.randint(2, 6)
    nums = [rng.randint(1, 9) for _ in range(depth + 1)]
    expr = str(nums[0])
    value = nums[0]
    ops = []
    for n in nums[1:]:
        op = rng.choice(["+", "-", "*"])
        ops.append(op)
        expr = f"({expr} {op} {n})"
        if op == "+":
            value = value + n
        elif op == "-":
            value = value - n
        else:
            value = value * n
    src = f"""public class {name} {{
  public static int run() {{
    return {expr};
  }}
  public static void main(String[] args) {{
    System.out.println(run());
  }}
}}
"""
    return Sample(family, index, name, src, {"expr": expr, "depth": depth}, f"{value}\n", seed)


def write_sources(samples: Iterable[Sample], directory: Path) -> list[Path]:
    directory.mkdir(parents=True, exist_ok=True)
    paths = []
    for sample in samples:
        path = directory / f"{sample.class_name}.java"
        path.write_text(sample.source, encoding="utf-8")
        paths.append(path)
    return paths


def compile_and_verify_family(
    samples: list[Sample],
    identity: CompilerIdentity,
    work: Path,
    *,
    debug: str = "nodebug",
) -> dict[str, Any]:
    src_dir = work / "src"
    dest = work / "classes"
    paths = write_sources(samples, src_dir)
    compiled = compile_sources(identity, paths, dest, debug=debug)
    if compiled["rc"] != 0:
        raise InfraError(f"generator compile failed: {compiled['stderr']}")
    java_bin = java_command(identity)
    results = []
    failures = []
    for sample in samples:
        class_file = dest / f"{sample.class_name}.class"
        if not class_file.is_file():
            failures.append({"sample": sample.class_name, "error": "missing class file", "class": "generator_error"})
            continue
        digest = hashlib.sha256(class_file.read_bytes()).hexdigest()
        ran = verify_and_run(dest, sample.class_name, java_bin=java_bin, trusted=True)
        ok = ran["verified_and_ran"] and ran["stdout"] == sample.expected_stdout
        row = {
            "class_name": sample.class_name,
            "family": sample.family,
            "source_sha256": sample.source_sha256(),
            "class_sha256": digest,
            "stdout": ran["stdout"],
            "expected": sample.expected_stdout,
            "rc": ran["rc"],
            "stderr": ran["stderr"],
            "ok": ok,
            "provenance": sample.provenance(),
            "compiler": identity.coverage_key(),
        }
        results.append(row)
        if not ok:
            failures.append({"sample": sample.class_name, "error": ran["stderr"], "class": "generator_error", "row": row})
    return {
        "compile": compiled,
        "results": results,
        "failures": failures,
        "ok": not failures,
        "count": len(samples),
        "verified": sum(1 for r in results if r["ok"]),
    }


def corrupt_class(legal: bytes, kind: str) -> bytes:
    if kind == "bad_magic":
        return b"\x00\x00\x00\x00" + legal[4:]
    if kind == "truncated":
        return legal[: max(16, len(legal) // 5)]
    if kind == "cp_overflow":
        if len(legal) < 10:
            return legal
        count = struct.unpack_from(">H", legal, 8)[0]
        mutated = bytearray(legal)
        struct.pack_into(">H", mutated, 8, min(0xFFFF, count + 5000))
        return bytes(mutated)
    raise ValueError(kind)


def class_looks_legal_enough_to_load(data: bytes) -> bool:
    return len(data) >= 10 and data[:4] == b"\xca\xfe\xba\xbe"


_CLASS_NAME = re.compile(rb"T28[A-Za-z]+[0-9]{3}")


def dump_inventory(rows: list[dict[str, Any]], path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(rows, indent=2, sort_keys=True) + "\n", encoding="utf-8")

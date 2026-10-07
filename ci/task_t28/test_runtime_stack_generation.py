"""A stack corpus must contain runtime arithmetic after javac has optimized it."""
import ast
import re
import subprocess
import tempfile
import unittest
from pathlib import Path

from tools.generators_holdout.generate import generate_family, write_sources
from tools.generators_holdout.identity import compile_sources, discover_compilers, java_command
from tools.generators_holdout.pipeline import run_pipeline


def _integer_expression(node):
    # Independent recursive syntax-tree oracle. No source execution or eval.
    if isinstance(node, ast.Constant) and type(node.value) is int:
        return node.value
    if not isinstance(node, ast.BinOp):
        raise AssertionError("unexpected arithmetic grammar")
    left, right = _integer_expression(node.left), _integer_expression(node.right)
    if isinstance(node.op, ast.Add):
        result = left + right
    elif isinstance(node.op, ast.Sub):
        result = left - right
    elif isinstance(node.op, ast.Mult):
        result = left * right
    else:
        raise AssertionError("unexpected arithmetic operator")
    return (result + (1 << 31)) % (1 << 32) - (1 << 31)


class TestRuntimeStackGeneration(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.compiler = discover_compilers()["javac21_release8"]
        if cls.compiler is None:
            raise AssertionError("javac 21 is required; missing execution is not PASS")
        cls.samples = generate_family("stack")

    def test_sample_provenance_has_stable_indices_and_seeded_inputs(self):
        self.assertEqual([s.index for s in self.samples], list(range(100)))
        self.assertEqual(self.samples, generate_family("stack"))
        for sample in self.samples:
            self.assertEqual(len(sample.params["operands"]), sample.params["depth"] + 1)
            self.assertEqual(len(sample.params["operators"]), sample.params["depth"])

    def test_every_stack_sample_retains_runtime_arithmetic(self):
        with tempfile.TemporaryDirectory(prefix="t28-runtime-stack-") as raw:
            work = Path(raw)
            sources = write_sources(self.samples, work / "src")
            driver = work / "StackCorpusDriver.java"
            driver.write_text("public class StackCorpusDriver {public static void main(String[] a){" +
                              "".join(f"System.out.println({s.class_name}.run());" for s in self.samples) + "}}")
            expected = []
            for sample in self.samples:
                tree = ast.parse(sample.params.get("oracle_expr", sample.params["expr"]), mode="eval")
                row = str(_integer_expression(tree.body)) + "\n"
                self.assertEqual(sample.expected_stdout, row)
                expected.append(row)
            for debug in ("nodebug", "debug"):
                classes = work / debug
                built = compile_sources(self.compiler, [*sources, driver], classes, debug=debug)
                self.assertEqual(built["rc"], 0, built)
                javap = Path(self.compiler.home) / "bin" / "javap"
                dumped = subprocess.run([str(javap), "-classpath", str(classes), "-c", "-p",
                                         *(s.class_name for s in self.samples)],
                                        capture_output=True, text=True, timeout=30, check=True).stdout
                for sample in self.samples:
                    with self.subTest(debug=debug, sample=sample.class_name):
                        section = dumped.split("public class " + sample.class_name + " {", 1)[1].split("\npublic class ", 1)[0]
                        arithmetic = re.findall(r"^\s*\d+:\s+(iadd|isub|imul)\b", section, re.M)
                        self.assertEqual(len(arithmetic), sample.params["depth"],
                                         "javac folded away the advertised stack operations")
                ran = subprocess.run([java_command(self.compiler), "-Xverify:all", "-cp", str(classes),
                                      "StackCorpusDriver"], capture_output=True, text=True, timeout=30)
                self.assertEqual(ran.returncode, 0, ran.stderr)
                self.assertEqual(ran.stdout, "".join(expected))
                self.assertEqual(ran.stderr, "")

    def test_runtime_stack_reconstruction_uses_both_public_modes(self):
        # Authored, reviewed programs only. Foreign input still requires T30.
        with tempfile.TemporaryDirectory(prefix="t28-runtime-roundtrip-") as raw:
            for sample in self.samples[::25]:
                for debug in ("nodebug", "debug"):
                    for mode in ("precision", "compatibility"):
                        with self.subTest(sample=sample.class_name, debug=debug, mode=mode):
                            result = run_pipeline(sample.source, self.compiler,
                                                  Path(raw) / sample.class_name / debug / mode,
                                                  mode=mode, debug=debug, trusted=True)
                            self.assertEqual(result.failure_class, "pass", result.to_dict())
                            self.assertEqual(result.original_stdout, sample.expected_stdout)
                            self.assertEqual(result.rebuilt_stdout, sample.expected_stdout)
                            self.assertEqual(result.stub_methods, [])
                            self.assertEqual(result.stages["original_verify_run"].stderr, "")
                            self.assertEqual(result.stages["rebuild_verify_run"].stderr, "")

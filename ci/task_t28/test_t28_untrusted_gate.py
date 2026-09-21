"""Untrusted class/JAR must not use host java. Reviewed fixtures stay trusted=True."""

from __future__ import annotations

import os
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock

from test_t28_contracts import _compilers
from tools.generators_holdout import identity as identity_mod
from tools.generators_holdout.identity import compile_sources, java_command, verify_and_run
from tools.generators_holdout.classfile import class_enclosing_info
from tools.generators_holdout.pipeline import (
    InvalidBinaryName,
    class_relpath,
    run_pipeline,
    run_pipeline_from_class_bytes,
    source_relpath,
)
from tools.sandbox_worker.detect import detect
from tools.sandbox_worker.executor import SandboxWorker
from tools.sandbox_worker.policy import Limits
from tools.sandbox_worker.untrusted_exec import (
    DEFAULT_UNTRUSTED_LIMITS,
    StagingError,
    _limits_for_timeout,
    _rel_files,
    run_untrusted_class_dir,
)

FIXTURES = Path(__file__).resolve().parents[2] / "tools" / "generators_holdout" / "fixtures"


def _obs(**kwargs):
    extra = {
        "leftover_query_failed": False,
        "leftover_cleanup_failed": False,
        "leftover_verified": True,
        "backend_extra": {"image": "javajive-sandbox-toolchain:t30"},
    }
    extra.update(kwargs.pop("extra", {}))
    defaults = dict(
        argv=["docker", "run", "java", "-cp", "/inputs", "Baseline"],
        exit_code=0,
        stdout="7\n",
        stderr="",
        timed_out=False,
        did_execute=True,
        status="ok",
        reason="ok",
        leftover_host_pids=[],
        leftover_containers=[],
        backend="docker",
        extra=extra,
        mount_inventory={"image": "javajive-sandbox-toolchain:t30"},
    )
    defaults.update(kwargs)
    return mock.Mock(**defaults)


def _no_host_java(real_run):
    """Spy: javac/version keep working; host `java` is a hard failure."""

    def _run(argv, env=None, timeout=30):
        if argv and Path(str(argv[0])).name == "java":
            raise AssertionError(f"host java invoked: {argv}")
        return real_run(argv, env=env, timeout=timeout)

    return _run


def _require_docker(test: unittest.TestCase) -> None:
    available = [p.name for p in detect().probes if p.available and p.name in {"docker", "podman"}]
    if not available:
        test.fail("infra_error: docker/podman is required for this contract and is not available")
    worker = SandboxWorker()
    if worker.backend not in {"docker", "podman"}:
        test.fail(f"infra_error: expected docker/podman backend, got {worker.backend!r}")


def _docker_argv(stage) -> list:
    argv = list(stage.argv or [])
    if not argv:
        raise AssertionError(f"missing worker argv: {stage}")
    if Path(str(argv[0])).name not in {"docker", "podman"}:
        raise AssertionError(f"expected docker/podman argv, got {argv}")
    if "java" not in argv and not any(Path(str(x)).name == "java" for x in argv):
        raise AssertionError(f"expected java inside worker argv, got {argv}")
    return argv


class TestBinaryNamePaths(unittest.TestCase):
    def test_package_inner_application_source_and_class_paths(self) -> None:
        self.assertEqual(source_relpath("demo.Pack"), "demo/Pack.java")
        self.assertEqual(class_relpath("demo.Pack"), "demo/Pack.class")
        # No metadata: `$` is a legal identifier, not an automatic inner-class strip.
        self.assertEqual(source_relpath("demo.Outer$Inner"), "demo/Outer$Inner.java")
        self.assertEqual(class_relpath("demo.Outer$Inner"), "demo/Outer$Inner.class")
        self.assertEqual(source_relpath("demo.Outer$1"), "demo/Outer$1.java")
        self.assertEqual(class_relpath("demo.Outer$1"), "demo/Outer$1.class")
        self.assertEqual(source_relpath("Dollar$Thing"), "Dollar$Thing.java")
        self.assertEqual(class_relpath("Dollar$Thing"), "Dollar$Thing.class")
        self.assertEqual(source_relpath("App"), "App.java")
        self.assertEqual(class_relpath("App"), "App.class")

    def test_rejects_absolute_slash_and_escaping_class_names(self) -> None:
        for bad in ("/tmp/Evil", "C:\\Evil", "../Evil", "foo/../bar", "foo/bar", "foo\\bar", "", "..", ".", "1Bad"):
            with self.assertRaises(InvalidBinaryName):
                source_relpath(bad)
            with self.assertRaises(InvalidBinaryName):
                class_relpath(bad)

    def test_legal_dollar_top_level_compiles_and_nested_uses_enclosing_metadata(self) -> None:
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        dollar_src = (
            "package demo;\n"
            "public class Dollar$Thing {\n"
            "  public static void main(String[] a) { System.out.println(7); }\n"
            "}\n"
        )
        outer_src = (
            "package demo;\n"
            "public class Outer {\n"
            "  public static class Inner {\n"
            "    public static int v() { return 7; }\n"
            "  }\n"
            "  public static void main(String[] a) { System.out.println(Inner.v()); }\n"
            "}\n"
        )
        with tempfile.TemporaryDirectory(prefix="t28-dollar-nested-") as raw:
            work = Path(raw)
            dollar_path = work / source_relpath("demo.Dollar$Thing", source=dollar_src)
            dollar_path.parent.mkdir(parents=True, exist_ok=True)
            dollar_path.write_text(dollar_src, encoding="utf-8")
            self.assertEqual(dollar_path.name, "Dollar$Thing.java")
            compiled = compile_sources(ident, [dollar_path], work / "dollar", debug="nodebug")
            self.assertEqual(compiled["rc"], 0, compiled.get("stderr"))
            self.assertTrue((work / "dollar" / "demo" / "Dollar$Thing.class").is_file())
            stripped = work / "Dollar.java"
            stripped.write_text(dollar_src, encoding="utf-8")
            compiled_bad = compile_sources(ident, [stripped], work / "dollar-bad", debug="nodebug")
            self.assertNotEqual(compiled_bad["rc"], 0)

            outer_path = work / source_relpath("demo.Outer", source=outer_src)
            outer_path.parent.mkdir(parents=True, exist_ok=True)
            outer_path.write_text(outer_src, encoding="utf-8")
            compiled_o = compile_sources(ident, [outer_path], work / "outer", debug="nodebug")
            self.assertEqual(compiled_o["rc"], 0, compiled_o.get("stderr"))
            inner_bytes = (work / "outer" / "demo" / "Outer$Inner.class").read_bytes()
            info = class_enclosing_info(inner_bytes)
            self.assertEqual(info["kind"], "nested_member")
            self.assertEqual(info["enclosing"], "demo.Outer")
            self.assertEqual(
                source_relpath("demo.Outer$Inner", class_bytes=inner_bytes),
                "demo/Outer.java",
            )
            self.assertEqual(
                source_relpath("demo.Outer$Inner", source=outer_src),
                "demo/Outer.java",
            )
            top_info = class_enclosing_info((work / "dollar" / "demo" / "Dollar$Thing.class").read_bytes())
            self.assertEqual(top_info["kind"], "top_level")
            self.assertIsNone(top_info["enclosing"])

    def test_packaged_public_class_rejects_dotted_filename(self) -> None:
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        src = (
            "package demo;\n"
            "public class Pack {\n"
            "  public static void main(String[] a) { System.out.println(7); }\n"
            "}\n"
        )
        with tempfile.TemporaryDirectory(prefix="t28-pack-path-") as raw:
            work = Path(raw)
            good = work / source_relpath("demo.Pack", source=src)
            good.parent.mkdir(parents=True)
            good.write_text(src, encoding="utf-8")
            compiled = compile_sources(ident, [good], work / "ok", debug="nodebug")
            self.assertEqual(compiled["rc"], 0, compiled.get("stderr"))
            self.assertTrue((work / "ok" / "demo" / "Pack.class").is_file())
            bad = work / "demo.Pack.java"
            bad.write_text(src, encoding="utf-8")
            compiled_bad = compile_sources(ident, [bad], work / "bad", debug="nodebug")
            self.assertNotEqual(compiled_bad["rc"], 0, "dotted filename must fail javac for public Pack")

    def test_escaping_class_name_is_invalid_input_not_an_artifact_write(self) -> None:
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        src = "public class Safe { public static void main(String[] a) { System.out.println(7); } }\n"
        with tempfile.TemporaryDirectory(prefix="t28-escape-name-") as raw:
            work = Path(raw)
            result = run_pipeline(src, ident, work, class_name="../Evil", trusted=True)
            self.assertEqual(result.failure_class, "invalid_input")
            self.assertFalse(any(work.rglob("Evil.java")))


class TestUntrustedHostJavaGate(unittest.TestCase):
    def test_untrusted_flag_never_invokes_host_java(self) -> None:
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        with tempfile.TemporaryDirectory(prefix="t28-untrusted-") as raw:
            work = Path(raw)
            src = FIXTURES / "Baseline.java"
            compiled = compile_sources(ident, [src], work, debug="nodebug")
            self.assertEqual(compiled["rc"], 0)
            real = identity_mod._run
            with mock.patch("tools.generators_holdout.identity._run", wraps=real) as host_run:
                host_run.side_effect = _no_host_java(real)
                with mock.patch(
                    "tools.sandbox_worker.untrusted_exec.SandboxWorker"
                ) as worker_cls:
                    worker_cls.return_value.run.return_value = _obs(
                        did_execute=False,
                        status="unsupported",
                        reason="capability_unsupported",
                        exit_code=1,
                        stdout="",
                    )
                    ran = verify_and_run(work, "Baseline", java_bin=java_command(ident), trusted=False)
            host_java = [c for c in host_run.call_args_list if c.args and Path(c.args[0][0]).name == "java"]
            self.assertEqual(host_java, [])
            self.assertFalse(ran.get("host_java"))
            self.assertFalse(ran.get("verified_and_ran"))
            self.assertNotEqual(ran.get("stage"), "run_ok")
            self.assertEqual(ran.get("stage"), "capability_unsupported")

    def test_cleanup_fail_zero_exit_is_never_pass_through_pipeline(self) -> None:
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        src = (FIXTURES / "Baseline.java").read_text(encoding="utf-8")
        with tempfile.TemporaryDirectory(prefix="t28-pipe-untrusted-") as raw:
            work = Path(raw)
            compiled = compile_sources(ident, [FIXTURES / "Baseline.java"], work / "cls", debug="nodebug")
            self.assertEqual(compiled["rc"], 0)
            class_bytes = (work / "cls" / "Baseline.class").read_bytes()
            calls = {"n": 0}

            def fake_run(_job):
                calls["n"] += 1
                if calls["n"] == 1:
                    return _obs()
                return _obs(
                    status="infra_error",
                    reason="leftover_process",
                    exit_code=0,
                    stdout="7\n",
                    leftover_host_pids=[9],
                    extra={
                        "leftover_cleanup_failed": True,
                        "leftover_verified": False,
                        "leftover_query_failed": False,
                    },
                )

            real = identity_mod._run
            with mock.patch("tools.generators_holdout.identity._run", wraps=real) as host_run:
                host_run.side_effect = _no_host_java(real)
                with mock.patch("tools.sandbox_worker.untrusted_exec.SandboxWorker") as worker_cls:
                    worker_cls.return_value.run.side_effect = fake_run
                    result = run_pipeline_from_class_bytes(
                        src,
                        class_bytes,
                        ident,
                        Path(raw) / "pipe",
                        mode="precision",
                        trusted=False,
                    )
            java_calls = [c for c in host_run.call_args_list if c.args and Path(c.args[0][0]).name == "java"]
            self.assertEqual(java_calls, [], msg=java_calls)
            self.assertEqual(calls["n"], 2, msg="original and rebuilt workers must both run")
            rebuild = result.stages.get("rebuild_verify_run")
            self.assertIsNotNone(rebuild, msg=result.to_dict())
            self.assertEqual(rebuild.status, "leftover_process", msg=result.to_dict())
            self.assertEqual(result.failure_class, "infra_error", msg=result.to_dict())
            self.assertNotEqual(rebuild.status, "run_ok")
            self.assertNotEqual(result.failure_class, "pass")

    def test_zero_exit_cleanup_fail_is_not_run_ok_stage(self) -> None:
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        with tempfile.TemporaryDirectory(prefix="t28-leftover-stage-") as raw:
            work = Path(raw)
            compiled = compile_sources(ident, [FIXTURES / "Baseline.java"], work, debug="nodebug")
            self.assertEqual(compiled["rc"], 0)
            with mock.patch("tools.sandbox_worker.untrusted_exec.SandboxWorker") as worker_cls:
                worker_cls.return_value.run.return_value = _obs(
                    status="ok",
                    reason="ok",
                    exit_code=0,
                    stdout="7\n",
                    leftover_host_pids=[77],
                    extra={"leftover_cleanup_failed": True, "leftover_verified": False},
                )
                ran = verify_and_run(work, "Baseline", trusted=False)
            self.assertFalse(ran["verified_and_ran"])
            self.assertEqual(ran["stage"], "leftover_process")
            self.assertNotEqual(ran["stage"], "run_ok")

    def test_trusted_fixture_may_use_host_java(self) -> None:
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        with tempfile.TemporaryDirectory(prefix="t28-trusted-") as raw:
            work = Path(raw)
            src = FIXTURES / "Baseline.java"
            compiled = compile_sources(ident, [src], work, debug="nodebug")
            self.assertEqual(compiled["rc"], 0)
            ran = verify_and_run(work, "Baseline", java_bin=java_command(ident), trusted=True)
            self.assertTrue(ran["verified_and_ran"])
            self.assertEqual(ran["stdout"], "7\n")

    def test_benign_packaged_class_original_and_rebuilt_in_docker(self) -> None:
        _require_docker(self)
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        src = (
            "package demo;\n"
            "public class Pack {\n"
            "  public static void main(String[] a) { System.out.println(7); }\n"
            "}\n"
        )
        with tempfile.TemporaryDirectory(prefix="t28-pkg-docker-") as raw:
            work = Path(raw)
            src_file = work / "Pack.java"
            src_file.write_text(src, encoding="utf-8")
            dest = work / "classes"
            compiled = compile_sources(ident, [src_file], dest, debug="nodebug")
            self.assertEqual(compiled["rc"], 0, compiled.get("stderr"))
            class_file = dest / "demo" / "Pack.class"
            self.assertTrue(class_file.is_file(), class_file)
            real = identity_mod._run
            with mock.patch("tools.generators_holdout.identity._run", wraps=real) as host_run:
                host_run.side_effect = _no_host_java(real)
                result = run_pipeline_from_class_bytes(
                    src,
                    class_file.read_bytes(),
                    ident,
                    work / "pipe",
                    mode="precision",
                    class_name="demo.Pack",
                    trusted=False,
                )
            java_calls = [c for c in host_run.call_args_list if c.args and Path(c.args[0][0]).name == "java"]
            self.assertEqual(java_calls, [], msg="host java must not run packaged class")
            self.assertEqual(result.failure_class, "pass", msg=result.to_dict())
            orig = result.stages["original_verify_run"]
            rebuilt = result.stages["rebuild_verify_run"]
            self.assertEqual(orig.status, "run_ok", msg=orig.to_dict())
            self.assertEqual(rebuilt.status, "run_ok", msg=rebuilt.to_dict())
            self.assertEqual(result.original_stdout.strip(), "7")
            self.assertEqual(result.rebuilt_stdout.strip(), "7")
            orig_argv = _docker_argv(orig)
            rebuilt_argv = _docker_argv(rebuilt)
            self.assertNotEqual(orig_argv, [])
            self.assertNotEqual(rebuilt_argv, [])
            rebuild_compile = result.stages["rebuild_compile"]
            compile_argv = " ".join(rebuild_compile.argv)
            self.assertIn("demo/Pack.java", compile_argv.replace("\\", "/"))
            self.assertNotIn("demo.Pack.java", compile_argv)
            self.assertTrue((work / "pipe" / "rebuilt-precision" / "demo" / "Pack.java").is_file())


class TestExtraCpAndLimits(unittest.TestCase):
    def test_limits_timeout_replace_preserves_all_fields(self) -> None:
        custom = Limits(
            memory_bytes=1234567,
            pids=9,
            cpu_seconds=7,
            cpus="0.2",
            output_bytes=50,
            artifact_bytes=51,
            fsize_bytes=111,
            timeout_seconds=30,
            nofile=13,
        )
        got = _limits_for_timeout(custom, 5)
        self.assertEqual(got.timeout_seconds, 5)
        self.assertEqual(got.memory_bytes, 1234567)
        self.assertEqual(got.pids, 9)
        self.assertEqual(got.cpu_seconds, 7)
        self.assertEqual(got.cpus, "0.2")
        self.assertEqual(got.output_bytes, 50)
        self.assertEqual(got.artifact_bytes, 51)
        self.assertEqual(got.fsize_bytes, 111)
        self.assertEqual(got.nofile, 13)
        defaulted = _limits_for_timeout(DEFAULT_UNTRUSTED_LIMITS, 10)
        self.assertEqual(defaulted.fsize_bytes, DEFAULT_UNTRUSTED_LIMITS.fsize_bytes)
        self.assertEqual(defaulted.nofile, DEFAULT_UNTRUSTED_LIMITS.nofile)
        self.assertEqual(defaulted.cpu_seconds, DEFAULT_UNTRUSTED_LIMITS.cpu_seconds)

    def test_rel_files_file_jar_and_dir_jars_on_classpath(self) -> None:
        with tempfile.TemporaryDirectory(prefix="t28-rel-files-") as raw:
            root = Path(raw)
            (root / "app").mkdir()
            (root / "app" / "Main.class").write_bytes(b"class")
            nested_in_classdir = root / "app" / "lib.jar"
            nested_in_classdir.write_bytes(b"PK\x05\x06" + b"\x00" * 18)
            jar = root / "dep.jar"
            jar.write_bytes(b"PK\x05\x06" + b"\x00" * 18)
            files, cp = _rel_files(root / "app", [jar])
            self.assertIn("Main.class", files)
            self.assertNotIn("lib.jar", files)
            self.assertTrue(any(k.endswith("dep.jar") for k in files))
            self.assertEqual(cp[0], "/inputs")
            self.assertTrue(any(e.endswith("dep.jar") for e in cp), cp)
            self.assertFalse(any(e.endswith("lib.jar") for e in cp), cp)

            extra_dir = root / "libdir"
            (extra_dir / "lib").mkdir(parents=True)
            (extra_dir / "lib" / "Lib.class").write_bytes(b"libclass")
            nested_jar = extra_dir / "more.jar"
            nested_jar.write_bytes(b"PK\x05\x06" + b"\x00" * 18)
            files2, cp2 = _rel_files(root / "app", [extra_dir])
            self.assertIn("extra0/lib/Lib.class", files2)
            self.assertNotIn("extra0/more.jar", files2)
            self.assertEqual(cp2, ["/inputs", "/inputs/extra0"])

            files3, cp3 = _rel_files(root / "app", [extra_dir, jar])
            self.assertEqual(cp3, ["/inputs", "/inputs/extra0", "/inputs/extra1/dep.jar"])

    def test_rel_files_missing_and_symlink_are_not_silent(self) -> None:
        with tempfile.TemporaryDirectory(prefix="t28-rel-neg-") as raw:
            root = Path(raw)
            (root / "app").mkdir()
            (root / "app" / "Main.class").write_bytes(b"class")
            with self.assertRaises(StagingError) as missing:
                _rel_files(root / "app", [root / "no-such.jar"])
            self.assertEqual(missing.exception.stage, "invalid_input")
            txt = root / "notes.txt"
            txt.write_text("nope", encoding="utf-8")
            with self.assertRaises(StagingError) as unsupported:
                _rel_files(root / "app", [txt])
            self.assertEqual(unsupported.exception.stage, "unsupported")
            outside = root / "outside.jar"
            outside.write_bytes(b"PK")
            link = root / "escape.jar"
            link.symlink_to(outside)
            # extra_cp path itself is a symlink
            with self.assertRaises(StagingError) as linked:
                _rel_files(root / "app", [link])
            self.assertEqual(linked.exception.stage, "invalid_input")

            outside_dir = root / "outside-dir"
            outside_dir.mkdir()
            (outside_dir / "Leak.class").write_bytes(b"leak")
            nested_link = root / "app" / "link"
            nested_link.symlink_to(outside_dir)
            with self.assertRaises(StagingError) as dirlink:
                _rel_files(root / "app", None)
            self.assertEqual(dirlink.exception.stage, "invalid_input")
            self.assertIn("directory symlink", str(dirlink.exception))

            nested_link.unlink()
            file_link = root / "app" / "Escaped.class"
            file_link.symlink_to(root / "outside.jar")
            with self.assertRaises(StagingError) as flink:
                _rel_files(root / "app", None)
            self.assertEqual(flink.exception.stage, "invalid_input")

    def test_main_depending_on_file_jar_and_classdir_in_docker(self) -> None:
        _require_docker(self)
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        lib_src = (
            "package lib;\n"
            "public class Lib {\n"
            "  public static int value() { return 7; }\n"
            "}\n"
        )
        main_src = (
            "package app;\n"
            "public class Main {\n"
            "  public static void main(String[] a) { System.out.println(lib.Lib.value()); }\n"
            "}\n"
        )
        with tempfile.TemporaryDirectory(prefix="t28-extra-cp-") as raw:
            work = Path(raw)
            (work / "Lib.java").write_text(lib_src, encoding="utf-8")
            (work / "Main.java").write_text(main_src, encoding="utf-8")
            compiled = compile_sources(
                ident, [work / "Lib.java", work / "Main.java"], work / "all", debug="nodebug"
            )
            self.assertEqual(compiled["rc"], 0, compiled.get("stderr"))
            main_dir = work / "main"
            (main_dir / "app").mkdir(parents=True)
            os.replace(work / "all" / "app" / "Main.class", main_dir / "app" / "Main.class")
            jar = work / "lib.jar"
            with zipfile.ZipFile(jar, "w") as zf:
                zf.write(work / "all" / "lib" / "Lib.class", "lib/Lib.class")
            lib_dir = work / "libdir"
            (lib_dir / "lib").mkdir(parents=True)
            os.replace(work / "all" / "lib" / "Lib.class", lib_dir / "lib" / "Lib.class")

            custom = Limits(
                memory_bytes=256 * 1024 * 1024,
                pids=64,
                cpu_seconds=19,
                cpus="0.5",
                output_bytes=200 * 1024,
                artifact_bytes=200 * 1024,
                fsize_bytes=400 * 1024,
                timeout_seconds=30,
                nofile=180,
            )
            real = identity_mod._run
            with mock.patch("tools.generators_holdout.identity._run", wraps=real) as host_run:
                host_run.side_effect = _no_host_java(real)
                jar_run = run_untrusted_class_dir(
                    main_dir,
                    "app.Main",
                    extra_cp=[jar],
                    timeout=20,
                    limits=custom,
                )
                dir_run = run_untrusted_class_dir(
                    main_dir,
                    "app.Main",
                    extra_cp=[lib_dir],
                    timeout=20,
                    limits=custom,
                )
            java_calls = [c for c in host_run.call_args_list if c.args and Path(c.args[0][0]).name == "java"]
            self.assertEqual(java_calls, [])
            self.assertEqual(jar_run["stage"], "run_ok", msg=jar_run)
            self.assertEqual(jar_run["stdout"].strip(), "7", msg=jar_run)
            self.assertTrue(jar_run["verified_and_ran"], msg=jar_run)
            self.assertTrue(any(e.endswith("lib.jar") for e in jar_run["cp_entries"]), jar_run["cp_entries"])
            self.assertEqual(dir_run["stage"], "run_ok", msg=dir_run)
            self.assertEqual(dir_run["stdout"].strip(), "7", msg=dir_run)
            self.assertTrue(dir_run["verified_and_ran"], msg=dir_run)
            self.assertIn("/inputs/extra0", dir_run["cp_entries"])
            self.assertEqual(jar_run["limits_fsize_bytes"], 400 * 1024)
            self.assertEqual(jar_run["limits_nofile"], 180)
            self.assertEqual(jar_run["limits_cpu_seconds"], 19)
            self.assertEqual(jar_run["limits"]["timeout_seconds"], 20)
            self.assertEqual(jar_run["limits"]["fsize_bytes"], 400 * 1024)
            self.assertEqual(jar_run["limits"]["output_bytes"], 200 * 1024)
            from tools.sandbox_worker.constants import CONTAINER_JDK, TOOLCHAIN_IMAGE

            self.assertEqual(jar_run.get("worker_image"), TOOLCHAIN_IMAGE, jar_run)
            self.assertEqual(jar_run.get("worker_java_home"), CONTAINER_JDK, jar_run)
            argv = jar_run.get("argv") or []
            self.assertTrue(argv and Path(str(argv[0])).name in {"docker", "podman"}, argv)
            argv_blob = " ".join(str(x) for x in argv)
            self.assertIn("fsize=409600:409600", argv_blob)
            self.assertIn("nofile=180:180", argv_blob)

            missing = run_untrusted_class_dir(main_dir, "app.Main", extra_cp=[work / "missing.jar"])
            self.assertEqual(missing["stage"], "invalid_input")
            self.assertFalse(missing["verified_and_ran"])
            self.assertFalse(any(e.endswith("more.jar") for e in dir_run.get("cp_entries") or []))

    def test_conflicting_lib_dir_vs_nested_jar_matches_host_cp_order(self) -> None:
        _require_docker(self)
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        main_src = (
            "package app;\n"
            "public class Main {\n"
            "  public static void main(String[] a) { System.out.println(lib.Lib.value()); }\n"
            "}\n"
        )
        dir_lib_src = "package lib;\npublic class Lib { public static int value() { return 1; } }\n"
        jar_lib_src = "package lib;\npublic class Lib { public static int value() { return 2; } }\n"
        with tempfile.TemporaryDirectory(prefix="t28-cp-order-") as raw:
            work = Path(raw)
            (work / "Main.java").write_text(main_src, encoding="utf-8")
            dir_src = work / "dirsrc"
            jar_src = work / "jarsrc"
            dir_src.mkdir()
            jar_src.mkdir()
            (dir_src / "Lib.java").write_text(dir_lib_src, encoding="utf-8")
            (jar_src / "Lib.java").write_text(jar_lib_src, encoding="utf-8")
            compiled_main = compile_sources(
                ident, [work / "Main.java", dir_src / "Lib.java"], work / "mainall", debug="nodebug"
            )
            self.assertEqual(compiled_main["rc"], 0, compiled_main.get("stderr"))
            self.assertEqual(compile_sources(ident, [jar_src / "Lib.java"], work / "jarlib", debug="nodebug")["rc"], 0)
            main_dir = work / "main"
            (main_dir / "app").mkdir(parents=True)
            os.replace(work / "mainall" / "app" / "Main.class", main_dir / "app" / "Main.class")
            lib_dir = work / "libdir"
            (lib_dir / "lib").mkdir(parents=True)
            os.replace(work / "mainall" / "lib" / "Lib.class", lib_dir / "lib" / "Lib.class")
            jar_path = work / "lib-jar.jar"
            with zipfile.ZipFile(jar_path, "w") as zf:
                zf.write(work / "jarlib" / "lib" / "Lib.class", "lib/Lib.class")
            with zipfile.ZipFile(lib_dir / "hidden.jar", "w") as zf:
                zf.write(work / "jarlib" / "lib" / "Lib.class", "lib/Lib.class")

            host = verify_and_run(main_dir, "app.Main", extra_cp=[lib_dir], trusted=True, java_bin=java_command(ident))
            self.assertEqual(host["stage"], "run_ok", msg=host)
            self.assertEqual(host["stdout"].strip(), "1", msg="host -cp dir must not load nested jar")

            real = identity_mod._run
            with mock.patch("tools.generators_holdout.identity._run", wraps=real) as host_run:
                host_run.side_effect = _no_host_java(real)
                iso_dir = run_untrusted_class_dir(main_dir, "app.Main", extra_cp=[lib_dir])
                iso_jar = run_untrusted_class_dir(main_dir, "app.Main", extra_cp=[jar_path])
                dir_then_jar = run_untrusted_class_dir(main_dir, "app.Main", extra_cp=[lib_dir, jar_path])
                jar_then_dir = run_untrusted_class_dir(main_dir, "app.Main", extra_cp=[jar_path, lib_dir])
            self.assertEqual([c for c in host_run.call_args_list if c.args and Path(c.args[0][0]).name == "java"], [])
            self.assertEqual(iso_dir["stage"], "run_ok", msg=iso_dir)
            self.assertTrue(iso_dir["verified_and_ran"], msg=iso_dir)
            self.assertEqual(iso_dir["stdout"].strip(), host["stdout"].strip())
            self.assertEqual(iso_dir["cp_entries"], ["/inputs", "/inputs/extra0"])
            self.assertFalse(any(str(e).endswith("hidden.jar") for e in iso_dir["cp_entries"]))
            for arm, expect in ((iso_jar, "2"), (dir_then_jar, "1"), (jar_then_dir, "2")):
                self.assertEqual(arm["stage"], "run_ok", msg=arm)
                self.assertTrue(arm["verified_and_ran"], msg=arm)
                self.assertEqual(arm["stdout"].strip(), expect, msg=arm)
            self.assertEqual(dir_then_jar["cp_entries"][0], "/inputs")
            self.assertTrue(dir_then_jar["cp_entries"][1].endswith("extra0"))
            self.assertTrue(dir_then_jar["cp_entries"][2].endswith("lib-jar.jar"))


class TestDollarAndNestedPipeline(unittest.TestCase):
    def test_dollar_thing_original_and_rebuilt_in_docker(self) -> None:
        _require_docker(self)
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        src = (
            "package demo;\n"
            "public class Dollar$Thing {\n"
            "  public static void main(String[] a) { System.out.println(7); }\n"
            "}\n"
        )
        with tempfile.TemporaryDirectory(prefix="t28-dollar-docker-") as raw:
            work = Path(raw)
            result = run_pipeline(src, ident, work / "pipe", mode="precision", trusted=False)
            self.assertEqual(result.failure_class, "pass", msg=result.to_dict())
            self.assertEqual(result.stages["original_verify_run"].status, "run_ok")
            self.assertEqual(result.stages["rebuild_verify_run"].status, "run_ok")
            self.assertEqual(result.original_stdout.strip(), "7")
            self.assertEqual(result.rebuilt_stdout.strip(), "7")
            _docker_argv(result.stages["original_verify_run"])
            _docker_argv(result.stages["rebuild_verify_run"])
            compile_argv = " ".join(result.stages["rebuild_compile"].argv)
            self.assertIn("Dollar$Thing.java", compile_argv.replace("\\", "/"))
            self.assertNotIn("/Dollar.java", compile_argv.replace("\\", "/"))
            self.assertTrue((work / "pipe" / "rebuilt-precision" / "demo" / "Dollar$Thing.java").is_file())

    def test_nested_outer_supported_inner_without_enclosing_cu_unsupported(self) -> None:
        _require_docker(self)
        compilers = _compilers()
        ident = compilers["javac21_release8"]
        outer_src = (
            "package demo;\n"
            "public class Outer {\n"
            "  public static class Inner {\n"
            "    public static void main(String[] a) { System.out.println(7); }\n"
            "  }\n"
            "  public static void main(String[] a) { Inner.main(a); }\n"
            "}\n"
        )
        inner_only = (
            "package demo;\n"
            "public class Inner {\n"
            "  public static void main(String[] a) { System.out.println(7); }\n"
            "}\n"
        )
        with tempfile.TemporaryDirectory(prefix="t28-nested-fam-") as raw:
            work = Path(raw)
            outer_result = run_pipeline(outer_src, ident, work / "outer", mode="precision", trusted=False)
            self.assertEqual(outer_result.stages["compile"].status, "ok", msg=outer_result.to_dict())
            self.assertEqual(outer_result.stages["original_verify_run"].status, "run_ok")
            self.assertEqual(outer_result.original_stdout.strip(), "7")
            self.assertTrue((work / "outer" / "artifacts" / "demo" / "Outer.java").is_file())
            # Rebuild of Outer is a decompiler oracle (Inner body may be omitted) — not a harness path pass.
            self.assertIn(outer_result.failure_class, {"pass", "behavior"}, msg=outer_result.to_dict())
            inner_bytes = (work / "outer" / "original" / "demo" / "Outer$Inner.class").read_bytes()
            info = class_enclosing_info(inner_bytes)
            self.assertEqual(info["kind"], "nested_member")
            self.assertEqual(info["enclosing"], "demo.Outer")
            from tools.generators_holdout.pipeline import nested_rebuild_unsupported

            self.assertIsNotNone(nested_rebuild_unsupported(inner_only, inner_bytes))
            inner_result = run_pipeline_from_class_bytes(
                inner_only,
                inner_bytes,
                ident,
                work / "inner-only",
                mode="precision",
                class_name="demo.Outer$Inner",
                trusted=False,
            )
            self.assertEqual(inner_result.stages["original_verify_run"].status, "run_ok", msg=inner_result.to_dict())
            self.assertEqual(inner_result.original_stdout.strip(), "7")
            self.assertEqual(inner_result.failure_class, "unsupported", msg=inner_result.to_dict())
            self.assertEqual(inner_result.stages["rebuild_compile"].status, "unsupported")


if __name__ == "__main__":
    unittest.main()

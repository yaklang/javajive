import hashlib
import json
import tempfile
import unittest
import zipfile
from pathlib import Path

from jdk_interface_code_generate import extend


class IndexedCodeGenerationTest(unittest.TestCase):
    def fixture(self, root):
        archive, catalog, bundle = [root / name for name in ("original.zip", "catalog.json", "bundle.zip")]
        payloads = {"p/Base": b"original-base", "p/Child": b"original-child"}
        with zipfile.ZipFile(archive, "w") as z:
            for name, raw in payloads.items():
                z.writestr(name + ".class", raw)
        archive_hash = hashlib.sha256(archive.read_bytes()).hexdigest()
        profile = {"release": 8, "archive_sha256": archive_hash,
                   "classes": {"p/Base": {"IsInterface": False, "Parents": []},
                               "p/Child": {"IsInterface": False, "Parents": ["p/Base"]}},
                   "provenance": {name: {"archive": archive.name, "entry": name + ".class",
                                         "sha256": hashlib.sha256(raw).hexdigest()}
                                  for name, raw in payloads.items()}}
        catalog.write_text(json.dumps({"profiles": [profile]}))
        manifest = {"schema": 1, "profiles": [{"release": 8, "archive_sha256": archive_hash,
                     "classes": {"p/Base": hashlib.sha256(payloads["p/Base"]).hexdigest()}}]}
        with zipfile.ZipFile(bundle, "w") as z:
            z.writestr("manifest.json", json.dumps(manifest))
            z.writestr("8/p/Base.class", payloads["p/Base"])
            z.writestr("legal/NOTICE", b"unchanged original notice")
        return archive, catalog, bundle, profile, payloads

    def test_concrete_declarations_and_ancestry_use_exact_bytes(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, catalog, bundle, _, payloads = self.fixture(Path(directory))
            extend(catalog, bundle, {"8": str(archive)})
            with zipfile.ZipFile(bundle) as z:
                for name, raw in payloads.items():
                    self.assertEqual(z.read("8/" + name + ".class"), raw)
                self.assertEqual(z.read("legal/NOTICE"), b"unchanged original notice")
            first = bundle.read_bytes()
            extend(catalog, bundle, {"8": str(archive)})
            self.assertEqual(bundle.read_bytes(), first)

    def test_inconsistent_original_evidence_does_not_replace_bundle(self):
        for mutation in ("archive", "entry", "ancestor"):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as directory:
                archive, catalog, bundle, profile, _ = self.fixture(Path(directory))
                before = bundle.read_bytes()
                if mutation == "archive":
                    with zipfile.ZipFile(archive, "a") as z:
                        z.writestr("extra.class", b"foreign")
                elif mutation == "entry":
                    profile["provenance"]["p/Child"]["sha256"] = "0" * 64
                else:
                    profile["classes"]["p/Child"]["Parents"] = ["p/Missing"]
                catalog.write_text(json.dumps({"profiles": [profile]}))
                with self.assertRaises((AssertionError, KeyError)):
                    extend(catalog, bundle, {"8": str(archive)})
                self.assertEqual(bundle.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()

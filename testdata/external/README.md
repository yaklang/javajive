# External container samples

Real-world Java and Android container files used by `test/external` and the
package-level format tests. The files are treated as **data only**; tests parse
them and never execute their contents.

## Running the opt-in tests

The real external corpus is excluded from default test runs. Run it explicitly
with:

```bash
JAVAJIVE_EXTERNAL_EVAL=1 go test ./test/external
```

Tests that read a locally installed JDK runtime image or generate images with
`jlink` are independently opt-in:

```bash
JAVAJIVE_JIMAGE_INTEGRATION=1 go test ./jimage ./test/external
```

Synthetic JImage unit tests remain part of ordinary `go test ./jimage` runs.

## Sources and licenses

All samples come from Apache-2.0 projects. Keep the source URL in this file when
replacing a sample.

| File | Source | License |
| --- | --- | --- |
| `tomcat-context.war` | <https://github.com/apache/tomcat/blob/main/test/deployment/context.war> | Apache-2.0 |
| `tomcat-sample.war` | <https://github.com/apache/tomcat/blob/main/webapps/docs/appdev/sample/sample.war> | Apache-2.0 |
| `tomcat-broken.war` | <https://github.com/apache/tomcat/blob/main/test/deployment/broken.war> | Apache-2.0 |
| `bundletool-app.apk` | `google/bundletool` `src/test/resources/.../apk/com.test.app.apk` | Apache-2.0 |
| `bundletool-install-time.aab` | `google/bundletool` `src/test/resources/.../bundle/install-time-permanent-modules.aab` | Apache-2.0 |
| `bundletool-universal.apks` | Generated from `bundletool-install-time.aab` with bundletool 1.18.3 (`--mode=universal`) | Apache-2.0 |
| `bundletool-splits.apks` | Generated from `bundletool-install-time.aab` with bundletool 1.18.3 (`--mode=default`) | Apache-2.0 |
| `androguard-invalid.apk` | <https://github.com/androguard/androguard/blob/master/tests/data/APK/Invalid.apk> | Apache-2.0 |
| `androguard-test-debug.apk` | <https://github.com/androguard/androguard/blob/master/tests/data/APK/Test-debug.apk> | Apache-2.0 |
| `androguard-tc-debug.apk` | <https://github.com/androguard/androguard/blob/master/tests/data/APK/TC-debug.apk> | Apache-2.0 |
| `dex2jar-dex038.dex` | <https://github.com/pxb1988/dex2jar/blob/2.x/dex-translator/src/test/resources/dexes/dex038.dex> | Apache-2.0 |
| `dex2jar-dex039.dex` | <https://github.com/pxb1988/dex2jar/blob/2.x/dex-translator/src/test/resources/dexes/dex039.dex> | Apache-2.0 |
| `apktool-dynamic-dex.apk` | <https://github.com/iBotPeaches/Apktool/blob/main/brut.apktool/apktool-lib/src/test/resources/dynamic_dex/dynamic_dex.apk> | Apache-2.0 |
| `apktool-issue2543.dex` | <https://github.com/iBotPeaches/Apktool/blob/main/brut.apktool/apktool-lib/src/test/resources/issue2543/classes.dex> | Apache-2.0 |
| `androidx-tracing-1.2.0.aar` | <https://dl.google.com/dl/android/maven2/androidx/tracing/tracing/1.2.0/tracing-1.2.0.aar> | Apache-2.0 |
| `sample.xapk` | Synthetic XAPK built from `bundletool-app.apk` plus a resource-only split | Apache-2.0 |
| `sample.apkm` | Synthetic APKM built from `bundletool-app.apk` plus a resource-only split | Apache-2.0 |
| `sample.ear` | Synthetic EAR built from `tomcat-sample.war` plus a nested JAR | Apache-2.0 |

## Expected behavior

| Sample | Classification | Inspection | Operation |
| --- | --- | --- | --- |
| `tomcat-context.war` | `war` | complete | resource-only archive |
| `tomcat-sample.war` | `war` | complete | flat/structured export yields `WEB-INF/classes/mypackage/Hello.java` |
| `tomcat-broken.war` | `war` | complete | valid ZIP; the broken part is the web descriptor |
| `bundletool-app.apk` | `apk` | complete | `dex2jar` converts 1 DEX / 10 classes |
| `bundletool-install-time.aab` | `aab` | complete | `dex2jar` converts 3 DEX / 1,298 classes |
| `bundletool-universal.apks` | `apks` | complete | `dex2jar` prefers `universal.apk`; 3 DEX / 1,298 classes |
| `bundletool-splits.apks` | `apks` | complete | 181 nested APKs; 4 DEX / 1,298 classes after split dedupe |
| `androguard-invalid.apk` | `apk` | complete | valid DEX despite the fixture name; 286 classes |
| `androguard-test-debug.apk` | `apk` | complete | 7 classes |
| `androguard-tc-debug.apk` | `apk` | complete | 13 classes |
| `dex2jar-dex038.dex` | `dex` | partial | `dex2jar` rejects the `call_site_id` section (strict subset) |
| `dex2jar-dex039.dex` | `dex` | partial | `dex2jar` rejects the `method_handle` section (strict subset) |
| `apktool-dynamic-dex.apk` | `apk` | partial | `dex2jar` rejects the DEX SHA-1 mismatch |
| `apktool-issue2543.dex` | `dex` | complete | 1 class / 1 method |
| `androidx-tracing-1.2.0.aar` | `aar` | complete | AAR export decompiles `classes.jar` (3 classes) |
| `sample.xapk` | `xapk` | complete | `dex2jar` converts 1 DEX / 10 classes |
| `sample.apkm` | `apkm` | complete | `dex2jar` converts 1 DEX / 10 classes |
| `sample.ear` | `ear` | complete | `EAR/lib/sample.jar!/mypackage/Hello.java` is decompiled |

## Regenerating the generated samples

`bundletool-universal.apks` and `bundletool-splits.apks`:

```bash
java -jar bundletool-all-1.18.3.jar build-apks \
  --bundle=bundletool-install-time.aab \
  --output=bundletool-universal.apks --mode=universal --overwrite
java -jar bundletool-all-1.18.3.jar build-apks \
  --bundle=bundletool-install-time.aab \
  --output=bundletool-splits.apks --mode=default --overwrite
```

`sample.xapk`, `sample.apkm`, and `sample.ear` were built with this Python
script (standard library only):

```python
import io, json, sys, zipfile

base_apk, war, outdir = sys.argv[1], sys.argv[2], sys.argv[3]

def zip_bytes(entries):
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        for name, data in entries:
            z.writestr(name, data)
    return buf.getvalue()

apk = open(base_apk, "rb").read()
resource_apk = zip_bytes([("res/raw/config.txt", b"config")])

open(f"{outdir}/sample.xapk", "wb").write(zip_bytes([
    ("manifest.json", json.dumps({"xapk_version": 2, "package_name": "com.test.app"}).encode()),
    ("base.apk", apk),
    ("config.en.apk", resource_apk),
    ("icon.png", b"\x89PNG\r\n\x1a\n"),
]))
open(f"{outdir}/sample.apkm", "wb").write(zip_bytes([
    ("info.json", json.dumps({"name": "sample", "package_name": "com.test.app"}).encode()),
    ("base.apk", apk),
    ("split_config.en.apk", resource_apk),
]))

with zipfile.ZipFile(war) as z:
    hello = z.read("WEB-INF/classes/mypackage/Hello.class")
sample_jar = zip_bytes([
    ("META-INF/MANIFEST.MF", b"Manifest-Version: 1.0\r\n\r\n"),
    ("mypackage/Hello.class", hello),
])
open(f"{outdir}/sample.ear", "wb").write(zip_bytes([
    ("META-INF/application.xml", b'<?xml version="1.0" encoding="UTF-8"?>\n<application/>\n'),
    ("EAR/lib/sample.jar", sample_jar),
]))
```

The real 181-APK split bundle is useful for checking the nested-APK bound; the
translator default is 512 nested APKs and the test asserts the split bundle
translates the same 1,298 classes as the universal bundle.

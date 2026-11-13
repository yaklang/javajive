package dex2jar_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/internal/jimagefixture"
)

func TestDexToJarAPKS(t *testing.T) {
	apk, err := os.ReadFile(filepath.Join("testdata", "accept", "multidex.apk"))
	if err != nil {
		t.Fatal(err)
	}
	resourceSplit := makeAPKEntries(t, map[string][]byte{"res/raw/config.txt": []byte("config")})
	bundle := makeAPKEntries(t, map[string][]byte{
		"base.apk":  apk,
		"split.apk": resourceSplit,
	})
	jar, report, err := dex2jar.DexToJar(bundle, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexToJar: %v", err)
	}
	classes := jarClassNames(t, jar)
	if len(classes) != report.Classes || len(classes) < 2 {
		t.Fatalf("classes = %v, report.Classes = %d", classes, report.Classes)
	}
	if report.DEXFiles != 2 {
		t.Fatalf("DEX files = %d, want 2", report.DEXFiles)
	}
}

func TestDexToJarAAB(t *testing.T) {
	dex, err := os.ReadFile(filepath.Join("testdata", "accept", "arithmetic.dex"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := makeAPKEntries(t, map[string][]byte{
		"BundleConfig.pb":      {1},
		"base/dex/classes.dex": dex,
	})
	jar, report, err := dex2jar.DexToJar(bundle, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexToJar: %v", err)
	}
	classes := jarClassNames(t, jar)
	if len(classes) != 1 || classes[0] != "accept/Arithmetic.class" {
		t.Fatalf("classes = %v", classes)
	}
	if report.DEXFiles != 1 || report.Classes != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestDexToJarXAPK(t *testing.T) {
	apk, err := os.ReadFile(filepath.Join("testdata", "accept", "multidex.apk"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := makeAPKEntries(t, map[string][]byte{
		"manifest.json": []byte(`{"xapk_version":2}`),
		"base.apk":      apk,
	})
	jar, report, err := dex2jar.DexToJar(bundle, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexToJar: %v", err)
	}
	if len(jarClassNames(t, jar)) != report.Classes || report.Classes < 2 {
		t.Fatalf("report = %#v", report)
	}
}

func TestDexFileToJarAPKSAndClassification(t *testing.T) {
	apk, err := os.ReadFile(filepath.Join("testdata", "accept", "multidex.apk"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := makeAPKEntries(t, map[string][]byte{"base.apk": apk})
	src := filepath.Join(t.TempDir(), "app.apks")
	if err := os.WriteFile(src, bundle, 0o644); err != nil {
		t.Fatal(err)
	}
	kind, err := dex2jar.ClassifyInputFile(src)
	if err != nil {
		t.Fatalf("ClassifyInputFile: %v", err)
	}
	if kind != dex2jar.ContainerAPKS {
		t.Fatalf("kind = %q, want apks", kind)
	}
	dst := filepath.Join(t.TempDir(), "classes.jar")
	report, err := dex2jar.DexFileToJar(src, dst, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexFileToJar: %v", err)
	}
	if report.Classes < 2 {
		t.Fatalf("report = %#v", report)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("output JAR: %v", err)
	}
}

func TestClassifyInputJMOD(t *testing.T) {
	payload := makeAPK(t, []byte("dex\n035\x00"))
	jmod := append([]byte{'J', 'M', 1, 0}, payload...)
	if got := dex2jar.ClassifyInput(jmod, "java.base.jmod"); got != dex2jar.ContainerJMOD {
		t.Fatalf("ClassifyInput = %q, want jmod", got)
	}
	if got := dex2jar.DetectInputKind(jmod); got != dex2jar.InputJMOD {
		t.Fatalf("DetectInputKind = %v, want InputJMOD", got)
	}
}

func TestClassifyInputJImage(t *testing.T) {
	data, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/demo/Foo.class", Data: []byte{0xca, 0xfe, 0xba, 0xbe}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := dex2jar.ClassifyInput(data, "modules"); got != dex2jar.ContainerJImage {
		t.Fatalf("ClassifyInput = %q, want jimage", got)
	}
	if got := dex2jar.DetectInputKind(data); got != dex2jar.InputJIMAGE {
		t.Fatalf("DetectInputKind = %v, want InputJIMAGE", got)
	}
	if _, _, err := dex2jar.DexToJar(data, dex2jar.Options{}); err == nil || !strings.Contains(err.Error(), "unsupported DEX input") {
		t.Fatalf("DexToJar error = %v", err)
	}
}

func TestDexToJarRejectsJMOD(t *testing.T) {
	payload := makeAPK(t, []byte("dex\n035\x00"))
	jmod := append([]byte{'J', 'M', 1, 0}, payload...)
	_, _, err := dex2jar.DexToJar(jmod, dex2jar.Options{})
	if err == nil || !strings.Contains(err.Error(), "unsupported DEX input") {
		t.Fatalf("error = %v", err)
	}
}

func jarClassNames(t *testing.T, raw []byte) []string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open JAR: %v", err)
	}
	var names []string
	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".class") {
			names = append(names, file.Name)
		}
	}
	sort.Strings(names)
	return names
}

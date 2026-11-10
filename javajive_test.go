package javajive_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jj "github.com/yaklang/javajive"
	"github.com/yaklang/javajive/internal/jimagefixture"
)

const helloStringStream = "aced000574000568656c6c6f"

func testdataClass(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("classparser", "testdata", "invisible_anno.class"))
	if err != nil {
		t.Fatalf("read testdata class: %v", err)
	}
	return data
}

func TestDecompileExport(t *testing.T) {
	src, err := jj.Decompile(testdataClass(t))
	if err != nil {
		t.Fatalf("Decompile: %v", err)
	}
	if !strings.Contains(src, "class InvisibleAnnoSeed") {
		t.Fatalf("unexpected source:\n%s", src)
	}
}

func TestParseClassExport(t *testing.T) {
	obj, err := jj.ParseClass(testdataClass(t))
	if err != nil {
		t.Fatalf("ParseClass: %v", err)
	}
	if got := obj.GetClassName(); got != "InvisibleAnnoSeed" {
		t.Fatalf("class name = %q", got)
	}
	if got := obj.GetSupperClassName(); got != "java/lang/Object" {
		t.Fatalf("super name = %q", got)
	}
}

func TestSerializationRoundTripExport(t *testing.T) {
	objs, err := jj.ParseSerializedHex(helloStringStream)
	if err != nil {
		t.Fatalf("ParseSerializedHex: %v", err)
	}
	if got := jj.MarshalSerializedHex(objs...); got != helloStringStream {
		t.Fatalf("marshal round-trip = %s want %s", got, helloStringStream)
	}

	jsonBytes, err := jj.SerializedToJSON(objs...)
	if err != nil {
		t.Fatalf("SerializedToJSON: %v", err)
	}
	if !strings.Contains(string(jsonBytes), "TC_STRING") {
		t.Fatalf("json missing TC_STRING: %s", jsonBytes)
	}

	restored, err := jj.SerializedFromJSON(jsonBytes)
	if err != nil {
		t.Fatalf("SerializedFromJSON: %v", err)
	}
	if got := jj.MarshalSerializedHex(restored...); got != helloStringStream {
		t.Fatalf("json round-trip = %s want %s", got, helloStringStream)
	}
}

func TestDecompileFileExport(t *testing.T) {
	src, err := jj.DecompileFile(filepath.Join("classparser", "testdata", "invisible_anno.class"))
	if err != nil {
		t.Fatalf("DecompileFile: %v", err)
	}
	if !strings.Contains(src, "InvisibleAnnoSeed") {
		t.Fatalf("unexpected source:\n%s", src)
	}
}

func zipWith(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestClassifyInputExport(t *testing.T) {
	apk := zipWith(t, map[string][]byte{"classes.dex": []byte("dex\n035\x00")})
	cases := []struct {
		name string
		data []byte
		file string
		want jj.ContainerKind
	}{
		{"jmod", append([]byte{'J', 'M', 1, 0}, zipWith(t, nil)...), "java.base.jmod", jj.ContainerJMOD},
		{"apks", zipWith(t, map[string][]byte{"base.apk": apk}), "app.apks", jj.ContainerAPKS},
		{"aab", zipWith(t, map[string][]byte{"base/dex/classes.dex": []byte("dex\n035\x00")}), "app.aab", jj.ContainerAAB},
		{"jar", zipWith(t, map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\n\r\n")}), "library.jar", jj.ContainerJAR},
		{"unknown", []byte("not an artifact"), "notes.txt", jj.ContainerUnknown},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := jj.ClassifyInput(test.data, test.file); got != test.want {
				t.Fatalf("ClassifyInput = %q, want %q", got, test.want)
			}
		})
	}
	if jj.DetectInputKind(cases[0].data) != jj.InputJMOD {
		t.Fatal("DetectInputKind did not report JMOD")
	}
	if !jj.IsAndroidContainer(jj.ContainerAPKS) || jj.IsAndroidContainer(jj.ContainerJAR) {
		t.Fatal("IsAndroidContainer groups are wrong")
	}
	if !jj.IsJavaArchiveContainer(jj.ContainerJMOD) || jj.IsJavaArchiveContainer(jj.ContainerAPK) {
		t.Fatal("IsJavaArchiveContainer groups are wrong")
	}
}

func TestClassifyInputFileExport(t *testing.T) {
	apk := zipWith(t, map[string][]byte{"classes.dex": []byte("dex\n035\x00")})
	bundle := zipWith(t, map[string][]byte{"base.apk": apk})
	path := filepath.Join(t.TempDir(), "app.apks")
	if err := os.WriteFile(path, bundle, 0o644); err != nil {
		t.Fatal(err)
	}
	kind, err := jj.ClassifyInputFile(path)
	if err != nil {
		t.Fatalf("ClassifyInputFile: %v", err)
	}
	if kind != jj.ContainerAPKS {
		t.Fatalf("kind = %q, want apks", kind)
	}
}

func TestJImageToJarExport(t *testing.T) {
	classBytes := testdataClass(t)
	image, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/InvisibleAnnoSeed.class", Data: classBytes},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "modules")
	if err := os.WriteFile(src, image, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := jj.ClassifyInput(image, "modules"); got != jj.ContainerJImage {
		t.Fatalf("ClassifyInput = %q, want jimage", got)
	}
	if got := jj.DetectInputKind(image); got != jj.InputJIMAGE {
		t.Fatalf("DetectInputKind = %v, want InputJIMAGE", got)
	}
	if !jj.IsJavaArchiveContainer(jj.ContainerJImage) {
		t.Fatal("JImage is not grouped as a Java archive")
	}

	dst := filepath.Join(dir, "image.jar")
	if err := jj.JImageToJar(src, dst); err != nil {
		t.Fatalf("JImageToJar: %v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open converted JAR: %v", err)
	}
	if len(zipReader.File) != 1 || zipReader.File[0].Name != "InvisibleAnnoSeed.class" {
		t.Fatalf("JAR entries = %#v", zipReader.File)
	}
}

func TestDecompileJImageArchiveExport(t *testing.T) {
	classBytes := testdataClass(t)
	image, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/test/InvisibleAnnoSeed.class", Data: classBytes},
		{Path: "/test/conf/app.properties", Data: []byte("key=value\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "modules")
	if err := os.WriteFile(src, image, 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "src")
	if err := jj.DecompileArchive(src, outDir); err != nil {
		t.Fatalf("DecompileArchive: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(outDir, "InvisibleAnnoSeed.java"))
	if err != nil {
		t.Fatalf("read decompiled source: %v", err)
	}
	if !strings.Contains(string(source), "class InvisibleAnnoSeed") {
		t.Fatalf("unexpected source:\n%s", source)
	}
	if _, err := os.Stat(filepath.Join(outDir, "test/conf/app.properties")); err != nil {
		t.Fatalf("JImage resource missing: %v", err)
	}
}

func TestDecompileArchiveWithAnalysisBudgetExport(t *testing.T) {
	dir := t.TempDir()
	jarPath := filepath.Join(dir, "seed.jar")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entry, err := zw.Create("InvisibleAnnoSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(testdataClass(t)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jarPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "src")
	if err := jj.DecompileArchiveWithAnalysisBudget(jarPath, outDir, 2_000_000); err != nil {
		t.Fatalf("DecompileArchiveWithAnalysisBudget: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(outDir, "InvisibleAnnoSeed.java"))
	if err != nil {
		t.Fatalf("read decompiled source: %v", err)
	}
	if !strings.Contains(string(src), "class InvisibleAnnoSeed") {
		t.Fatalf("unexpected source:\n%s", src)
	}

	if err := jj.DecompileArchiveWithAnalysisBudget(jarPath, outDir, -1); err == nil {
		t.Fatal("negative analysis budget accepted")
	}
}

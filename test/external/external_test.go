package external_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/jarwar"
	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/inspect"
	"github.com/yaklang/javajive/internal/inputkind"
	"github.com/yaklang/javajive/internal/testfixtures"
	"github.com/yaklang/javajive/jimage"
)

type sampleCase struct {
	name   string
	kind   inputkind.Kind
	status inspect.Status
}

var externalSamples = []sampleCase{
	{"tomcat-context.war", inputkind.WAR, inspect.StatusComplete},
	{"tomcat-sample.war", inputkind.WAR, inspect.StatusComplete},
	{"tomcat-broken.war", inputkind.WAR, inspect.StatusComplete},
	{"bundletool-app.apk", inputkind.APK, inspect.StatusComplete},
	{"bundletool-install-time.aab", inputkind.AAB, inspect.StatusComplete},
	{"bundletool-universal.apks", inputkind.APKS, inspect.StatusComplete},
	{"bundletool-splits.apks", inputkind.APKS, inspect.StatusComplete},
	{"androguard-invalid.apk", inputkind.APK, inspect.StatusComplete},
	{"androguard-test-debug.apk", inputkind.APK, inspect.StatusComplete},
	{"androguard-tc-debug.apk", inputkind.APK, inspect.StatusComplete},
	{"dex2jar-dex038.dex", inputkind.DEX, inspect.StatusPartial},
	{"dex2jar-dex039.dex", inputkind.DEX, inspect.StatusPartial},
	{"apktool-dynamic-dex.apk", inputkind.APK, inspect.StatusPartial},
	{"apktool-issue2543.dex", inputkind.DEX, inspect.StatusComplete},
	{"androidx-tracing-1.2.0.aar", inputkind.AAR, inspect.StatusComplete},
	{"sample.xapk", inputkind.XAPK, inspect.StatusComplete},
	{"sample.apkm", inputkind.APKM, inspect.StatusComplete},
	{"sample.ear", inputkind.EAR, inspect.StatusComplete},
}

func readSample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(testfixtures.Path(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func TestExternalSamplesClassifyAndInspect(t *testing.T) {
	for _, sample := range externalSamples {
		t.Run(sample.name, func(t *testing.T) {
			data := readSample(t, sample.name)
			if kind := inputkind.DetectBytes(data, sample.name); kind != sample.kind {
				t.Fatalf("classification = %q, want %q", kind, sample.kind)
			}
			report := inspect.AnalyzeWithOptions(data, sample.name, inspect.Options{Disassembly: false})
			if report.Artifact.Format != string(sample.kind) {
				t.Fatalf("inspect format = %q, want %q", report.Artifact.Format, sample.kind)
			}
			if report.Status != sample.status {
				t.Fatalf("inspect status = %q, want %q (%#v)", report.Status, sample.status, report.Diagnostics)
			}
			if sample.status == inspect.StatusPartial && len(report.Diagnostics) == 0 {
				t.Fatal("partial report has no diagnostics")
			}
		})
	}
}

func TestExternalDecompileJavaArchives(t *testing.T) {
	t.Run("war", func(t *testing.T) {
		dst := t.TempDir()
		if err := jarwar.AutoDecompile(testfixtures.Path("tomcat-sample.war"), dst); err != nil {
			t.Fatalf("decompile WAR: %v", err)
		}
		body, err := os.ReadFile(filepath.Join(dst, "WEB-INF", "classes", "mypackage", "Hello.java"))
		if err != nil {
			t.Fatalf("read WAR source: %v", err)
		}
		if !strings.Contains(string(body), "class Hello") {
			t.Fatalf("unexpected WAR source:\n%s", body)
		}
	})

	t.Run("aar", func(t *testing.T) {
		dst := t.TempDir()
		if err := jarwar.AutoDecompile(testfixtures.Path("androidx-tracing-1.2.0.aar"), dst); err != nil {
			t.Fatalf("decompile AAR: %v", err)
		}
		body, err := os.ReadFile(filepath.Join(dst, "classes.jar", "androidx", "tracing", "Trace.java"))
		if err != nil {
			t.Fatalf("read AAR source: %v", err)
		}
		if !strings.Contains(string(body), "Trace") {
			t.Fatalf("unexpected AAR source:\n%s", body)
		}
	})

	t.Run("ear flat", func(t *testing.T) {
		dst := t.TempDir()
		if err := jarwar.AutoDecompile(testfixtures.Path("sample.ear"), dst); err != nil {
			t.Fatalf("decompile EAR: %v", err)
		}
		body, err := os.ReadFile(filepath.Join(dst, "EAR", "lib", "sample.jar", "mypackage", "Hello.java"))
		if err != nil {
			t.Fatalf("read flat EAR source: %v", err)
		}
		if !strings.Contains(string(body), "class Hello") {
			t.Fatalf("unexpected flat EAR source:\n%s", body)
		}
	})

	t.Run("ear structured", func(t *testing.T) {
		dst := t.TempDir()
		report, err := jarwar.AutoDecompileWithOptions(testfixtures.Path("sample.ear"), dst, jarwar.ArchiveOptions{Structured: true})
		if err != nil {
			t.Fatalf("structured EAR export: %v", err)
		}
		if len(report.Archives) != 1 || report.Archives[0].Path != "EAR/lib/sample.jar" {
			t.Fatalf("nested archives = %#v", report.Archives)
		}
		body, err := os.ReadFile(filepath.Join(dst, "sources", "EAR", "lib", "sample.jar!", "mypackage", "Hello.java"))
		if err != nil {
			t.Fatalf("read structured EAR source: %v", err)
		}
		if !strings.Contains(string(body), "class Hello") {
			t.Fatalf("unexpected structured EAR source:\n%s", body)
		}
	})
}

func TestExternalAndroidTranslation(t *testing.T) {
	cases := []struct {
		name     string
		dexFiles int
		classes  int
	}{
		{"bundletool-app.apk", 1, 10},
		{"bundletool-install-time.aab", 3, 1298},
		{"bundletool-universal.apks", 3, 1298},
		{"bundletool-splits.apks", 4, 1298},
		{"androguard-test-debug.apk", 1, 7},
		{"androguard-tc-debug.apk", 1, 13},
		{"androguard-invalid.apk", 1, 286},
		{"apktool-issue2543.dex", 1, 1},
		{"sample.xapk", 1, 10},
		{"sample.apkm", 1, 10},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "classes.jar")
			report, err := dex2jar.DexFileToJar(testfixtures.Path(test.name), dst, dex2jar.Options{})
			if err != nil {
				t.Fatalf("DexFileToJar: %v", err)
			}
			if report.DEXFiles != test.dexFiles || report.Classes != test.classes {
				t.Fatalf("report = %#v, want %d DEX / %d classes", report, test.dexFiles, test.classes)
			}
			if _, err := os.Stat(dst); err != nil {
				t.Fatalf("output JAR: %v", err)
			}
		})
	}
}

func TestExternalAndroidRejections(t *testing.T) {
	cases := []struct {
		name     string
		contains string
	}{
		{"dex2jar-dex038.dex", "call_site_id"},
		{"dex2jar-dex039.dex", "method_handle"},
		{"apktool-dynamic-dex.apk", "SHA-1"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "classes.jar")
			_, err := dex2jar.DexFileToJar(testfixtures.Path(test.name), dst, dex2jar.Options{})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want %q", err, test.contains)
			}
			if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
				t.Fatalf("transactional failure left an output JAR: %v", statErr)
			}
		})
	}
}

func TestExternalRenamedSamples(t *testing.T) {
	cases := []struct {
		name string
		file string
		kind inputkind.Kind
	}{
		{"bundletool-app.apk", "renamed.zip", inputkind.APK},
		{"bundletool-install-time.aab", "renamed.zip", inputkind.AAB},
		{"bundletool-universal.apks", "renamed.zip", inputkind.APKS},
		{"tomcat-sample.war", "renamed.zip", inputkind.WAR},
		{"androidx-tracing-1.2.0.aar", "renamed.zip", inputkind.AAR},
		{"sample.ear", "renamed.zip", inputkind.EAR},
		{"sample.xapk", "renamed.apkx", inputkind.XAPK},
		{"sample.apkm", "renamed.zip", inputkind.APKM},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			data := readSample(t, test.name)
			if kind := inputkind.DetectBytes(data, test.file); kind != test.kind {
				t.Fatalf("renamed classification = %q, want %q", kind, test.kind)
			}
		})
	}
}

func jdkHome(t *testing.T) string {
	t.Helper()
	candidates := make([]string, 0, 3)
	if home := os.Getenv("JAVA_HOME"); home != "" {
		candidates = append(candidates, home)
	}
	if output, err := exec.Command("/usr/libexec/java_home").Output(); err == nil {
		candidates = append(candidates, strings.TrimSpace(string(output)))
	}
	if javac, err := exec.LookPath("javac"); err == nil {
		if resolved, err := filepath.EvalSymlinks(javac); err == nil {
			javac = resolved
		}
		candidates = append(candidates, filepath.Dir(filepath.Dir(javac)))
	}
	for _, home := range candidates {
		if _, err := os.Stat(filepath.Join(home, "lib", "modules")); err == nil {
			return home
		}
	}
	t.Skip("no JDK runtime image available")
	return ""
}

func TestExternalJModAndJImage(t *testing.T) {
	home := jdkHome(t)

	t.Run("jmod", func(t *testing.T) {
		path := filepath.Join(home, "jmods", "java.base.jmod")
		if _, err := os.Stat(path); err != nil {
			t.Skipf("java.base.jmod not found: %v", err)
		}
		jarFS, err := classparser.NewJarFSFromLocal(path)
		if err != nil {
			t.Fatalf("open JMOD: %v", err)
		}
		defer jarFS.Close()
		source, err := jarFS.ReadFile("classes/java/lang/Object.class")
		if err != nil {
			t.Fatalf("decompile JMOD Object.class: %v", err)
		}
		if !strings.Contains(string(source), "class Object") {
			t.Fatalf("unexpected JMOD source:\n%s", source)
		}
	})

	t.Run("jimage", func(t *testing.T) {
		path := filepath.Join(home, "lib", "modules")
		reader, err := jimage.OpenFile(path)
		if err != nil {
			t.Fatalf("open JImage: %v", err)
		}
		defer reader.Close()
		object, err := reader.Read("java.base/java/lang/Object.class")
		if err != nil {
			t.Fatalf("read JImage Object.class: %v", err)
		}
		if !bytes.HasPrefix(object, []byte{0xca, 0xfe, 0xba, 0xbe}) {
			t.Fatalf("JImage Object.class magic = %x", object[:4])
		}
	})
}

func TestExternalUnsupportedDEXVersion040(t *testing.T) {
	data := readSample(t, "apktool-issue2543.dex")
	if len(data) < 8 {
		t.Fatal("DEX fixture is too short")
	}
	data[4], data[5], data[6], data[7] = '0', '4', '0', 0
	if _, _, err := dex2jar.DexToJar(data, dex2jar.Options{}); err == nil || !strings.Contains(err.Error(), `unsupported DEX version "040"`) {
		t.Fatalf("DexToJar error = %v", err)
	}
	report := inspect.AnalyzeWithOptions(data, "version-040.dex", inspect.Options{Disassembly: false})
	if report.Status != inspect.StatusPartial {
		t.Fatalf("inspect status = %q, want partial", report.Status)
	}
}

func mustNotPanic(t *testing.T, label string, action func()) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("%s panicked: %v", label, recovered)
		}
	}()
	action()
}

func TestExternalMalformedTruncations(t *testing.T) {
	samples := []string{
		"tomcat-sample.war",
		"tomcat-broken.war",
		"bundletool-app.apk",
		"androguard-test-debug.apk",
		"apktool-dynamic-dex.apk",
		"dex2jar-dex038.dex",
		"dex2jar-dex039.dex",
		"apktool-issue2543.dex",
		"androidx-tracing-1.2.0.aar",
		"sample.xapk",
		"sample.apkm",
		"sample.ear",
	}
	options := dex2jar.Options{
		MaxDEXSize:      1 << 20,
		MaxAPKSize:      2 << 20,
		MaxTotalDEXSize: 2 << 20,
		MaxDEXFiles:     8,
		MaxNestedAPKs:   8,
	}
	for _, name := range samples {
		data := readSample(t, name)
		lengths := []int{0, 1, 4, 8, len(data) / 4, len(data) / 2, len(data) - 1}
		seen := make(map[int]bool)
		for _, length := range lengths {
			if length < 0 || length > len(data) || seen[length] {
				continue
			}
			seen[length] = true
			truncated := data[:length]
			label := fmt.Sprintf("%s/%d", name, length)
			mustNotPanic(t, label+" classify", func() {
				_ = inputkind.DetectBytes(truncated, name)
			})
			mustNotPanic(t, label+" inspect", func() {
				_ = inspect.AnalyzeWithOptions(truncated, name, inspect.Options{Disassembly: false})
			})
			mustNotPanic(t, label+" dex classify", func() {
				_ = dex2jar.ClassifyInput(truncated, name)
			})
			if inputkind.IsAndroid(inputkind.DetectBytes(truncated, name)) {
				mustNotPanic(t, label+" dex convert", func() {
					_, _, _ = dex2jar.DexToJar(truncated, options)
				})
			}
		}
	}
}

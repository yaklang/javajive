package cross

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// mrVersionsRe matches a Multi-Release jar's versioned entry path (META-INF/versions/<N>/...)
// and captures the release number N.
var mrVersionsRe = regexp.MustCompile(`(?:^|/)META-INF/versions/(\d+)/`)

// mrFileRelease returns the javac --release value for one decompiled source file: the Multi-Release
// version N for a `META-INF/versions/N/` unit, or def for a base-tree unit. An MR jar's versioned
// classes are BY DEFINITION built for a later JDK (snakeyaml's versions/9 Logger uses
// java.lang.System.Logger, a JDK9 API); compiling them with the base tree's --release 8 fails
// unconditionally for ANY decompiler, so it is a harness artifact rather than a decompiler defect.
func mrFileRelease(f string, def int) int {
	if m := mrVersionsRe.FindStringSubmatch(filepath.ToSlash(f)); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > def {
			return n
		}
	}
	return def
}

// splitMRFiles partitions decompiled .java files into the base tree and the per-release
// META-INF/versions/N groups of a Multi-Release jar (JEP 238). Versioned units must be compiled in a
// SEPARATE javac pass: (a) with `--release N`, because they target that JDK's APIs, and (b) apart from
// the base tree, because a versioned class declares the same package+name as its base counterpart
// ("duplicate class" in a single pass). Returns the sorted release numbers for deterministic iteration.
func splitMRFiles(files []string) (base []string, versioned map[int][]string, releases []int) {
	versioned = map[int][]string{}
	for _, f := range files {
		if n := mrFileRelease(f, 8); n > 8 {
			versioned[n] = append(versioned[n], f)
			continue
		}
		base = append(base, f)
	}
	for n := range versioned {
		releases = append(releases, n)
	}
	sort.Ints(releases)
	return base, versioned, releases
}

// lookJavac returns the path to javac, skipping the test when no JDK is present.
func lookJavac(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found in PATH; skipping Java cross-test")
	}
	return p
}

// lookJava returns the path to java, skipping the test when no JRE is present.
func lookJava(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java not found in PATH; skipping Java cross-test")
	}
	return p
}

// writeSources writes name->source files into dir.
func writeSources(t *testing.T, dir string, sources map[string]string) {
	t.Helper()
	for name, src := range sources {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// compileJava compiles every .java file in dir into .class files in the same dir.
func compileJava(t *testing.T, dir string, sources map[string]string) {
	t.Helper()
	javac := lookJavac(t)
	writeSources(t, dir, sources)

	var files []string
	for name := range sources {
		if strings.HasSuffix(name, ".java") {
			files = append(files, name)
		}
	}

	args := append([]string{"-encoding", "UTF-8", "-d", dir}, files...)
	cmd := exec.Command(javac, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac failed: %v\n%s", err, out)
	}
}

// runJava runs `java -cp dir <class> args...` and returns combined output.
func runJava(t *testing.T, dir, class string, args ...string) []byte {
	t.Helper()
	java := lookJava(t)
	full := append([]string{"-cp", dir, class}, args...)
	cmd := exec.Command(java, full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("java run failed: %v\n%s", err, out)
	}
	return out
}

// zipClassesToJar packs every .class file under dir into a .jar (a plain zip),
// preserving the relative directory layout (i.e. the package path).
func zipClassesToJar(t *testing.T, dir, jarPath string) {
	t.Helper()
	f, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".class") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, bytes.NewReader(data))
		return err
	})
	if err != nil {
		t.Fatalf("build jar: %v", err)
	}
}

// readClass reads the named .class file from dir.
func readClass(t *testing.T, dir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read class %s: %v", name, err)
	}
	return data
}

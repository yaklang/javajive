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

// mrPathRelease returns the Multi-Release version N encoded in a `META-INF/versions/N/` path, or def
// for a base-tree unit. splitMRFiles uses this (path only) so a Java 9 API in a BASE-tree file is
// not emitted under META-INF/versions/N.
func mrPathRelease(f string, def int) int {
	if m := mrVersionsRe.FindStringSubmatch(filepath.ToSlash(f)); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > def {
			return n
		}
	}
	return def
}

// java9SslAlpnCallRe matches a DIRECT invocation of the Java 9 SSL ALPN methods
// (SSLSocket.getApplicationProtocol / SSLParameters.setApplicationProtocols). Reflective
// Class.getMethod("getApplicationProtocol", ...) in Jdk9Platform must NOT match — those compile
// under --release 8. okhttp Android10Platform calls the methods directly under @IgnoreJRERequirement
// (compiled against the Android SDK); --release 8 then reports "cannot find symbol".
var java9SslAlpnCallRe = regexp.MustCompile(`\w+\.(getApplicationProtocol\(\)|setApplicationProtocols\()`)

func isJava9SslAlpnFile(f string) bool {
	b, err := os.ReadFile(f)
	if err != nil {
		return false
	}
	return java9SslAlpnCallRe.Match(b)
}

// mrFileRelease returns the javac --release value for one decompiled source file: the Multi-Release
// version N for a `META-INF/versions/N/` unit, 9 for a base-tree unit that directly calls Java 9 SSL
// ALPN methods (okhttp Android10Platform), or def otherwise. An MR jar's versioned classes are BY
// DEFINITION built for a later JDK (snakeyaml's versions/9 Logger uses java.lang.System.Logger);
// compiling them with the base tree's --release 8 fails unconditionally for ANY decompiler, so it is
// a harness artifact rather than a decompiler defect. The Android10Platform case is the same shape:
// a faithfully-decompiled unit targeting a newer API surface than --release 8.
func mrFileRelease(f string, def int) int {
	if n := mrPathRelease(f, def); n != def {
		return n
	}
	if def < 9 && isJava9SslAlpnFile(f) {
		return 9
	}
	return def
}

// classMajorToRelease maps a class-file major version to javac --release.
// Majors below 52 (Java 8) still compile as --release 8 source.
func classMajorToRelease(major int) int {
	if major < 52 {
		return 8
	}
	return major - 44
}

// jarBaseRelease returns the javac --release needed for a jar's BASE tree: the
// highest class-file major among non-Multi-Release, non-module-info entries
// (at least 8). logback 1.4 / HikariCP 5 are Java 11; freemarker's _Java16Impl
// is 16. META-INF/versions/N/ units keep their own pass via splitMRFiles.
func jarBaseRelease(jarPath string) int {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return 8
	}
	defer zr.Close()
	maxRel := 8
	hdr := make([]byte, 8)
	for _, f := range zr.File {
		n := f.Name
		if !strings.HasSuffix(n, ".class") || strings.HasSuffix(n, "module-info.class") {
			continue
		}
		if strings.Contains(n, "META-INF/versions/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		_, err = io.ReadFull(rc, hdr)
		rc.Close()
		if err != nil || hdr[0] != 0xca || hdr[1] != 0xfe || hdr[2] != 0xba || hdr[3] != 0xbe {
			continue
		}
		major := int(hdr[6])<<8 | int(hdr[7])
		if rel := classMajorToRelease(major); rel > maxRel {
			maxRel = rel
		}
	}
	return maxRel
}

// splitMRFiles partitions decompiled .java files into the base tree and the per-release
// META-INF/versions/N groups of a Multi-Release jar (JEP 238). Versioned units must be compiled in a
// SEPARATE javac pass: (a) with `--release N`, because they target that JDK's APIs, and (b) apart from
// the base tree, because a versioned class declares the same package+name as its base counterpart
// ("duplicate class" in a single pass). Returns the sorted release numbers for deterministic iteration.
func splitMRFiles(files []string) (base []string, versioned map[int][]string, releases []int) {
	versioned = map[int][]string{}
	for _, f := range files {
		// Path-only: a Java 9 ALPN unit in the BASE tree must stay in `base` so treeCompileToDir
		// can emit it next to its Java 8 siblings, not under META-INF/versions/9.
		if n := mrPathRelease(f, 8); n > 8 {
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

func TestJarBaseReleaseFollowsClassMajor(t *testing.T) {
	cases := []struct {
		key  string
		want int
	}{
		{"codec", 8},
		{"logback", 11},
		{"hikaricp", 11},
		{"freemarker", 16},
		{"asm", 8},
	}
	for _, c := range cases {
		spec, ok := jarSpecs[c.key]
		if !ok {
			t.Fatalf("missing jarSpec %q", c.key)
		}
		p := resolveJar(spec.relPath)
		if p == "" {
			t.Skipf("jar %s not under ~/.m2", spec.relPath)
		}
		if got := jarBaseRelease(p); got != c.want {
			t.Errorf("%s jarBaseRelease=%d want %d", c.key, got, c.want)
		}
	}
}

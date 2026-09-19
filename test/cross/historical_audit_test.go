package cross

// This opt-in observation harness does not count a completed measurement as a
// successful round trip. Consumers must compare the durable per-jar observations.
import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type historicalCompilePass struct{ Release, Sources, ExitCode int }
type historicalInput struct {
	Path, SHA256 string
	Deps         []string
}
type historicalManifest struct {
	ArtifactRoot string
	Artifacts    map[string]string
	Jars         map[string]historicalInput
}
type historicalJarObservation struct {
	Jar, Revision, InputSHA256, Compiler                                     string
	InputClasses, SourceUnits, DecompileFailures, StubMethods                int
	StubUnits                                                                map[string]int
	SourceHashes                                                             map[string]string
	CompilePasses                                                            []historicalCompilePass
	CompilerErrors                                                           int
	CompileSucceeded                                                         bool
	OriginalVerifyOK, OriginalVerifyFail, RebuiltVerifyOK, RebuiltVerifyFail int
	RebuiltClasses                                                           []string
	Seconds                                                                  float64
	Completed                                                                bool
}

func historicalWrite(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
}
func historicalHash(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func TestHistoricalJarAudit(t *testing.T) {
	manifestPath := os.Getenv("HISTORICAL_JAR_MANIFEST")
	if manifestPath == "" {
		t.Skip("set HISTORICAL_JAR_MANIFEST and HISTORICAL_JAR_REPORT for complete corpus observations")
	}
	for _, key := range []string{"MAXFILES", "ROUNDTRIP_JAR", "PROFILE_JAR", "KILL_SWITCH"} {
		if os.Getenv(key) != "" {
			t.Fatalf("%s must be unset for the full audit", key)
		}
	}
	for _, tool := range []string{"javac", "java"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatal(err)
		}
	}
	root := os.Getenv("HISTORICAL_JAR_REPORT")
	if root == "" {
		t.Fatal("HISTORICAL_JAR_REPORT required")
	}
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest historicalManifest
	if err = json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ArtifactRoot == "" || len(manifest.Artifacts) == 0 {
		t.Fatal("manifest must pin all target and dependency hashes")
	}
	pinned := map[string]string{}
	for rel, hash := range manifest.Artifacts {
		path := filepath.Join(manifest.ArtifactRoot, filepath.FromSlash(rel))
		if historicalHash(t, path) != hash {
			t.Fatalf("artifact changed: %s", rel)
		}
		pinned[path] = hash
	}
	names := jarKeys()
	if len(manifest.Jars) != len(names) {
		t.Fatalf("manifest has %d jars, want all %d", len(manifest.Jars), len(names))
	}
	for _, name := range names {
		input, ok := manifest.Jars[name]
		if !ok {
			t.Fatalf("missing manifest jar %s", name)
		}
		if historicalHash(t, input.Path) != input.SHA256 {
			t.Fatalf("input changed: %s", name)
		}
		for _, dep := range input.Deps {
			if pinned[dep] == "" {
				t.Fatalf("dependency has no verified hash: %s", dep)
			}
		}
	}
	javac := lookJavac(t)
	version, _ := exec.Command(javac, "-version").CombinedOutput()
	// Force reflection to resolve declared signatures as well as loading classes.
	verifier := strings.Replace(verifierSource, "Class.forName(cn,false,cl); ok++;", "Class<?> c=Class.forName(cn,false,cl); c.getDeclaredConstructors(); c.getDeclaredMethods(); c.getDeclaredFields(); ok++;", 1)
	verifierDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(verifierDir, "Verifier.java"), []byte(verifier), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-d", verifierDir, filepath.Join(verifierDir, "Verifier.java")).CombinedOutput(); err != nil {
		t.Fatalf("verifier: %v: %s", err, out)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			input := manifest.Jars[name]
			dir := filepath.Join(root, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "observation.json")); err == nil {
				t.Fatal("refusing to overwrite prior observations; use a fresh report directory")
			}
			o := historicalJarObservation{Jar: name, Revision: os.Getenv("HISTORICAL_JAR_REVISION"), InputSHA256: input.SHA256, Compiler: strings.TrimSpace(string(version)), StubUnits: map[string]int{}, SourceHashes: map[string]string{}}
			start := time.Now()
			defer func() {
				o.Seconds = time.Since(start).Seconds()
				historicalWrite(t, filepath.Join(dir, "observation.json"), o)
			}()
			o.InputClasses = len(classEntries(t, input.Path))
			// The target jar is deliberately excluded from the dependency classpath.
			for _, dep := range input.Deps {
				if historicalHash(t, dep) == input.SHA256 {
					t.Fatal("original target appears in dependency classpath")
				}
			}
			cp := withEnvShims(t, strings.Join(input.Deps, string(os.PathListSeparator)))
			srcRoot := filepath.Join(dir, "sources")
			files, units, fail := decompileAll(t, input.Path, srcRoot, 0)
			o.SourceUnits = units
			o.DecompileFailures = fail
			for _, f := range files {
				rel, _ := filepath.Rel(srcRoot, f)
				b, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				n := strings.Count(string(b), "/* yak-decompiler:")
				if n > 0 {
					o.StubUnits[filepath.ToSlash(rel)] = n
					o.StubMethods += n
				}
				o.SourceHashes[filepath.ToSlash(rel)] = historicalHash(t, f)
			}
			clsRoot := filepath.Join(dir, "classes")
			var raw string
			o.CompilerErrors, raw, o.CompilePasses = historicalTreeCompile(t, files, cp, clsRoot, compileRelease(jarSpecs[name], input.Path))
			if err := os.WriteFile(filepath.Join(dir, "javac.log"), []byte(raw), 0644); err != nil {
				t.Fatal(err)
			}
			o.CompileSucceeded = len(files) > 0 && len(o.CompilePasses) > 0 && o.DecompileFailures == 0
			for _, pass := range o.CompilePasses {
				if pass.ExitCode != 0 {
					o.CompileSucceeded = false
				}
			}
			if err := os.MkdirAll(clsRoot, 0755); err != nil {
				t.Fatal(err)
			}
			repackaged := filepath.Join(dir, "rebuilt.jar")
			zipClassesToJar(t, clsRoot, repackaged)
			zr, err := zip.OpenReader(repackaged)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range zr.File {
				if strings.HasSuffix(f.Name, ".class") {
					o.RebuiltClasses = append(o.RebuiltClasses, f.Name)
				}
			}
			zr.Close()
			sort.Strings(o.RebuiltClasses)
			if len(o.RebuiltClasses) == 0 {
				o.CompileSucceeded = false
			}
			var vraw string
			o.OriginalVerifyOK, o.OriginalVerifyFail, vraw = verifyJarLoads(t, verifierDir, input.Path, cp)
			if err := os.WriteFile(filepath.Join(dir, "verify-original.log"), []byte(vraw), 0644); err != nil {
				t.Fatal(err)
			}
			o.RebuiltVerifyOK, o.RebuiltVerifyFail, vraw = verifyJarLoads(t, verifierDir, repackaged, cp)
			if err := os.WriteFile(filepath.Join(dir, "verify-rebuilt.log"), []byte(vraw), 0644); err != nil {
				t.Fatal(err)
			}
			o.Completed = true
			t.Logf("[%s] input=%d sources=%d decompileFail=%d stubs=%d compile=%t errors=%d originalVerify=%d/%d rebuiltVerify=%d/%d", name, o.InputClasses, o.SourceUnits, o.DecompileFailures, o.StubMethods, o.CompileSucceeded, o.CompilerErrors, o.OriginalVerifyOK, o.OriginalVerifyFail, o.RebuiltVerifyOK, o.RebuiltVerifyFail)
		})
	}
	fmt.Printf("Historical audit observations saved to %s; individual status fields determine outcomes.\n", root)
}

func historicalTreeCompile(t *testing.T, files []string, classpath, outDir string, minRelease int) (errCount int, raw string, passes []historicalCompilePass) {
	t.Helper()
	javac := lookJavac(t)
	if minRelease < 8 {
		minRelease = 8
	}
	base, versioned, releases := splitMRFiles(files)
	var base8, j9 []string
	for _, f := range base {
		if isJava9SslAlpnFile(f) {
			j9 = append(j9, f)
		} else {
			base8 = append(base8, f)
		}
	}
	run := func(fs []string, release int, dst, cp, sourcepath string) string {
		if len(fs) == 0 {
			return ""
		}
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		args := append(append([]string{}, javacLocaleArgs...),
			"-encoding", "UTF-8", "--release", strconv.Itoa(release), "-nowarn", "-proc:none", "-Xmaxerrs", "100000")
		if cp != "" {
			args = append(args, "-cp", cp)
		}
		if sourcepath != "" {
			// Versioned Multi-Release units live under META-INF/versions/N/. javac infers
			// a sourcepath from that tree, then reports missing sibling packages that
			// only exist in the BASE tree (log4j-core versions/9 importing
			// org.apache.logging.log4j.core.pattern / .time). Point sourcepath at a
			// view of the base `org/` tree (no module-info.java) so those resolve.
			args = append(args, "-sourcepath", sourcepath)
		}
		args = append(args, "-d", dst)
		args = append(args, fs...)
		cmd := exec.Command(javac, args...)
		cmd.Dir = dst // keep javac.<ts>.args argfile out of test/cross on huge command lines
		out, err := cmd.CombinedOutput()
		exitCode := 0
		if err != nil {
			exitCode = -1
			if e, ok := err.(*exec.ExitError); ok {
				exitCode = e.ExitCode()
			}
		}
		passes = append(passes, historicalCompilePass{Release: release, Sources: len(fs), ExitCode: exitCode})
		return string(out)
	}
	// okhttp Android10Platform calls Java 9 SSL ALPN methods directly (Android SDK 29 /
	// @IgnoreJRERequirement). It is referenced by Platform.findAndroidPlatform in the BASE
	// tree, so it cannot be a second pass (Platform.java would then fail with "cannot find
	// symbol: class Android10Platform"). Compile the whole base tree at --release 9 instead.
	baseRelease := minRelease
	if len(j9) > 0 {
		if baseRelease < 9 {
			baseRelease = 9
		}
		base8 = append(base8, j9...)
	}
	if len(base8) > 0 {
		raw = run(base8, baseRelease, outDir, classpath, "")
	}
	mrSrc := ""
	if len(releases) > 0 {
		srcRoot := ""
		for _, f := range base8 {
			slash := filepath.ToSlash(f)
			if strings.Contains(slash, "/META-INF/") {
				continue
			}
			if i := strings.Index(slash, "/org/"); i >= 0 {
				srcRoot = filepath.FromSlash(slash[:i])
				break
			}
		}
		mrSrc = filepath.Join(outDir, ".jdec-empty-src")
		if err := os.MkdirAll(mrSrc, 0o755); err != nil {
			t.Fatalf("mkdir empty sourcepath: %v", err)
		}
		if srcRoot != "" {
			baseSrc := filepath.Join(outDir, ".jdec-base-src")
			if err := os.MkdirAll(baseSrc, 0o755); err == nil {
				orgLink := filepath.Join(baseSrc, "org")
				if err := os.Symlink(filepath.Join(srcRoot, "org"), orgLink); err == nil {
					mrSrc = baseSrc
				}
			}
		}
	}
	for _, n := range releases {
		cp := outDir
		if classpath != "" {
			cp = outDir + string(os.PathListSeparator) + classpath
		}
		raw += run(versioned[n], n, filepath.Join(outDir, "META-INF", "versions", strconv.Itoa(n)), cp, mrSrc)
	}
	return strings.Count(raw, ": error:"), raw, passes
}

package cross

// 北极星 harness:「反编译 -> javac 整树重编译 -> 重新打包成 jar -> 外部 JVM 逐类 load+verify
// -> 调用差分」。它回答用户的核心诉求:复杂 JAR 反编译之后还能编译回去, 还能重新打包被调用。
//
// 为什么用整树 (tree) 而不是逐文件 (iso) 口径:反编译器把嵌套类发成独立的扁平单元
// `Outer$Inner.java` (dumper.go 的架构), 这种扁平 `$` 类型引用只有在兄弟扁平源一起编译 (tree)
// 时才解析得到; 单文件 iso 用原始 jar 当 classpath 时, javac 把 jar 里的嵌套类按源名
// `Outer.Inner` 索引, 解析不到 `Outer$Inner`, 于是报海量 "cannot find symbol"。那是 iso 口径的
// 系统性假阳性, 不是反编译缺陷, 也不阻碍重打包。重打包必须整树, 所以本 harness 用 tree。
//
// 两个测试:
//   - TestSyntheticJarRoundTrip: 只需 javac/java (无需 ~/.m2), CI 常驻承重。合成一个多类程序
//     (顶层类 + 静态嵌套类 + 独立顶层类 + enum+switch + 泛型 + lambda), 走完整链路并断言重打包
//     jar 的运行输出与原始字节码 jar 逐字节一致, 且每个类都能 load+verify。这是往返能力的回归闸门。
//   - TestJarRoundTripRepackage: opt-in (ROUNDTRIP_JAR=<jar|all>), 需 ~/.m2 真实 jar。对真实
//     jar 跑整链路并报告 tree 错误数 / verify 通过数。provenClean 集合 (codec/gson/fastjson2/
//     snakeyaml/jsoup/commons-lang3/guava/spring) 硬断言 tree=0 且 -Xverify:all fail=0。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// verifierSource is a tiny external program: open a jar, and for every .class entry force the JVM to
// load+link it under -Xverify:all so the bytecode verifier runs. Prints `VERIFY ok=<n> fail=<m>`.
const verifierSource = `import java.io.*; import java.util.*; import java.util.jar.*; import java.net.*;
public class Verifier {
  public static void main(String[] a) throws Exception {
    URL[] urls = new URL[a.length];
    for (int i = 0; i < a.length; i++) urls[i] = new File(a[i]).toURI().toURL();
    URLClassLoader cl = new URLClassLoader(urls, ClassLoader.getSystemClassLoader());
    int ok=0, fail=0; List<String> fails=new ArrayList<>();
    try (JarFile jf = new JarFile(a[0])) {
      for (Enumeration<JarEntry> e=jf.entries(); e.hasMoreElements();) {
        JarEntry je=e.nextElement(); String n=je.getName();
        if(!n.endsWith(".class")||n.endsWith("module-info.class")) continue;
        if(n.startsWith("META-INF/")) continue; // MR versioned alternates: path does not map to a loadable class name

        String cn=n.substring(0,n.length()-6).replace('/','.');
        try { Class.forName(cn,false,cl); ok++; }
        catch(Throwable t){ fail++; if(fails.size()<40) fails.add(cn+" -> "+t); }
      }
    }
    System.out.println("VERIFY ok="+ok+" fail="+fail);
    for(String f:fails) System.out.println("  FAIL "+f);
  }
}
`

// buildVerifier compiles Verifier.java once into its own dir and returns that dir.
func buildVerifier(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Verifier.java"), []byte(verifierSource), 0o644); err != nil {
		t.Fatalf("write Verifier.java: %v", err)
	}
	javac := lookJavac(t)
	cmd := exec.Command(javac, "-d", dir, filepath.Join(dir, "Verifier.java"))
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile Verifier: %v\n%s", err, out)
	}
	return dir
}

// verifyJarLoads runs the verifier over jarPath under -Xverify:all and returns (ok, fail, rawOutput).
// extraCP is the same compile-time dependency path used to javac the decompiled tree (jars and
// shim dirs). Guava AbstractFuture extends failureaccess's InternalFutureFailureAccess; spring
// cglib tasks extend ant Task — those types are not inside the repackaged jar, just as they are
// not inside the original. Measuring load+verify without them is an environment false-positive.
func verifyJarLoads(t *testing.T, verifierDir, jarPath string, extraCP string) (ok, fail int, raw string) {
	t.Helper()
	java := lookJava(t)
	args := []string{
		// JDK-internal Xalan types (freemarker SunInternalXalanXPathSupport$1) live in
		// java.xml but are not exported; the original jar fails the same load without
		// these exports. Harmless for jars that do not implement those interfaces.
		"--add-exports", "java.xml/com.sun.org.apache.xml.internal.utils=ALL-UNNAMED",
		"--add-exports", "java.xml/com.sun.org.apache.xpath.internal=ALL-UNNAMED",
		"--add-exports", "java.xml/com.sun.org.apache.xpath.internal.objects=ALL-UNNAMED",
		"-Xverify:all", "-cp", verifierDir, "Verifier", jarPath,
	}
	if extraCP != "" {
		for _, p := range strings.Split(extraCP, string(os.PathListSeparator)) {
			if p != "" {
				args = append(args, p)
			}
		}
	}
	cmd := exec.Command(java, args...)
	out, err := cmd.CombinedOutput()
	raw = string(out)
	if err != nil {
		t.Fatalf("run Verifier on %s: %v\n%s", jarPath, err, raw)
	}
	for _, ln := range strings.Split(raw, "\n") {
		if strings.HasPrefix(ln, "VERIFY ") {
			for _, tok := range strings.Fields(ln) {
				if v, found := strings.CutPrefix(tok, "ok="); found {
					ok, _ = strconv.Atoi(v)
				}
				if v, found := strings.CutPrefix(tok, "fail="); found {
					fail, _ = strconv.Atoi(v)
				}
			}
		}
	}
	return ok, fail, raw
}

// treeCompileToDir compiles every file together (deps on classpath) into outDir and returns the javac
// error-line count plus raw output. Unlike recompileTree it keeps the produced .class files for repackage.
// A Multi-Release jar's `META-INF/versions/N/` units are compiled in a SEPARATE second pass with
// `--release N` (they target that JDK's APIs and duplicate base-tree class names; see splitMRFiles),
// with the base output on the classpath and their classes emitted under outDir/META-INF/versions/N so
// the repackaged jar preserves the MR layout.
func treeCompileToDir(t *testing.T, files []string, classpath, outDir string) (errCount int, raw string) {
	return treeCompileToDirAt(t, files, classpath, outDir, 8)
}

// treeCompileToDirAt is treeCompileToDir with an explicit minimum --release for
// the base tree. Java 11+ jars (logback 1.4, HikariCP 5) must not be forced to
// --release 8; the ALPN bump to 9 still applies when minRelease < 9.
func treeCompileToDirAt(t *testing.T, files []string, classpath, outDir string, minRelease int) (errCount int, raw string) {
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
		out, _ := cmd.CombinedOutput()
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
	return strings.Count(raw, ": error:"), raw
}

// TestJarRoundTripRepackage drives the full north-star chain on a real ~/.m2 jar and reports how far
// it round-trips. codec is hard-asserted (proven: 0 tree errors, all classes verify); other jars are
// measured-and-reported so the gap to a clean repackage is visible.
func TestJarRoundTripRepackage(t *testing.T) {
	target := os.Getenv("ROUNDTRIP_JAR")
	if target == "" {
		t.Skip("set ROUNDTRIP_JAR=<codec|fastjson2|guava|spring|all> to run the repackage round-trip")
	}
	lookJavac(t)
	lookJava(t)
	verifierDir := buildVerifier(t)
	maxFiles, _ := strconv.Atoi(os.Getenv("MAXFILES"))

	names := []string{target}
	if target == "all" {
		names = jarKeys()
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			spec, ok := jarSpecs[name]
			if !ok {
				t.Fatalf("unknown jar %q (have: %v)", name, jarKeys())
			}
			jarPath := resolveJar(spec.relPath)
			if jarPath == "" {
				t.Skipf("jar %s not found under %s; skipping", spec.relPath, m2Repo())
			}
			deps := resolveDeps(spec.depGlob)
			// Complete JDK-internal packages (sun.misc for guava, jdk.jfr for spring-core) that
			// --release 8 hides but a faithful decompilation legitimately imports (see jdk_sunmisc_test.go
			// / jdk_jfr_test.go). Harmless for jars that do not import them.
			cp := withEnvShims(t, strings.Join(deps, string(os.PathListSeparator)))

			srcRoot := t.TempDir()
			files, units, decompFail := decompileAll(t, jarPath, srcRoot, maxFiles)

			clsRoot := t.TempDir()
			treeErr, raw := treeCompileToDirAt(t, files, cp, clsRoot, compileRelease(spec, jarPath))

			repackaged := filepath.Join(t.TempDir(), name+"-recompiled.jar")
			zipClassesToJar(t, clsRoot, repackaged)
			vok, vfail, vraw := verifyJarLoads(t, verifierDir, repackaged, cp)

			t.Logf("[%s] units=%d decompileFail=%d treeErr=%d repackagedVerify(ok=%d fail=%d)",
				name, units, decompFail, treeErr, vok, vfail)
			if vfail != 0 {
				t.Logf("[%s] verify fails:\n%s", name, firstNLines(vraw, 40))
			}

			// provenClean jars have demonstrated the full north-star chain (decompile → 0 tree errors →
			// repackage → external JVM -Xverify:all on every class). Lock each so any regression in the
			// round-trip capability fails CI loudly. Other jars are reported, not asserted, until cleared.
			provenClean := map[string]bool{
				"codec":         true, // commons-codec 1.15
				"gson":          true, // gson 2.8.9
				"fastjson2":     true, // fastjson2 2.0.43
				"snakeyaml":     true, // snakeyaml 2.2
				"jsoup":         true, // jsoup 1.10.2
				"commons-lang3": true, // commons-lang3 3.12.0
				"guava":         true, // guava 28.2-android (tree 0, 1892/1892 verify with failureaccess on CP)
				"spring":        true, // spring-core 5.3.27 (tree 0, 952/952 verify with optional deps on CP)
				"jackson":       true, // jackson-databind 2.15.4 (tree 0/773, verify 785/785)
				"okhttp":        true, // okhttp 3.14.9 (tree 0/200, verify 199/199)
				"collections4":  true, // commons-collections4 4.4 (tree 0/524, verify 528/528)
				"netty":         true, // netty-handler 4.1.108.Final (tree 0/356, verify 366/366)
				"log4j":         true, // log4j-core 2.23.1 (tree 0/1184, verify 1165/1165)
				"protobuf":      true, // protobuf-java 3.21.9 (tree 0/672, verify 703/703)
				"asm":           true, // asm 9.7 (tree 0/38, verify 38/38)
				"slf4j":         true, // slf4j-api 2.0.13 (tree 0/54, verify 55/55)
				"joda-time":     true, // joda-time 2.10.13 (tree 0/247, verify 247/247)
				"jedis":         true, // jedis 3.8.0 (tree 0/748, verify 748/748)
				"hikaricp":      true, // HikariCP 5.0.1 (tree 0/75, verify 75/75)
				"logback":       true, // logback-core 1.4.14 (tree 0/453, verify 462/462)
				"pool2":         true, // commons-pool2 2.11.1 (tree 0/80, verify 81/81)
				"picocli":       true, // picocli 4.3.2 (tree 0/216, verify 217/217)
				"httpclient":    true, // httpclient 4.5.14 (tree 0/470, verify 478/478)
				"junit":         true, // junit 4.13.2 (tree 0/346, verify 351/351)
				"javassist":     true, // javassist 3.30.2-GA (tree 0/426, verify 426/426)
				"xstream":       true, // xstream 1.4.20 (tree 0/498, verify 498/498)
				"commons-io":    true, // commons-io 2.16.0 (tree 0/346, verify 332/332)
				"compress":      true, // commons-compress 1.26.2 (tree 0/566, verify 542/542)
				"caffeine":      true, // caffeine 2.9.3 (tree 0/687, verify 692/692)
				"rxjava":        true, // rxjava 2.2.21 (tree 0/1653, verify 1663/1663)
				"math3":         true, // commons-math3 3.6.1 (tree 0/1280, verify 1324/1324)
				"assertj":       true, // assertj-core 3.24.2 (tree 0/812, verify 816/816)
				"zxing":         true, // zxing-core 3.3.3 (tree 0/260, verify 275/275)
				"freemarker":    true, // freemarker 2.3.33 (tree 0/1308, verify 1308/1308)
				"mockito":       true, // mockito-core 4.5.1 (tree 0/567, verify 570/570)
				"spring-beans":  true, // spring-beans 5.3.27 (tree 0/391, verify 379/379)
				"lucene":        true, // lucene-core 8.11.1 (tree 0/2166, verify 2146/2146)
			}
			if provenClean[name] {
				if treeErr != 0 {
					t.Errorf("%s must tree-recompile with 0 errors, got %d:\n%s", name, treeErr, firstNLines(raw, 40))
				}
				if vfail != 0 {
					t.Errorf("%s repackaged jar must verify all classes, got %d failures:\n%s", name, vfail, firstNLines(vraw, 40))
				}
			}
		})
	}
}

// TestSyntheticJarRoundTrip is the CI-resident load-bearing guard for the whole round-trip capability:
// source -> javac -> jar -> JavaJive decompile -> javac re-compile -> repackage -> run, asserting the
// repackaged jar's runtime output is byte-identical to the original and every class load+verifies. It
// needs only javac/java (no ~/.m2), so it runs everywhere the JDK is present.
func TestSyntheticJarRoundTrip(t *testing.T) {
	lookJavac(t)
	lookJava(t)

	// A deliberately multi-class program: top-level driver, a separate top-level helper, a static
	// nested class, an enum switched on, generics, a lambda, varargs, try/catch. All constructs that
	// the flat-unit decompiler reconstructs cleanly. Output is fully deterministic.
	sources := map[string]string{
		"app/Main.java": `package app;
import java.util.*;
public class Main {
  static class Box<T> { final T v; Box(T v){this.v=v;} T get(){return v;} }
  enum Op { ADD, MUL, NEG }
  static int apply(Op op, int a, int b){
    switch(op){ case ADD: return a+b; case MUL: return a*b; case NEG: return -a; default: return 0; }
  }
  @SafeVarargs static <T> int count(T... xs){ return xs.length; }
  public static void main(String[] args){
    StringBuilder sb = new StringBuilder();
    for(Op op: Op.values()) sb.append(op).append('=').append(apply(op,6,7)).append(';');
    Box<String> b = new Box<>("yak");
    sb.append("box=").append(b.get()).append(';');
    List<Integer> xs = new ArrayList<>(Arrays.asList(3,1,2));
    Collections.sort(xs, (x,y)->y-x);
    sb.append("sorted=").append(xs).append(';');
    sb.append("count=").append(count("a","b","c")).append(';');
    try { sb.append(Helper.risky(0)); } catch(RuntimeException e){ sb.append("caught:").append(e.getMessage()); }
    System.out.println(sb.toString());
  }
}
`,
		"app/Helper.java": `package app;
public class Helper {
  static String risky(int n){
    if(n==0) throw new IllegalStateException("zero");
    return "ok"+n;
  }
}
`,
	}

	srcDir := t.TempDir()
	compileJava(t, srcDir, sources)

	// Original jar from javac output, and its ground-truth runtime output.
	origJar := filepath.Join(t.TempDir(), "orig.jar")
	zipClassesToJar(t, srcDir, origJar)
	want := runJar(t, origJar, "app.Main")

	// Decompile the original jar with the production JarFS path into flat .java units.
	decDir := t.TempDir()
	files, units, _ := decompileAll(t, origJar, decDir, 0)
	if len(files) == 0 {
		t.Fatal("decompiled no files")
	}
	_ = units

	// Re-compile the decompiled sources together, repackage, and run again.
	reDir := t.TempDir()
	if errc, raw := treeCompileToDir(t, files, "", reDir); errc != 0 {
		t.Fatalf("decompiled synthetic jar failed to re-compile (%d errors):\n%s", errc, raw)
	}
	rebuiltJar := filepath.Join(t.TempDir(), "rebuilt.jar")
	zipClassesToJar(t, reDir, rebuiltJar)

	// Every class in the rebuilt jar must load+verify.
	verifierDir := buildVerifier(t)
	if vok, vfail, vraw := verifyJarLoads(t, verifierDir, rebuiltJar, ""); vfail != 0 {
		t.Fatalf("rebuilt jar failed verification: ok=%d fail=%d\n%s", vok, vfail, vraw)
	}

	got := runJar(t, rebuiltJar, "app.Main")
	if got != want {
		t.Fatalf("round-trip changed runtime behavior:\n original: %q\n rebuilt:  %q", want, got)
	}
	t.Logf("synthetic round-trip OK: %d decompiled units, output identical: %q", len(files), strings.TrimSpace(got))
}

// runJar runs `java -cp <jar> <mainClass>` and returns combined output (fatal on non-zero exit).
func runJar(t *testing.T, jarPath, mainClass string) string {
	t.Helper()
	java := lookJava(t)
	cmd := exec.Command(java, "-cp", jarPath, mainClass)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s from %s: %v\n%s", mainClass, jarPath, err, out)
	}
	return string(out)
}

func firstNLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

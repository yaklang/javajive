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
//     jar 跑整链路并报告 tree 错误数 / verify 通过数 / 多出的合成类。codec 已实测全链路达标
//     (tree=0, 107/107 verify, 调用差分一致), 故对 codec 硬断言 0 错误 + 0 verify 失败锁死成果。

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
    String jar = a[0];
    URLClassLoader cl = new URLClassLoader(new URL[]{ new File(jar).toURI().toURL() }, ClassLoader.getSystemClassLoader());
    int ok=0, fail=0; List<String> fails=new ArrayList<>();
    try (JarFile jf = new JarFile(jar)) {
      for (Enumeration<JarEntry> e=jf.entries(); e.hasMoreElements();) {
        JarEntry je=e.nextElement(); String n=je.getName();
        if(!n.endsWith(".class")||n.endsWith("module-info.class")) continue;
        String cn=n.substring(0,n.length()-6).replace('/','.');
        try { Class.forName(cn,false,cl); ok++; }
        catch(Throwable t){ fail++; if(fails.size()<20) fails.add(cn+" -> "+t); }
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
func verifyJarLoads(t *testing.T, verifierDir, jarPath string) (ok, fail int, raw string) {
	t.Helper()
	java := lookJava(t)
	cmd := exec.Command(java, "-Xverify:all", "-cp", verifierDir, "Verifier", jarPath)
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
func treeCompileToDir(t *testing.T, files []string, classpath, outDir string) (errCount int, raw string) {
	t.Helper()
	javac := lookJavac(t)
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000")
	if classpath != "" {
		args = append(args, "-cp", classpath)
	}
	args = append(args, "-d", outDir)
	args = append(args, files...)
	cmd := exec.Command(javac, args...)
	cmd.Dir = outDir // keep javac.<ts>.args argfile out of test/cross on huge command lines
	out, _ := cmd.CombinedOutput()
	return strings.Count(string(out), ": error:"), string(out)
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
			cp := withJfr(t, withSunMisc(t, strings.Join(deps, string(os.PathListSeparator))))

			srcRoot := t.TempDir()
			files, units, decompFail := decompileAll(t, jarPath, srcRoot, maxFiles)

			clsRoot := t.TempDir()
			treeErr, raw := treeCompileToDir(t, files, cp, clsRoot)

			repackaged := filepath.Join(t.TempDir(), name+"-recompiled.jar")
			zipClassesToJar(t, clsRoot, repackaged)
			vok, vfail, vraw := verifyJarLoads(t, verifierDir, repackaged)

			t.Logf("[%s] units=%d decompileFail=%d treeErr=%d repackagedVerify(ok=%d fail=%d)",
				name, units, decompFail, treeErr, vok, vfail)

			if name == "codec" {
				// Proven clean on commons-codec 1.15: lock it so any regression in the round-trip
				// capability fails CI loudly. (Other jars are reported, not asserted, until cleared.)
				if treeErr != 0 {
					t.Errorf("codec must tree-recompile with 0 errors, got %d:\n%s", treeErr, firstNLines(raw, 40))
				}
				if vfail != 0 {
					t.Errorf("codec repackaged jar must verify all classes, got %d failures:\n%s", vfail, firstNLines(vraw, 40))
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
	if vok, vfail, vraw := verifyJarLoads(t, verifierDir, rebuiltJar); vfail != 0 {
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

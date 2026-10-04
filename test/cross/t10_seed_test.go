package cross

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive"
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

const t10FinallyOverrideSrc = `public class FinallyOverride {
  static String log="";
  static int f(boolean override){try{log+="T";return 1;}finally{log+="F";if(override)return 2;}}
  static int g(){try{throw new IllegalStateException("original");}finally{log+="G";}}
  public static void main(String[] a){System.out.println(f(false));System.out.println(f(true));try{g();}catch(IllegalStateException e){System.out.println(e.getMessage());}System.out.println(log);}
}
`

const t10MonitorReleaseSrc = `public class MonitorRelease {
  static final Object LOCK=new Object();
  static boolean held(){return Thread.holdsLock(LOCK);}
  static void f(){synchronized(LOCK){System.out.println(held());throw new IllegalStateException("lock");}}
  public static void main(String[] a){System.out.println(held());try{f();}catch(IllegalStateException e){System.out.println(e.getMessage());}System.out.println(held());synchronized(LOCK){System.out.println(held());}System.out.println(held());}
}
`

func t10NormalizeOut(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func TestT10_C01_FinallyOverride(t *testing.T) {
	t.Run("T10-C01", testT10C01Finally)
}
func testT10C01Finally(t *testing.T) {
	driver := `public class Driver {public static void main(String[] args){FinallyOverride.main(args);}}`
	sources := map[string]string{"FinallyOverride.java": t10FinallyOverrideSrc}
	dir := t.TempDir()
	writeSources(t, dir, sources)
	writeSources(t, dir, map[string]string{"Driver.java": driver})
	auditCommand(t, dir, auditTool(t, "javac"), "--release", "8", "-d", dir, "FinallyOverride.java", "Driver.java")
	got := t10NormalizeOut(auditCommand(t, dir, auditTool(t, "java"), "-Xverify:all", "-cp", dir, "Driver"))
	if got != "1\n2\noriginal\nTFTFG" {
		t.Fatalf("original oracle: got %q", got)
	}
	auditSourceSet(t, sources, driver, javajive.Precision)
}

func TestT10_C02_MonitorRelease(t *testing.T) {
	t.Run("T10-C02", testT10C02Monitor)
}
func testT10C02Monitor(t *testing.T) {
	driver := `public class Driver {public static void main(String[] args){MonitorRelease.main(args);}}`
	sources := map[string]string{"MonitorRelease.java": t10MonitorReleaseSrc}
	dir := t.TempDir()
	writeSources(t, dir, sources)
	writeSources(t, dir, map[string]string{"Driver.java": driver})
	auditCommand(t, dir, auditTool(t, "javac"), "--release", "8", "-d", dir, "MonitorRelease.java", "Driver.java")
	got := t10NormalizeOut(auditCommand(t, dir, auditTool(t, "java"), "-Xverify:all", "-cp", dir, "Driver"))
	if got != "false\ntrue\nlock\nfalse\ntrue\nfalse" {
		t.Fatalf("original oracle: got %q", got)
	}
	auditSourceSet(t, sources, driver, javajive.Precision)
}

func TestT10_C03_ResourceSuppressedChain(t *testing.T) {
	t.Run("T10-C03", testT10C03TWR)
}
func testT10C03TWR(t *testing.T) {
	// Full-family harness: seed_runner.py only compiles the primary class, so every
	// application class is decompiled and rebuilt here. Original .class files never
	// appear on the rebuilt classpath (see auditSourceSet).
	sources := map[string]string{
		"CloseA.java": `public class CloseA implements AutoCloseable {
  static int closes;
  public void close() throws Exception { closes++; throw new Exception("A"); }
}
`,
		"CloseB.java": `public class CloseB implements AutoCloseable {
  static int closes;
  public void close() throws Exception { closes++; throw new Exception("B"); }
}
`,
		"TWRFamily.java": `public class TWRFamily {
  public static void run() throws Exception {
    try (CloseA a = new CloseA(); CloseB b = new CloseB()) {
      throw new RuntimeException("body");
    }
  }
}
`,
	}
	driver := `public class Driver {public static void main(String[] args){try{TWRFamily.run();}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getMessage());for(Throwable s:e.getSuppressed()){System.out.println(s.getClass().getName()+":"+s.getMessage());}}System.out.println(CloseA.closes);System.out.println(CloseB.closes);}}`
	dir := t.TempDir()
	writeSources(t, dir, sources)
	writeSources(t, dir, map[string]string{"Driver.java": driver})
	auditCommand(t, dir, auditTool(t, "javac"), "--release", "8", "-d", dir, "CloseA.java", "CloseB.java", "TWRFamily.java", "Driver.java")
	got := t10NormalizeOut(auditCommand(t, dir, auditTool(t, "java"), "-Xverify:all", "-cp", dir, "Driver"))
	if got != "java.lang.RuntimeException:body\njava.lang.Exception:B\njava.lang.Exception:A\n1\n1" {
		t.Fatalf("original TWR oracle: got %q", got)
	}
	auditSourceSet(t, sources, driver, javajive.Precision)
}

func TestT10_C05_MonitorEnterNullTiming(t *testing.T) {
	t.Run("T10-C05", testT10C05NullTiming)
}
func testT10C05NullTiming(t *testing.T) {
	// Null monitorenter must throw NPE without a matching monitorexit.
	nullCode := []byte{core.OP_ACONST_NULL, core.OP_MONITORENTER, core.OP_ICONST_1, core.OP_IRETURN}
	raw := auditClass(nullCode, "()I", 1)
	r, err := javajive.DecompileWithOptions(raw, javajive.DecompileOptions{Mode: javajive.Precision})
	if err != nil {
		t.Fatal(err)
	}
	src := strings.ToLower(r.Source)
	if strings.Count(src, "monitorexit") > 0 {
		t.Fatalf("emitted monitorexit for an unacquired lock:\n%s", r.Source)
	}
	// Acquire-then-fail: synchronized(lock){throw} via ordinary Java so exception timing is compared.
	auditRoundTrip(t, "",
		`public static int f(Object lock){synchronized(lock){throw new IllegalStateException("held");}}`,
		`try{Fixture.f(null);System.out.println("returned");}catch(Throwable e){System.out.println(e.getClass().getName());}
Object o=new Object();try{Fixture.f(o);System.out.println("returned");}catch(Throwable e){System.out.println(e.getClass().getName()+":"+Thread.holdsLock(o));}`,
		"-g:none", javajive.Precision)
}

func TestT10_C06_OverlappingProtectedRangesAPI(t *testing.T) {
	t.Run("T10-C06", testT10C06OverlapAPI)
}
func testT10C06OverlapAPI(t *testing.T) {
	code := []byte{
		core.OP_ICONST_1, core.OP_ISTORE_0,
		core.OP_ILOAD_0, core.OP_ICONST_1, core.OP_IDIV, core.OP_ISTORE_0,
		core.OP_ILOAD_0, core.OP_ICONST_1, core.OP_IDIV, core.OP_ISTORE_0,
		core.OP_ILOAD_0, core.OP_ICONST_1, core.OP_IDIV, core.OP_IRETURN,
		core.OP_ASTORE_1, core.OP_ICONST_1, core.OP_IRETURN,
		core.OP_ASTORE_1, core.OP_ICONST_2, core.OP_IRETURN,
	}
	// catch_type must be CONSTANT_Class (CP#4 = java/lang/Object). CP#1 is Utf8.
	raw := auditClassWithExceptions(code, "()I", 2, [][4]int{{2, 10, 14, 4}, {6, 13, 17, 4}})
	r, err := javajive.DecompileWithOptions(raw, javajive.DecompileOptions{Mode: javajive.Precision})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status == "complete" && len(r.StubMethods) == 0 {
		t.Fatalf("overlapping protected ranges silently structured as complete:\n%s", r.Source)
	}
	if r.Status != "partial" && r.Status != "unsupported" {
		t.Fatalf("status=%s want partial/unsupported diagnostics=%+v", r.Status, r.Diagnostics)
	}
	found := false
	for _, d := range r.Diagnostics {
		if strings.Contains(d.Message, "unsupported_overlapping_protected_ranges") ||
			strings.Contains(d.Message, "unsupported") {
			found = true
		}
	}
	if !found && len(r.StubMethods) == 0 {
		t.Fatalf("missing overlapping diagnostic: %+v", r)
	}
}

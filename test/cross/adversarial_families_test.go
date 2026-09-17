package cross

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/javajive"
)

// auditObservation keeps recompilation, verification and execution separate. A
// compiler success never fills in a semantic result, and no original classes
// are on the rebuilt classpath. The driver is an oracle, not a decompile target.
type auditObservation struct {
	Mode             javajive.DecompileMode `json:"mode"`
	Rules            []string               `json:"rules_applied"`
	InputHash        string                 `json:"input_hash"`
	Compiler         string                 `json:"compiler"`
	Debug            string                 `json:"debug"`
	Decompiled       bool                   `json:"decompiled"`
	Recompiled       bool                   `json:"recompiled"`
	OriginalVerified bool                   `json:"original_verified"`
	RebuiltVerified  bool                   `json:"rebuilt_verified"`
	Stub             bool                   `json:"stub"`
	Original         string                 `json:"original"`
	Rebuilt          string                 `json:"rebuilt"`
	Equal            bool                   `json:"equal"`
}

// Reflection triggers verification without initializing or executing the target.
const auditVerifierSource = `public class AuditVerifier { public static void main(String[] names) throws Exception { for (String name : names) { Class<?> c=Class.forName(name, false, AuditVerifier.class.getClassLoader()); c.getDeclaredMethods(); c.getDeclaredConstructors(); c.getDeclaredFields(); } } }`

func auditTool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("semantic validation requires %s: %v", name, err)
	}
	return path
}

func auditCommand(t *testing.T, dir, tool string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", tool, args, err, out)
	}
	return string(out)
}

func auditRoundTrip(t *testing.T, pkg, body, driver, debug string, mode javajive.DecompileMode) {
	t.Helper()
	javac, java := auditTool(t, "javac"), auditTool(t, "java")
	original, rebuilt := t.TempDir(), t.TempDir()
	prefix := ""
	if pkg != "" {
		prefix = "package " + pkg + ";\n"
	}
	source := prefix + "public class Fixture {\n" + body + "\n}\n"
	runner := prefix + "public class Driver {public static void main(String[] args) throws Throwable {" + driver + "}}"
	writeSources(t, original, map[string]string{"Fixture.java": source, "Driver.java": runner, "AuditVerifier.java": auditVerifierSource})
	record := auditObservation{Debug: debug, Mode: mode}
	defer func() {
		b, _ := json.Marshal(record)
		t.Logf("semantic observation: %s", b)
		// Optional durable evidence for CI/local audit without changing the default workspace.
		if dir := os.Getenv("JDEC_SEMANTIC_REPORT_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			name := strings.NewReplacer("/", "_", "\\", "_").Replace(t.Name()) + ".json"
			data, _ := json.MarshalIndent(record, "", "  ")
			if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}()
	record.Compiler = strings.TrimSpace(auditCommand(t, original, javac, "-version"))
	auditCommand(t, original, javac, "--release", "8", debug, "-d", original, "Fixture.java", "Driver.java", "AuditVerifier.java")
	fqn := "Fixture"
	main := "Driver"
	if pkg != "" {
		fqn = pkg + ".Fixture"
		main = pkg + ".Driver"
	}
	raw := readClass(t, original, strings.ReplaceAll(fqn, ".", "/")+".class")
	record.InputHash = fmt.Sprintf("%x", sha256.Sum256(raw))
	auditCommand(t, original, java, "-Xverify:all", "-cp", original, "AuditVerifier", fqn)
	record.OriginalVerified = true
	record.Original = auditCommand(t, original, java, "-Xverify:all", "-cp", original, main)
	result, err := javajive.DecompileWithOptions(raw, javajive.DecompileOptions{Mode: mode})
	src := result.Source
	for _, rule := range result.RulesApplied {
		record.Rules = append(record.Rules, rule.Rule)
	}
	if err != nil {
		t.Fatal(err)
	}
	record.Decompiled = true
	record.Stub = result.Status != "complete" || len(result.StubMethods) > 0
	if record.Stub {
		t.Fatalf("decompilation produced a stub:\n%s", src)
	}
	writeSources(t, rebuilt, map[string]string{"Fixture.java": src, "Driver.java": runner, "AuditVerifier.java": auditVerifierSource})
	t.Logf("decompiled source:\n%s", src)
	auditCommand(t, rebuilt, javac, "--release", "8", "-cp", rebuilt, "-d", rebuilt, "Fixture.java", "Driver.java", "AuditVerifier.java")
	record.Recompiled = true
	auditCommand(t, rebuilt, java, "-Xverify:all", "-cp", rebuilt, "AuditVerifier", fqn)
	record.RebuiltVerified = true
	record.Rebuilt = auditCommand(t, rebuilt, java, "-Xverify:all", "-cp", rebuilt, main)
	record.Equal = record.Original == record.Rebuilt
	if !record.Equal {
		t.Fatalf("behavior changed\noriginal: %q\nrebuilt: %q", record.Original, record.Rebuilt)
	}

}

func TestAuditSemanticFamilies(t *testing.T) {
	cases := []struct{ name, body, driver string }{
		{"A01_dense_negative", `public static int f(int x) {switch(x){case -2:return 12;case -1:return 11;case 0:return 10;default:return 99;}}`, `for(int x=-4;x<4;x++) System.out.println(Fixture.f(x));`},
		{"A02_sparse_extremes", `public static int f(int x) {switch(x){case Integer.MIN_VALUE:return 12;case -1:return 11;case 1048576:case Integer.MAX_VALUE:return 10;default:return 99;}}`, `for(int x:new int[]{Integer.MIN_VALUE,-2,-1,0,1048576,Integer.MAX_VALUE}) System.out.println(Fixture.f(x));`},
		{"A02_shared_default", `public static int f(int x) {switch(x){case -1:case 0:default:return 7;case 8:return 9;}}`, `for(int x:new int[]{-1,0,8,99}) System.out.println(Fixture.f(x));`},
		{"A04_float_slot3", `public static float f(float a,float b,float c,float d){return d;}`, `for(float x:new float[]{4.0f,Float.NaN,-0.0f,Float.POSITIVE_INFINITY}) System.out.println(Float.floatToIntBits(Fixture.f(1,2,3,x)));`},
		{"A14_exception_identity", `public static Throwable saved; public static void f(Throwable e) throws Throwable {try {throw e;}catch(Throwable x){saved=x;throw x;}}`, `Throwable e=new java.io.IOException("original");try{Fixture.f(e);}catch(Throwable x){System.out.println(x.getClass().getName()+":"+(x==e)+":"+(Fixture.saved==e)+":"+x.getCause());}`},
		{"A15_fallthrough", `public static int hits;public static void f(int x){switch(x){case 0:case 8:hits++;default:throw new IllegalStateException();}}`, `for(int x:new int[]{0,8,7}){Fixture.hits=0;try{Fixture.f(x);System.out.println("returned");}catch(Throwable e){System.out.println(Fixture.hits+":"+e.getClass().getName());}}`},
		{"A15_renamed_keys", `public static int hits;public static void f(int x){switch(x){case 1:case 9:hits++;default:throw new IllegalStateException();}}`, `for(int x:new int[]{1,9,7}){Fixture.hits=0;try{Fixture.f(x);System.out.println("returned");}catch(Throwable e){System.out.println(Fixture.hits+":"+e.getClass().getName());}}`},
		{"A15_switch_common_tail", `public static int f(int x,boolean b){try{int y=1;switch(x){case 4:return 44;case 3:y=3;break;case 1:case 2:if(b)return 12;case 5:if(!b){y=5;if(x==5)return 55;}break;}return y+100;}catch(RuntimeException e){return 99;}}`, `for(int x=0;x<7;x++)for(boolean b:new boolean[]{true,false})System.out.println(Fixture.f(x,b));`},
		{"A21_switch_selector_join", `public static int hits;public static int refresh(){hits++;return 0;}public static int f(int x){int n=0;outer:while(true){switch(x==-1?refresh():x){case 1:case 2:break;default:break outer;}switch(x==-1?refresh():x){case 1:n+=3;x=2;continue;case 2:n+=5;x=-1;continue;default:throw new IllegalStateException();}}return n+7;}`, `for(int x=-1;x<4;x++)System.out.println(Fixture.f(x));System.out.println(Fixture.hits);`},
		{"A21_switch_loop_exit", `public static int f(int x){int n=0;outer:while(true){switch(x){case 1:case 2:break;default:break outer;}switch(x){case 1:n+=3;x=2;continue;case 2:n+=5;x=0;continue;default:throw new IllegalStateException();}}return n+7;}`, `for(int x=0;x<4;x++)System.out.println(Fixture.f(x));`},
		{"A15_real_break", `public static int hits;public static void f(int x){switch(x){case 0:case 8:hits++;break;default:throw new IllegalStateException();}}`, `for(int x:new int[]{0,8,7}){Fixture.hits=0;try{Fixture.f(x);System.out.println("returned:"+Fixture.hits);}catch(Throwable e){System.out.println(Fixture.hits+":"+e.getClass().getName());}}`},
		{"A28_literals", `public static String f(){return "Integer::intValue|this.getMatchers()|new TreeSet(Comparator.comparing(Class::getName()))|(Supplier<Object>)(";}`, `System.out.println(Fixture.f());`},
	}
	cases = append(cases, []struct{ name, body, driver string }{
		{"A05_parameter_reuse", `public static int f(Object x){int n=x.hashCode();x="fresh";return n+x.toString().length();}`, `System.out.println(Fixture.f(Integer.valueOf(7)));`},
		{"A06_sibling_locals", `public static int f(boolean b){if(b){String x="one";return x.length();}else{Integer x=17;return x.intValue();}}`, `System.out.println(Fixture.f(true));System.out.println(Fixture.f(false));`},
		{"A07_diamond_null", `public static Object f(int x){Object y;if(x<0)y=null;else if(x==0)y="zero";else y=Integer.valueOf(x);return y;}`, `for(int x:new int[]{-1,0,9})System.out.println(Fixture.f(x));`},
		{"A08_loop_carried", `public static int f(int n){int a=1,b=0;while(n-->0){int x=a;a=a+b;b=x;}return a*31+b;}`, `for(int x=0;x<12;x++)System.out.println(Fixture.f(x));`},
		{"A09_iinc_category2", `public static long f(long x,int y){y+=127;y-=128;long z=x+y;return z+y;}`, `System.out.println(Fixture.f(1234567890123L,10));`},
		{"A10_throw_site", `public static void fail(int w,int at){if(w==at)throw new IllegalArgumentException();} public static int f(int w){int x=0;try{x=1;fail(w,1);x=2;fail(w,2);return 3;}catch(IllegalArgumentException e){return x;}}`, `for(int x=0;x<4;x++)System.out.println(Fixture.f(x));`},
		{"A11_handler_order", `public static int f(int n){try{if(n==0)throw new IllegalArgumentException();if(n==1)throw new IllegalStateException();return 7;}catch(IllegalArgumentException e){return 1;}catch(RuntimeException e){return 2;}}`, `for(int x=0;x<3;x++)System.out.println(Fixture.f(x));`},
		{"A12_finally_capture", `public static int hits;public static int f(int n){int x=n;try{return x;}finally{x=99;hits++;}}`, `System.out.println(Fixture.f(3));System.out.println(Fixture.hits);`},
		{"A12_finally_override", `public static int hits;public static int f(int n){try{if(n==0)return 1;throw new IllegalArgumentException();}finally{hits++;if(n==2)return 2;}}`, `for(int x=0;x<3;x++){try{System.out.println(Fixture.f(x));}catch(Throwable e){System.out.println(e.getClass().getName());}System.out.println(Fixture.hits);}`},
		{"A13_resources_suppressed", `public static String trace="";public static AutoCloseable open(String s){return ()->{trace+=s;throw new java.io.IOException(s);};}public static void f() throws Exception {try(AutoCloseable a=open("a");AutoCloseable b=open("b")){throw new IllegalArgumentException("body");}}`, `try{Fixture.f();}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getMessage());for(Throwable x:e.getSuppressed())System.out.println(x.getClass().getName()+":"+x.getMessage());}System.out.println(Fixture.trace);`},
		{"A25_binary_identity", `public static Object f(boolean b){Object x;if(b)x=new java.sql.Date(0);else x=new java.util.Date(0);return x;}`, `for(boolean b:new boolean[]{true,false})System.out.println(Fixture.f(b).getClass().getName());`},
		{"A26_instanceof_arm", `public static String f(Object x){Object y;if(x instanceof String)y=new StringBuilder();else y=new StringBuffer();return y.getClass().getName();}`, `System.out.println(Fixture.f("x"));System.out.println(Fixture.f(Integer.valueOf(1)));`},
		{"A26_nested_not_subtype", `public static Object f(boolean b){Object x;if(b)x=java.util.AbstractMap.class;else x=java.util.AbstractMap.SimpleEntry.class;return x;}`, `for(boolean b:new boolean[]{true,false})System.out.println(((Class)Fixture.f(b)).getName());`},
		{"A01_default_middle", `public static int f(int x){int y=0;switch(x){case -1:y++;default:y+=2;case 7:y+=4;}return y;}`, `for(int x:new int[]{-1,0,7})System.out.println(Fixture.f(x));`},
		{"A16_shortcircuit_effects", `public static int trace;public static boolean g(int x){trace=trace*10+x;return x!=2;}public static int f(int n){return (g(1)&&(n==0||g(2)))?(g(3)?7:8):9;}`, `for(int n=0;n<2;n++){Fixture.trace=0;System.out.println(Fixture.f(n)+":"+Fixture.trace);}`},
		{"A17_array_postinc", `public static int hits;public static int idx(){hits++;return 0;}public static int f(){int[] a=new int[]{3};int x=a[idx()]++;return x*100+a[0]*10+hits;}`, `System.out.println(Fixture.f());`},
		{"A18_self_array_init", `public static int f(){int[] a=new int[3];a[0]=7;a[1]=a[0]+2;a[2]=a[1]+3;return a[0]*100+a[1]*10+a[2];}`, `System.out.println(Fixture.f());`},
		{"A18_effect_order", `public static int trace;public static int g(int n){trace=trace*10+n;return n;}public static int f(){int[] a=new int[1];a[0]=g(1);g(2);return a[0];}`, `System.out.println(Fixture.f()+":"+Fixture.trace);`},
		{"A18_handler_partial_array", `public static int g(){throw new IllegalStateException();}public static int f(){int[] a=null;try{a=new int[2];a[0]=7;a[1]=g();return 99;}catch(RuntimeException e){return a[0]+a[1];}}`, `System.out.println(Fixture.f());`},
		{"A19_float_compare", `public static int f(double x,double y){int r=0;if(x<y)r|=1;if(x<=y)r|=2;if(x>y)r|=4;if(x>=y)r|=8;if(x==y)r|=16;if(x!=y)r|=32;return r;}`, `for(double x:new double[]{Double.NaN,-0.0,0.0,Double.POSITIVE_INFINITY,-1,1})for(double y:new double[]{Double.NaN,0.0,1})System.out.println(Fixture.f(x,y));`},
		{"A20_integer_edges", `public static long f(int x,int s){return (x/-1)+(x>>>s)+((byte)x)+((char)x);}`, `for(int x:new int[]{Integer.MIN_VALUE,-1,0,255,Integer.MAX_VALUE})for(int s:new int[]{0,31,32,63})System.out.println(Fixture.f(x,s));`},
		{"A21_latch_effects", `public static int hits;public static int step(int n){hits++;return n+2;}public static int f(int n){int r=0;for(int i=0;i<n;i=step(i)){if(i==2)continue;if(i==6)continue;r+=i;}return r;}`, `System.out.println(Fixture.f(10));System.out.println(Fixture.hits);`},
		{"A22_infinite_arm", `public static int hits;public static int f(boolean b){if(b){while(true){hits++;}}hits+=7;return hits;}`, `System.out.println(Fixture.f(false));`},
		{"A24_generic_binding", `public static String pick(Object x){return "object";}public static String pick(String x){return "string";}public static <T> String f(T x){return pick(x);}`, `System.out.println(Fixture.f("x"));`},
		{"A27_final_constructor", `public final String value;public Fixture(boolean b){if(b)value="yes";else value="no";}public String f(){return value;}`, `System.out.println(new Fixture(true).f());System.out.println(new Fixture(false).f());`},
		{"A29_lambda_context", `public static boolean f(int x){java.util.function.IntSupplier s=()->x+2;return s.getAsInt()>3;}`, `for(int x=0;x<4;x++)System.out.println(Fixture.f(x));`},
		{"A30_monitor", `public static volatile int hits;public static int f(Object a,Object b){synchronized(a){hits++;synchronized(b){hits+=2;}return hits;}}`, `Object a=new Object();System.out.println(Fixture.f(a,a));System.out.println(Thread.holdsLock(a));`},
	}...)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, debug := range []string{"-g", "-g:none"} {
				t.Run(debug, func(t *testing.T) {
					pkg := "audit"
					if tc.name == "A28_literals" {
						pkg = "org.mockito"
					}
					for _, mode := range []javajive.DecompileMode{javajive.Precision, javajive.Compatibility} {
						t.Run(string(mode), func(t *testing.T) { auditRoundTrip(t, pkg, tc.body, tc.driver, debug, mode) })
					}
				})
			}
		})
	}
}

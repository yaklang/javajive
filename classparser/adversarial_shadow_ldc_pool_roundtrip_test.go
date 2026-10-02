package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Public shadow analysis must consume actual CP literals, not only test IRs
// with Const manually filled. This does not migrate or validate an SSA emitter:
// the default source path remains separately reconstructed and JVM-compared.
func TestAdversarialShadowLDCPoolKindsRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	var source strings.Builder
	source.WriteString("public class ShadowLDCPoolReview { static final float FIRST_FLOAT=1.25f; static final String FIRST_STRING=\"small\";\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&source, "static final int F%d=%d;\n", i, 1000000+i)
	}

	source.WriteString("static int wide(int value){switch(value){\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&source, "case %d:return %d;\n", i, 2000000+i)
	}
	source.WriteString("default:return -7654321;}}\n")
	source.WriteString(`
 static int first(){return F0;}
 static int last(){return F299;}
 static float fraction(){return FIRST_FLOAT;}
 static double wideFraction(){return 3.25;}
 static String text(){return FIRST_STRING;}
 static Class<?> type(){return String.class;}
 static Class<?> objectArrayType(){return String[][].class;}
 static Class<?> primitiveArrayType(){return int[][].class;}
 static String append(){return new StringBuilder().append("x").append(7L).toString();}
 static long swap(int n){long a=3L,b=7L;for(int i=0;i<n;i++){long old=a;a=b;b=old;}return a*10L+b;}
 static int saved(int d,int next){int a=7;try{int b=1/d;a=next;return b+1/(d-1);}catch(ArithmeticException ex){return a;}}
 public static void main(String[] args){System.out.println(first());System.out.println(last());System.out.println(wide(299));System.out.println(Float.floatToRawIntBits(fraction()));System.out.println(Double.doubleToRawLongBits(wideFraction()));System.out.println(text());System.out.println(type()==String.class);System.out.println(objectArrayType()==String[][].class);System.out.println(primitiveArrayType()==int[][].class);System.out.println(append());for(int i=0;i<6;i++)System.out.println(swap(i));System.out.println(saved(0,11));System.out.println(saved(1,11));}
}`)
	for _, release := range []string{"8", "17"} {
		for _, debug := range []string{"-g", "-g:none"} {
			t.Run(release+"/"+debug, func(t *testing.T) {
				original := t.TempDir()
				file := filepath.Join(original, "ShadowLDCPoolReview.java")
				if err := os.WriteFile(file, []byte(source.String()), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", release, debug, "-d", original, file).CombinedOutput(); err != nil {
					t.Fatalf("original: %v\n%s", err, out)
				}
				want := t04RunJava(t, java, original, "ShadowLDCPoolReview")
				if strings.TrimSpace(want) != "1000000\n1000299\n2000299\n1067450368\n4614500768194494464\nsmall\ntrue\ntrue\ntrue\nx7\n37\n73\n37\n73\n37\n73\n7\n11" {
					t.Fatalf("independent original JVM oracle: %q", want)
				}
				raw, err := os.ReadFile(filepath.Join(original, "ShadowLDCPoolReview.class"))
				if err != nil {
					t.Fatal(err)
				}
				obj, err := Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				seen := map[int]bool{}
				for _, m := range obj.Methods {
					for _, attr := range m.Attributes {
						if code, ok := attr.(*CodeAttribute); ok {
							decoder := core.NewDecompiler(code.Code, nil)
							if err := decoder.ParseOpcode(); err != nil {
								t.Fatal(err)
							}
							for _, op := range decoder.Opcodes() {
								if op != nil && op.Instr != nil {
									seen[op.Instr.OpCode] = true
								}
							}
						}
					}
				}
				for _, opcode := range []int{core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W} {
					if !seen[opcode] {
						t.Fatalf("authored fixture did not exercise actual CP opcode0x%x", opcode)
					}
				}
				for _, mode := range []DecompileMode{Precision, Compatibility} {
					t.Run(string(mode), func(t *testing.T) {
						off, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, EnvSnapshot: map[string]string{}})
						if err != nil {
							t.Fatal(err)
						}
						on, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, EnableShadowIR: true, EnvSnapshot: map[string]string{}})
						if err != nil {
							t.Fatal(err)
						}
						if on.Source != off.Source || on.Status != off.Status || !reflect.DeepEqual(on.RulesApplied, off.RulesApplied) {
							t.Fatal("shadow changed default source/status/rules")
						}
						if on.Status != "complete" || len(on.StubMethods) > 0 {
							t.Fatalf("default source not supported: %s %v", on.Status, on.StubMethods)
						}
						if len(on.Shadow) != len(obj.Methods) {
							t.Fatalf("missing method observations: %d/%d", len(on.Shadow), len(obj.Methods))
						}
						for _, observation := range on.Shadow {
							if observation.Status != "ok" || observation.Hash == "" || observation.Version == 0 {
								t.Fatalf("original CP failed production frame/SSA/lowering %s: %+v", observation.Method, observation)
							}
						}
						rebuilt := t.TempDir()
						file := filepath.Join(rebuilt, "ShadowLDCPoolReview.java")
						if err := os.WriteFile(file, []byte(on.Source), 0600); err != nil {
							t.Fatal(err)
						}
						if out, err := exec.Command(javac, "-proc:none", "--release", release, "-d", rebuilt, file).CombinedOutput(); err != nil {
							t.Fatalf("rebuild: %v\n%s\n%s", err, out, on.Source)
						}
						if got := t04RunJava(t, java, rebuilt, "ShadowLDCPoolReview"); got != want {
							t.Fatalf("default printer behavior changed: got%q want%q", got, want)
						}
					})
				}
			})
		}
	}
}

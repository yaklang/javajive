package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A short type/method spelling supplies no declaration or dispatch proof.
// This Comparator is an authored ordinary declaration, not the JDK interface.
// Its factory and compare method have observable effects that compareTo lacks.
func TestAdversarialFactoryCallCannotBeReplacedBySourceSpelling(t *testing.T) {
	javac, java := t04Tools(t)
	for _, owner := range []string{"FactoryCallOwner", "RenamedFactoryCaller"} {
		t.Run(owner, func(t *testing.T) {
			sources := map[string]string{
				owner + ".java": strings.ReplaceAll(`public class FactoryCallOwner{
 public static int run(int unused,FactoryWordToken left,FactoryWordToken right){return Comparator.naturalOrder().compare(left,right);}
 public static String data(){return "assertj return Comparator.naturalOrder().compare(var1,var2)";}
}`, "FactoryCallOwner", owner),
				"Comparator.java": `public class Comparator{
 static final Comparator INSTANCE=new Comparator();static{FactoryWordState.trace+="I";}
 public static Comparator naturalOrder(){FactoryWordState.trace+="F";FactoryWordState.calls++;if(FactoryWordState.mode==1)throw FactoryWordState.fault;return FactoryWordState.mode==2?null:INSTANCE;}
 public int compare(FactoryWordToken left,FactoryWordToken right){FactoryWordState.trace+="C";return left==null?right==null?7:31:right==null?-31:100+left.key*7-right.key*3;}
}`,
				"FactoryWordDriver.java": strings.ReplaceAll(`class FactoryWordState{
 static String trace="";static int calls,mode;static final RuntimeException fault=new IllegalStateException("same");
}
class FactoryWordToken implements Comparable<FactoryWordToken>{
 final int key;FactoryWordToken(int key){this.key=key;}
 public int compareTo(FactoryWordToken right){FactoryWordState.trace+="T";return -900;}
}
class FactoryWordDriver{
 public static void main(String[]args){int rows=0;
  FactoryWordToken one=new FactoryWordToken(-3),two=new FactoryWordToken(5);
  for(int mode=0;mode<3;mode++)for(FactoryWordToken[] pair:new FactoryWordToken[][]{{one,two},{null,two},{one,null},{null,null}}){
   FactoryWordState.trace="";FactoryWordState.calls=0;FactoryWordState.mode=mode;int got=0;Throwable thrown=null;
   try{got=FactoryCallOwner.run(0,pair[0],pair[1]);}catch(Throwable e){thrown=e;}
   int expected=pair[0]==null?pair[1]==null?7:31:pair[1]==null?-31:100+pair[0].key*7-pair[1].key*3;
   if(mode==0?thrown!=null||got!=expected:mode==1?thrown!=FactoryWordState.fault:!(thrown instanceof NullPointerException))throw new AssertionError("factory/selected implementation/failure identity",thrown);
   String trace=(rows==0?"I":"")+(mode==0?"FC":"F");
   if(FactoryWordState.calls!=1||!FactoryWordState.trace.equals(trace))throw new AssertionError("factory invocation disappeared or was duplicated: "+FactoryWordState.trace);
   rows++;
  }
  if(!FactoryCallOwner.data().equals("assertj return Comparator.naturalOrder().compare(var1,var2)"))throw new AssertionError("application data changed");rows++;
  System.out.println(rows+":factory:binding:effects:identity");
 }
}`, "FactoryCallOwner", owner),
			}
			files := nativeCompileSourceReleaseClasses(t, sources, "none", "8")
			write := func(dir, name string, raw []byte) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			original := t.TempDir()
			for name, raw := range files {
				write(original, name, raw)
			}
			const want = "13:factory:binding:effects:identity\n"
			if got := t04RunJava(t, java, original, "FactoryWordDriver"); got != want {
				t.Fatalf("independent original caller: %q want %q", got, want)
			}
			t.Log("independent original factory-spelling JVM: " + strings.TrimSpace(want))
			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{
						Mode: mode, SourceCompiler: ModernJavac, TargetSourceVersion: 8, EnvSnapshot: map[string]string{},
						Resolve: func(name string) ([]byte, bool) { b, known := files[name+".class"]; return bytes.Clone(b), known },
					})
					if err != nil || result.Status != "complete" || len(result.StubMethods) != 0 || strings.Contains(result.Source, DecompileStubMarker) {
						t.Fatalf("incomplete source: %v status=%s diagnostics=%+v\n%s", err, result.Status, result.Diagnostics, result.Source)
					}
					out := t.TempDir()
					for name, raw := range files {
						if name != owner+".class" {
							write(out, name, raw)
						}
					}
					path := filepath.Join(out, owner+".java")
					write(out, owner+".java", []byte(result.Source))
					if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-cp", out, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatalf("rebuild without original target: %v\n%s\n%s", err, log, result.Source)
					}
					if got := t04RunJava(t, java, out, "FactoryWordDriver"); got != want {
						t.Fatalf("unchanged original binary caller: %q want %q\n%s", got, want, result.Source)
					}
					rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
					if err != nil {
						t.Fatal(err)
					}
					if a, b := constructorInvocationThrowsDeclarations(t, files[owner+".class"]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
						t.Fatalf("declarations changed:\n%s\n%s", a, b)
					}
					if a, b := arrayProducerCheckedInvocations(t, files[owner+".class"]), arrayProducerCheckedInvocations(t, rebuilt); a != b {
						t.Fatalf("factory, receiver or selected invocation changed:\n%s\n%s", a, b)
					}
					t.Log("fully rebuilt factory-spelling JVM: " + strings.TrimSpace(want))
				})
			}
		})
	}
}

func TestAdversarialNaturalFactorySourceWordsCannotEraseInvocation(t *testing.T) {
	t.Setenv("JDEC_ASSERTJ_REMAINING_OFF", "")
	for _, source := range []string{
		`class C{String marker="assertj";int run(Token var1,Token var2){return Comparator.naturalOrder().compare(var1,var2);}}`,
		`class C{String data="assertj return Comparator.naturalOrder().compare(var1,var2)";}`,
	} {
		if got := fixAssertjRemainingReconstructs(source); got != source {
			t.Fatalf("source spelling cannot erase a factory or mutate literal data:\nbefore %s\nafter %s", source, got)
		}
	}
}

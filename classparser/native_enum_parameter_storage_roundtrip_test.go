package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The original driver models state transitions with ordinals and modular
// arithmetic, independently of the target switch. Equal-typed parameters,
// a preceding wide word, repeated loop stores, null and throwing assignment
// producers exercise declaration identity rather than an entry-value guess.
func nativeMutableEnumFixture(shape, owner string) map[string]string {
	terminal := `switch(first){case A:return base+delta+0;case B:return base+delta+1;default:return base+delta+2;}`
	body, model := "", ""
	switch shape {
	case "replace":
		body, model = `first=other;`+terminal, `state=other;`
	case "branch":
		body, model = `if(flag)first=other;`+terminal, `state=flag?other:first;`
	case "loop":
		body = `for(int i=0;i<steps;i++){switch(first){case A:first=MutableOwnerMode.B;break;case B:first=MutableOwnerMode.C;break;default:first=MutableOwnerMode.A;}}return base+delta+first.ordinal();`
		model = `if(first==null)nil=true;else state=MutableOwnerMode.values()[(first.ordinal()+steps)%3];`
	case "nullable":
		body, model = `if(flag)first=null;`+terminal, `state=flag?null:first;`
	case "throwing producer":
		body, model = `first=MutableEffects.choose(other,flag);`+terminal, `state=other;thrown=flag;calls=1;`
	case "handler writer":
		body, model = `try{first=MutableEffects.choose(other,flag);}catch(IllegalStateException failure){first=MutableOwnerMode.C;}`+terminal, `state=flag?MutableOwnerMode.C:other;calls=1;`
	case "computed selector":
		body, model = `first=MutableOwnerMode.B;`+strings.Replace(terminal, "switch(first)", "switch(MutableEffects.select(first,other,padding))", 1), `state=MutableOwnerMode.B;calls=1;`
	default:
		panic("unknown authored fixture")
	}
	fixture := `public class MutableOwner{
 private final int base;public MutableOwner(int base){this.base=base;}
 public abstract static class Task{public abstract int run(long padding,MutableOwnerMode first,MutableOwnerMode other,int steps,boolean flag);}
 public Task make(final int delta){return new Task(){public int run(long padding,MutableOwnerMode first,MutableOwnerMode other,int steps,boolean flag){BODY}};}
}
class MutableDriver{public static void main(String[]a){int rows=0;
 for(int base:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int delta:new int[]{-1,0,Integer.MAX_VALUE}){
 MutableOwner.Task task=new MutableOwner(base).make(delta);
 if(task.getClass().getEnclosingMethod()==null||!task.getClass().getEnclosingMethod().getName().equals("make")||MutableOwner.Task.class.getDeclaringClass()!=MutableOwner.class)throw new AssertionError("declaration identity");
 for(MutableOwnerMode first:new MutableOwnerMode[]{MutableOwnerMode.A,MutableOwnerMode.B,MutableOwnerMode.C,null})for(MutableOwnerMode other:new MutableOwnerMode[]{MutableOwnerMode.A,MutableOwnerMode.B,MutableOwnerMode.C,null})for(int steps:new int[]{0,1,2,4,7})for(boolean flag:new boolean[]{false,true}){
 MutableOwnerMode state=first;boolean nil=false,thrown=false;int calls=0;MODEL
 nil=nil||state==null;MutableEffects.calls=0;
 try{int got=task.run(Long.MIN_VALUE,first,other,steps,flag);
 if(thrown||nil)throw new AssertionError("missing exception");
 int expected=java.math.BigInteger.valueOf(base).add(java.math.BigInteger.valueOf(delta)).add(java.math.BigInteger.valueOf(state.ordinal())).intValue();
 if(got!=expected)throw new AssertionError("mutable binding/state/overflow");
 }catch(NullPointerException failure){if(!nil||thrown)throw new AssertionError("unexpected null");}
 catch(IllegalStateException failure){if(!thrown||failure!=MutableEffects.marker)throw new AssertionError("exception identity");}
 if(MutableEffects.calls!=calls)throw new AssertionError("assignment producer count");rows++;
 }}System.out.println(rows+":mutable:binding:loop:null:effects:overflow:lexical");}}
class MutableEffects{static int calls;static final IllegalStateException marker=new IllegalStateException("original-marker");static MutableOwnerMode choose(MutableOwnerMode value,boolean fail){calls++;if(fail)throw marker;return value;}static MutableOwnerMode select(MutableOwnerMode first,MutableOwnerMode other,long padding){calls++;return padding==Long.MIN_VALUE?first:other;}}`
	fixture = strings.Replace(fixture, "BODY", body, 1)
	fixture = strings.Replace(fixture, "MODEL", model, 1)
	fixture = strings.ReplaceAll(fixture, "MutableOwner", owner)
	return map[string]string{owner + ".java": fixture, owner + "Mode.java": "public enum " + owner + "Mode{A,B,C}"}
}

func TestAdversarialEnumSwitchMutableParameterStorageRoundTrip(t *testing.T) {
	for _, shape := range []string{"replace", "branch", "loop", "nullable", "throwing producer", "handler writer", "computed selector"} {
		t.Run(shape, func(t *testing.T) {
			testNativeEnumSwitchSourceFixture(t, nativeMutableEnumFixture(shape, "MutableOwner"), "MutableOwner", "MutableDriver", "2400:mutable:binding:loop:null:effects:overflow:lexical\n")
		})
	}
}

func TestAdversarialEnumSwitchMutableParameterNativeCompilerRoundTrip(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independently compiled native lowering")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original compiler", err, string(version))
	}
	for _, shape := range []string{"loop", "handler writer", "computed selector"} {
		t.Run(shape, func(t *testing.T) {
			sources := nativeMutableEnumFixture(shape, "RenamedMutableScope")
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				dir := t.TempDir()
				paths := []string{}
				for name, source := range sources {
					path := filepath.Join(dir, name)
					if err := os.WriteFile(path, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
					paths = append(paths, path)
				}
				if out, err := exec.Command(javac, append([]string{"-proc:none", "-source", "8", "-target", "8", "-g:" + debug, "-d", dir}, paths...)...).CombinedOutput(); err != nil {
					t.Fatal("original compile", err, string(out))
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				files := map[string][]byte{}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".class") {
						raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
						if err != nil {
							t.Fatal(err)
						}
						files[entry.Name()] = raw
					}
				}
				return files
			}, NativeJavac8, javac, []string{"RenamedMutableScope"}, "MutableDriver", "2400:mutable:binding:loop:null:effects:overflow:lexical\n", nil, nativeLexicalExactSignatures)
		})
	}
}

// Actually compile behavior-changing programs and run the unchanged original
// driver. An oracle that merely approves any compiling switch would miss these.
func TestAdversarialEnumSwitchMutableParameterOracleRejectsBehavioralMutants(t *testing.T) {
	_, java := t04Tools(t)
	mutants := []struct{ shape, from, to string }{
		{"replace", "first=other;", "first=first;"},
		{"branch", "if(flag)first=other;", "if(!flag)first=other;"},
		{"loop", "first=MutableOwnerMode.B;", "first=MutableOwnerMode.C;"},
		{"nullable", "if(flag)first=null;", "if(false)first=null;"},
		{"throwing producer", "first=MutableEffects.choose(other,flag);", "first=MutableEffects.choose(other,flag);first=MutableEffects.choose(other,flag);"},
		{"handler writer", "first=MutableOwnerMode.C;", "first=other;"},
		{"computed selector", "MutableEffects.select(first,other,padding)", "MutableEffects.select(other,first,padding)"},
	}
	for _, mutant := range mutants {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(mutant.shape+"/"+debug, func(t *testing.T) {
				sources := nativeMutableEnumFixture(mutant.shape, "MutableOwner")
				if strings.Count(sources["MutableOwner.java"], mutant.from) != 1 {
					t.Fatal("mutation must touch exactly one target operation")
				}
				sources["MutableOwner.java"] = strings.Replace(sources["MutableOwner.java"], mutant.from, mutant.to, 1)
				files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
				dir := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				out, err := exec.Command(java, "-Xverify:all", "-cp", dir, "MutableDriver").CombinedOutput()
				if err == nil || !strings.Contains(string(out), "java.lang.AssertionError") {
					t.Fatalf("unchanged original oracle admitted behavioral mutant: %v\n%s", err, out)
				}
			})
		}
	}
}

package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeOrderedAssertionFixture = `class OrderedEffects {static String trace=""; static RuntimeException poison=new RuntimeException("poison");}
class OrderedOwner { boolean gate(int x){OrderedEffects.trace+="P";if(x<0)throw OrderedEffects.poison;return x!=0;} boolean predicate(int x){OrderedEffects.trace+="C";if(x<0)throw OrderedEffects.poison;return x!=0;} Object payload(Object t){OrderedEffects.trace+="M";return t;} class Child {int run(int a,int b,int c,Object t){if(GUARD){assert predicate(c):payload(t);}OrderedEffects.trace+="E";return c+40;}} Child make(){return new Child();} }
class OrderedDriver {public static void main(String[]z){boolean enabled=ENABLED;ClassLoader.getSystemClassLoader().setClassAssertionStatus("OrderedOwner",enabled);OrderedOwner.Child c=new OrderedOwner().make();int[][]cases={{0,1,0},{1,0,0},{1,1,0},{1,1,2},{-1,1,2},{1,-1,2},{1,1,-1}};for(int[]v:cases){OrderedEffects.trace="";String prefix="P";boolean throwsGate=v[0]<0;boolean active=v[0]!=0&&!throwsGate;if(TWO&&active){prefix+="P";throwsGate=v[1]<0;active=v[1]!=0&&!throwsGate;}String want=prefix;if(throwsGate){try{c.run(v[0],v[1],v[2],"msg");throw new AssertionError("missing gate poison");}catch(RuntimeException e){if(e!=OrderedEffects.poison)throw new AssertionError("gate identity");}}else if(enabled&&active&&v[2]<0){want+="C";try{c.run(v[0],v[1],v[2],"msg");throw new AssertionError("missing predicate poison");}catch(RuntimeException e){if(e!=OrderedEffects.poison)throw new AssertionError("predicate identity");}}else if(enabled&&active&&v[2]==0){want+="CM";try{c.run(v[0],v[1],v[2],"msg");throw new AssertionError("missing assertion");}catch(AssertionError e){if(!"msg".equals(e.getMessage()))throw new AssertionError("message",e);}}else{want+=(enabled&&active?"C":"")+"E";if(c.run(v[0],v[1],v[2],"msg")!=v[2]+40)throw new AssertionError("return");}if(!OrderedEffects.trace.equals(want))throw new AssertionError("order:"+OrderedEffects.trace+":"+want);}System.out.println("ordered-assertion:effects:short-circuit:identity:"+enabled);}}`

func TestNativeAssertionOrderedPrefixRoundTrip(t *testing.T) {
	testNativeAssertionOrderedPrefix(t, false)
}
func TestNativeAssertionOrderedPrefixOriginalCompiler(t *testing.T) {
	if os.Getenv("JAVA8_JAVAC") == "" {
		t.Skip("JAVA8_JAVAC required for original compiler")
	}
	testNativeAssertionOrderedPrefix(t, true)
}
func testNativeAssertionOrderedPrefix(t *testing.T, native bool) {
	t.Helper()
	for _, two := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, renamed := range []bool{false, true} {
				t.Run(fmt.Sprintf("two=%t/enabled=%t/renamed=%t", two, enabled, renamed), func(t *testing.T) {
					guard := "gate(a)"
					if two {
						guard += "&&gate(b)"
					}
					f := strings.NewReplacer("GUARD", guard, "ENABLED", fmt.Sprint(enabled), "TWO", fmt.Sprint(two)).Replace(nativeOrderedAssertionFixture)
					owner := "OrderedOwner"
					if renamed {
						f = strings.NewReplacer("OrderedOwner", "OtherScope", "gate(", "outerPredicate(", "predicate(", "innerPredicate(", "payload(", "detail(", "run(", "advance(").Replace(f)
						owner = "OtherScope"
					}
					want := fmt.Sprintf("ordered-assertion:effects:short-circuit:identity:%t\n", enabled)
					if native {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": f}, debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, "OrderedDriver", want, nil, nativeLexicalExactSignatures)
					} else {
						testNativePrivateSetterCompiledFixture(t, owner, "OrderedDriver", want, func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeLexicalExactSignatures)
					}
				})
			}
		}
	}
}

// The unchanged driver rejects code that compiles but changes assertion domains.
func TestNativeAssertionOrderedPrefixOracleRejectsMutants(t *testing.T) {
	javac, java := t04Tools(t)
	body := `if(GUARD){assert predicate(c):payload(t);}OrderedEffects.trace+="E";return c+40;`
	mutants := []struct{ name, body string }{{"original", body}, {"gate inside assertion", `assert !gate(a)||predicate(c):payload(t);OrderedEffects.trace+="E";return c+40;`}, {"eager predicate", `boolean q=predicate(c);if(GUARD){assert q:payload(t);}OrderedEffects.trace+="E";return c+40;`}, {"repeated gate", `if(gate(a)&&GUARD){assert predicate(c):payload(t);}OrderedEffects.trace+="E";return c+40;`}, {"lost continuation", `if(GUARD){assert predicate(c):payload(t);}return c+40;`}, {"eager message", `Object m=payload(t);if(GUARD){assert predicate(c):m;}OrderedEffects.trace+="E";return c+40;`}}
	for _, mutant := range mutants {
		t.Run(mutant.name, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				f := strings.Replace(nativeOrderedAssertionFixture, body, mutant.body, 1)
				f = strings.NewReplacer("GUARD", "gate(a)", "ENABLED", fmt.Sprint(enabled), "TWO", "false").Replace(f)
				dir := t.TempDir()
				p := filepath.Join(dir, "OrderedOwner.java")
				if err := os.WriteFile(p, []byte(f), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, p).CombinedOutput(); err != nil {
					t.Fatalf("oracle compile: %v %s", err, out)
				}
				for _, mode := range []string{"-ea", "-da"} {
					out, err := exec.Command(java, mode, "-cp", dir, "OrderedDriver").CombinedOutput()
					wantRejected := mutant.name != "original" && (mutant.name != "gate inside assertion" || !enabled)
					if !wantRejected {
						if err != nil || string(out) != fmt.Sprintf("ordered-assertion:effects:short-circuit:identity:%t\n", enabled) {
							t.Fatalf("valid assertion domain: %v %s", err, out)
						}
					} else if err == nil {
						t.Fatal("mutant accepted", enabled, mode)
					}
				}
			}
		})
	}
}

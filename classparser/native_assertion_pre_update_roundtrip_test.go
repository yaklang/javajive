package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const nativePreUpdateAssertionFixture = `class PreOwner{static String trace="";int payload(int n){trace+="M";return n;}class Child{int run(int value){assert UPDATE>0:payload(value);trace+="E";return value;}}Child make(){return new Child();}}
class PreDriver{public static void main(String[]args){boolean enabled=ENABLED;ClassLoader.getSystemClassLoader().setClassAssertionStatus("PreOwner",enabled);PreOwner.Child c=new PreOwner().make();int[]v={-2,-1,0,1,Integer.MAX_VALUE,Integer.MIN_VALUE};for(int x:v){PreOwner.trace="";int expected=enabled?x+DELTA:x;boolean failure=enabled&&expected<=0;try{int r=c.run(x);if(failure||r!=expected)throw new AssertionError("value/required assertion");}catch(AssertionError e){if(!failure||!String.valueOf(expected).equals(e.getMessage()))throw new AssertionError("failure value",e);}if(!PreOwner.trace.equals(failure?"M":"E"))throw new AssertionError("order");}System.out.println("pre-update:"+enabled);}}`

func TestNativeAssertionPreUpdateKeepsOriginalDomains(t *testing.T) {
	testNativeAssertionPreUpdate(t, false)
}

func TestNativeAssertionPreUpdateOriginalCompiler(t *testing.T) {
	if os.Getenv("JAVA8_JAVAC") == "" {
		t.Skip("JAVA8_JAVAC required for original compiler")
	}
	testNativeAssertionPreUpdate(t, true)
}

func testNativeAssertionPreUpdate(t *testing.T, native bool) {
	t.Helper()
	for _, enabled := range []bool{false, true} {
		for _, delta := range []int{1, -1} {
			for _, renamed := range []bool{false, true} {
				t.Run(fmt.Sprintf("enabled=%t/delta=%d/renamed=%t", enabled, delta, renamed), func(t *testing.T) {
					op := "++value"
					if delta < 0 {
						op = "--value"
					}
					f := strings.NewReplacer("UPDATE", op, "ENABLED", fmt.Sprint(enabled), "DELTA", fmt.Sprint(delta)).Replace(nativePreUpdateAssertionFixture)
					owner := "PreOwner"
					if renamed {
						owner = "AlternateScope"
						f = strings.NewReplacer("PreOwner", owner, "Child", "Member", "payload(", "detail(", "run(", "evaluate(").Replace(f)
						f = regexp.MustCompile(`\bvalue\b`).ReplaceAllString(f, "argument")
					}
					want := fmt.Sprintf("pre-update:%t\n", enabled)
					if native {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": f}, debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, "PreDriver", want, nil, nativeLexicalExactSignatures)
					} else {
						testNativePrivateSetterCompiledFixture(t, owner, "PreDriver", want, func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeLexicalExactSignatures)
					}
				})
			}
		}
	}
}

const nativeUpdateEvaluationOrderFixture = `class UpdateOwner {
static String trace="";
static int stamp(int x){trace+="P";return x*7;}
class Child {
 int ordered(int value){return stamp(value)+(UPDATE);}
 int twice(int value){return (UPDATE)+(UPDATE);}
 int prefixCopy(int value){int old=UPDATE;return old*17+value;}
 int postfixCopy(int value){int old=POST;return old*17+value;}
 int snapshot(int value){int old=value;UPDATE;return old*17+value;}
 int operands(int value){return value+(UPDATE);}
}
Child make(){return new Child();}}
class UpdateDriver {public static void main(String[]args){UpdateOwner.Child c=new UpdateOwner().make();int[]cases={-2,-1,0,1,2,Integer.MAX_VALUE,Integer.MIN_VALUE};for(int x:cases){int next=x+DELTA;UpdateOwner.trace="";if(c.ordered(x)!=x*7+next||!UpdateOwner.trace.equals("P"))throw new AssertionError("call/update order");if(c.twice(x)!=next+(next+DELTA))throw new AssertionError("repeated update");if(c.prefixCopy(x)!=next*17+next)throw new AssertionError("prefix result");if(c.postfixCopy(x)!=x*17+next)throw new AssertionError("postfix result");if(c.snapshot(x)!=x*17+next)throw new AssertionError("snapshot");if(c.operands(x)!=x+next)throw new AssertionError("old operand");}System.out.println("update:order:old:new:overflow");}}`

func TestNativeUpdateOriginalOperandEvaluationOrder(t *testing.T) {
	for _, native := range []bool{false, true} {
		if native && os.Getenv("JAVA8_JAVAC") == "" {
			continue
		}
		for _, delta := range []int{1, -1} {
			t.Run(fmt.Sprintf("native=%t/delta=%d", native, delta), func(t *testing.T) {
				pre, post := "++value", "value++"
				if delta < 0 {
					pre, post = "--value", "value--"
				}
				f := strings.NewReplacer("UPDATE", pre, "POST", post, "DELTA", fmt.Sprint(delta)).Replace(nativeUpdateEvaluationOrderFixture)
				if native {
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
						return localGetterCompileJavac8(t, map[string]string{"UpdateOwner.java": f}, debug)
					}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"UpdateOwner"}, "UpdateDriver", "update:order:old:new:overflow\n", nil, nativeLexicalExactSignatures)
				} else {
					testNativePrivateSetterCompiledFixture(t, "UpdateOwner", "UpdateDriver", "update:order:old:new:overflow\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) }, nativeLexicalExactSignatures)
				}
			})
		}
	}
}

// The driver derives the expected value, message and effects without reading
// generated source. Some domain mutations are equivalent in one assertion
// mode; require rejection only where their behavior really differs.
func TestNativeAssertionPreUpdateOracleRejectsMutants(t *testing.T) {
	javac, java := t04Tools(t)
	body := `assert UPDATE>0:payload(value);trace+="E";return value;`
	mutants := []struct {
		name, body        string
		enabled, disabled bool
	}{
		{"original", body, false, false},
		{"eager update", `UPDATE;assert value>0:payload(value);trace+="E";return value;`, false, true},
		{"repeated update", `assert UPDATE>0&&UPDATE>0:payload(value);trace+="E";return value;`, true, false},
		{"postfix substituted", `assert value++>0:payload(value);trace+="E";return value;`, true, false},
		{"lost update", `assert value>0:payload(value);trace+="E";return value;`, true, false},
		{"eager message", `int message=payload(value);assert UPDATE>0:message;trace+="E";return value;`, true, true},
	}
	for _, mutant := range mutants {
		t.Run(mutant.name, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				f := strings.Replace(nativePreUpdateAssertionFixture, body, mutant.body, 1)
				if f == nativePreUpdateAssertionFixture && mutant.name != "original" {
					t.Fatal("mutant body not applied")
				}
				f = strings.NewReplacer("UPDATE", "++value", "DELTA", "1", "ENABLED", fmt.Sprint(enabled)).Replace(f)
				dir := t.TempDir()
				path := filepath.Join(dir, "PreOwner.java")
				if err := os.WriteFile(path, []byte(f), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, path).CombinedOutput(); err != nil {
					t.Fatalf("compile oracle: %v %s", err, out)
				}
				for _, mode := range []string{"-ea", "-da"} {
					out, err := exec.Command(java, mode, "-cp", dir, "PreDriver").CombinedOutput()
					reject := mutant.disabled
					if enabled {
						reject = mutant.enabled
					}
					if reject {
						if err == nil {
							t.Fatal("mutant accepted", enabled, mode)
						}
					} else if err != nil || string(out) != fmt.Sprintf("pre-update:%t\n", enabled) {
						t.Fatalf("valid mode: %v %s", err, out)
					}
				}
			}
		})
	}
}

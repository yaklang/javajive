package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// All methods are compiled into one fixture so the immediate-width bank adds
// algorithm coverage without a javac invocation per arithmetic boundary.
func nativeCompoundUpdateFixture(enabled bool, renamed bool) (string, string) {
	deltas := []int{-32768, -129, -128, 0, 2, 127, 128, 32767}
	var methods, calls strings.Builder
	for i, delta := range deltas {
		fmt.Fprintf(&methods, "int update%d(int value){assert (value+=%d)>0:payload(value);trace+=\"E\";return value;}\n", i, delta)
		fmt.Fprintf(&calls, "case %d: return c.update%d(value);\n", i, i)
	}
	source := fmt.Sprintf(`class CompoundOwner {static String trace="";int payload(int value){trace+="M";return value;}class Child {%s}Child make(){return new Child();}}
class CompoundDriver {
 static int run(CompoundOwner.Child c,int method,int value){switch(method){%s default:throw new AssertionError("bad method");}}
 public static void main(String[]args){boolean enabled=%t;ClassLoader loader=ClassLoader.getSystemClassLoader();loader.setDefaultAssertionStatus(!enabled);loader.setClassAssertionStatus("CompoundOwner",enabled);CompoundOwner.Child c=new CompoundOwner().make();loader.setClassAssertionStatus("CompoundOwner",!enabled);int[]deltas={-32768,-129,-128,0,2,127,128,32767};int[]values={Integer.MIN_VALUE,Integer.MIN_VALUE+1,-32769,-129,-128,-2,-1,0,1,2,127,128,32767,Integer.MAX_VALUE-1,Integer.MAX_VALUE};for(int method=0;method<deltas.length;method++){for(int value:values){long exact=(long)value+deltas[method];int next=enabled?(int)exact:value;boolean failure=enabled&&next<=0;CompoundOwner.trace="";try{int got=run(c,method,value);if(failure||got!=next)throw new AssertionError("required update/result");}catch(AssertionError e){if(!failure||!Integer.toString(next).equals(e.getMessage()))throw new AssertionError("wrong failure",e);}if(!CompoundOwner.trace.equals(failure?"M":"E"))throw new AssertionError("effect order");}}System.out.println("compound:"+enabled+":120");}}
`, methods.String(), calls.String(), enabled)
	owner := "CompoundOwner"
	if renamed {
		owner = "OtherArithmeticScope"
		source = strings.NewReplacer("CompoundOwner", owner, "Child", "Part", "payload(", "detail(", "update", "compute").Replace(source)
		source = regexp.MustCompile(`\bvalue\b`).ReplaceAllString(source, "argument")
	}
	return source, owner
}

func TestNativeAssertionCompoundUpdateKeepsSignedWordAndDomain(t *testing.T) {
	testNativeAssertionCompoundUpdate(t, false)
}
func TestNativeAssertionCompoundUpdateOriginalCompiler(t *testing.T) {
	if os.Getenv("JAVA8_JAVAC") == "" {
		t.Skip("JAVA8_JAVAC required for original compiler")
	}
	testNativeAssertionCompoundUpdate(t, true)
}
func testNativeAssertionCompoundUpdate(t *testing.T, native bool) {
	for _, enabled := range []bool{false, true} {
		for _, renamed := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/renamed=%t", enabled, renamed), func(t *testing.T) {
				source, owner := nativeCompoundUpdateFixture(enabled, renamed)
				want := fmt.Sprintf("compound:%t:120\n", enabled)
				if native {
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
						return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
					}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, "CompoundDriver", want, nil, nativeLexicalExactSignatures)
				} else {
					testNativePrivateSetterCompiledFixture(t, owner, "CompoundDriver", want, func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeLexicalExactSignatures)
				}
			})
		}
	}
}

func TestNativeCompoundUpdatePreservesOriginalOperands(t *testing.T) {
	var methods, calls strings.Builder
	for i, delta := range []int{2, 128, -32768} {
		fmt.Fprintf(&methods, `int ordered%d(int value){return stamp(value)+(value+=%d);}int twice%d(int value){return (value+=%d)+(value+=%d);}int snapshot%d(int value){return value+(value+=%d);}int copied%d(int value){int prior=(value+=%d);return prior*17+value;}int arguments%d(int value){return record(value,(value+=%d),value);}int guarded%d(boolean enabled,int value){if(enabled&&(value+=%d)>0){trace+="T";}else{trace+="F";}return value;}`, i, delta, i, delta, delta, i, delta, i, delta, i, delta, i, delta)
		fmt.Fprintf(&calls, `case %d:if(c.ordered%d(value)!=(int)(7L*value+next)||!WordUpdateOwner.trace.equals("P"))throw new AssertionError("call/update");if(c.twice%d(value)!=(int)(2L*value+3L*delta))throw new AssertionError("two updates");if(c.snapshot%d(value)!=(int)((long)value+next))throw new AssertionError("old operand");if(c.copied%d(value)!=(int)(18L*next))throw new AssertionError("updated copy");if(c.arguments%d(value)!=(int)(31L*value+18L*next)||!WordUpdateOwner.trace.equals("PR"))throw new AssertionError("ordered arguments");WordUpdateOwner.trace="";if(c.guarded%d(false,value)!=value||!WordUpdateOwner.trace.equals("F"))throw new AssertionError("disabled short circuit");WordUpdateOwner.trace="";if(c.guarded%d(true,value)!=next||!WordUpdateOwner.trace.equals(next>0?"T":"F"))throw new AssertionError("enabled short circuit");break;`, i, i, i, i, i, i, i, i)
	}
	source := fmt.Sprintf(`class WordUpdateOwner {static String trace="";static int stamp(int n){trace+="P";return n*7;}static int record(int old,int updated,int observed){trace+="R";if(updated!=observed)throw new AssertionError("following read");return old*31+updated*17+observed;}class Child{%s}Child make(){return new Child();}}
class WordUpdateDriver {public static void main(String[]args){WordUpdateOwner.Child c=new WordUpdateOwner().make();int[]deltas={2,128,-32768};int[]values={Integer.MIN_VALUE,-32769,-129,-2,-1,0,1,2,127,32767,Integer.MAX_VALUE};for(int method=0;method<deltas.length;method++)for(int value:values){int delta=deltas[method];int next=(int)((long)value+delta);WordUpdateOwner.trace="";switch(method){%s default:throw new AssertionError("method");}}System.out.println("compound:operands:arguments:overflow:33");}}
`, methods.String(), calls.String())
	for _, native := range []bool{false, true} {
		if native && os.Getenv("JAVA8_JAVAC") == "" {
			continue
		}
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			if native {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"WordUpdateOwner.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"WordUpdateOwner"}, "WordUpdateDriver", "compound:operands:arguments:overflow:33\n", nil, nativeLexicalExactSignatures)
			} else {
				testNativePrivateSetterCompiledFixture(t, "WordUpdateOwner", "WordUpdateDriver", "compound:operands:arguments:overflow:33\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeLexicalExactSignatures)
			}
		})
	}
}

func TestNativeAssertionCompoundUpdateOracleRejectsMutants(t *testing.T) {
	javac, java := t04Tools(t)
	mutants := []struct {
		name, body        string
		enabled, disabled bool
	}{
		{"original", `assert (value+=DELTA)>0:payload(value);trace+="E";return value;`, false, false},
		{"eager update", `value+=DELTA;assert value>0:payload(value);trace+="E";return value;`, false, true},
		{"repeated update", `assert (value+=DELTA)>0&&(value+=DELTA)>0:payload(value);trace+="E";return value;`, true, false},
		{"lost update", `assert value>0:payload(value);trace+="E";return value;`, true, false},
		{"old result", `assert (value+=DELTA)>0:payload(value);trace+="E";return value-DELTA;`, true, true},
		{"eager message", `int message=payload(value);assert (value+=DELTA)>0:message;trace+="E";return value;`, true, true},
		{"unsigned immediate", `assert (value+=Math.abs(DELTA))>0:payload(value);trace+="E";return value;`, true, false},
	}
	originalDrivers := map[bool][]byte{}
	for _, mutant := range mutants {
		t.Run(mutant.name, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				source, _ := nativeCompoundUpdateFixture(enabled, false)
				for _, delta := range []int{-32768, -129, -128, 0, 2, 127, 128, 32767} {
					body := fmt.Sprintf(`assert (value+=%d)>0:payload(value);trace+="E";return value;`, delta)
					replacement := strings.ReplaceAll(mutant.body, "DELTA", fmt.Sprintf("(%d)", delta))
					if !strings.Contains(source, body) {
						t.Fatal("missing original body")
					}
					source = strings.Replace(source, body, replacement, 1)
				}
				dir := t.TempDir()
				path := filepath.Join(dir, "CompoundOwner.java")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, path).CombinedOutput(); err != nil {
					t.Fatalf("compile oracle: %v %s", err, out)
				}
				driver, err := os.ReadFile(filepath.Join(dir, "CompoundDriver.class"))
				if err != nil {
					t.Fatal(err)
				}
				if mutant.name == "original" {
					originalDrivers[enabled] = driver
				} else if !bytes.Equal(driver, originalDrivers[enabled]) {
					t.Fatal("mutant changed the independent original driver")
				}
				for _, mode := range []string{"-ea", "-da"} {
					out, err := exec.Command(java, mode, "-cp", dir, "CompoundDriver").CombinedOutput()
					reject := mutant.disabled
					if enabled {
						reject = mutant.enabled
					}
					if reject {
						if err == nil {
							t.Fatal("mutant accepted", enabled, mode)
						}
					} else if err != nil || string(out) != fmt.Sprintf("compound:%t:120\n", enabled) {
						t.Fatalf("valid mode: %v %s", err, out)
					}
				}
			}
		})
	}
}

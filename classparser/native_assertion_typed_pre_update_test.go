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

func nativeTypedPreFixture(enabled, renamed bool, mutant string) (string, string, string) {
	source, owner, want := nativeTypedPostFixture(enabled, false, "")
	for _, typ := range nativeTypedPostCases {
		for _, increment := range []bool{true, false} {
			op, suffix := "++", "up"
			if !increment {
				op, suffix = "--", "down"
			}
			original := fmt.Sprintf(`%s %s%s(%s value){assert value%s>0:payload(value);trace+="E";return value;}`, typ.name, typ.name, suffix, typ.name, op)
			body := fmt.Sprintf(`assert %svalue>0:payload(value);trace+="E";return value;`, op)
			switch mutant {
			case "eager update":
				body = fmt.Sprintf(`%svalue;assert value>0:payload(value);trace+="E";return value;`, op)
			case "old condition":
				body = fmt.Sprintf(`assert value%s>0:payload(value);trace+="E";return value;`, op)
			case "lost update":
				body = `assert value>0:payload(value);trace+="E";return value;`
			case "repeated update":
				body = fmt.Sprintf(`assert %svalue>0&&%svalue>0:payload(value);trace+="E";return value;`, op, op)
			case "eager message":
				body = fmt.Sprintf(`%s detail=payload(value);assert %svalue>0:detail;trace+="E";return value;`, typ.name, op)
			case "NaN accepted":
				body = fmt.Sprintf(`assert !(%svalue<=0):payload(value);trace+="E";return value;`, op)
			}
			if !strings.Contains(source, original) {
				panic("missing typed update method")
			}
			source = strings.Replace(source, original, fmt.Sprintf(`%s %s%s(%s value){%s}`, typ.name, typ.name, suffix, typ.name, body), 1)
		}
	}
	source = strings.ReplaceAll(source, `failure=enabled&&!(x>0)`, `failure=enabled&&!(next>0)`)
	source = strings.ReplaceAll(source, "typed:post:", "typed:pre:")
	want = strings.ReplaceAll(want, "typed:post:", "typed:pre:")
	if renamed {
		owner = "OtherUpdatedDomain"
		source = strings.NewReplacer("TypedPostOwner", owner, "Child", "Part", "payload(", "detail(").Replace(source)
		source = regexp.MustCompile(`\bvalue\b`).ReplaceAllString(source, "argument")
	}
	return source, owner, want
}

func TestNativeAssertionTypedPreUpdatePreservesNewWordAndDomain(t *testing.T) {
	testNativeAssertionTypedPreUpdate(t, false)
}
func TestNativeAssertionTypedPreUpdateOriginalCompiler(t *testing.T) {
	if os.Getenv("JAVA8_JAVAC") == "" {
		t.Skip("JAVA8_JAVAC required for original compiler")
	}
	testNativeAssertionTypedPreUpdate(t, true)
}
func testNativeAssertionTypedPreUpdate(t *testing.T, native bool) {
	for _, enabled := range []bool{false, true} {
		for _, renamed := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/renamed=%t", enabled, renamed), func(t *testing.T) {
				source, owner, want := nativeTypedPreFixture(enabled, renamed, "")
				if native {
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
						return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
					}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, "TypedPostDriver", want, nil, nativeLexicalExactSignatures)
				} else {
					testNativePrivateSetterCompiledFixture(t, owner, "TypedPostDriver", want, func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeLexicalExactSignatures)
				}
			})
		}
	}
}

func TestNativeAssertionTypedPreUpdateOracleRejectsMutants(t *testing.T) {
	javac, java := t04Tools(t)
	originalDrivers := map[bool][]byte{}
	for _, mutant := range []string{"original", "eager update", "old condition", "lost update", "repeated update", "eager message", "NaN accepted"} {
		t.Run(mutant, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				source, owner, want := nativeTypedPreFixture(enabled, false, mutant)
				dir := t.TempDir()
				path := filepath.Join(dir, owner+".java")
				if e := os.WriteFile(path, []byte(source), 0600); e != nil {
					t.Fatal(e)
				}
				if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, path).CombinedOutput(); e != nil {
					t.Fatalf("compile oracle:%v %s", e, out)
				}
				driver, e := os.ReadFile(filepath.Join(dir, "TypedPostDriver.class"))
				if e != nil {
					t.Fatal(e)
				}
				if mutant == "original" {
					originalDrivers[enabled] = driver
				} else if !bytes.Equal(driver, originalDrivers[enabled]) {
					t.Fatal("mutant changed independent original driver")
				}
				for _, mode := range []string{"-ea", "-da"} {
					out, e := exec.Command(java, mode, "-cp", dir, "TypedPostDriver").CombinedOutput()
					reject := mutant != "original" && mutant != "eager update" && enabled || !enabled && (mutant == "eager update" || mutant == "eager message")
					if reject {
						if e == nil {
							t.Fatal("mutant accepted", enabled, mode)
						}
					} else if e != nil || string(out) != want {
						t.Fatalf("equivalent mode:%v %s", e, out)
					}
				}
			}
		})
	}
}

func TestNativeTypedPreUpdatePreservesArgumentsAndHandlers(t *testing.T) {
	var members, methods, oracle, checks strings.Builder
	rows := 0
	for _, typ := range nativeTypedPostCases {
		n := typ.name
		members.WriteString(strings.ReplaceAll(`static TYPE[] pack(TYPE a,TYPE b,TYPE c){trace+="R";return new TYPE[]{a,b,c};}static void sink(TYPE x){trace+="S:"+String.valueOf(x);throw error;}`, "TYPE", n))
		methods.WriteString(strings.ReplaceAll(`TYPE[] argsTYPE(TYPE value){return pack(value,++value,value);}TYPE[] copiedTYPE(TYPE value){TYPE prior=++value;return new TYPE[]{prior,value};}TYPE guardTYPE(boolean enabled,TYPE value){if(enabled&&++value>0)trace+="T";else trace+="F";return value;}TYPE caughtTYPE(TYPE value){try{sink(++value);}catch(IllegalStateException e){if(e!=error)throw e;trace+=":H";}return value;}`, "TYPE", n))
		oracle.WriteString(strings.NewReplacer("TYPE", n, "NEXT", typ.next, "EQUAL", typ.equal).Replace(`static TYPE nextTYPE(TYPE x,int delta){return NEXT;}static boolean eqTYPE(TYPE got,TYPE next){return EQUAL;}`))
		checks.WriteString(strings.NewReplacer("TYPE", n, "INPUTS", typ.inputs).Replace(`for(TYPE x:new TYPE[]{INPUTS}){TYPE next=nextTYPE(x,1);TypedOperandOwner.trace="";TYPE[] a=c.argsTYPE(x);if(a.length!=3||!eqTYPE(a[0],x)||!eqTYPE(a[1],next)||!eqTYPE(a[2],next)||!TypedOperandOwner.trace.equals("R"))throw new AssertionError("TYPE arguments x="+x+" expected="+next+" got="+java.util.Arrays.toString(a)+" trace="+TypedOperandOwner.trace);a=c.copiedTYPE(x);if(a.length!=2||!eqTYPE(a[0],next)||!eqTYPE(a[1],next))throw new AssertionError("copied updated value");for(boolean enabled:new boolean[]{false,true}){TypedOperandOwner.trace="";if(!eqTYPE(c.guardTYPE(enabled,x),enabled?next:x)||!TypedOperandOwner.trace.equals(enabled&&next>0?"T":"F"))throw new AssertionError("short circuit");}TypedOperandOwner.trace="";if(!eqTYPE(c.caughtTYPE(x),next)||!TypedOperandOwner.trace.equals("S:"+String.valueOf(next)+":H"))throw new AssertionError("handler identity/order");rows++;}`))
		rows += strings.Count(typ.inputs, ",") + 1
	}
	source := fmt.Sprintf(`class TypedOperandOwner {static String trace="";static final IllegalStateException error=new IllegalStateException("original");%s class Child{%s}Child make(){return new Child();}}
class TypedOperandDriver {%s public static void main(String[]args){TypedOperandOwner.Child c=new TypedOperandOwner().make();int rows=0;%s System.out.println("typed:pre:operands:"+rows);}}
`, members.String(), methods.String(), oracle.String(), checks.String())
	want := fmt.Sprintf("typed:pre:operands:%d\n", rows)
	for _, native := range []bool{false, true} {
		if native && os.Getenv("JAVA8_JAVAC") == "" {
			continue
		}
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			if native {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"TypedOperandOwner.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"TypedOperandOwner"}, "TypedOperandDriver", want, nil, nativeLexicalExactSignatures)
			} else {
				testNativePrivateSetterCompiledFixture(t, "TypedOperandOwner", "TypedOperandDriver", want, func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeLexicalExactSignatures)
			}
		})
	}
}

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

// The driver computes stored values without ++/--. BigInteger supplies signed
// wrap/narrowing, exact BigDecimal arithmetic supplies double rounding, and a
// double intermediate is exact enough for a float plus/minus one. Raw bits
// distinguish signed zero and retain noncanonical NaN payloads.
var nativeTypedPostCases = []struct{ name, inputs, next, equal string }{
	{"long", "-2L,-1L,0L,1L,Long.MAX_VALUE,Long.MIN_VALUE", "java.math.BigInteger.valueOf(x).add(java.math.BigInteger.valueOf(delta)).longValue()", "got==next"},
	{"float", "-2f,-0f,0f,1f,Float.MIN_VALUE,Float.MAX_VALUE,16777216f,Float.NEGATIVE_INFINITY,Float.POSITIVE_INFINITY,Float.NaN,Float.intBitsToFloat(0x7fc12345)", "(float)((double)x+delta)", "Float.floatToRawIntBits(got)==Float.floatToRawIntBits(next)"},
	{"double", "-2d,-0d,0d,1d,Double.MIN_VALUE,Double.MAX_VALUE,9007199254740992d,Double.NEGATIVE_INFINITY,Double.POSITIVE_INFINITY,Double.NaN,Double.longBitsToDouble(0x7ff8123456789abcL)", "Double.isNaN(x)||Double.isInfinite(x)?x:new java.math.BigDecimal(x).add(java.math.BigDecimal.valueOf(delta)).doubleValue()", "Double.doubleToRawLongBits(got)==Double.doubleToRawLongBits(next)"},
	{"byte", "-2,-1,0,1,Byte.MIN_VALUE,Byte.MAX_VALUE", "java.math.BigInteger.valueOf(x).add(java.math.BigInteger.valueOf(delta)).byteValue()", "got==next"},
	{"short", "-2,-1,0,1,Short.MIN_VALUE,Short.MAX_VALUE", "java.math.BigInteger.valueOf(x).add(java.math.BigInteger.valueOf(delta)).shortValue()", "got==next"},
	{"char", "0,1,127,128,Character.MIN_VALUE,Character.MAX_VALUE", "(char)(java.math.BigInteger.valueOf(x).add(java.math.BigInteger.valueOf(delta)).intValue())", "got==next"},
}

func nativeTypedPostFixture(enabled, renamed bool, mutant string) (string, string, string) {
	var members, methods, checks strings.Builder
	rows := 0
	for _, typ := range nativeTypedPostCases {
		fmt.Fprintf(&members, `%s payload(%s n){trace+="M";return n;}`, typ.name, typ.name)
		for _, delta := range []int{1, -1} {
			op, suffix := "++", "up"
			if delta < 0 {
				op, suffix = "--", "down"
			}
			body := fmt.Sprintf(`assert value%s>0:payload(value);trace+="E";return value;`, op)
			switch mutant {
			case "eager update":
				body = fmt.Sprintf(`%s old=value;value%s;assert old>0:payload(value);trace+="E";return value;`, typ.name, op)
			case "new condition":
				body = fmt.Sprintf(`assert %svalue>0:payload(value);trace+="E";return value;`, op)
			case "lost update":
				body = `assert value>0:payload(value);trace+="E";return value;`
			case "repeated update":
				body = fmt.Sprintf(`assert value%s>0&&value%s>0:payload(value);trace+="E";return value;`, op, op)
			case "eager message":
				body = fmt.Sprintf(`%s detail=payload(value);assert value%s>0:detail;trace+="E";return value;`, typ.name, op)
			case "NaN accepted":
				body = fmt.Sprintf(`assert !(value%s<=0):payload(value);trace+="E";return value;`, op)
			}
			fmt.Fprintf(&methods, `%s %s%s(%s value){%s}`, typ.name, typ.name, suffix, typ.name, body)
			fmt.Fprintf(&checks, `{int delta=%d;for(%s x:new %s[]{%s}){%s next=enabled?(%s):x;boolean failure=enabled&&!(x>0);TypedPostOwner.trace="";try{%s got=c.%s%s(x);if(failure||!(%s))throw new AssertionError("update/result");}catch(AssertionError e){if(!failure||!String.valueOf(next).equals(e.getMessage()))throw new AssertionError("wrong failure",e);}if(!TypedPostOwner.trace.equals(failure?"M":"E"))throw new AssertionError("effect order");rows++;}}`, delta, typ.name, typ.name, typ.inputs, typ.name, typ.next, typ.name, typ.name, suffix, typ.equal)
			rows += strings.Count(typ.inputs, ",") + 1
		}
	}
	source := fmt.Sprintf(`class TypedPostOwner {static String trace="";%s class Child {%s}Child make(){return new Child();}}
class TypedPostDriver {public static void main(String[]args){boolean enabled=%t;ClassLoader loader=ClassLoader.getSystemClassLoader();loader.setDefaultAssertionStatus(!enabled);loader.setClassAssertionStatus("TypedPostOwner",enabled);TypedPostOwner.Child c=new TypedPostOwner().make();loader.setClassAssertionStatus("TypedPostOwner",!enabled);int rows=0;%s System.out.println("typed:post:"+enabled+":"+rows);}}
`, members.String(), methods.String(), enabled, checks.String())
	owner := "TypedPostOwner"
	if renamed {
		owner = "OtherScalarDomain"
		source = strings.NewReplacer("TypedPostOwner", owner, "Child", "Part", "payload(", "detail(").Replace(source)
		source = regexp.MustCompile(`\bvalue\b`).ReplaceAllString(source, "argument")
	}
	return source, owner, fmt.Sprintf("typed:post:%t:%d\n", enabled, rows)
}

func TestNativeAssertionTypedPostUpdatePreservesOldWordAndStoredValue(t *testing.T) {
	testNativeAssertionTypedPostUpdate(t, false)
}
func TestNativeAssertionTypedPostUpdateOriginalCompiler(t *testing.T) {
	if os.Getenv("JAVA8_JAVAC") == "" {
		t.Skip("JAVA8_JAVAC required for original compiler")
	}
	testNativeAssertionTypedPostUpdate(t, true)
}
func testNativeAssertionTypedPostUpdate(t *testing.T, native bool) {
	for _, enabled := range []bool{false, true} {
		for _, renamed := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/renamed=%t", enabled, renamed), func(t *testing.T) {
				source, owner, want := nativeTypedPostFixture(enabled, renamed, "")
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

func TestNativeAssertionTypedPostUpdateOracleRejectsMutants(t *testing.T) {
	javac, java := t04Tools(t)
	originalDrivers := map[bool][]byte{}
	for _, mutant := range []string{"original", "eager update", "new condition", "lost update", "repeated update", "eager message", "NaN accepted"} {
		t.Run(mutant, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				source, owner, want := nativeTypedPostFixture(enabled, false, mutant)
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

func TestNativeTypedPostUpdatePreservesOperandOrderAndHandlers(t *testing.T) {
	var members, methods, oracle, checks strings.Builder
	rows := 0
	for _, typ := range nativeTypedPostCases {
		n := typ.name
		fmt.Fprintf(&members, `static %s stamp(%s x){trace+="P";return x;}static %s[] pack(%s a,%s b,%s c){trace+="R";return new %s[]{a,b,c};}static void sink(%s x){trace+="S:"+String.valueOf(x);throw error;}`, n, n, n, n, n, n, n, n)
		methods.WriteString(strings.ReplaceAll(`TYPE[] argsTYPE(TYPE value){return pack(value,value++,value);}TYPE[] priorTYPE(TYPE value){return pack(stamp(value),value++,value);}TYPE[] twiceTYPE(TYPE value){return pack(value++,value--,value);}TYPE[] copiedTYPE(TYPE value){TYPE prior=value++;return new TYPE[]{prior,value};}TYPE guardTYPE(boolean enabled,TYPE value){if(enabled&&value++>0)trace+="T";else trace+="F";return value;}TYPE caughtTYPE(TYPE value){try{sink(value++);}catch(IllegalStateException e){if(e!=error)throw e;trace+=":H";}return value;}`, "TYPE", n))
		oracle.WriteString(strings.NewReplacer("TYPE", n, "NEXT", typ.next, "EQUAL", typ.equal).Replace(`static TYPE nextTYPE(TYPE x,int delta){return NEXT;}static boolean eqTYPE(TYPE got,TYPE next){return EQUAL;}`))
		checks.WriteString(strings.NewReplacer("TYPE", n, "INPUTS", typ.inputs).Replace(`for(TYPE x:new TYPE[]{INPUTS}){TYPE next=nextTYPE(x,1),after=nextTYPE(next,-1);TypedOperandOwner.trace="";TYPE[] a=c.argsTYPE(x);if(a.length!=3||!eqTYPE(a[0],x)||!eqTYPE(a[1],x)||!eqTYPE(a[2],next)||!TypedOperandOwner.trace.equals("R"))throw new AssertionError("old arguments");TypedOperandOwner.trace="";a=c.priorTYPE(x);if(!eqTYPE(a[0],x)||!eqTYPE(a[1],x)||!eqTYPE(a[2],next)||!TypedOperandOwner.trace.equals("PR"))throw new AssertionError("call order");TypedOperandOwner.trace="";a=c.twiceTYPE(x);if(!eqTYPE(a[0],x)||!eqTYPE(a[1],next)||!eqTYPE(a[2],after)||!TypedOperandOwner.trace.equals("R"))throw new AssertionError("two updates");a=c.copiedTYPE(x);if(a.length!=2||!eqTYPE(a[0],x)||!eqTYPE(a[1],next))throw new AssertionError("copied old value");for(boolean enabled:new boolean[]{false,true}){TypedOperandOwner.trace="";if(!eqTYPE(c.guardTYPE(enabled,x),enabled?next:x)||!TypedOperandOwner.trace.equals(enabled&&x>0?"T":"F"))throw new AssertionError("short circuit");}TypedOperandOwner.trace="";if(!eqTYPE(c.caughtTYPE(x),next)||!TypedOperandOwner.trace.equals("S:"+String.valueOf(x)+":H"))throw new AssertionError("handler identity/order");rows++;}`))
		rows += strings.Count(typ.inputs, ",") + 1
	}
	source := fmt.Sprintf(`class TypedOperandOwner {static String trace="";static final IllegalStateException error=new IllegalStateException("original");%s class Child{%s}Child make(){return new Child();}}
class TypedOperandDriver {%s public static void main(String[]args){TypedOperandOwner.Child c=new TypedOperandOwner().make();int rows=0;%s System.out.println("typed:operands:"+rows);}}
`, members.String(), methods.String(), oracle.String(), checks.String())
	want := fmt.Sprintf("typed:operands:%d\n", rows)
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

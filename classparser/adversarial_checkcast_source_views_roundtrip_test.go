package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Original JVM CHECKCAST permits null and reference values whose more precise
// Java source types are statically disjoint. This is original-vs-rebuilt behavior
// evidence; the source bridge must be a check-free widening view, not a new
// target check, null guard, producer copy, or generic inference rewrite.
func TestAdversarialOriginalCheckCastSourceViewsRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	cases := []struct {
		name, source, target, value string
		valid                       bool
	}{
		{"integerList", "Integer", "java.util.List", "Integer.valueOf(7)", false},
		{"integerRunnable", "Integer", "Runnable", "Integer.valueOf(7)", false},
		{"integerArray", "Integer", "Number[]", "Integer.valueOf(7)", false},
		{"stringList", "String", "java.util.List", "\"x\"", false},
		{"stringArray", "String", "Integer[]", "\"x\"", false},
		{"primitiveInterface", "int[]", "java.util.List", "new int[]{7}", false},
		{"referenceArray", "String[]", "Integer[]", "new String[]{\"x\"}", false},
		{"primitiveArray", "int[]", "long[]", "new int[]{7}", false},
		{"comparable", "Integer", "Comparable", "Integer.valueOf(7)", true},
		{"serializable", "String", "java.io.Serializable", "\"x\"", true},
		{"cloneable", "int[]", "Cloneable", "new int[]{7}", true},
		{"arrayWidening", "String[]", "Object[]", "new String[]{\"x\"}", true},
		{"originalFinal", "ReferenceCastOwner", "Runnable", "new ReferenceCastOwner()", false},
		{"openInterface", "Number", "Runnable", "Integer.valueOf(7)", false},
	}
	var owner, driver, want strings.Builder
	owner.WriteString(`public final class ReferenceCastOwner {static String trace="";static final RuntimeException SAME=new RuntimeException("same");static int seen;`)
	driver.WriteString(`public class ReferenceCastDriver {public static void main(String[]args){`)
	for _, c := range cases {
		fmt.Fprintf(&owner, `static %s take%s(%s x,boolean fail){trace+="P;";seen++;if(fail)throw SAME;return x;}static %s %s(%s x,boolean fail){trace+="A;";try{return (%s)(Object)take%s(x,fail);}catch(ClassCastException e){trace+="C;";throw e;}finally{trace+="F;";}}`, c.source, c.name, c.source, c.target, c.name, c.source, c.target, c.name)
		// Null, incompatible/compatible value, producer failure. Independent output
		// includes protected-region entry, producer once, typed catch and finally.
		fmt.Fprintf(&driver, `ReferenceCastOwner.trace="";ReferenceCastOwner.seen=0;System.out.println(ReferenceCastOwner.%s(null,false)==null);System.out.println(ReferenceCastOwner.seen+":"+ReferenceCastOwner.trace);`, c.name)
		want.WriteString("true\n1:A;P;F;\n")
		fmt.Fprintf(&driver, `ReferenceCastOwner.trace="";ReferenceCastOwner.seen=0;%s v%s=%s;try{System.out.println(ReferenceCastOwner.%s(v%s,false)==(Object)v%s);}catch(ClassCastException e){System.out.println("CCE");}System.out.println(ReferenceCastOwner.seen+":"+ReferenceCastOwner.trace);`, c.source, c.name, c.value, c.name, c.name, c.name)
		if c.valid {
			want.WriteString("true\n1:A;P;F;\n")
		} else {
			want.WriteString("CCE\n1:A;P;C;F;\n")
		}
		fmt.Fprintf(&driver, `ReferenceCastOwner.trace="";ReferenceCastOwner.seen=0;try{ReferenceCastOwner.%s(v%s,true);System.out.println("lost");}catch(RuntimeException e){System.out.println(e==ReferenceCastOwner.SAME);}System.out.println(ReferenceCastOwner.seen+":"+ReferenceCastOwner.trace);`, c.name, c.name)
		want.WriteString("true\n1:A;P;F;\n")
	}
	owner.WriteString("}")
	driver.WriteString("}}")
	// Raw-string construction above deliberately keeps Java escapes in literals.
	source := strings.ReplaceAll(owner.String(), `\"`, `"`)
	driverSource := strings.ReplaceAll(driver.String(), `\"`, `"`)
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			ownerFile := filepath.Join(original, "ReferenceCastOwner.java")
			driverFile := filepath.Join(original, "ReferenceCastDriver.java")
			if err := os.WriteFile(ownerFile, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(driverFile, []byte(driverSource), 0600); err != nil {
				t.Fatal(err)
			}
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, ownerFile, driverFile).CombinedOutput(); e != nil {
				t.Fatalf("original%v %s", e, out)
			}
			if got := t04RunJava(t, java, original, "ReferenceCastDriver"); got != want.String() {
				t.Fatalf("independent original oracle got%q want%q", got, want.String())
			}
			raw, e := os.ReadFile(filepath.Join(original, "ReferenceCastOwner.class"))
			if e != nil {
				t.Fatal(e)
			}
			resolver := func(n string) ([]byte, bool) {
				b, e := os.ReadFile(filepath.Join(original, n+".class"))
				return b, e == nil
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					var r DecompileResult
					var err error
					if mode == "legacy" {
						r.Source, err = DecompileWithResolver(raw, resolver)
					} else {
						r, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolver})
					}
					if err != nil || len(r.StubMethods) > 0 {
						t.Fatalf("decompile%v %v", err, r.StubMethods)
					}
					// A legal open class->interface cast must not receive a blind Object view.
					openBody := reviewedControlBody(t, r.Source, `\bopenInterface\s*\(`)
					if strings.Contains(openBody, "java.lang.Object") {
						t.Fatalf("legal nonfinal interface cast widened: %s", openBody)
					}
					rebuilt := t.TempDir()
					f := filepath.Join(rebuilt, "ReferenceCastOwner.java")
					if e := os.WriteFile(f, []byte(r.Source), 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", rebuilt, f).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt%v %s\n%s", e, out, r.Source)
					}
					if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+original, "ReferenceCastDriver"); got != want.String() {
						t.Fatalf("different cast/exception/effect behavior got%q want%q\n%s", got, want.String(), r.Source)
					}
				})
			}
		})
	}
}

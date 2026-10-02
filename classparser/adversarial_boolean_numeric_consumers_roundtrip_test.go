package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Mutating only the field descriptor preserves code, frame widths and the int
// computational category. The original JVM independently supplies Z-store
// narrowing; the driver never reads the field through an I descriptor.
func TestAdversarialBooleanNumericConsumersRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	source := `class BooleanNumericOwner {
 static int flag; static String trace=""; static void set(int word){flag=word;trace="";}
 static int seen(int n){trace+="N;";return n;} static int add(int n){return flag+seen(n);}static int subtract(int n){return seen(n)-flag;}static int multiply(int n){return flag*seen(n);}static int divide(int n){return seen(n)/flag;}static int remainder(int n){return flag%seen(n);}
 static int left(int n){return flag<<seen(n);}static int right(int n){return seen(n)>>flag;}static int unsigned(int n){return seen(n)>>>flag;}static int negate(){return -flag;}
 static boolean equal(int n){return flag==seen(n);}static boolean notEqual(int n){return seen(n)!=flag;}static boolean less(int n){return flag<seen(n);}static boolean greaterEqual(int n){return seen(n)>=flag;}
 static boolean predicate(){trace+="B;";return flag!=0;}static boolean compared(int n){return seen(n)==(predicate()?1:0);}static boolean reversed(int n){return (predicate()?1:0)!=seen(n);}
 static long widen(){return (long)flag;}static float floatValue(){return (float)flag;}static double doubleValue(){return (double)flag;}
}
public class BooleanNumericDriver {public static void main(String[]args){for(int word:new int[]{0,1,2,3,-1,-2})for(int n:new int[]{0,1,2,-1,-2,33,Integer.MIN_VALUE}){BooleanNumericOwner.set(word);try{System.out.println(BooleanNumericOwner.add(n)+":"+BooleanNumericOwner.subtract(n)+":"+BooleanNumericOwner.multiply(n)+":"+BooleanNumericOwner.left(n)+":"+BooleanNumericOwner.right(n)+":"+BooleanNumericOwner.unsigned(n)+":"+BooleanNumericOwner.negate()+":"+BooleanNumericOwner.equal(n)+":"+BooleanNumericOwner.notEqual(n)+":"+BooleanNumericOwner.less(n)+":"+BooleanNumericOwner.greaterEqual(n)+":"+BooleanNumericOwner.compared(n)+":"+BooleanNumericOwner.reversed(n)+":"+BooleanNumericOwner.widen()+":"+BooleanNumericOwner.floatValue()+":"+BooleanNumericOwner.doubleValue()+":"+BooleanNumericOwner.trace);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+BooleanNumericOwner.trace);}for(int op=0;op<2;op++){try{System.out.println(op==0?BooleanNumericOwner.divide(n):BooleanNumericOwner.remainder(n));}catch(Throwable e){System.out.println(e.getClass().getName()+":"+BooleanNumericOwner.trace);}}}}}`
	for _, debug := range []string{"-g", "-g:none"} {
		dir := t.TempDir()
		src := filepath.Join(dir, "BooleanNumericDriver.java")
		if e := os.WriteFile(src, []byte(source), 0600); e != nil {
			t.Fatal(e)
		}
		if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, src).CombinedOutput(); e != nil {
			t.Fatalf("original %v %s", e, out)
		}
		raw, e := os.ReadFile(filepath.Join(dir, "BooleanNumericOwner.class"))
		if e != nil {
			t.Fatal(e)
		}
		o, e := Parse(raw)
		if e != nil {
			t.Fatal(e)
		}
		changed := 0
		for _, f := range o.Fields {
			n, _ := o.getUtf8(f.NameIndex)
			if n == "flag" {
				u := o.ConstantPool[f.DescriptorIndex-1].(*ConstantUtf8Info)
				if u.Value != "I" {
					t.Fatal("unexpected original field descriptor")
				}
				u.Value = "Z"
				changed++
			}
		}
		if changed != 1 {
			t.Fatal(changed)
		}
		raw = o.Bytes()
		if e := os.WriteFile(filepath.Join(dir, "BooleanNumericOwner.class"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		want := t04RunJava(t, java, dir, "BooleanNumericDriver")
		resolve := func(n string) ([]byte, bool) { b, e := os.ReadFile(filepath.Join(dir, n+".class")); return b, e == nil }
		for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
			var result DecompileResult
			if mode == "legacy" {
				result.Source, e = DecompileWithResolver(raw, resolve)
			} else {
				result, e = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
			}
			if e != nil || len(result.StubMethods) > 0 {
				t.Fatalf("%s %v %v", mode, e, result.StubMethods)
			}
			rebuilt := t.TempDir()
			src := filepath.Join(rebuilt, "BooleanNumericOwner.java")
			os.WriteFile(src, []byte(result.Source), 0600)
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, src).CombinedOutput(); e != nil {
				t.Fatalf("%s rebuild %v %s\n%s", mode, e, out, result.Source)
			}
			if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "BooleanNumericDriver"); got != want {
				t.Fatalf("%s different computational words: got %q want %q\n%s", mode, got, want, result.Source)
			}
		}
	}
}

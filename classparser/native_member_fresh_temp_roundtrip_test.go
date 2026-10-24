package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func mixedFreshEnclosingInitializerFixture(shape string) (string, string) {
	owner, fixture := lexicalInitializerCaptureShape(shape)
	_, driver, found := strings.Cut(fixture, "\n")
	if !found {
		panic("missing independently compiled input and driver")
	}
	driver = strings.Replace(driver, `new AssertionError("scope")`, `new AssertionError("scope result="+result.getClass().getName()+"/"+result.getClass().isAnonymousClass()+" cursor="+cursor.getClass().getName()+"/"+cursor.getClass().isAnonymousClass()+" method="+cursor.getClass().getEnclosingMethod())`, 1)
	root := fmt.Sprintf(`import java.util.*;class %s<T> {
 final Iterable<T> input;
 %s(Iterable<T> input){this.input=input;}
 class Stage {
  Iterable<T> create(){return new Iterable<T>() {
   public Iterator<T> iterator(){return new Iterator<T>() {
    final Iterator<T> backing=%s.this.input.iterator();
    public boolean hasNext(){return backing.hasNext();}
    public T next(){return backing.next();}
    public void remove(){throw new UnsupportedOperationException();}
   };}
  };}
 }
 static <T> Iterable<T> produce(Iterable<T> input){return new %s<T>(input).new Stage().create();}
}
`, owner, owner, owner, owner)
	if shape == "renamed" {
		root = strings.ReplaceAll(root, "backing", "snapshotCursor")
	}
	if shape == "generic-intermediate" {
		root = strings.Replace(root, "class Stage {", "class Stage<Marker> {", 1)
		root = strings.Replace(root, "new Stage()", "new Stage<Object>()", 1)
	}
	return owner, root + driver
}

func testNativeFreshEnclosingMixedInitializer(t *testing.T, native bool) {
	for _, shape := range []string{"reference", "array-elements", "renamed", "generic-intermediate"} {
		t.Run(shape, func(t *testing.T) {
			owner, fixture := mixedFreshEnclosingInitializerFixture(shape)
			want := "6:lexical:initializer:producer:identity:scope\n"
			if native {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for independent compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, fixture, owner, debug)
				}, NativeJavac8, compiler, []string{owner}, "LexicalInitializerDriver", want, nil, nativeLexicalExactSignatures)
				return
			}
			testNativePrivateSetterCompiledFixture(t, owner, "LexicalInitializerDriver", want, func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialFreshEnclosingMixedInitializer(t *testing.T) {
	testNativeFreshEnclosingMixedInitializer(t, false)
}

func TestAdversarialFreshEnclosingMixedInitializerNativeCompiler(t *testing.T) {
	testNativeFreshEnclosingMixedInitializer(t, true)
}

// The driver remains an original binary. It checks constructor/argument order,
// abrupt completion identity, inner ownership and overflow with a BigInteger
// oracle rather than executing reconstructed source as its own reference.
func testNativeFreshEnclosingTemporaryEffects(t *testing.T, native bool) {
	for _, owner := range []string{"FreshTemporaryOwner", "RenamedTemporaryOwner"} {
		t.Run(owner, func(t *testing.T) {
			source := `public class FreshTemporaryOwner {
 public static int trace,fail;
 public static final RuntimeException fault=new RuntimeException("identity");
 public static long mark(int phase){trace=trace*10+phase;if(fail==phase)throw fault;return phase;}
 public class Branch {final long seed; public Branch(long n){mark(1);seed=n;}
  public class Leaf {public final long value;public Leaf(long n,double d){mark(3);value=seed+n+(long)d;}}}
 public Branch.Leaf make(long seed,double d){return new Branch(seed).new Leaf(mark(2),d);}
}
class FreshTemporaryDriver {public static void main(String[]args){int count=0;
 for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(double d:new double[]{Double.NEGATIVE_INFINITY,-1.0,Double.NaN,1.0,Double.POSITIVE_INFINITY})for(int phase=0;phase<=3;phase++){
 FreshTemporaryOwner.trace=0;FreshTemporaryOwner.fail=phase;
 try{FreshTemporaryOwner.Branch.Leaf leaf=new FreshTemporaryOwner().make(seed,d);
 long want=java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(2)).add(java.math.BigInteger.valueOf((long)d)).longValue();
 if(phase!=0||leaf.value!=want||leaf.getClass().getDeclaringClass()!=FreshTemporaryOwner.Branch.class||leaf.getClass().getEnclosingClass()!=FreshTemporaryOwner.Branch.class)throw new AssertionError("result/owner/overflow");
 }catch(RuntimeException ex){if(phase==0||ex!=FreshTemporaryOwner.fault)throw new AssertionError("exception identity");}
 int wantTrace=phase==1?1:phase==2?12:123;if(FreshTemporaryOwner.trace!=wantTrace)throw new AssertionError("order");count++;}
 System.out.println(count+":fresh:order:exception:overflow:binary");}}
`
			source = strings.ReplaceAll(source, "FreshTemporaryOwner", owner)
			if native {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, compiler, []string{owner}, "FreshTemporaryDriver", "100:fresh:order:exception:overflow:binary\n", nil, nativeLexicalExactSignatures)
			} else {
				testNativePrivateSetterCompiledFixture(t, owner, "FreshTemporaryDriver", "100:fresh:order:exception:overflow:binary\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
				}, nativeLexicalExactSignatures)
			}
		})
	}
}
func TestAdversarialFreshEnclosingTemporaryEffects(t *testing.T) {
	testNativeFreshEnclosingTemporaryEffects(t, false)
}
func TestAdversarialFreshEnclosingTemporaryEffectsNativeCompiler(t *testing.T) {
	testNativeFreshEnclosingTemporaryEffects(t, true)
}

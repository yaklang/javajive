package javaclassparser

import (
	"fmt"

	"strings"
	"testing"
)

// Fresh enclosing operands must retain their physical NEW/initialization
// identities through multiple lexical layers, private constructor bridges and
// mixed anonymous captures. The independent BigInteger oracle checks overflow.
func TestAdversarialMemberFreshEnclosingAllocationPreservesOriginalProtocol(t *testing.T) {
	for _, root := range []string{"SuperForestOwner", "RenamedSuperForestOwner"} {
		for _, deep := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deep=%v", root, deep), func(t *testing.T) {
				source := `public class SuperForestOwner {
 private final long bias;private SuperForestOwner(long bias){this.bias=bias;}
 public interface Value{long get();}
 public class Base{public final long base;Base(long n){base=n;}}
 public class Layer {public class View extends Base{private final long seed;private View(long seed){super(seed);this.seed=seed;}public Value value(final long delta){return new Value(){public long get(){return SuperForestOwner.this.bias+View.this.seed+base+delta;}};}}public View make(long seed){return new View(seed);}}
 public Layer.View make(long seed){return new Layer().new View(seed);}
 public static SuperForestOwner create(long bias){return new SuperForestOwner(bias);}
}

class SuperForestDriver{public static void main(String[]args){int count=0;for(long bias:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){SuperForestOwner owner=SuperForestOwner.create(bias);SuperForestOwner.Layer.View view=owner.make(seed);SuperForestOwner.Value value=view.value(delta);if(view.getClass().getDeclaringClass()!=SuperForestOwner.Layer.class||value.getClass().getEnclosingClass()!=SuperForestOwner.Layer.View.class||!value.getClass().getEnclosingMethod().getName().equals("value"))throw new AssertionError("named/anonymous/super ownership");long want=java.math.BigInteger.valueOf(bias).add(java.math.BigInteger.valueOf(seed).multiply(java.math.BigInteger.valueOf(2))).add(java.math.BigInteger.valueOf(delta)).longValue();if(value.get()!=want||view.base!=seed)throw new AssertionError("enclosing super/capture/binding/overflow");count++;}System.out.println(count+":named:super:anonymous:overflow");}}
`
				if deep {
					source = strings.Replace(source, " public class Layer {", " public class Scope { public class Layer {", 1)
					source = strings.Replace(source, " public Layer.View make", " Layer layer(){return new Layer();}} public Scope.Layer.View make", 1)
					source = strings.ReplaceAll(source, "return new Layer().new View(seed)", "return new Scope().new Layer().new View(seed)")
					source = strings.ReplaceAll(source, "SuperForestOwner.Layer", "SuperForestOwner.Scope.Layer")
				}
				source = strings.ReplaceAll(source, "SuperForestOwner", root)
				testNativePrivateSetterCompiledFixture(t, root, "SuperForestDriver", "125:named:super:anonymous:overflow\n", func(t *testing.T, debug string) map[string][]byte {
					files := nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
					z := nativeArchive(t, files)
					defer z.Close()
					obj, err := Parse(files[root+".class"])
					if err != nil {
						t.Fatal(err)
					}
					p := z.nativeMemberReader(obj).planNativeMemberFamily()
					if p == nil {
						t.Fatal("original named plan missing")
					}
					if p.children[root+"$Layer$View"] == nil && p.children[root+"$Scope$Layer$View"] == nil {
						t.Fatal("original owned constructor missing")
					}
					return files
				})
			})
		}
	}
}

func TestAdversarialMemberFreshEnclosingAllocationPreservesEffectsAndExceptions(t *testing.T) {
	for _, root := range []string{"FreshEffectOwner", "OtherFreshEffectOwner"} {
		t.Run(root, func(t *testing.T) {
			source := `public class FreshEffectOwner {
 public static int trace,fail;
 public static final RuntimeException fault=new RuntimeException("original identity");
 public static long mark(int phase){trace=trace*10+phase;if(fail==phase)throw fault;return phase;}
 public class Branch {public Branch(){mark(1);}public class Leaf {public final long value;public Leaf(long n,double d){mark(3);value=n+(long)d;}}}
 public Branch.Leaf make(){return new Branch().new Leaf(mark(2),4.0);}
}
class FreshEffectDriver {public static void main(String[]args){for(int phase=0;phase<=3;phase++){FreshEffectOwner.trace=0;FreshEffectOwner.fail=phase;try{FreshEffectOwner.Branch.Leaf v=new FreshEffectOwner().make();if(phase!=0||v.value!=6)throw new AssertionError("result");}catch(RuntimeException ex){if(phase==0||ex!=FreshEffectOwner.fault)throw new AssertionError("exception identity");}int want=phase==1?1:phase==2?12:123;if(FreshEffectOwner.trace!=want)throw new AssertionError("constructor/argument/exception order");System.out.println(phase+":"+FreshEffectOwner.trace);}}}`
			source = strings.ReplaceAll(source, "FreshEffectOwner", root)
			testNativePrivateSetterCompiledFixture(t, root, "FreshEffectDriver", "0:123\n1:1\n2:12\n3:123\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
			})
		})
	}
}

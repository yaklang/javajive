package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const nativeFreshStaticFixture = `
class FreshStep {
 public static int fail;public static String trace="";public static Object owner,leaf;
 public static final RuntimeException fault=new IllegalStateException("identity"),wrong=new IllegalArgumentException("wrong overload");
 public static int mark(int phase){trace+=phase;if(fail==phase)throw fault;return phase;}
}
class FreshWrap {
 static{FreshStep.mark(5);}
 public static long accept(Object raw,long tail){FreshStep.mark(6);return ((FreshStaticOwner.Leaf)raw).value+tail;}
 public static long accept(FreshStaticOwner.Leaf wrong,long tail){throw FreshStep.wrong;}
}
class FreshTop {static{FreshStep.mark(8);}public static long pass(long value){FreshStep.mark(9);return value^9L;}}
public class FreshStaticOwner {
 private final long seed;
 public FreshStaticOwner(long seed){FreshStep.owner=this;FreshStep.mark(1);this.seed=seed;}
 public class Leaf {public final long value;public Leaf(long delta,double fraction){FreshStep.leaf=this;FreshStep.mark(3);value=seed+delta+(long)fraction;}}
 public static long make(long seed,double fraction){try{return BODY;}finally{FreshStep.mark(7);}}
}
class FreshStaticDriver {
 static Object field(Class<?> c,String n)throws Exception{java.lang.reflect.Field f=c.getDeclaredField(n);f.setAccessible(true);return f.get(null);}
 public static void main(String[]args)throws Exception{int rows=0;
 for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(double fraction:new double[]{Double.NEGATIVE_INFINITY,-1.0,Double.NaN,1.0,Double.POSITIVE_INFINITY})for(int fail:PHASES){
  java.net.URL url=new java.io.File(System.getProperty("java.class.path")).toURI().toURL();
  try(java.net.URLClassLoader loader=new java.net.URLClassLoader(new java.net.URL[]{url},null)){
   Class<?> step=Class.forName("FreshStep",true,loader),owner=Class.forName("FreshStaticOwner",true,loader);
   java.lang.reflect.Field mode=step.getDeclaredField("fail");mode.setAccessible(true);mode.setInt(null,fail);
   java.lang.reflect.Method make=owner.getDeclaredMethod("make",long.class,double.class);make.setAccessible(true);
   Object result;try{result=make.invoke(null,seed,fraction);}catch(java.lang.reflect.InvocationTargetException error){result=error.getCause();}
   Object fault=field(step,"fault");
   if(fail==0){long wanted=java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(2)).add(java.math.BigInteger.valueOf((long)fraction)).add(java.math.BigInteger.valueOf(4)).longValue();if(NESTED)wanted=java.math.BigInteger.valueOf(wanted).xor(java.math.BigInteger.valueOf(9)).longValue();if(!(result instanceof Long)||((Long)result).longValue()!=wanted)throw new AssertionError("result/overflow/overload");}
   else if(fail==5||fail==8){if(!(result instanceof ExceptionInInitializerError)||((Throwable)result).getCause()!=fault)throw new AssertionError("class initializer identity",result instanceof Throwable?(Throwable)result:null);}
   else if(result!=fault)throw new AssertionError("abrupt completion identity",result instanceof Throwable?(Throwable)result:null);
   String expected="";for(int phase:ORDER){expected+=phase;if(fail==phase)break;}if(fail!=0&&fail!=7)expected+="7";
   if(!expected.equals(field(step,"trace")))throw new AssertionError("evaluation order "+field(step,"trace")+" wanted "+expected);
   Object outer=field(step,"owner"),leaf=field(step,"leaf");if(outer==null||outer.getClass()!=owner)throw new AssertionError("owner publication");
   java.lang.reflect.Field originalSeed=owner.getDeclaredField("seed");originalSeed.setAccessible(true);if(originalSeed.getLong(outer)!=(fail==1?0:seed))throw new AssertionError("field initialized before throwing producer");
   if((fail!=1&&fail!=2)!=(leaf!=null))throw new AssertionError("member publication");
   if(leaf!=null){if(leaf.getClass().getDeclaringClass()!=owner)throw new AssertionError("member ownership");java.lang.reflect.Field captured=leaf.getClass().getDeclaredField("this$0");captured.setAccessible(true);if(captured.get(leaf)!=outer)throw new AssertionError("enclosing identity");}
  }rows++;
 }System.out.println(rows+":first-argument:fresh:class-init:overflow:identity");}
}`

func TestAdversarialFreshStaticFirstArgument(t *testing.T) {
	for _, shape := range []string{"direct", "nested wrappers", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			expression := "FreshWrap.accept((Object)new FreshStaticOwner(seed).new Leaf(FreshStep.mark(2),fraction),FreshStep.mark(4))"
			nested := "false"
			phases := "new int[]{0,1,2,3,4,5,6,7}"
			order := "new int[]{1,2,3,4,5,6,7}"
			want := "200:first-argument:fresh:class-init:overflow:identity\n"
			if shape == "nested wrappers" {
				expression = "FreshTop.pass(" + expression + ")"
				nested = "true"
				phases = "new int[]{0,1,2,3,4,5,6,7,8,9}"
				order = "new int[]{1,2,3,4,5,6,8,9,7}"
				want = "250:first-argument:fresh:class-init:overflow:identity\n"
			}
			source := strings.NewReplacer("BODY", expression, "NESTED", nested, "PHASES", phases, "ORDER", order).Replace(nativeFreshStaticFixture)
			root, driver := "FreshStaticOwner", "FreshStaticDriver"
			if shape == "renamed" {
				source = strings.NewReplacer("FreshStatic", "OtherStatic", "FreshStep", "OtherStep", "FreshWrap", "OtherWrap", "FreshTop", "OtherTop").Replace(source)
				root, driver = "OtherStaticOwner", "OtherStaticDriver"
			}
			for _, native := range []bool{false, true} {
				name := "modern"
				if native {
					name = "native8"
				}
				t.Run(name, func(t *testing.T) {
					if native {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return localGetterCompileJavac8(t, map[string]string{root + ".java": source}, debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{root}, driver, want, nil, nativeLexicalExactSignatures)
					} else {
						testNativePrivateSetterCompiledFixture(t, root, driver, want, func(t *testing.T, debug string) map[string][]byte {
							return nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
						}, nativeLexicalExactSignatures)
					}
				})
			}
		})
	}
}

func TestAdversarialFreshEnclosingInExternalStaticCall(t *testing.T) {
	source := strings.Replace(nativeCheckedHandlerLoopFixture, "BODY", "while(count<limit){LoopCatchOps.tick(count);count++;}", 1)
	source = strings.Replace(source, "new LoopCatchOwner().worker().run(limit)", "new LoopCatchOwner().new Worker().run(limit)", 1)
	for _, native := range []bool{false, true} {
		name := "modern"
		if native {
			name = "native8"
		}
		t.Run(name, func(t *testing.T) {
			if native {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"LoopCatchOwner.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"LoopCatchOwner"}, "LoopCatchDriver", "15:caught-loop:identity:effects\n", nil, nativeLexicalExactSignatures)
			} else {
				testNativePrivateSetterCompiledFixture(t, "LoopCatchOwner", "LoopCatchDriver", "15:caught-loop:identity:effects\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{"LoopCatchOwner.java": source}, debug, "8")
				}, nativeLexicalExactSignatures)
			}
		})
	}
}

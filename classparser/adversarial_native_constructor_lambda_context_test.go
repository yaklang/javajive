package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The original base and driver stay binary. A callback in the base constructor
// reads the capture before its source-level body. Its failure also publishes
// the partly constructed object, so moving capture stores cannot pass this
// oracle merely because successful constructions eventually have the values.
const nativeConstructorLambdaContextFixture = `import java.util.function.Supplier;
abstract class CtorLambdaBase {
 final Object seen; final int number; final long wideValue;
 CtorLambdaBase(Object input,int n){seen=peek();wideValue=wide();number=n;CtorLambdaEffects.calls++;CtorLambdaEffects.published=this;if(n==-7)throw CtorLambdaEffects.failure;}
 abstract Object peek(); abstract long wide();
}
class CtorLambdaHolder {
 final Supplier<CtorLambdaBase> factory;
 CtorLambdaHolder(Supplier<CtorLambdaBase> f){factory=f;CtorLambdaEffects.holders++;}
}
class CtorLambdaEffects {
 static int calls,holders,produced; static Object published;
 static final RuntimeException failure=new IllegalArgumentException("identity");
 static int input(int n){produced++;return n;}
}
public class CtorLambdaOwner extends CtorLambdaHolder {
 CtorLambdaOwner(final Object input,final int number){super(()->new CtorLambdaBase(input,CtorLambdaEffects.input(number)){Object peek(){return input;}long wide(){return 0;}});}
}
class CtorLambdaDriver {
 public static void main(String[]args)throws Exception {
  int rows=0;Object token=new Object();
  for(Object input:new Object[]{null,token,new String("same")})for(int n:new int[]{Integer.MIN_VALUE,-7,-1,0,1,Integer.MAX_VALUE}){
   Object expected=input;long expectedWide=0;
   CtorLambdaEffects.calls=0;CtorLambdaEffects.holders=0;CtorLambdaEffects.produced=0;CtorLambdaEffects.published=null;
   CtorLambdaOwner owner=new CtorLambdaOwner(input,n);
   if(CtorLambdaEffects.calls!=0||CtorLambdaEffects.produced!=0||CtorLambdaEffects.holders!=1)throw new AssertionError("deferred original allocation and argument effects");
   Object previous=null;
   for(int repeat=0;repeat<2;repeat++){
    CtorLambdaEffects.published=null;
    try{CtorLambdaBase value=owner.factory.get();if(n==-7||value.seen!=expected||value.peek()!=expected||value.number!=n||value.wideValue!=expectedWide||value.wide()!=expectedWide)throw new AssertionError("capture before super or wide arithmetic");}
    catch(RuntimeException failure){if(n!=-7||failure!=CtorLambdaEffects.failure)throw new AssertionError("original throw identity",failure);}
    CtorLambdaBase actual=(CtorLambdaBase)CtorLambdaEffects.published;
    if(actual==null||actual==previous||actual.seen!=expected||actual.wideValue!=expectedWide||CtorLambdaEffects.calls!=repeat+1||CtorLambdaEffects.produced!=repeat+1)throw new AssertionError("publication/allocation/callback/argument ordering");
    previous=actual;
    Class<?> type=actual.getClass();java.lang.reflect.Method enclosing=type.getEnclosingMethod();java.lang.reflect.Field field=type.getDeclaredField("val$input");field.setAccessible(true);
    if(!type.isAnonymousClass()||type.getModifiers()!=0||enclosing==null||!enclosing.isSynthetic()||!java.lang.reflect.Modifier.isStatic(enclosing.getModifiers())||enclosing.getDeclaringClass()!=CtorLambdaOwner.class||!field.isSynthetic()||field.get(actual)!=input)throw new AssertionError("physical helper and original lexical scope");
    rows++;
   }
  }
  System.out.println(rows+":constructor:lambda:lexical:capture");
 }
}`

func nativeConstructorLambdaContextSource(owner, layout string) (string, string) {
	source, want := nativeConstructorLambdaContextFixture, "36:constructor:lambda:lexical:capture\n"
	if layout == "wide-and-equal-reference" {
		// The inherited field named number would shadow an outer formal of
		// that name inside the anonymous body. Give the captured arithmetic
		// formal its own source binder; the unchanged driver supplies its oracle.
		source = strings.NewReplacer(
			"final Object input,final int number", "final Object input,final long salt,final Object other,final int delta",
			"CtorLambdaEffects.input(number)", "CtorLambdaEffects.input(delta)",
			"Object peek(){return input;}long wide(){return 0;}", "Object peek(){return input==other?input:other;}long wide(){return salt^delta;}",
			"})for(int n:", "})for(Object other:new Object[]{null,token,new String(\"same\")})for(long salt:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE,0x100000001L})for(int n:",
			"Object expected=input;long expectedWide=0;", "Object expected=input==other?input:other;long expectedWide=java.math.BigInteger.valueOf(salt).xor(java.math.BigInteger.valueOf(n)).longValue();",
			"new CtorLambdaOwner(input,n)", "new CtorLambdaOwner(input,salt,other,n)",
		).Replace(source)
		want = "540:constructor:lambda:lexical:capture\n"
	}
	return strings.ReplaceAll(source, "CtorLambdaOwner", owner), want
}

func TestAdversarialNativeConstructorLambdaPreinitContextRoundTrip(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for independent constructor-lambda context")
	}
	for _, owner := range []string{"CtorLambdaOwner", "ChangedConstructorScope"} {
		for _, layout := range []string{"reference", "wide-and-equal-reference"} {
			t.Run(owner+"/"+layout, func(t *testing.T) {
				source, want := nativeConstructorLambdaContextSource(owner, layout)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, source, owner, debug)
				}, NativeJavac8, compiler, []string{owner}, "CtorLambdaDriver", want, nil, nativeLexicalExactSignatures)
			})
		}
	}
}

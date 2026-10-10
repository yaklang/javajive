package javaclassparser

import (
	"strings"
	"testing"
)

// An unchanged original caller observes anonymous scope, delayed reads, receiver
// identity, effects and failures. Binary shape and generic signatures are
// independent contracts, including actual compiler-generated accessors.
const anonymousReturnedLambdaBodyFixture = `interface ReturnedReader {Object read();Object owner();}
abstract class ReturnedBase {abstract java.util.function.Supplier<ReturnedReader> supplier();abstract void set(Object value);}
class ReturnedOwner {private final Object token;ReturnedOwner(Object token){this.token=token;}
 ReturnedBase make(){return new ReturnedBase(){private Object current=token;
  Object identity(){return this;} public void set(Object value){current=value;}
  public java.util.function.Supplier<ReturnedReader> supplier(){return () -> new ReturnedReader(){
   public Object read(){return current;}public Object owner(){return identity();}
  };}
 };}
}
class ReturnedDriver {public static void main(String[]args){Object shared=new Object();int rows=0;
 for(Object token:new Object[]{null,shared,new String("token")})for(Object next:new Object[]{null,shared,new String("next")}){
  ReturnedBase receiver=new ReturnedOwner(token).make();
  java.util.function.Supplier<ReturnedReader> supplier=receiver.supplier();
  ReturnedReader first=supplier.get(),second=supplier.get();
  if(!first.getClass().isAnonymousClass()||first.getClass().getEnclosingClass()!=receiver.getClass())throw new AssertionError("returned lambda original anonymous scope");
  if(first==second||first.owner()!=receiver||second.owner()!=receiver)throw new AssertionError("returned lambda body receiver identity");
  if(first.read()!=token)throw new AssertionError("returned lambda body initial value");receiver.set(next);
  if(first.read()!=next||second.read()!=next)throw new AssertionError("returned lambda body delayed identity");rows++;
 }System.out.println(rows+":returned:lambda");}}
`

func TestAdversarialAnonymousLambdaReturnedBodyPreservesLexicalReceiver(t *testing.T) {
	for _, shape := range []string{"receiver", "volatile-field", "local-shadow", "wide-field", "deep-chain", "effects-and-exceptions"} {
		t.Run(shape, func(t *testing.T) {
			for _, rename := range []string{"original", "renamed"} {
				t.Run(rename, func(t *testing.T) {
					source, owner, expected := anonymousReturnedLambdaBodyFixture, "ReturnedOwner", "9:returned:lambda\n"
					switch shape {
					case "volatile-field":
						source = strings.Replace(source, "private Object current=token", "private volatile Object current=token", 1)
					case "local-shadow":
						source = strings.Replace(source, "public Object read(){return current;}", "public Object read(){Object observed=current;Object current=new Object();if(current==observed)throw new AssertionError(\"local shadow identity\");return observed;}", 1)
					case "wide-field":
						source = strings.NewReplacer(
							"interface ReturnedReader {Object read();Object owner();}", "interface ReturnedReader {Object read();Object owner();long word();}",
							"private final Object token;ReturnedOwner(Object token){this.token=token;}", "private final Object token;private final long seed;ReturnedOwner(Object token,long seed){this.token=token;this.seed=seed;}",
							"private Object current=token;", "private Object current=token;private long stamp=seed;",
							"set(Object value){current=value;}", "set(Object value){current=value;stamp=~stamp;}",
							"public Object read(){return current;}", "public Object read(){return current;}public long word(){return stamp;}",
							"for(Object next:new Object[]{null,shared,new String(\"next\")}){", "for(Object next:new Object[]{null,shared,new String(\"next\")})for(long seed:new long[]{Long.MIN_VALUE,-1,Long.MAX_VALUE}){",
							"new ReturnedOwner(token).make()", "new ReturnedOwner(token,seed).make()",
							"if(first.read()!=token)", "if(first.read()!=token||first.word()!=seed||second.word()!=seed)",
							"if(first.read()!=next||second.read()!=next)", "if(first.read()!=next||second.read()!=next||first.word()!=~seed||second.word()!=~seed)",
						).Replace(source)
						expected = "27:returned:lambda\n"
					case "effects-and-exceptions":
						source = strings.NewReplacer(
							"class ReturnedOwner {", "class ReturnedEffects {static String trace=\"\";static boolean fail;static final IllegalStateException failure=new IllegalStateException(\"original\");}class ReturnedOwner {",
							"Object identity(){return this;}", "Object identity(){ReturnedEffects.trace+=\"I\";if(ReturnedEffects.fail)throw ReturnedEffects.failure;return this;}",
							"set(Object value){current=value;}", "set(Object value){ReturnedEffects.trace+=\"M\";current=value;}",
							"supplier(){return", "supplier(){ReturnedEffects.trace+=\"F\";return",
							"read(){return current;}", "read(){ReturnedEffects.trace+=\"R\";return current;}",
							"ReturnedBase receiver=new", "ReturnedEffects.trace=\"\";ReturnedEffects.fail=false;ReturnedBase receiver=new",
							"identity\");rows++;", "identity\");ReturnedEffects.fail=true;try{second.owner();throw new AssertionError(\"missing original call failure\");}catch(IllegalStateException e){if(e!=ReturnedEffects.failure)throw new AssertionError(\"call failure identity\");}ReturnedEffects.fail=false;if(!ReturnedEffects.trace.equals(\"FIIRMRRI\"))throw new AssertionError(\"lexical consumer once/order\");rows++;",
						).Replace(source)
					case "deep-chain":
						source = strings.NewReplacer(
							"return () -> new ReturnedReader(){", "return () -> new Object(){ReturnedReader build(){return new ReturnedReader(){",
							"  };}", "  };}}.build();}",
							"first.getClass().getEnclosingClass()!=receiver.getClass()", "first.getClass().getEnclosingClass().getEnclosingClass()!=receiver.getClass()",
						).Replace(source)
					}
					if rename == "renamed" {
						source = strings.NewReplacer("ReturnedOwner", "DeferredGraph", "current", "payload", "stamp", "wideWord").Replace(source)
						owner = "DeferredGraph"
					}
					testNativePrivateSetterCompiledFixture(t, owner, "ReturnedDriver", expected, func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeLexicalExactSignatures)
				})
			}
		})
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

// The unchanged caller mutates the anonymous receiver after creating each
// supplier. Capturing a field snapshot or an enclosing receiver cannot satisfy
// the original delayed read, receiver identity, exception identity and trace.
const anonymousBoundLambdaReceiverFixture = `abstract class BoundLambdaBase {
 abstract java.util.function.Supplier<Object> supplier(); abstract void set(Object value);
}
class BoundLambdaEffects {
 static String trace=""; static boolean fail;
 static final IllegalArgumentException failure=new IllegalArgumentException("original");
}
class BoundLambdaOwner {
 private final Object token; BoundLambdaOwner(Object token){this.token=token;}
 BoundLambdaBase make(){return new BoundLambdaBase(){
  private Object current=token;
  public void set(Object value){BoundLambdaEffects.trace+="M";current=value;}
  public java.util.function.Supplier<Object> supplier(){BoundLambdaEffects.trace+="F";return ()->{
   BoundLambdaEffects.trace+="L";
   if(!this.getClass().isAnonymousClass())throw new AssertionError("bound lambda original receiver");
   if(BoundLambdaEffects.fail)throw BoundLambdaEffects.failure;
   return this.current;
  };}
 };}
}
class BoundLambdaDriver {
 public static void main(String[]args){int rows=0;Object shared=new Object();
  for(Object token:new Object[]{null,shared,new String("token")})for(Object next:new Object[]{null,shared,new String("next")}){
   BoundLambdaBase receiver=new BoundLambdaOwner(token).make();BoundLambdaEffects.trace="";BoundLambdaEffects.fail=false;
   java.util.function.Supplier<Object> first=receiver.supplier(),second=receiver.supplier();
   if(first.get()!=token)throw new AssertionError("bound lambda initial value");
   receiver.set(next);
   if(first.get()!=next||second.get()!=next)throw new AssertionError("bound lambda delayed receiver identity");
   BoundLambdaEffects.fail=true;
   try{second.get();throw new AssertionError("bound lambda missing failure");}catch(IllegalArgumentException e){
    if(e!=BoundLambdaEffects.failure)throw new AssertionError("bound lambda failure identity");
   }
   if(!BoundLambdaEffects.trace.equals("FFLMLLL"))throw new AssertionError("bound lambda once/order");
   rows++;
  }System.out.println(rows+":anonymous:bound:lambda\n");
 }
}`

func TestAdversarialAnonymousBoundLambdaKeepsDelayedReceiver(t *testing.T) {
	for _, shape := range []string{"receiver", "receiver-and-wide-captures"} {
		t.Run(shape, func(t *testing.T) {
			for _, rename := range []string{"original", "renamed"} {
				t.Run(rename, func(t *testing.T) {
					source, owner := anonymousBoundLambdaReceiverFixture, "BoundLambdaOwner"
					if shape == "receiver-and-wide-captures" {
						source = strings.NewReplacer(
							"supplier();", "supplier(Object capture,long salt);",
							"supplier(){", "supplier(Object capture,long salt){",
							"static String trace=\"\"; static boolean fail;", "static String trace=\"\"; static boolean fail; static Object captured; static long observed;",
							"BoundLambdaEffects.trace+=\"L\";", "BoundLambdaEffects.trace+=\"L\";BoundLambdaEffects.captured=capture;BoundLambdaEffects.observed=salt;",
							"first=receiver.supplier(),second=receiver.supplier()", "first=receiver.supplier(token,Long.MIN_VALUE),second=receiver.supplier(next,Long.MAX_VALUE)",
							"if(first.get()!=token)", "if(first.get()!=token||BoundLambdaEffects.captured!=token||BoundLambdaEffects.observed!=Long.MIN_VALUE)",
							"if(first.get()!=next||second.get()!=next)", "if(first.get()!=next||BoundLambdaEffects.captured!=token||BoundLambdaEffects.observed!=Long.MIN_VALUE||second.get()!=next||BoundLambdaEffects.captured!=next||BoundLambdaEffects.observed!=Long.MAX_VALUE)",
						).Replace(source)
					}
					if rename == "renamed" {
						source = strings.NewReplacer("BoundLambdaOwner", "DeferredEnvelope", "current", "payload").Replace(source)
						owner = "DeferredEnvelope"
					}
					testNativePrivateSetterFixture(t, source, owner, "BoundLambdaDriver", "9:anonymous:bound:lambda\n\n")
				})
			}
		})
	}
}

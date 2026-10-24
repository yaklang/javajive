package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Private accesses in a lambda body are lowered in a later compiler phase.
// An anonymous method's direct accesses must retain their original accessor
// ABI even when the callback escapes and runs after later owner mutations.
const deferredLambdaRegistrationFixture = `
interface RegisteredCallback {void apply(int value);}
interface RegisteredView {boolean first(RegisteredQueue queue);boolean second(RegisteredQueue queue);int total();}
class RegisteredQueue {
 RegisteredCallback first,second;int count;
 boolean add(RegisteredCallback callback){if(count++==0)first=callback;else second=callback;return count%2==0;}
}
class RegisteredOwner {
 private int sum;
 private void left(int value){sum+=value;}
 private void right(int value){sum^=value;}
 void set(int value){sum=value;}
 RegisteredView make(){
  return new RegisteredView(){
   public boolean first(RegisteredQueue queue){return queue.add(value -> left(value));}
   public boolean second(RegisteredQueue queue){return queue.add(value -> right(value));}
   public int total(){return sum;}
  };
 }
}
class RegisteredDriver {
 public static void main(String[]args){
  int rows=0;int[] words={Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE};
  for(int initial:words)for(int next:words)for(int delta:words)for(boolean reverse:new boolean[]{false,true}){
   RegisteredOwner owner=new RegisteredOwner(),other=new RegisteredOwner();owner.set(initial);other.set(initial);
   RegisteredView view=owner.make(),otherView=other.make();RegisteredQueue queue=new RegisteredQueue();
   if(view.first(queue)||!view.second(queue)||queue.count!=2||queue.first==null||queue.second==null)throw new AssertionError("registration factory evaluation");
   owner.set(next);other.set(~next);int expected=next;
   for(int repeat=0;repeat<2;repeat++){
    if(reverse){queue.second.apply(delta);queue.first.apply(delta);expected=java.math.BigInteger.valueOf(expected^delta).add(java.math.BigInteger.valueOf(delta)).intValue();}
    else{queue.first.apply(delta);queue.second.apply(delta);expected=java.math.BigInteger.valueOf(expected).add(java.math.BigInteger.valueOf(delta)).intValue()^delta;}
    if(view.total()!=expected||otherView.total()!=~next)throw new AssertionError("deferred private access receiver/value");
   }
   if(!view.getClass().isAnonymousClass()||view.getClass().getEnclosingClass()!=RegisteredOwner.class||!view.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("anonymous source ownership");
   rows++;
  }
  System.out.println(rows+":deferred:private-accessors:retained:receiver:modular-arithmetic");
 }
}`

func TestAdversarialDeferredLambdaPrivateAccessorRegistration(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, simple := range []string{"RegisteredOwner", "DifferentRegisteredOwner"} {
			for _, capture := range []string{"receiver", "parameter"} {
				t.Run(compiler.name+"/"+simple+"/"+capture, func(t *testing.T) {
					source := strings.ReplaceAll(deferredLambdaRegistrationFixture, "RegisteredOwner", simple)
					if capture == "parameter" {
						source = strings.ReplaceAll(source, "RegisteredQueue queue)", "RegisteredQueue queue,int bias)")
						source = strings.ReplaceAll(source, "value -> left(value)", "value -> left(value+bias)")
						source = strings.ReplaceAll(source, "value -> right(value)", "value -> right(value+bias)")
						source = strings.ReplaceAll(source, "view.first(queue)", "view.first(queue,initial)")
						source = strings.ReplaceAll(source, "view.second(queue)", "view.second(queue,initial)")
						// Keep the binary caller's inputs unchanged; independently
						// compute the captured-parameter contribution in the oracle.
						source = strings.ReplaceAll(source, "java.math.BigInteger.valueOf(delta)", "java.math.BigInteger.valueOf(step)")
						source = strings.ReplaceAll(source, "expected^delta", "expected^step")
						source = strings.ReplaceAll(source, ".intValue()^delta", ".intValue()^step")
						source = strings.Replace(source, "owner.set(next);other.set(~next);int expected=next;", "owner.set(next);other.set(~next);int expected=next;int step=java.math.BigInteger.valueOf(delta).add(java.math.BigInteger.valueOf(initial)).intValue();", 1)
					}
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return localGetterCompileJavac8(t, map[string]string{simple + ".java": source}, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{simple}, "RegisteredDriver", "250:deferred:private-accessors:retained:receiver:modular-arithmetic\n", nil, nil, []string{"normal"}, nativeLexicalExactSignatures)
				})
			}
		}
	}
}

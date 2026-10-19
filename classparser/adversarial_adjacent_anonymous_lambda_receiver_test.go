package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The boolean consumer is adjacent to invokedynamic, but retains the lambda.
// Its receiver is the original anonymous THIS, even after later field writes.
// This exercises the inline-capture path rather than a declared snapshot.
const adjacentAnonymousReceiverFixture = `
interface AdjacentCallback {int apply(int delta);}
interface AdjacentView {boolean enqueue(AdjacentQueue queue);void set(int value);}
class AdjacentEffects {static int calls;static final RuntimeException failure=new IllegalStateException("original");}
class AdjacentQueue {
 AdjacentCallback pending;boolean result,fail;
 boolean add(AdjacentCallback callback){AdjacentEffects.calls++;pending=callback;if(fail)throw AdjacentEffects.failure;return result;}
}
class AdjacentOwner {
 AdjacentView make(final int initial) {
  return new AdjacentView() {
   int current=initial;
   public void set(int value){current=value;}
   public boolean enqueue(AdjacentQueue queue){return queue.add(delta -> current+delta);}
  };
 }
}
class AdjacentDriver {
 public static void main(String[]args) {
  int rows=0;
  int[] words={Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE};
  for(int initial:words)for(int next:words)for(int delta:words)for(boolean result:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true}) {
   AdjacentView first=new AdjacentOwner().make(initial),other=new AdjacentOwner().make(initial);
   AdjacentQueue queue=new AdjacentQueue();queue.result=result;queue.fail=fail;
   int before=AdjacentEffects.calls;
   try{boolean actual=first.enqueue(queue);if(fail||actual!=result)throw new AssertionError("boolean consumer result");}
   catch(RuntimeException actual){if(!fail||actual!=AdjacentEffects.failure)throw new AssertionError("consumer exception identity",actual);}
   if(AdjacentEffects.calls!=before+1||queue.pending==null)throw new AssertionError("retained capture/evaluation count");
   first.set(next);other.set(~next);
   int expected=java.math.BigInteger.valueOf(next).add(java.math.BigInteger.valueOf(delta)).intValue();
   if(queue.pending.apply(delta)!=expected||queue.pending.apply(delta)!=expected)throw new AssertionError("delayed anonymous receiver identity/value");
   if(!first.getClass().isAnonymousClass()||first.getClass().getEnclosingClass()!=AdjacentOwner.class||!first.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("original anonymous scope");
   rows++;
  }
  System.out.println(rows+":adjacent:boolean:retained:anonymous-this:identity:exceptions");
 }
}`

func TestAdversarialAdjacentAnonymousLambdaKeepsOriginalReceiver(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, pkg := range []string{"", "adjacent.receiver"} {
			for _, simple := range []string{"AdjacentOwner", "DifferentAdjacentOwner"} {
				t.Run(compiler.name+"/"+pkg+"/"+simple, func(t *testing.T) {
					source := strings.ReplaceAll(adjacentAnonymousReceiverFixture, "AdjacentOwner", simple)
					owner, driver := simple, "AdjacentDriver"
					if pkg != "" {
						source = "package " + pkg + ";\n" + source
						owner = strings.ReplaceAll(pkg, ".", "/") + "/" + simple
						driver = pkg + "." + driver
					}
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return localGetterCompileJavac8(t, map[string]string{simple + ".java": source}, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{owner}, driver, "500:adjacent:boolean:retained:anonymous-this:identity:exceptions\n", nil, nil, []string{"normal"}, nativeLexicalExactSignatures)
				})
			}
		}
	}
}

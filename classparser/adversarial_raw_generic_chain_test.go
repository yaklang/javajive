package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialRawGenericFluentChainKeepsEffectsAndErasure(t *testing.T) {
	// The declaring class has a formal, but step/apply have no method variables.
	// A raw receiver therefore remains raw; argument-witness inference must not
	// recursively speculate through the same prefix twice at every link.
	source := `import java.util.function.Function;
class RawChainBox<T> {
 static int steps,callbacks;static final IllegalStateException failure=new IllegalStateException("original");
 final T value;RawChainBox(T value){this.value=value;}
 RawChainBox<T> step(){steps++;return this;}
 Integer apply(Function<? super T,Integer> f){return f.apply(value);}
}
public class RawChainDriver {
 static int run(final Object token,Object value,final boolean fail){
  RawChainBox box=new RawChainBox(value);
  Function<Object,Integer> f=x->{RawChainBox.callbacks++;if(fail||x!=token)throw RawChainBox.failure;return Integer.valueOf(17);};
  return box` + strings.Repeat(".step()", 32) + `.apply(f);
 }
 public static void main(String[]args){Object token=new Object();
  for(Object value:new Object[]{token,null,new Object()})for(boolean fail:new boolean[]{false,true}){
   RawChainBox.steps=0;RawChainBox.callbacks=0;
   try{int result=run(token,value,fail);if(value!=token||fail||result!=17)throw new AssertionError("result");System.out.println("result:"+result);}
   catch(IllegalStateException failure){if(failure!=RawChainBox.failure||(!fail&&value==token))throw new AssertionError("exception identity");System.out.println("failure");}
   if(RawChainBox.steps!=32||RawChainBox.callbacks!=1)throw new AssertionError("chain evaluated differently");
   System.out.println(RawChainBox.steps+":"+RawChainBox.callbacks);
  }
 }
}`
	roundTripGenericFlow(t, "RawChainDriver", source, Precision, Compatibility, "legacy")
}

package javaclassparser

import "testing"

// An inherited matches(Object) and a matches(String) overload are different
// invoke targets. A source-name replacement can compile while selecting the
// wrong method. The helper methods remain original independent declarations.
func TestAdversarialMethodReceiverIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "org.assertj.core.probe.MethodReceiverIdentity", `package org.assertj.core.probe;
import java.util.function.*;
public class MethodReceiverIdentity {
  static boolean bound(ReceiverPath path,String value) {
    Predicate<String> predicate=path::matches;
    return predicate.test(value);
  }
  static boolean inline(ReceiverPath path,String value) {
    Predicate<String> predicate=text->path.matches(text);
    return predicate.test(value);
  }
  static boolean condition(Condition owner,String value) {
    Predicate<String> predicate=owner::matches;
    return predicate.test(value);
  }
  public static void main(String[] args){
    System.out.println("literal:var2.matches(l0):var2::matches");
    ReceiverOracle.run();
  }
}
class Condition {
  boolean matches(Object value){ReceiverOracle.mark("object");return value==null;}
}
class ReceiverPath extends Condition {
  final String prefix;ReceiverPath(String prefix){this.prefix=prefix;}
  boolean matches(String value){ReceiverOracle.mark("string");return value!=null && value.startsWith(prefix);}
}
class ReceiverOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String text){trace.append(text).append(';');if(++step==failAt)throw new IllegalStateException(text);}
  static void run(){
    String[] values={null,"","a","abc","other"};
    for(int method=0;method<3;method++)for(int absent=0;absent<2;absent++)for(int prefix=0;prefix<2;prefix++)for(int v=0;v<values.length;v++)for(failAt=0;failAt<3;failAt++) {
      trace=new StringBuilder();step=0;ReceiverPath receiver=absent==0?new ReceiverPath(prefix==0?"":"a"):null;String outcome;
      try {
        boolean result=method==0?MethodReceiverIdentity.bound(receiver,values[v]):method==1?MethodReceiverIdentity.inline(receiver,values[v]):MethodReceiverIdentity.condition(receiver,values[v]);
        outcome=String.valueOf(result);
      }catch(Throwable error){outcome=error.getClass().getSimpleName();}
      System.out.println(method+":"+absent+":"+prefix+":"+v+":"+failAt+":"+outcome+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}

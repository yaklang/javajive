package javaclassparser

import "testing"

func TestAdversarialConditionalFunctionalValueRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "ConditionalFunctional", `import java.util.function.*;
public class ConditionalFunctional {
  Consumer<String> consumer;
  StringBuilder trace=new StringBuilder();
  ConditionalFunctional(boolean a,boolean b) {
    consumer=(a || b) ? this::record : value -> {};
  }
  void record(String s) { trace.append(s); }
  public static void main(String[] args) {
    for(int bits=0;bits<4;bits++) {
      ConditionalFunctional c=new ConditionalFunctional((bits&1)!=0,(bits&2)!=0);
      c.consumer.accept("x");c.consumer.accept("y");
      System.out.print(bits+":"+c.trace+";");
    }
  }
}`)
}

func TestAdversarialConditionalGenericFunctionalValueRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "ConditionalGenericFunctional", `import java.util.function.*;
class FunctionalNode<K,V> {
 final K key;final V value;
 FunctionalNode(K key,V value){this.key=key;this.value=value;}
}
public class ConditionalGenericFunctional<K,V> {
 final Consumer<FunctionalNode<K,V>> consumer;
 StringBuilder trace=new StringBuilder();
 ConditionalGenericFunctional(boolean a,boolean b) {
  consumer=(a || b) ? this::record : value -> {};
 }
 void record(FunctionalNode<K,V> n) { trace.append(n.key).append(n.value); }
 public static void main(String[] args) {
  for(int bits=0;bits<4;bits++) {
   ConditionalGenericFunctional<String,Integer> c=new ConditionalGenericFunctional<>((bits&1)!=0,(bits&2)!=0);
   c.consumer.accept(new FunctionalNode<>("a",7));
   c.consumer.accept(new FunctionalNode<>("b",9));
   System.out.print(bits+":"+c.trace+";");
  }
 }
}`)
}

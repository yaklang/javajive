package javaclassparser

import "testing"

func TestAdversarialMixedWildcardConsumerRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "MixedWildcardConsumer", `interface ResourceMapping<A,B> {B apply(A value);}
interface ResourceChild<X,Y> extends ResourceMapping<Y,X> {}
public class MixedWildcardConsumer<T> {
  final ResourceMapping<? super T,? extends String> direct;
  final ResourceChild<? extends String,? super T> inherited;
  int calls;
  MixedWildcardConsumer(ResourceMapping<? super T,? extends String> direct,ResourceChild<? extends String,? super T> inherited) {
    this.direct=direct;this.inherited=inherited;
  }
  String direct(Object value) {calls++;return direct.apply((T)value);}
  String inherited(Object value) {calls++;return inherited.apply((T)value);}
  public static void main(String[] args) {
    MixedWildcardConsumer<String> strings=new MixedWildcardConsumer<>(s -> s==null ? "null" : s+"!",s -> s==null ? "NULL" : s+"?");
    MixedWildcardConsumer<Integer> integers=new MixedWildcardConsumer<>(n -> n==null ? "null" : "n="+(n+1),n -> n==null ? "NULL" : "N="+(n-1));
    for(Object value:new Object[]{"text",Integer.valueOf(7),null,new Object()}) {
      for(int mode=0;mode<4;mode++) {
        try {System.out.print((mode==0?strings.direct(value):mode==1?strings.inherited(value):mode==2?integers.direct(value):integers.inherited(value))+":");}
        catch(ClassCastException e) {System.out.print("cast:");}
        System.out.print(strings.calls+":"+integers.calls+";");
      }
    }
  }
}`, Precision, Compatibility, "legacy")
}

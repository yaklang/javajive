package javaclassparser

import "testing"

// A generic factory erases its result to Object; the self-bounded receiver then
// introduces another CHECKCAST after every call. Rebuilding the branch must
// retain the entire selected chain, with each call/cast before later arguments.
func TestAdversarialTernaryCallChainCastsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ChainCastChoice", `interface ChainBase<T> {
  T negate();
}
interface ChainValue<T extends ChainValue<T>> extends ChainBase<T> {}
interface ChainFactory<T> { T one(); T zero(); }
class ChainPair<T extends ChainValue<T>> {
  final T first,second,third;
  ChainPair(T first,T second) { ChainOracle.mark("pair");this.first=first;this.second=second;this.third=null; }
  ChainPair(T first,T second,T third) { ChainOracle.mark("triple");this.first=first;this.second=second;this.third=third; }
  public String toString() {return first+":"+second+":"+third;}
}
public class ChainCastChoice<T extends ChainValue<T>> {
  ChainPair<T> select(ChainFactory<T> factory,boolean positive) {
    return new ChainPair<T>(positive ? factory.one() : factory.one().negate(),factory.zero());
  }
  ChainPair<T> longer(ChainFactory<T> factory,boolean positive) {
    return new ChainPair<T>(positive ? factory.one().negate() : factory.one().negate().negate(),factory.zero());
  }
  ChainPair<T> three(ChainFactory<T> factory,boolean positive) {
    return new ChainPair<T>(positive ? factory.one() : factory.one().negate(),factory.zero(),factory.zero());
  }
  ChainPair<T> caught(ChainFactory<T> factory,boolean positive) {
    try {return new ChainPair<T>(positive ? factory.one() : factory.one().negate(),factory.zero());} catch (ClassCastException ex) {
      ChainOracle.mark("caught");return new ChainPair<T>(null,factory.zero());
    }
  }
  public static void main(String[] args) {ChainOracle.run();}
}
class ChainScalar implements ChainValue<ChainScalar> {
  final int n;
  ChainScalar(int n) {this.n=n;}
  public ChainScalar negate() {ChainOracle.mark("negate");return new ChainScalar(-n);}
  public String toString() {return Integer.toString(n);}
}
class ChainOracle {
  static StringBuilder trace;
  static int failAt,step,payload;
  static void mark(String event) {
    trace.append(event).append(';');
    if (++step==failAt) throw new IllegalStateException(event);
  }
  static void run() {
    ChainCastChoice<ChainScalar> choice=new ChainCastChoice<>();
    ChainFactory factory=new ChainFactory() {
      public Object one() {mark("one");return payload==0 ? new ChainScalar(7) : payload==1 ? null : "wrong";}
      public Object zero() {mark("zero");return new ChainScalar(0);}
    };
    for (int variant=0;variant<4;variant++) for (int sign=0;sign<2;sign++)
      for (payload=0;payload<3;payload++) for (failAt=0;failAt<7;failAt++) {
        trace=new StringBuilder();step=0;
        String outcome;
        try {
          ChainPair pair=variant==0 ? choice.select(factory,sign==0) : variant==1 ? choice.longer(factory,sign==0) : variant==2 ? choice.caught(factory,sign==0) : choice.three(factory,sign==0);
          outcome="value:"+pair;
        } catch (RuntimeException ex) {outcome=ex.getClass().getSimpleName();}
        System.out.println(variant+":"+sign+":"+payload+":"+failAt+":"+outcome+":"+trace);
      }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}

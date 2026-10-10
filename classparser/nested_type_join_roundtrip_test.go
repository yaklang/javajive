package javaclassparser

import "testing"

// A binary-name prefix describes lexical nesting, not inheritance. The local
// web already proves the base type from both allocation arms. Source recovery
// must retain it even when that base is one arm and shares the owner's $ prefix.
func TestAdversarialNestedTypeJoinRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedTypeJoin", `public class NestedTypeJoin {
  static int forward(boolean lazy,int value) {
    JoinOwner$Base result=lazy ? new JoinOwner$Lazy(value) : new JoinOwner$Base(value);
    return JoinOracle.consume(result)+result.value();
  }
  static int reverse(boolean lazy,int value) {
    JoinOwner$Base result=lazy ? new JoinOwner$Base(value) : new JoinOwner$Lazy(value);
    return JoinOracle.consume(result)+result.value();
  }
  static int siblings(boolean lazy,int value) {
    JoinOwner$Base result=lazy ? new JoinOwner$Lazy(value) : new JoinOwner$Other(value);
    return JoinOracle.consume(result)+result.value();
  }
  public static void main(String[] args) {JoinOracle.run();}
}
class JoinOwner {}
class JoinOwner$Base {
  final int value;
  JoinOwner$Base(int value){JoinOracle.mark("base");this.value=value;}
  int value(){JoinOracle.mark("base-read");return value;}
}
class JoinOwner$Lazy extends JoinOwner$Base {
  JoinOwner$Lazy(int value){super(value);JoinOracle.mark("lazy");}
  int value(){JoinOracle.mark("lazy-read");return value*7;}
}
class JoinOwner$Other extends JoinOwner$Base {
  JoinOwner$Other(int value){super(value);JoinOracle.mark("other");}
  int value(){JoinOracle.mark("other-read");return -value;}
}
class JoinOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String label){trace.append(label).append(';');if(++step==failAt)throw new IllegalStateException(label);}
  static int consume(JoinOwner$Base value){mark("consume-base");return value.value();}
  static int consume(Object value){mark("consume-object");return 99;}
  static void run(){
    for(int method=0;method<3;method++)for(int flag=0;flag<2;flag++)for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(failAt=0;failAt<8;failAt++){
      trace=new StringBuilder();step=0;String result;
      try {int n=method==0 ? NestedTypeJoin.forward(flag==0,value) : method==1 ? NestedTypeJoin.reverse(flag==0,value) : NestedTypeJoin.siblings(flag==0,value);result="value:"+n;}
      catch(Throwable ex){result=ex.getClass().getSimpleName()+":"+ex.getMessage();}
      System.out.println(method+":"+flag+":"+value+":"+failAt+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}

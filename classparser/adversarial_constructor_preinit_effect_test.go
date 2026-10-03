package javaclassparser

import "testing"

const constructorPreinitEffectFixture = `
interface PreEffectsProduce {int apply(int n);}
class PreEffectsBox implements PreEffectsProduce {
 int calls; final RuntimeException failure=new IllegalStateException("seed");
 public int apply(int n){calls++;if(n==-7)throw failure;return (n<<2)^0x76543210;}
 long wide(long n){calls++;if(n==-7)throw failure;return (n<<17)^0x123456789abcdefL;}
 static long seed(PreEffectsBox box,long n){return box.wide(n);}
}
class PreEffectsVirtualParent {final int value;PreEffectsVirtualParent(PreEffectsBox box,int n){this(box.apply(n));}private PreEffectsVirtualParent(int value){this.value=value;}}
class PreEffectsInterfaceParent {final int value;PreEffectsInterfaceParent(PreEffectsProduce op,int n){this(op.apply(n));}private PreEffectsInterfaceParent(int value){this.value=value;}}
class PreEffectsStaticParent {final long value;PreEffectsStaticParent(PreEffectsBox box,long n){this(PreEffectsBox.seed(box,n));}private PreEffectsStaticParent(long value){this.value=value;}}
class PreEffectsOwner {
 PreEffectsVirtualParent virtual(PreEffectsBox box,int n,final Object token){return new PreEffectsVirtualParent(box,n){Object capture(){return token;}};}
 PreEffectsInterfaceParent inter(PreEffectsProduce op,int n,final Object token){return new PreEffectsInterfaceParent(op,n){Object capture(){return token;}};}
 PreEffectsStaticParent stat(PreEffectsBox box,long n,final Object token){return new PreEffectsStaticParent(box,n){Object capture(){return token;}};}
}
class PreEffectsOracle {
 static Object capture(Object value)throws Exception{return value.getClass().getDeclaredMethod("capture").invoke(value);}
 static void run()throws Exception {
  PreEffectsOwner owner=new PreEffectsOwner();Object token=new Object();
  for(int kind=0;kind<3;kind++){
   try{if(kind==0)owner.virtual(null,1,token);else if(kind==1)owner.inter(null,1,token);else owner.stat(null,1,token);throw new AssertionError("null provider accepted");}
   catch(NullPointerException expected){System.out.println(kind+":null");}
  }
  for(Object t:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-7,-1,0,1,Integer.MAX_VALUE})for(int kind=0;kind<3;kind++){
   PreEffectsBox box=new PreEffectsBox();
   try{
    Object result=kind==0?owner.virtual(box,n,t):kind==1?owner.inter(box,n,t):owner.stat(box,n,t);
    if(n==-7||box.calls!=1||capture(result)!=t)throw new AssertionError("evaluation/capture identity");
    long actual=kind==0?((PreEffectsVirtualParent)result).value:kind==1?((PreEffectsInterfaceParent)result).value:((PreEffectsStaticParent)result).value;
    long want=kind==2?(((long)n<<17)^0x123456789abcdefL):((n<<2)^0x76543210);
    if(actual!=want)throw new AssertionError("wide/scalar argument");System.out.println(kind+":"+n+":"+actual+":"+(t==token));
   }catch(RuntimeException failure){if(n!=-7||failure!=box.failure||box.calls!=1)throw new AssertionError("exception identity/order",failure);System.out.println(kind+":"+n+":failure");}
  }
 }
}
public class PreEffectsDriver {public static void main(String[]args)throws Exception{PreEffectsOracle.run();}}
`

func TestAdversarialConstructorReceiverFreeCallsBeforeInitialization(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "PreEffectsDriver", constructorPreinitEffectFixture, nil,
		[]string{"PreEffectsOwner", "PreEffectsOwner$1", "PreEffectsOwner$2", "PreEffectsOwner$3", "PreEffectsVirtualParent", "PreEffectsInterfaceParent", "PreEffectsStaticParent"}, true,
		Precision, Compatibility, "legacy")
}

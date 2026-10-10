package javaclassparser

import "testing"

const finalReceiverEffectFixture = `
class FinalEffectsBox {
 int calls;final IllegalStateException failure=new IllegalStateException("original");
 int seed(int n){calls++;if(n==-7)throw failure;return (n<<3)^0x12345678;}
}
class FinalEffectsParent {final int value;FinalEffectsParent(FinalEffectsBox box,int n){value=box.seed(n);}}
class FinalEffectsOwner {
 final Object token;FinalEffectsOwner(Object token){this.token=token;}
 final class Child extends FinalEffectsParent {Child(FinalEffectsBox box,int n){super(box,n);}Object capture(){return FinalEffectsOwner.this.token;}}
 FinalEffectsParent make(FinalEffectsBox box,int n){return new Child(box,n);}
}
class FinalEffectsOracle {
 static void run()throws Exception{Object token=new Object();
  for(Object t:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-7,-1,0,1,Integer.MAX_VALUE}){
   FinalEffectsOwner owner=new FinalEffectsOwner(t);FinalEffectsBox box=new FinalEffectsBox();
   try{FinalEffectsParent result=owner.make(box,n);
    if(n==-7||box.calls!=1||result.value!=((n<<3)^0x12345678)||((FinalEffectsOwner.Child)result).capture()!=t)throw new AssertionError("capture/result/effect");
    if(!java.lang.reflect.Modifier.isFinal(result.getClass().getModifiers()))throw new AssertionError("receiver became extensible");
    System.out.println(n+":"+result.value+":"+(t==token));
   }catch(IllegalStateException failure){if(n!=-7||failure!=box.failure||box.calls!=1)throw new AssertionError("throw identity/order",failure);System.out.println("failure");}
  }
 }
}
public class FinalEffectsDriver {public static void main(String[]args)throws Exception{FinalEffectsOracle.run();}}
`

func TestAdversarialFinalReceiverFreeEffectsAfterInitialization(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "FinalEffectsDriver", finalReceiverEffectFixture, nil,
		[]string{"FinalEffectsOwner", "FinalEffectsOwner$Child"}, true, Precision, Compatibility, "legacy")
}

package javaclassparser

import "testing"

const constructorEffectBranchFixture = `
class BranchEffectsRoot {
 final int number;
 BranchEffectsRoot(int n){if(n<0){number=-n;}else{number=n+7;}}
 Object captured(){return null;}
}
class BranchEffectsParent extends BranchEffectsRoot {
 final int copied;
 BranchEffectsParent(int n){super(n);int copy;if(n==0){copy=number;}else{copy=number+3;}copied=copy;}
}
class BranchEffectsOwner {
 BranchEffectsParent make(final Object token,int n){return new BranchEffectsParent(n){Object captured(){return token;}};}
}
class BranchEffectsOracle {
 static void run(){BranchEffectsOwner owner=new BranchEffectsOwner();Object token=new Object();
  for(Object value:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-7,-1,0,1,7,Integer.MAX_VALUE}){
   BranchEffectsParent result=owner.make(value,n);int number=n<0?-n:n+7;int copy=n==0?number:number+3;
   if(result.captured()!=value||result.number!=number||result.copied!=copy)throw new AssertionError("branch/capture/overflow");
   System.out.println(n+":"+result.number+":"+result.copied+":"+(value==token));
  }
 }
}
public class BranchEffectsDriver {public static void main(String[]args){BranchEffectsOracle.run();}}
`

func TestAdversarialConstructorReceiverEffectsAcrossConditionalParents(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "BranchEffectsDriver", constructorEffectBranchFixture, nil,
		[]string{"BranchEffectsRoot", "BranchEffectsParent", "BranchEffectsOwner", "BranchEffectsOwner$1"}, true, Precision, Compatibility, "legacy")
}

package javaclassparser

import "testing"

// The driver/oracle stay original. The complete capturing class family is
// replaced, including the outer declaration whose original InnerClasses table
// would otherwise make javac enroll the original nested types while compiling
// their flattened replacement units.
// Parent and child each have their own same-spelled synthetic outer field.
const constructorReceiverEffectFixture = `
class ReceiverEffectsOwner {
 class Root<T> {
  final T value; final int number; final long wide; final int constant;
  Root(T value,int number,long wide){this.value=value;this.number=number+7;this.wide=wide+1;this.constant=19;}
  Object captured(){return null;}
 }
 class Parent<T> extends Root<T> {
  final int copied;
  Parent(T value,int number,long wide){super(value,number,wide);copied=this.number+3;}
 }
 Parent<Object> make(final Object token,int number,long wide){return new Parent<Object>(token,number,wide){Object captured(){return token;}};}
}
class ReceiverEffectsOracle {
 static void run(){ReceiverEffectsOwner owner=new ReceiverEffectsOwner();Object token=new Object();
  for(Object value:new Object[]{null,token})for(int number:new int[]{Integer.MIN_VALUE,-2,0,2,Integer.MAX_VALUE})for(long wide:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){
   ReceiverEffectsOwner.Parent<Object> result=owner.make(value,number,wide);
   if(result.value!=value||result.captured()!=value||result.number!=number+7||result.copied!=number+10||result.wide!=wide+1||result.constant!=19)throw new AssertionError("receiver/capture/field identity");
   System.out.println(number+":"+wide+":"+(value==token));
  }
 }
}
public class ReceiverEffectsDriver {public static void main(String[]args){ReceiverEffectsOracle.run();}}
`

func TestAdversarialConstructorReceiverEffectsAcrossCapturedParents(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "ReceiverEffectsDriver", constructorReceiverEffectFixture, nil,
		[]string{"ReceiverEffectsOwner", "ReceiverEffectsOwner$Root", "ReceiverEffectsOwner$Parent", "ReceiverEffectsOwner$1"}, true, Precision, Compatibility, "legacy")
}

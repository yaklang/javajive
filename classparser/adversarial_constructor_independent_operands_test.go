package javaclassparser

import "testing"

// The protected receiver never reaches these operations. Their original
// order still determines cast/allocation failure priority and class effects.
func TestAdversarialConstructorIndependentStaticCastAndArrayEffects(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "IndependentOperandsDriver", `
class IndependentOperandsEffects {static final Object marker=new Object();static int initialized;}
class IndependentOperandsState {static final Object marker=init();static Object init(){IndependentOperandsEffects.initialized++;return IndependentOperandsEffects.marker;}}
class IndependentOperandsParent {
 final String text;final boolean matched;final int[] ints;final Object[] refs;final Object state;
 IndependentOperandsParent(Object value,int n){text=(String)value;matched=value instanceof String;ints=new int[n];refs=new Object[n];state=IndependentOperandsState.marker;}
 Object capture(){return null;}
}
class IndependentOperandsOwner {final Object token;IndependentOperandsOwner(Object t){token=t;}final class Member extends IndependentOperandsParent {Member(Object value,int n){super(value,n);}Object capture(){return IndependentOperandsOwner.this.token;}}IndependentOperandsParent make(Object value,int n){return new Member(value,n);}}
class IndependentOperandsOracle {static void run(){Object token=new Object();Object wrong=new Object();
 for(Object t:new Object[]{null,token})for(Object value:new Object[]{null,"text",wrong})for(int n:new int[]{-1,0,3}){
  int before=IndependentOperandsEffects.initialized;
  try{
   IndependentOperandsParent out=new IndependentOperandsOwner(t).make(value,n);
   if(value==wrong||n<0||out.capture()!=t||out.text!=value||out.matched!=(value instanceof String)||out.ints.length!=n||out.refs.length!=n||out.state!=IndependentOperandsEffects.marker||IndependentOperandsEffects.initialized!=1)throw new AssertionError("operand/capture identity");
   System.out.println("ok:"+n+":"+(t==token)+":"+(value==null));
  }catch(ClassCastException failure){if(value!=wrong||IndependentOperandsEffects.initialized!=before)throw new AssertionError("cast priority/effects",failure);System.out.println("cast");}
   catch(NegativeArraySizeException failure){if(value==wrong||n>=0||IndependentOperandsEffects.initialized!=before)throw new AssertionError("array priority/effects",failure);System.out.println("array");}
 }
}}
public class IndependentOperandsDriver {public static void main(String[]args){IndependentOperandsOracle.run();}}
`, nil, []string{"IndependentOperandsOwner", "IndependentOperandsOwner$Member"}, true, Precision, Compatibility, "legacy")
}

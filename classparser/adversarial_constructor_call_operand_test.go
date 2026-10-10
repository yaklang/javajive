package javaclassparser

import "testing"

// The first callee can fail before the second argument is evaluated. Neither
// callee receives the uninitialized capturing receiver. Rebuilt implementations
// replace the complete owner family; the independent oracle stays original.
func TestAdversarialConstructorCaptureCallOperandsRoundTrip(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "CallOperandDriver", `
class CallOperandEffects {static String trace="";static final RuntimeException failure=new IllegalArgumentException("call");}
interface CallOperandSource {Object get(int mode);long wide();}
class CallOperandBox implements CallOperandSource {
 final Object token;final long number;CallOperandBox(Object t,long n){token=t;number=n;}
 public Object get(int mode){CallOperandEffects.trace+="G";if(mode==1)throw CallOperandEffects.failure;return token;}
 public long wide(){CallOperandEffects.trace+="W";return number;}
 static Object make(Object token,int mode){CallOperandEffects.trace+="S";if(mode==2)throw CallOperandEffects.failure;return token;}
}
class CallOperandParent {final Object value;final long number;final int length;CallOperandParent(Object v,long n,int l){value=v;number=n;length=l;}Object capture(){return null;}}
class CallOperandOwner {
 final Object token;CallOperandOwner(Object t){token=t;}
 class Virtual extends CallOperandParent {Virtual(CallOperandBox b,int mode,Object[] array){super(b.get(mode),b.wide(),array.length);}Object capture(){return CallOperandOwner.this.token;}}
 class Interface extends CallOperandParent {Interface(CallOperandSource b,int mode,Object[] array){super(b.get(mode),b.wide(),array.length);}Object capture(){return CallOperandOwner.this.token;}}
 class Static extends CallOperandParent {Static(Object t,int mode,Object[] array){super(CallOperandBox.make(t,mode),Long.MIN_VALUE,array.length);}Object capture(){return CallOperandOwner.this.token;}}
 CallOperandParent create(int kind,CallOperandBox b,int mode,Object[] array){if(kind==0)return new Virtual(b,mode,array);if(kind==1)return new Interface(b,mode,array);return new Static(b==null?null:b.token,mode,array);}
}
class CallOperandOracle {static void run(){Object token=new Object();CallOperandOwner owner=new CallOperandOwner(token);int rows=0;
 for(int kind=0;kind<3;kind++)for(int mode=0;mode<3;mode++)for(Object value:new Object[]{null,token})for(long wide:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(int shape=0;shape<3;shape++){
 CallOperandBox box=shape==0?null:new CallOperandBox(value,wide);Object[] array=shape==2?null:new Object[3];CallOperandEffects.trace="";
 String trace=kind==2?"S":box==null?"":"G";boolean payload=kind==2?mode==2:box!=null&&mode==1;boolean failed=payload||kind<2&&box==null||array==null;
 if(!payload&&kind<2&&box!=null)trace+="W";
 try{CallOperandParent p=owner.create(kind,box,mode,array);if(failed||p.value!=(kind==2&&box==null?null:value)||p.number!=(kind==2?Long.MIN_VALUE:wide)||p.length!=3||p.capture()!=token)throw new AssertionError("value/width/capture");}
 catch(RuntimeException e){if(!failed||payload&&e!=CallOperandEffects.failure||!payload&&!(e instanceof NullPointerException))throw new AssertionError("failure identity/priority",e);}
 if(!trace.equals(CallOperandEffects.trace))throw new AssertionError("effect order");rows++;}
 System.out.println("rows:"+rows);
}}
public class CallOperandDriver {public static void main(String[]args){CallOperandOracle.run();}}
`, nil, []string{"CallOperandOwner", "CallOperandOwner$Virtual", "CallOperandOwner$Interface", "CallOperandOwner$Static"}, true, Precision, Compatibility, "legacy")
}

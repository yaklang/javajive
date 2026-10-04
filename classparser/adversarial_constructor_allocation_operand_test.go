package javaclassparser

import "testing"

func TestAdversarialConstructorCaptureAllocationOperandsRoundTrip(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "AllocationOperandDriver", `
class AllocationOperandEffects {static String trace="";static int initialized;static final RuntimeException failure=new IllegalArgumentException("allocation ctor");}
class AllocationOperandBox {final Object value;AllocationOperandBox(Object v){value=v;}Object read(int mode){AllocationOperandEffects.trace+="R";if(mode==1)throw AllocationOperandEffects.failure;return value;}}
class AllocationOperandCarrier {static {AllocationOperandEffects.initialized++;}final Object value;AllocationOperandCarrier(Object v,int mode){AllocationOperandEffects.trace+="C";if(mode==2)throw AllocationOperandEffects.failure;value=v;}}
class AllocationOperandGeneric<T> {final T value;AllocationOperandGeneric(T v){value=v;}T read(){return value;}}
class AllocationOperandOverload {final Object value;final int tag;AllocationOperandOverload(Object v){value=v;tag=1;}AllocationOperandOverload(String v){value=v;tag=2;}}
class AllocationOperandParent {final Object value;final int length;AllocationOperandParent(Object v,int n){value=v;length=n;}Object capture(){return null;}}
class AllocationOperandOwner {
 final Object token;AllocationOperandOwner(Object t){token=t;}
 class Single extends AllocationOperandParent {Single(AllocationOperandBox b,int mode,Object[] a){super(new AllocationOperandCarrier(b.read(mode),mode),a.length);}Object capture(){return AllocationOperandOwner.this.token;}}
 class Nested extends AllocationOperandParent {Nested(AllocationOperandBox b,int mode,Object[] a){super(new AllocationOperandCarrier(new AllocationOperandCarrier(b.read(mode),mode),mode),a.length);}Object capture(){return AllocationOperandOwner.this.token;}}
 class Erased extends AllocationOperandParent {Erased(AllocationOperandGeneric<String> b){super(new AllocationOperandOverload((Object)b.read()),0);}}
 AllocationOperandParent erased(AllocationOperandGeneric<String> b){return new Erased(b);}
 AllocationOperandParent create(boolean nested,AllocationOperandBox b,int mode,Object[] a){return nested?new Nested(b,mode,a):new Single(b,mode,a);}
}
class AllocationOperandOracle {static void run(){Object token=new Object();AllocationOperandOwner owner=new AllocationOperandOwner(token);int rows=0;
 for(boolean nested:new boolean[]{false,true})for(int mode=0;mode<3;mode++)for(Object value:new Object[]{null,token})for(int shape=0;shape<3;shape++){
 AllocationOperandBox box=shape==0?null:new AllocationOperandBox(value);Object[] a=shape==2?null:new Object[4];AllocationOperandEffects.trace="";
 String trace=box==null?"":"R";boolean payload=box!=null&&mode>0, failed=payload||box==null||a==null;
 if(box!=null&&mode!=1)trace+="C";if(nested&&box!=null&&mode==0)trace+="C";
 try{AllocationOperandParent p=owner.create(nested,box,mode,a);if(failed||p.length!=4||p.capture()!=token)throw new AssertionError("capture/length");Object v=((AllocationOperandCarrier)p.value).value;if(nested)v=((AllocationOperandCarrier)v).value;if(v!=value)throw new AssertionError("allocation identity");}
 catch(RuntimeException e){if(!failed||payload&&e!=AllocationOperandEffects.failure||!payload&&!(e instanceof NullPointerException))throw new AssertionError("failure priority",e);}
 if(!trace.equals(AllocationOperandEffects.trace)||AllocationOperandEffects.initialized!=1)throw new AssertionError("effects/class initialization");rows++;}
 for(String v:new String[]{null,new String("marker")}){AllocationOperandParent p=owner.erased(new AllocationOperandGeneric<String>(v));AllocationOperandOverload selected=(AllocationOperandOverload)p.value;if(selected.tag!=1||selected.value!=v)throw new AssertionError("original allocation descriptor binding");}
 System.out.println("rows:"+rows);
}}
public class AllocationOperandDriver {public static void main(String[]args){AllocationOperandOracle.run();}}
`, nil, []string{"AllocationOperandOwner", "AllocationOperandOwner$Single", "AllocationOperandOwner$Nested", "AllocationOperandOwner$Erased"}, true, Precision, Compatibility, "legacy")
}

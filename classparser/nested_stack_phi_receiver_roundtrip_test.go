package javaclassparser

import "testing"

// A different reference type and an instance cache exercise the same nested
// stack-phi ownership. Trace receiver production before argument evaluation,
// short-circuit bypass, null receiver, and original failure identity.
func TestAdversarialNestedStackPhiInstanceReceiverRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "NestedPhiReceiver", `class PhiItem {
 final int key;PhiItem(int key){this.key=key;}
 boolean matches(int value){NestedPhiReceiver.trace+="R";return value==key;}
}
public class NestedPhiReceiver {
 PhiItem cache;static String trace;static final RuntimeException failure=new IllegalStateException("original");
 static PhiItem produce(int mode){trace+="P";if(mode<0)throw failure;return mode==0?null:new PhiItem(mode);}
 static int argument(int mode){trace+="A";if(mode==3)throw failure;return mode;}
 boolean and(boolean enabled,int mode){return enabled && (cache==null?(cache=produce(mode)):cache).matches(argument(mode));}
 boolean or(boolean bypass,int mode){return bypass || (cache==null?(cache=produce(mode)):cache).matches(argument(mode));}
 String run(boolean enabled,int mode,int operation){try{return "value:"+(operation==0?and(enabled,mode):or(enabled,mode));}catch(Throwable error){return error.getClass().getName()+":"+(error==failure);}}
 public static void main(String[] args){
 for(boolean initialized:new boolean[]{false,true})for(boolean enabled:new boolean[]{false,true})for(int mode=-1;mode<=3;mode++)for(int operation=0;operation<2;operation++){
 NestedPhiReceiver owner=new NestedPhiReceiver();PhiItem original=initialized?new PhiItem(2):null;owner.cache=original;trace="";String result=owner.run(enabled,mode,operation);
 System.out.println(initialized+":"+enabled+":"+mode+":"+operation+":"+result+":"+trace+":"+(owner.cache==original)+":"+(owner.cache==null?"null":owner.cache.key));
 }
 }
}`, Precision, Compatibility, "legacy")
}

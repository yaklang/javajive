package javaclassparser

import "testing"

func TestAdversarialThisConstructorOverloadBindingRoundTrip(t *testing.T) {
	t.Setenv("JDEC_THIS_CTOR_OVERLOAD_CAST_OFF", "1")
	roundTripGenericFlow(t, "ThisConstructorBindingReview", `
interface CtorLookup<K,V>{}
class CtorLRU<K,V> implements CtorLookup<K,V>{}
class CtorBindingEffects {
 static String trace="";static int mode;static final java.io.IOException failure=new java.io.IOException("identity");
 static void seen(String label,int at)throws java.io.IOException{trace+=label+";";if(mode==at)throw failure;}
 static CtorLRU<Object,String> cache(CtorLRU<Object,String> value)throws java.io.IOException{seen("C",1);return value;}
 static Object token(Object value)throws java.io.IOException{seen("S",2);return value;}
 static int mark()throws java.io.IOException{seen("M",3);return 7;}
}
public class ThisConstructorBindingReview {
 final CtorLookup<Object,String> cache;final Object token;final int mark;
 ThisConstructorBindingReview()throws java.io.IOException{this((CtorLookup<Object,String>)null);}
 ThisConstructorBindingReview(CtorLRU<Object,String> value)throws java.io.IOException{this((CtorLookup<Object,String>)value);CtorBindingEffects.seen("N1",-1);}
 ThisConstructorBindingReview(CtorLookup<Object,String> value)throws java.io.IOException{CtorBindingEffects.seen("W1",-1);cache=value;token=null;mark=0;}
 ThisConstructorBindingReview(CtorLRU<Object,String> value,Object token,int mark)throws java.io.IOException{this((CtorLookup<Object,String>)value,token,mark);CtorBindingEffects.seen("N3",5);}
 ThisConstructorBindingReview(CtorLookup<Object,String> value,Object token,int mark)throws java.io.IOException{CtorBindingEffects.seen("W3",4);cache=value;this.token=token;this.mark=mark;}
 public static void main(String[]args)throws java.io.IOException{
  Object token=new Object();CtorLRU<Object,String> original=new CtorLRU<Object,String>();
  for(CtorLRU<Object,String> value:new CtorLRU[]{null,original})for(int mode=0;mode<=5;mode++){CtorBindingEffects.mode=mode;CtorBindingEffects.trace="";try{ThisConstructorBindingReview owner=new ThisConstructorBindingReview(CtorBindingEffects.cache(value),CtorBindingEffects.token(token),CtorBindingEffects.mark());System.out.println((owner.cache==value)+":"+(owner.token==token)+":"+owner.mark+":"+CtorBindingEffects.trace);}catch(Throwable caught){System.out.println((caught==CtorBindingEffects.failure)+":"+CtorBindingEffects.trace);}}
  CtorBindingEffects.mode=0;CtorBindingEffects.trace="";ThisConstructorBindingReview owner=new ThisConstructorBindingReview(original);System.out.println((owner.cache==original)+":"+CtorBindingEffects.trace);
  CtorBindingEffects.trace="";owner=new ThisConstructorBindingReview((CtorLRU<Object,String>)null);System.out.println((owner.cache==null)+":"+CtorBindingEffects.trace);
  CtorBindingEffects.trace="";owner=new ThisConstructorBindingReview();System.out.println((owner.cache==null)+":"+CtorBindingEffects.trace);
 }
}
`, Precision, Compatibility, "legacy")
}

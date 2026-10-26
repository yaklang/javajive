package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialGuardedFallbackWithIndependentProtection(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class GuardedFallback {
 public static void event(StringBuilder trace,int event,int fail,RuntimeException marker){trace.append(event).append(';');if(event==fail)throw marker;}
 public static int scale(int value,StringBuilder trace,int fail,RuntimeException marker){event(trace,1,fail,marker);return value;}
 public static long payload(long value,StringBuilder trace,int fail,RuntimeException marker){event(trace,2,fail,marker);return value;}
 public static long execute(int precision,int inputScale,long offset,long input,boolean plain,boolean enabled,StringBuilder trace,int fail,RuntimeException marker){
  try{event(trace,0,fail,marker);}catch(RuntimeException ex){throw ex;}long result=11;
  if(enabled){int scale;
   region:{
    if(precision<19&&(scale=scale(inputScale,trace,fail,marker))>=0&&offset!=-1L){
     long compact=payload(input,trace,fail,marker);
     if(compact!=Long.MIN_VALUE&&!plain){event(trace,3,fail,marker);result=(compact^(compact>>>32))+scale;break region;}
    }
    event(trace,4,fail,marker);result=37;
   }
   if(plain)event(trace,6,fail,marker);else event(trace,7,fail,marker);
  }
  event(trace,5,fail,marker);return result;
 }
}
class GuardedFallbackDriver {
 public static void main(String[]args){int rows=0;
  for(int precision:new int[]{0,18,19,30})for(int scale:new int[]{-1,0,2})for(long offset:new long[]{-1,7})for(long input:new long[]{Long.MIN_VALUE,-1,0,3,Long.MAX_VALUE})for(boolean plain:new boolean[]{false,true})for(boolean enabled:new boolean[]{false,true})for(int fail=-1;fail<8;fail++){
   boolean readsScale=enabled&&precision<19, readsPayload=readsScale&&scale>=0&&offset!=-1;
   boolean fast=readsPayload&&input!=Long.MIN_VALUE&&!plain;
   int[] events={0,readsScale?1:-1,readsPayload?2:-1,enabled?(fast?3:4):-1,enabled?(plain?6:7):-1,5};
   StringBuilder expected=new StringBuilder();boolean thrown=false;
   for(int e:events)if(e>=0){expected.append(e).append(';');if(e==fail){thrown=true;break;}}
   long wanted=!enabled?11:fast?(input^(input>>>32))+scale:37;
   StringBuilder actual=new StringBuilder();RuntimeException marker=new RuntimeException("original marker");
   try{long got=GuardedFallback.execute(precision,scale,offset,input,plain,enabled,actual,fail,marker);if(thrown||got!=wanted)throw new AssertionError("value "+got+" expected "+wanted);}
   catch(RuntimeException caught){if(!thrown||caught!=marker)throw new AssertionError("exception identity",caught);}
   if(!actual.toString().equals(expected.toString()))throw new AssertionError("trace "+actual+" expected "+expected);
   rows++;
  }System.out.println(rows+":guarded-fallback:short-circuit:outer-exit");
 }
}
`
	for _, name := range []string{"GuardedFallback", "ConditionalSelection"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "GuardedFallback", name), name, name+"Driver", "4320:guarded-fallback:short-circuit:outer-exit\n")
		})
	}
}

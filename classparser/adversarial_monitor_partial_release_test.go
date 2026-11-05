package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialMonitorPartialExitSkipsHeldContinuation(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class MonitorPartialExit {
 public static void event(Object lock,StringBuilder trace,int e,int value,int fail,RuntimeException marker){trace.append(e).append(':').append(value).append(':').append(Thread.holdsLock(lock)).append(';');if((fail&(1<<e))!=0)throw marker;}
 public static void execute(Object lock,String text,boolean early,boolean already,boolean keep,StringBuilder trace,int fail,RuntimeException marker){
  if(already)return;
  synchronized(lock){
   if(already)return;
   int selected;
   if(text!=null){boolean match=text.equals("hit");if(!match){event(lock,trace,0,0,fail,marker);if(early)return;}selected=match?7:-3;}
   else{if(early)return;selected=-3;}
   try{event(lock,trace,1,selected,fail,marker);if(keep)event(lock,trace,2,selected,fail,marker);}
   catch(RuntimeException caught){if(caught!=marker)throw caught;event(lock,trace,3,selected,fail,marker);}
  }
  event(lock,trace,4,0,fail,marker);
 }
}
class MonitorPartialExitDriver {
 public static void main(String[]args){int rows=0;
 for(String text:new String[]{null,"","miss","hit"})for(boolean early:new boolean[]{false,true})for(boolean already:new boolean[]{false,true})for(boolean keep:new boolean[]{false,true})for(int fail:new int[]{0,1,2,4,8,16,10,12,17,31}){
  StringBuilder expected=new StringBuilder();boolean thrown=false;
  if(!already){boolean match="hit".equals(text),invalid=text!=null&&!match;
   if(invalid){expected.append("0:0:true;");thrown=(fail&1)!=0;}
   boolean exit=early&&!match;
   if(!thrown&&!exit){int value=match?7:-3;expected.append("1:").append(value).append(":true;");boolean caught=(fail&2)!=0;
    if(!caught&&keep){expected.append("2:").append(value).append(":true;");caught=(fail&4)!=0;}
    if(caught){expected.append("3:").append(value).append(":true;");thrown=(fail&8)!=0;}
    if(!thrown){expected.append("4:0:false;");thrown=(fail&16)!=0;}
   }
  }
  Object lock=new Object();StringBuilder trace=new StringBuilder();RuntimeException marker=new RuntimeException("same throwable");
  try{MonitorPartialExit.execute(lock,text,early,already,keep,trace,fail,marker);if(thrown)throw new AssertionError("missing throw");}
  catch(RuntimeException caught){if(!thrown||caught!=marker)throw new AssertionError("throw identity",caught);}
  if(!trace.toString().equals(expected.toString()))throw new AssertionError("trace="+trace+" expected="+expected+" text="+text+" early="+early);
  if(Thread.holdsLock(lock))throw new AssertionError("monitor still held");rows++;
 }System.out.println(rows+":monitor:partial-exit:held-effects:exception-identity");
 }
}
`
	for _, name := range []string{"MonitorPartialExit", "ProtectedSelection"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "MonitorPartialExit", name), name, name+"Driver", "320:monitor:partial-exit:held-effects:exception-identity\n")
		})
	}
}
